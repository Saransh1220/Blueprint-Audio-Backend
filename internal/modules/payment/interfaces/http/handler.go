package http

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/gateway/middleware"
	"github.com/saransh1220/blueprint-audio/internal/modules/payment/application"
	"github.com/saransh1220/blueprint-audio/internal/modules/payment/domain"
	"github.com/saransh1220/blueprint-audio/internal/shared/money"
)

type PaymentHandler struct {
	service application.PaymentService
}

func NewPaymentHandler(service application.PaymentService) *PaymentHandler {
	return &PaymentHandler{service: service}
}

func (h *PaymentHandler) ProviderWebhook(w http.ResponseWriter, r *http.Request) {
	provider := strings.ToLower(strings.TrimSpace(r.PathValue("provider")))
	if provider == "" {
		provider = domain.ProviderDodo
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024) // 1MB cap
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "invalid webhook body", http.StatusBadRequest)
		return
	}

	headers := make(map[string]string)
	for k, v := range r.Header {
		if len(v) > 0 {
			headers[strings.ToLower(k)] = v[0]
		}
	}

	if err := h.service.HandleWebhook(r.Context(), provider, payload, headers); err != nil {
		traceID := middleware.GetRequestID(r.Context())
		switch {
		case errors.Is(err, domain.ErrInvalidSignature),
			errors.Is(err, domain.ErrTimestampToleranceExceeded),
			errors.Is(err, domain.ErrMissingWebhookSecret):
			log.Printf("[ALERT][PAYMENT_UNAUTHORIZED] trace_id=%s provider=%s error=%v", traceID, provider, err)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		case errors.Is(err, domain.ErrMalformedPayload),
			errors.Is(err, domain.ErrMissingPaymentID):
			log.Printf("[ALERT][PAYMENT_BAD_REQUEST] trace_id=%s provider=%s error=%v", traceID, provider, err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		case errors.Is(err, domain.ErrOrderNotFound),
			errors.Is(err, domain.ErrProviderMismatch),
			errors.Is(err, domain.ErrAmountMismatch),
			errors.Is(err, domain.ErrOrderNotPayable):
			// Permanent business rejection: log alert, return 200 so provider stops retrying!
			log.Printf("[ALERT][PAYMENT_REJECTED] trace_id=%s provider=%s reason=%v", traceID, provider, err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]any{"received": true, "status": "ignored"})
			return
		case strings.Contains(strings.ToLower(err.Error()), "not supported") || strings.Contains(strings.ToLower(err.Error()), "unknown"):
			log.Printf("[ALERT][PAYMENT_UNKNOWN_PROVIDER] trace_id=%s provider=%s error=%v", traceID, provider, err)
			http.Error(w, "not found", http.StatusNotFound)
			return
		default:
			log.Printf("[ALERT][PAYMENT_FAILURE] trace_id=%s provider=%s error=%v", traceID, provider, err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]bool{"received": true})
}

func (h *PaymentHandler) DodoWebhook(w http.ResponseWriter, r *http.Request) {
	h.ProviderWebhook(w, r)
}

func (h *PaymentHandler) CreateOrder(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.ContextKeyUserId).(uuid.UUID)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	// Parse request body
	var req struct {
		SpecID              string  `json:"spec_id"`
		LicenseOptionID     string  `json:"license_option_id"`
		Currency            string  `json:"currency,omitempty"`
		ExpectedPriceID     *string `json:"expected_price_id,omitempty"`
		ExpectedRevision    *int64  `json:"expected_revision,omitempty"`
		ExpectedAmountMinor *int    `json:"expected_amount_minor,omitempty"`
		ExpectedCurrency    *string `json:"expected_currency,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	// Parse UUIDs
	specID, err := uuid.Parse(req.SpecID)
	if err != nil {
		http.Error(w, "invalid spec_id", http.StatusBadRequest)
		return
	}
	licenseOptionID, err := uuid.Parse(req.LicenseOptionID)
	if err != nil {
		http.Error(w, "invalid license_option_id", http.StatusBadRequest)
		return
	}

	currency := strings.TrimSpace(req.Currency)
	if currency == "" {
		currency = middleware.BuyerCurrencyFromContext(r.Context())
	}

	var expectedPriceID *uuid.UUID
	if req.ExpectedPriceID != nil && strings.TrimSpace(*req.ExpectedPriceID) != "" {
		parsedPriceID, err := uuid.Parse(strings.TrimSpace(*req.ExpectedPriceID))
		if err != nil {
			http.Error(w, "invalid expected_price_id", http.StatusBadRequest)
			return
		}
		expectedPriceID = &parsedPriceID
	}

	input := application.CreateOrderInput{
		UserID:              userID,
		SpecID:              specID,
		LicenseOptionID:     licenseOptionID,
		Currency:            currency,
		ExpectedPriceID:     expectedPriceID,
		ExpectedRevision:    req.ExpectedRevision,
		ExpectedAmountMinor: req.ExpectedAmountMinor,
		ExpectedCurrency:    req.ExpectedCurrency,
	}

	order, err := h.service.CreateOrderWithOffer(r.Context(), input)
	if err != nil {
		var priceMismatch *application.PriceMismatchError
		if errors.As(err, &priceMismatch) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"code":          "PRICE_CHANGED",
				"message":       priceMismatch.Message,
				"current_offer": priceMismatch.CurrentOffer,
			})
			return
		}
		if errors.Is(err, money.ErrInvalidCurrency) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if errors.Is(err, money.ErrPriceUnavailable) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"code":    "PRICE_UNAVAILABLE",
				"message": "Selected license cannot be purchased in this currency",
			})
			return
		}
		http.Error(w, "failed to create order: "+err.Error(), http.StatusInternalServerError)
		return
	}
	// Return order
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(order)
}

func (h *PaymentHandler) VerifyPayment(w http.ResponseWriter, r *http.Request) {
	// Auth check
	userID, ok := r.Context().Value(middleware.ContextKeyUserId).(uuid.UUID)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Parse request
	var req struct {
		OrderID           string `json:"order_id"`
		RazorpayPaymentID string `json:"razorpay_payment_id"`
		RazorpaySignature string `json:"razorpay_signature"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Parse order ID
	orderID, err := uuid.Parse(req.OrderID)
	if err != nil {
		http.Error(w, "invalid order_id", http.StatusBadRequest)
		return
	}

	// Verify payment and issue license
	license, err := h.service.VerifyPayment(
		r.Context(),
		orderID,
		userID,
		req.RazorpayPaymentID,
		req.RazorpaySignature,
	)
	if err != nil {
		if errors.Is(err, domain.ErrProviderCompletesByWebhook) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]string{
				"error":   "PROVIDER_COMPLETES_BY_WEBHOOK",
				"message": "This order is confirmed via webhook",
			})
			return
		}

		traceID := middleware.GetRequestID(r.Context())
		log.Printf("[ALERT][PAYMENT_FAILURE] trace_id=%s order_id=%s user_id=%s error=%v", traceID, orderID, userID, err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Return success with license
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"license": license,
		"message": "Payment successful! License issued.",
	})
}

func (h *PaymentHandler) GetOrder(w http.ResponseWriter, r *http.Request) {
	// Auth check
	userID, ok := r.Context().Value(middleware.ContextKeyUserId).(uuid.UUID)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Get order ID from path
	orderID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid order_id", http.StatusBadRequest)
		return
	}

	// Fetch order
	order, err := h.service.GetOrder(r.Context(), orderID, userID)
	if err != nil {
		http.Error(w, "order not found", http.StatusNotFound)
		return
	}

	// Return order
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(order)
}

func (h *PaymentHandler) GetOrderStatus(w http.ResponseWriter, r *http.Request) {
	// Auth check
	userID, ok := r.Context().Value(middleware.ContextKeyUserId).(uuid.UUID)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Get order ID from path
	orderID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "order not found", http.StatusNotFound)
		return
	}

	// Fetch order status
	status, err := h.service.GetOrderStatus(r.Context(), orderID, userID)
	if err != nil {
		http.Error(w, "order not found", http.StatusNotFound)
		return
	}

	// Return order status
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func (h *PaymentHandler) GetUserOrders(w http.ResponseWriter, r *http.Request) {
	// Get authenticated user
	userID, ok := r.Context().Value(middleware.ContextKeyUserId).(uuid.UUID)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Parse pagination
	page := 1
	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	// Fetch orders
	orders, err := h.service.GetUserOrders(r.Context(), userID, page)
	if err != nil {
		http.Error(w, "failed to fetch orders", http.StatusInternalServerError)
		return
	}

	// Return orders
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(orders)
}

func (h *PaymentHandler) GetUserLicenses(w http.ResponseWriter, r *http.Request) {
	// Get authenticated user
	userID, ok := r.Context().Value(middleware.ContextKeyUserId).(uuid.UUID)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Parse pagination
	page := 1
	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	// Fetch licenses
	search := r.URL.Query().Get("q")
	licenseType := r.URL.Query().Get("type")
	licenses, total, err := h.service.GetUserLicenses(r.Context(), userID, page, search, licenseType)
	if err != nil {
		http.Error(w, "failed to fetch licenses", http.StatusInternalServerError)
		return
	}

	// Return licenses
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"data": licenses,
		"metadata": map[string]interface{}{
			"total":    total,
			"page":     page,
			"per_page": 5, // Limit 5 for testing
		},
	})
}

func (h *PaymentHandler) GetLicenseDownloads(w http.ResponseWriter, r *http.Request) {
	licenseIDStr := r.PathValue("id")
	licenseID, err := uuid.Parse(licenseIDStr)

	if err != nil {
		http.Error(w, `{"error": "invalid license ID"}`, http.StatusBadRequest)
		return
	}

	userID, ok := r.Context().Value(middleware.ContextKeyUserId).(uuid.UUID)

	if !ok {
		http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
		return
	}

	downloads, err := h.service.GetLicenseDownloads(r.Context(), licenseID, userID)

	if err != nil {
		// Determine status code based on error message
		statusCode := http.StatusInternalServerError
		if err.Error() == "license not found" || err.Error() == "spec not found" {
			statusCode = http.StatusNotFound
		} else if err.Error() == "unauthorized: you do not own this license" {
			statusCode = http.StatusForbidden
		} else if err.Error() == "license is not active" || err.Error() == "license has been revoked" {
			statusCode = http.StatusForbidden
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(downloads)
}

func (h *PaymentHandler) GetProducerOrders(w http.ResponseWriter, r *http.Request) {
	// Get authenticated user (producer)
	producerID, ok := r.Context().Value(middleware.ContextKeyUserId).(uuid.UUID)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Parse pagination
	page := 1
	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	limit := 10
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	// Fetch orders
	response, err := h.service.GetProducerOrders(r.Context(), producerID, page, limit)
	if err != nil {
		log.Printf("PaymentHandler.GetProducerOrders failed: %v", err)
		http.Error(w, "failed to fetch producer orders", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
