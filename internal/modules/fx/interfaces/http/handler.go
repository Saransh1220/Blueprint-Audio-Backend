package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/gateway/middleware"
	"github.com/saransh1220/blueprint-audio/internal/modules/fx/application"
	"github.com/saransh1220/blueprint-audio/internal/modules/fx/domain"
	"github.com/saransh1220/blueprint-audio/internal/shared/money"
)

type AuditLogger interface {
	Log(ctx context.Context, actorID *uuid.UUID, action, resourceType, resourceID string, beforeState, afterState any, ip, userAgent string) error
}

type FXHandler struct {
	service     *application.Service
	auditLogger AuditLogger
}

func NewFXHandler(service *application.Service, auditLogger AuditLogger) *FXHandler {
	return &FXHandler{
		service:     service,
		auditLogger: auditLogger,
	}
}

type pageMetadata struct {
	Total   int `json:"total"`
	Page    int `json:"page"`
	PerPage int `json:"per_page"`
}

type adminFXRatesResponse struct {
	Data     []*domain.Rate `json:"data"`
	Latest   []*domain.Rate `json:"latest"`
	Metadata pageMetadata   `json:"metadata"`
}

type latestRateResponse struct {
	Base      string    `json:"base"`
	Quote     string    `json:"quote"`
	Rate      string    `json:"rate"`
	AsOf      time.Time `json:"as_of"`
	FetchedAt time.Time `json:"fetched_at"`
	Source    string    `json:"source"`
	Stale     bool      `json:"stale"`
}

type SuggestPriceRequest struct {
	FromCurrency string `json:"from_currency"`
	ToCurrency   string `json:"to_currency"`
	AmountMinor  int    `json:"amount_minor"`
}

type SuggestPriceResponse struct {
	Currency      string    `json:"currency"`
	AmountMinor   int       `json:"amount_minor"`
	RateAsOf      time.Time `json:"rate_as_of"`
	RateFetchedAt time.Time `json:"rate_fetched_at"`
	Stale         bool      `json:"stale"`
	Source        string    `json:"source"`
	Rounding      string    `json:"rounding"`
}

func (h *FXHandler) AdminListRates(w http.ResponseWriter, r *http.Request) {
	page, limit := parsePagination(r)
	offset := (page - 1) * limit

	filter := domain.HistoryFilter{
		Base:   strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("base"))),
		Quote:  strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("quote"))),
		Status: domain.RateStatus(strings.ToLower(strings.TrimSpace(r.URL.Query().Get("status")))),
		Limit:  limit,
		Offset: offset,
	}

	history, total, err := h.service.GetHistory(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch fx rates history")
		return
	}

	latest, err := h.service.GetLatestPerPair(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch latest fx rates")
		return
	}

	if history == nil {
		history = []*domain.Rate{}
	}
	if latest == nil {
		latest = []*domain.Rate{}
	}

	resp := adminFXRatesResponse{
		Data:   history,
		Latest: latest,
		Metadata: pageMetadata{
			Total:   total,
			Page:    page,
			PerPage: limit,
		},
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *FXHandler) AdminRefreshRates(w http.ResponseWriter, r *http.Request) {
	res, err := h.service.Refresh(r.Context(), true)
	if err != nil {
		if errors.Is(err, domain.ErrCooldownActive) {
			writeError(w, http.StatusTooManyRequests, "refresh cooldown active: please wait before refreshing again")
			return
		}
		if errors.Is(err, domain.ErrLockNotAcquired) {
			writeError(w, http.StatusConflict, "fx refresh already in progress")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to refresh fx rates")
		return
	}

	if h.auditLogger != nil {
		var actorID *uuid.UUID
		if idVal := r.Context().Value(middleware.ContextKeyUserId); idVal != nil {
			if id, ok := idVal.(uuid.UUID); ok {
				actorID = &id
			}
		}
		ip := clientIP(r)
		userAgent := r.UserAgent()
		_ = h.auditLogger.Log(r.Context(), actorID, "fx:refresh", "fx_rates", "", nil, res, ip, userAgent)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":        "success",
		"rates_updated": res.RatesUpdated,
		"refreshed_at":  res.RefreshedAt,
	})
}

func (h *FXHandler) GetLatestRate(w http.ResponseWriter, r *http.Request) {
	base := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("base")))
	quote := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("quote")))

	if base == "" || quote == "" {
		writeError(w, http.StatusBadRequest, "base and quote query parameters are required")
		return
	}

	if _, ok := money.LookupCurrency(base); !ok {
		writeError(w, http.StatusBadRequest, "unsupported base currency")
		return
	}
	if _, ok := money.LookupCurrency(quote); !ok {
		writeError(w, http.StatusBadRequest, "unsupported quote currency")
		return
	}

	if base == quote {
		now := time.Now().UTC()
		writeJSON(w, http.StatusOK, latestRateResponse{
			Base:      base,
			Quote:     quote,
			Rate:      "1.00000000",
			AsOf:      now,
			FetchedAt: now,
			Source:    "identity",
			Stale:     false,
		})
		return
	}

	rate, err := h.service.GetLatestRate(r.Context(), base, quote)
	if err != nil {
		if errors.Is(err, domain.ErrNoRate) {
			writeError(w, http.StatusNotFound, "exchange rate not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to fetch exchange rate")
		return
	}

	if rate.Unusable {
		writeError(w, http.StatusConflict, "exchange rate exceeds maximum age and is unusable")
		return
	}

	writeJSON(w, http.StatusOK, latestRateResponse{
		Base:      rate.Base,
		Quote:     rate.Quote,
		Rate:      rate.RateDecimal,
		AsOf:      rate.AsOf,
		FetchedAt: rate.FetchedAt,
		Source:    rate.Source,
		Stale:     rate.Stale,
	})
}

func (h *FXHandler) SuggestPrice(w http.ResponseWriter, r *http.Request) {
	var req SuggestPriceRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "request must contain one JSON object")
		return
	}

	fromDef, ok := money.LookupCurrency(req.FromCurrency)
	if !ok {
		writeError(w, http.StatusBadRequest, "unsupported from_currency")
		return
	}
	toDef, ok := money.LookupCurrency(req.ToCurrency)
	if !ok {
		writeError(w, http.StatusBadRequest, "unsupported to_currency")
		return
	}
	if req.AmountMinor <= 0 {
		writeError(w, http.StatusBadRequest, "amount_minor must be positive")
		return
	}
	if req.AmountMinor > money.MaxAmountMinor {
		writeError(w, http.StatusBadRequest, "amount_minor exceeds maximum allowed limit")
		return
	}

	if fromDef.Code == toDef.Code {
		now := time.Now().UTC()
		writeJSON(w, http.StatusOK, SuggestPriceResponse{
			Currency:      toDef.Code,
			AmountMinor:   req.AmountMinor,
			RateAsOf:      now,
			RateFetchedAt: now,
			Stale:         false,
			Source:        "fx_suggestion",
			Rounding:      string(money.RoundingPolicyNone),
		})
		return
	}

	rate, err := h.service.GetLatestRate(r.Context(), fromDef.Code, toDef.Code)
	if err != nil {
		if errors.Is(err, domain.ErrNoRate) {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"error":   "RATE_UNAVAILABLE",
				"message": "exchange rate unavailable for requested pair",
			})
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to fetch exchange rate")
		return
	}

	if rate.Unusable {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error":   "RATE_UNAVAILABLE",
			"message": "exchange rate exceeds maximum age and is unusable",
		})
		return
	}

	rateRat := rate.GetBigRat()
	if rateRat == nil {
		parsed, parseErr := money.ParseRateDecimal(rate.RateDecimal)
		if parseErr != nil {
			writeError(w, http.StatusInternalServerError, "invalid exchange rate representation")
			return
		}
		rateRat = parsed
	}

	suggestedMinor, policy, err := money.SuggestSecondaryPrice(req.AmountMinor, fromDef, toDef, rateRat)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, SuggestPriceResponse{
		Currency:      toDef.Code,
		AmountMinor:   suggestedMinor,
		RateAsOf:      rate.AsOf,
		RateFetchedAt: rate.FetchedAt,
		Stale:         rate.Stale,
		Source:        "fx_suggestion",
		Rounding:      string(policy),
	})
}

func parsePagination(r *http.Request) (int, int) {
	page := 1
	if p, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && p > 0 {
		page = p
	}
	limit := 20
	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 && l <= 100 {
		limit = l
	} else if pp, err := strconv.Atoi(r.URL.Query().Get("per_page")); err == nil && pp > 0 && pp <= 100 {
		limit = pp
	}
	return page, limit
}

func clientIP(r *http.Request) string {
	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded != "" {
		parts := strings.Split(forwarded, ",")
		return strings.TrimSpace(parts[0])
	}
	return r.RemoteAddr
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
