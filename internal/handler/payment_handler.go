package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/middleware"
	"github.com/saransh1220/blueprint-audio/internal/service"
)

type PaymentHandler struct {
	service service.PaymentService
}

func NewPaymentHandler(service service.PaymentService) *PaymentHandler {
	return &PaymentHandler{service: service}
}

func (h *PaymentHandler) CreateOrder(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.ContextKeyUserId).(uuid.UUID)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	// 2. Parse request body
	var req struct {
		SpecID          string `json:"spec_id"`
		LicenseOptionID string `json:"license_option_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	// 3. Parse UUIDs
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
	// 4. Create order via service
	order, err := h.service.CreateOrder(r.Context(), userID, specID, licenseOptionID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// 5. Return order (with razorpay_order_id for frontend)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(order)

}

func (h *PaymentHandler) VerifyPayment(w http.ResponseWriter, r *http.Request) {
	// 1. Auth check
	_, ok := r.Context().Value(middleware.ContextKeyUserId).(uuid.UUID)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// 2. Parse request (from Razorpay frontend callback)
	var req struct {
		OrderID           string `json:"order_id"`
		RazorpayPaymentID string `json:"razorpay_payment_id"`
		RazorpaySignature string `json:"razorpay_signature"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// 3. Parse order ID
	orderID, err := uuid.Parse(req.OrderID)
	if err != nil {
		http.Error(w, "invalid order_id", http.StatusBadRequest)
		return
	}

	// 4. Verify payment and issue license
	license, err := h.service.VerifyPayment(
		r.Context(),
		orderID,
		req.RazorpayPaymentID,
		req.RazorpaySignature,
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// 5. Return success with license
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"license": license,
		"message": "Payment successful! License issued.",
	})
}

func (h *PaymentHandler) GetOrder(w http.ResponseWriter, r *http.Request) {
	// 1. Auth check
	_, ok := r.Context().Value(middleware.ContextKeyUserId).(uuid.UUID)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// 2. Get order ID from path (using r.PathValue like your spec handler)
	orderID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid order_id", http.StatusBadRequest)
		return
	}

	// 3. Fetch order
	order, err := h.service.GetOrder(r.Context(), orderID)
	if err != nil {
		http.Error(w, "order not found", http.StatusNotFound)
		return
	}

	// 4. Return order
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(order)
}

func (h *PaymentHandler) GetUserOrders(w http.ResponseWriter, r *http.Request) {
	// 1. Get authenticated user
	userID, ok := r.Context().Value(middleware.ContextKeyUserId).(uuid.UUID)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// 2. Parse page from query (matching your spec handler pattern)
	page := 1
	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	// 3. Fetch orders
	orders, err := h.service.GetUserOrders(r.Context(), userID, page)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 4. Return orders
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(orders)
}

func (h *PaymentHandler) GetUserLicenses(w http.ResponseWriter, r *http.Request) {
	// 1. Get authenticated user
	userID, ok := r.Context().Value(middleware.ContextKeyUserId).(uuid.UUID)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// 2. Parse pagination
	page := 1
	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	// 3. Fetch licenses
	licenses, err := h.service.GetUserLicenses(r.Context(), userID, page)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 4. Return licenses
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(licenses)
}
