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

func (h *PaymentHandler) DodoWebhook(w http.ResponseWriter, r *http.Request) {
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "invalid webhook body", http.StatusBadRequest)
		return
	}

	headers := map[string]string{
		"webhook-id":        r.Header.Get("webhook-id"),
		"webhook-timestamp": r.Header.Get("webhook-timestamp"),
		"webhook-signature": r.Header.Get("webhook-signature"),
	}
	if err := h.service.HandleWebhook(r.Context(), domain.ProviderDodo, payload, headers); err != nil {
		traceID := middleware.GetRequestID(r.Context())
		log.Printf("[ALERT][PAYMENT_FAILURE] trace_id=%s provider=%s error=%v", traceID, domain.ProviderDodo, err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"received": true})
}

func NewPaymentHandler(service application.PaymentService) *PaymentHandler {
	return &PaymentHandler{service: service}
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
		var mismatch *application.PriceMismatchError
		if errors.As(err, &mismatch) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]any{
				"code":          "PRICE_CHANGED",
				"error":         mismatch.Message,
				"current_offer": mismatch.CurrentOffer,
			})
			return
		}
		if errors.Is(err, money.ErrInvalidCurrency) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if errors.Is(err, money.ErrPriceUnavailable) {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
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
