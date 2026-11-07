package http

import (
	"context"
	"encoding/json"
	"errors"
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
