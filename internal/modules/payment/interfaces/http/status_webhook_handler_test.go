package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/modules/payment/application"
	"github.com/saransh1220/blueprint-audio/internal/modules/payment/domain"
	paymenthttp "github.com/saransh1220/blueprint-audio/internal/modules/payment/interfaces/http"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPaymentHandler_GetOrderStatus(t *testing.T) {
	orderID := uuid.New()

	t.Run("unauthorized without auth context", func(t *testing.T) {
		h := paymenthttp.NewPaymentHandler(mockPaymentService{})
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/orders/"+orderID.String()+"/status", nil)
		r.SetPathValue("id", orderID.String())
		h.GetOrderStatus(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("invalid uuid returns 404", func(t *testing.T) {
		h := paymenthttp.NewPaymentHandler(mockPaymentService{})
		w := httptest.NewRecorder()
		r := authedReq(http.MethodGet, "/orders/invalid-uuid/status", "")
		r.SetPathValue("id", "invalid-uuid")
		h.GetOrderStatus(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("service returns error returns 404", func(t *testing.T) {
		h := paymenthttp.NewPaymentHandler(mockPaymentService{
			getOrderStatusFn: func(ctx context.Context, oID, uID uuid.UUID) (*application.OrderStatusResponse, error) {
				return nil, errors.New("order not found")
			},
		})
		w := httptest.NewRecorder()
		r := authedReq(http.MethodGet, "/orders/"+orderID.String()+"/status", "")
		r.SetPathValue("id", orderID.String())
		h.GetOrderStatus(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("success returns 200 with status response", func(t *testing.T) {
		licID := uuid.New()
		exp := time.Now().Add(15 * time.Minute)
		h := paymenthttp.NewPaymentHandler(mockPaymentService{
			getOrderStatusFn: func(ctx context.Context, oID, uID uuid.UUID) (*application.OrderStatusResponse, error) {
				return &application.OrderStatusResponse{
					OrderID:           orderID,
					Status:            domain.OrderStatusPaid,
					Provider:          domain.ProviderRazorpay,
					ExpiresAt:         exp,
					RetryAfterSeconds: 0,
					License: &application.OrderLicenseSummary{
						ID:         licID,
						LicenseKey: "LIC-12345",
					},
				}, nil
			},
		})
		w := httptest.NewRecorder()
		r := authedReq(http.MethodGet, "/orders/"+orderID.String()+"/status", "")
		r.SetPathValue("id", orderID.String())
		h.GetOrderStatus(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

		var resp application.OrderStatusResponse
		err := json.NewDecoder(w.Body).Decode(&resp)
		require.NoError(t, err)
		assert.Equal(t, orderID, resp.OrderID)
		assert.Equal(t, domain.OrderStatusPaid, resp.Status)
		assert.Equal(t, domain.ProviderRazorpay, resp.Provider)
		require.NotNil(t, resp.License)
		assert.Equal(t, licID, resp.License.ID)
		assert.Equal(t, "LIC-12345", resp.License.LicenseKey)
	})
}

func TestPaymentHandler_ProviderWebhook(t *testing.T) {
	t.Run("401 on ErrInvalidSignature", func(t *testing.T) {
		h := paymenthttp.NewPaymentHandler(mockPaymentService{
			dodoWebhookFn: func(context.Context, []byte, map[string]string) error {
				return domain.ErrInvalidSignature
			},
		})
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/webhooks/razorpay", strings.NewReader(`{}`))
		r.SetPathValue("provider", "razorpay")
		h.ProviderWebhook(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.Contains(t, w.Body.String(), "unauthorized")
	})

	t.Run("401 on ErrTimestampToleranceExceeded", func(t *testing.T) {
		h := paymenthttp.NewPaymentHandler(mockPaymentService{
			dodoWebhookFn: func(context.Context, []byte, map[string]string) error {
				return domain.ErrTimestampToleranceExceeded
			},
		})
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/webhooks/dodo", strings.NewReader(`{}`))
		r.SetPathValue("provider", "dodo")
		h.ProviderWebhook(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("401 on ErrMissingWebhookSecret", func(t *testing.T) {
		h := paymenthttp.NewPaymentHandler(mockPaymentService{
			dodoWebhookFn: func(context.Context, []byte, map[string]string) error {
				return domain.ErrMissingWebhookSecret
			},
		})
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/webhooks/razorpay", strings.NewReader(`{}`))
		r.SetPathValue("provider", "razorpay")
		h.ProviderWebhook(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("400 on ErrMalformedPayload", func(t *testing.T) {
		h := paymenthttp.NewPaymentHandler(mockPaymentService{
			dodoWebhookFn: func(context.Context, []byte, map[string]string) error {
				return domain.ErrMalformedPayload
			},
		})
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/webhooks/razorpay", strings.NewReader(`{}`))
		r.SetPathValue("provider", "razorpay")
		h.ProviderWebhook(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "bad request")
	})

	t.Run("400 on ErrMissingPaymentID", func(t *testing.T) {
		h := paymenthttp.NewPaymentHandler(mockPaymentService{
			dodoWebhookFn: func(context.Context, []byte, map[string]string) error {
				return domain.ErrMissingPaymentID
			},
		})
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/webhooks/dodo", strings.NewReader(`{}`))
		r.SetPathValue("provider", "dodo")
		h.ProviderWebhook(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("200 ignored on permanent business rejection", func(t *testing.T) {
		rejections := []error{
			domain.ErrOrderNotFound,
			domain.ErrProviderMismatch,
			domain.ErrAmountMismatch,
			domain.ErrOrderNotPayable,
		}

		for _, rejErr := range rejections {
			h := paymenthttp.NewPaymentHandler(mockPaymentService{
				dodoWebhookFn: func(context.Context, []byte, map[string]string) error {
					return rejErr
				},
			})
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/webhooks/razorpay", strings.NewReader(`{}`))
			r.SetPathValue("provider", "razorpay")
			h.ProviderWebhook(w, r)

			assert.Equal(t, http.StatusOK, w.Code)
			assert.JSONEq(t, `{"received":true,"status":"ignored"}`, w.Body.String())
		}
	})

	t.Run("404 on unknown provider", func(t *testing.T) {
		h := paymenthttp.NewPaymentHandler(mockPaymentService{
			dodoWebhookFn: func(context.Context, []byte, map[string]string) error {
				return errors.New("unknown payment provider foo")
			},
		})
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/webhooks/foo", strings.NewReader(`{}`))
		r.SetPathValue("provider", "foo")
		h.ProviderWebhook(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("500 on unexpected service error", func(t *testing.T) {
		h := paymenthttp.NewPaymentHandler(mockPaymentService{
			dodoWebhookFn: func(context.Context, []byte, map[string]string) error {
				return errors.New("db connection failure")
			},
		})
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/webhooks/dodo", strings.NewReader(`{}`))
		r.SetPathValue("provider", "dodo")
		h.ProviderWebhook(w, r)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Contains(t, w.Body.String(), "internal server error")
	})

	t.Run("body exceeding 1MB returns 400", func(t *testing.T) {
		h := paymenthttp.NewPaymentHandler(mockPaymentService{})
		largeBody := strings.Repeat("A", 1024*1024+10)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/webhooks/razorpay", strings.NewReader(largeBody))
		r.SetPathValue("provider", "razorpay")
		h.ProviderWebhook(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestPaymentHandler_VerifyPayment_ProviderCompletesByWebhook(t *testing.T) {
	oid := uuid.NewString()
	h := paymenthttp.NewPaymentHandler(mockPaymentService{
		verifyFn: func(context.Context, uuid.UUID, uuid.UUID, string, string) (*domain.License, error) {
			return nil, domain.ErrProviderCompletesByWebhook
		},
	})

	w := httptest.NewRecorder()
	r := authedReq("POST", "/payments/verify", `{"order_id":"`+oid+`","razorpay_payment_id":"pay_1"}`)
	h.VerifyPayment(w, r)

	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Contains(t, w.Body.String(), "PROVIDER_COMPLETES_BY_WEBHOOK")
}
