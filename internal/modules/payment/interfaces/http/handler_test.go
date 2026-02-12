package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/gateway/middleware"
	"github.com/saransh1220/blueprint-audio/internal/modules/payment/application"
	"github.com/saransh1220/blueprint-audio/internal/modules/payment/domain"
	paymenthttp "github.com/saransh1220/blueprint-audio/internal/modules/payment/interfaces/http"
	"github.com/stretchr/testify/require"
)

type mockPaymentService struct {
	createOrderFn      func(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (*domain.Order, error)
	verifyFn           func(context.Context, uuid.UUID, string, string) (*domain.License, error)
	getOrderFn         func(context.Context, uuid.UUID) (*domain.Order, error)
	getUserOrdersFn    func(context.Context, uuid.UUID, int) ([]domain.Order, error)
	getUserLicensesFn  func(context.Context, uuid.UUID, int, string, string) ([]domain.License, int, error)
	getDownloadsFn     func(context.Context, uuid.UUID, uuid.UUID) (*application.LicenseDownloadsResponse, error)
	getProducerOrdersFn func(context.Context, uuid.UUID, int) (*application.ProducerOrderResponse, error)
}

func (m mockPaymentService) CreateOrder(ctx context.Context, u, s, l uuid.UUID) (*domain.Order, error) {
	return m.createOrderFn(ctx, u, s, l)
}
func (m mockPaymentService) VerifyPayment(ctx context.Context, o uuid.UUID, p, sig string) (*domain.License, error) {
	return m.verifyFn(ctx, o, p, sig)
}
func (m mockPaymentService) GetOrder(ctx context.Context, id uuid.UUID) (*domain.Order, error) {
	return m.getOrderFn(ctx, id)
}
func (m mockPaymentService) GetUserOrders(ctx context.Context, u uuid.UUID, p int) ([]domain.Order, error) {
	return m.getUserOrdersFn(ctx, u, p)
}
func (m mockPaymentService) GetUserLicenses(ctx context.Context, u uuid.UUID, p int, q, t string) ([]domain.License, int, error) {
	return m.getUserLicensesFn(ctx, u, p, q, t)
}
func (m mockPaymentService) GetLicenseDownloads(ctx context.Context, l, u uuid.UUID) (*application.LicenseDownloadsResponse, error) {
	return m.getDownloadsFn(ctx, l, u)
}
func (m mockPaymentService) GetProducerOrders(ctx context.Context, u uuid.UUID, p int) (*application.ProducerOrderResponse, error) {
	return m.getProducerOrdersFn(ctx, u, p)
}

func authedReq(method, path, body string) *http.Request {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	ctx := context.WithValue(r.Context(), middleware.ContextKeyUserId, uuid.New())
	return r.WithContext(ctx)
}

func TestPaymentHandler_BasicFlows(t *testing.T) {
	h := paymenthttp.NewPaymentHandler(mockPaymentService{
		createOrderFn: func(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (*domain.Order, error) {
			return &domain.Order{ID: uuid.New()}, nil
		},
		verifyFn: func(context.Context, uuid.UUID, string, string) (*domain.License, error) {
			return &domain.License{ID: uuid.New()}, nil
		},
		getOrderFn: func(context.Context, uuid.UUID) (*domain.Order, error) {
			return &domain.Order{ID: uuid.New()}, nil
		},
		getUserOrdersFn: func(context.Context, uuid.UUID, int) ([]domain.Order, error) { return []domain.Order{{ID: uuid.New()}}, nil },
		getUserLicensesFn: func(context.Context, uuid.UUID, int, string, string) ([]domain.License, int, error) {
			return []domain.License{{ID: uuid.New()}}, 1, nil
		},
		getDownloadsFn: func(context.Context, uuid.UUID, uuid.UUID) (*application.LicenseDownloadsResponse, error) {
			return &application.LicenseDownloadsResponse{LicenseID: uuid.NewString()}, nil
		},
		getProducerOrdersFn: func(context.Context, uuid.UUID, int) (*application.ProducerOrderResponse, error) {
			return &application.ProducerOrderResponse{Total: 1}, nil
		},
	})

	specID := uuid.NewString()
	licID := uuid.NewString()

	w := httptest.NewRecorder()
	h.CreateOrder(w, authedReq(http.MethodPost, "/orders", `{"spec_id":"`+specID+`","license_option_id":"`+licID+`"}`))
	require.Equal(t, http.StatusOK, w.Code)

	orderID := uuid.NewString()
	w = httptest.NewRecorder()
	h.VerifyPayment(w, authedReq(http.MethodPost, "/verify", `{"order_id":"`+orderID+`","razorpay_payment_id":"p","razorpay_signature":"s"}`))
	require.Equal(t, http.StatusOK, w.Code)

	w = httptest.NewRecorder()
	r := authedReq(http.MethodGet, "/orders/"+orderID, "")
	r.SetPathValue("id", orderID)
	h.GetOrder(w, r)
	require.Equal(t, http.StatusOK, w.Code)

	w = httptest.NewRecorder()
	h.GetUserOrders(w, authedReq(http.MethodGet, "/orders?page=2", ""))
	require.Equal(t, http.StatusOK, w.Code)

	w = httptest.NewRecorder()
	h.GetUserLicenses(w, authedReq(http.MethodGet, "/licenses?page=1&q=s&type=Basic", ""))
	require.Equal(t, http.StatusOK, w.Code)

	w = httptest.NewRecorder()
	r = authedReq(http.MethodGet, "/licenses/downloads/"+licID, "")
	r.SetPathValue("id", licID)
	h.GetLicenseDownloads(w, r)
	require.Equal(t, http.StatusOK, w.Code)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))

	w = httptest.NewRecorder()
	h.GetProducerOrders(w, authedReq(http.MethodGet, "/producer/orders?page=1", ""))
	require.Equal(t, http.StatusOK, w.Code)
}

func TestPaymentHandler_ErrorBranches(t *testing.T) {
	h := paymenthttp.NewPaymentHandler(mockPaymentService{
		createOrderFn: func(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (*domain.Order, error) { return nil, errors.New("x") },
		verifyFn: func(context.Context, uuid.UUID, string, string) (*domain.License, error) { return nil, errors.New("bad") },
		getOrderFn: func(context.Context, uuid.UUID) (*domain.Order, error) { return nil, errors.New("nf") },
		getUserOrdersFn: func(context.Context, uuid.UUID, int) ([]domain.Order, error) { return nil, errors.New("x") },
		getUserLicensesFn: func(context.Context, uuid.UUID, int, string, string) ([]domain.License, int, error) {
			return nil, 0, errors.New("x")
		},
		getDownloadsFn: func(context.Context, uuid.UUID, uuid.UUID) (*application.LicenseDownloadsResponse, error) {
			return nil, errors.New("license not found")
		},
		getProducerOrdersFn: func(context.Context, uuid.UUID, int) (*application.ProducerOrderResponse, error) {
			return nil, errors.New("x")
		},
	})

	w := httptest.NewRecorder()
	h.CreateOrder(w, httptest.NewRequest(http.MethodPost, "/orders", strings.NewReader("{}")))
	require.Equal(t, http.StatusUnauthorized, w.Code)

	w = httptest.NewRecorder()
	h.CreateOrder(w, authedReq(http.MethodPost, "/orders", `bad`))
	require.Equal(t, http.StatusBadRequest, w.Code)

	w = httptest.NewRecorder()
	h.VerifyPayment(w, authedReq(http.MethodPost, "/verify", `bad`))
	require.Equal(t, http.StatusBadRequest, w.Code)

	w = httptest.NewRecorder()
	r := authedReq(http.MethodGet, "/orders/bad", "")
	r.SetPathValue("id", "bad")
	h.GetOrder(w, r)
	require.Equal(t, http.StatusBadRequest, w.Code)

	w = httptest.NewRecorder()
	h.GetUserOrders(w, authedReq(http.MethodGet, "/orders?page=x", ""))
	require.Equal(t, http.StatusInternalServerError, w.Code)

	w = httptest.NewRecorder()
	h.GetUserLicenses(w, authedReq(http.MethodGet, "/licenses?page=x", ""))
	require.Equal(t, http.StatusInternalServerError, w.Code)

	w = httptest.NewRecorder()
	r = authedReq(http.MethodGet, "/licenses/downloads/bad", "")
	r.SetPathValue("id", "bad")
	h.GetLicenseDownloads(w, r)
	require.Equal(t, http.StatusBadRequest, w.Code)

	w = httptest.NewRecorder()
	r = authedReq(http.MethodGet, "/licenses/downloads/"+uuid.NewString(), "")
	r.SetPathValue("id", uuid.NewString())
	h.GetLicenseDownloads(w, r)
	require.Equal(t, http.StatusNotFound, w.Code)

	w = httptest.NewRecorder()
	h.GetProducerOrders(w, authedReq(http.MethodGet, "/producer/orders", ""))
	require.Equal(t, http.StatusInternalServerError, w.Code)
}
