package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/domain"
	"github.com/saransh1220/blueprint-audio/internal/dto"
	"github.com/saransh1220/blueprint-audio/internal/handler"
	"github.com/saransh1220/blueprint-audio/internal/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func withUser(req *http.Request, userID uuid.UUID) *http.Request {
	return req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyUserId, userID))
}

func TestPaymentHandler_CreateOrder(t *testing.T) {
	svc := new(mockPaymentService)
	h := handler.NewPaymentHandler(svc)
	userID := uuid.New()
	specID := uuid.New()
	licenseID := uuid.New()

	req := httptest.NewRequest(http.MethodPost, "/orders", bytes.NewBufferString("bad"))
	w := httptest.NewRecorder()
	h.CreateOrder(w, withUser(req, userID))
	assert.Equal(t, http.StatusBadRequest, w.Code)

	body, _ := json.Marshal(map[string]string{
		"spec_id":           specID.String(),
		"license_option_id": licenseID.String(),
	})
	req = httptest.NewRequest(http.MethodPost, "/orders", bytes.NewBuffer(body))
	svc.On("CreateOrder", mock.Anything, userID, specID, licenseID).Return(nil, errors.New("oops")).Once()
	w = httptest.NewRecorder()
	h.CreateOrder(w, withUser(req, userID))
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestPaymentHandler_VerifyGetOrderGetUserOrders(t *testing.T) {
	svc := new(mockPaymentService)
	h := handler.NewPaymentHandler(svc)
	userID := uuid.New()
	orderID := uuid.New()

	req := httptest.NewRequest(http.MethodPost, "/payments/verify", bytes.NewBufferString("{}"))
	w := httptest.NewRecorder()
	h.VerifyPayment(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	reqBody, _ := json.Marshal(map[string]string{
		"order_id":            orderID.String(),
		"razorpay_payment_id": "pay_1",
		"razorpay_signature":  "sig_1",
	})
	req = httptest.NewRequest(http.MethodPost, "/payments/verify", bytes.NewBuffer(reqBody))
	svc.On("VerifyPayment", mock.Anything, orderID, "pay_1", "sig_1").Return(nil, errors.New("invalid"))
	w = httptest.NewRecorder()
	h.VerifyPayment(w, withUser(req, userID))
	assert.Equal(t, http.StatusBadRequest, w.Code)

	req = httptest.NewRequest(http.MethodGet, "/orders/"+orderID.String(), nil)
	req.SetPathValue("id", orderID.String())
	svc.On("GetOrder", mock.Anything, orderID).Return(nil, errors.New("missing")).Once()
	w = httptest.NewRecorder()
	h.GetOrder(w, withUser(req, userID))
	assert.Equal(t, http.StatusNotFound, w.Code)

	req = httptest.NewRequest(http.MethodGet, "/orders?page=2", nil)
	svc.On("GetUserOrders", mock.Anything, userID, 2).Return(nil, errors.New("db")).Once()
	w = httptest.NewRecorder()
	h.GetUserOrders(w, withUser(req, userID))
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestPaymentHandler_GetUserLicensesAndDownloads(t *testing.T) {
	svc := new(mockPaymentService)
	h := handler.NewPaymentHandler(svc)
	userID := uuid.New()
	licenseID := uuid.New()

	req := httptest.NewRequest(http.MethodGet, "/licenses?q=abc&type=Basic&page=1", nil)
	svc.On("GetUserLicenses", mock.Anything, userID, 1, "abc", "Basic").Return([]domain.License{}, 0, nil).Once()
	w := httptest.NewRecorder()
	h.GetUserLicenses(w, withUser(req, userID))
	assert.Equal(t, http.StatusOK, w.Code)

	req = httptest.NewRequest(http.MethodGet, "/licenses/"+licenseID.String()+"/downloads", nil)
	req.SetPathValue("id", licenseID.String())
	svc.On("GetLicenseDownloads", mock.Anything, licenseID, userID).Return(nil, errors.New("license not found")).Once()
	w = httptest.NewRecorder()
	h.GetLicenseDownloads(w, withUser(req, userID))
	assert.Equal(t, http.StatusNotFound, w.Code)

	resp := &dto.LicenseDownloadsResponse{LicenseID: licenseID.String()}
	req = httptest.NewRequest(http.MethodGet, "/licenses/"+licenseID.String()+"/downloads", nil)
	req.SetPathValue("id", licenseID.String())
	svc.On("GetLicenseDownloads", mock.Anything, licenseID, userID).Return(resp, nil).Once()
	w = httptest.NewRecorder()
	h.GetLicenseDownloads(w, withUser(req, userID))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestPaymentHandler_SuccessBranches(t *testing.T) {
	svc := new(mockPaymentService)
	h := handler.NewPaymentHandler(svc)
	userID := uuid.New()
	specID := uuid.New()
	licenseOptionID := uuid.New()
	orderID := uuid.New()

	body, _ := json.Marshal(map[string]string{
		"spec_id":           specID.String(),
		"license_option_id": licenseOptionID.String(),
	})
	req := httptest.NewRequest(http.MethodPost, "/orders", bytes.NewBuffer(body))
	svc.On("CreateOrder", mock.Anything, userID, specID, licenseOptionID).Return(&domain.Order{ID: orderID}, nil).Once()
	w := httptest.NewRecorder()
	h.CreateOrder(w, withUser(req, userID))
	assert.Equal(t, http.StatusOK, w.Code)

	reqBody, _ := json.Marshal(map[string]string{
		"order_id":            orderID.String(),
		"razorpay_payment_id": "pay_1",
		"razorpay_signature":  "sig_1",
	})
	req = httptest.NewRequest(http.MethodPost, "/payments/verify", bytes.NewBuffer(reqBody))
	svc.On("VerifyPayment", mock.Anything, orderID, "pay_1", "sig_1").Return(&domain.License{ID: uuid.New()}, nil).Once()
	w = httptest.NewRecorder()
	h.VerifyPayment(w, withUser(req, userID))
	assert.Equal(t, http.StatusOK, w.Code)

	req = httptest.NewRequest(http.MethodGet, "/orders/"+orderID.String(), nil)
	req.SetPathValue("id", orderID.String())
	svc.On("GetOrder", mock.Anything, orderID).Return(&domain.Order{ID: orderID}, nil).Once()
	w = httptest.NewRecorder()
	h.GetOrder(w, withUser(req, userID))
	assert.Equal(t, http.StatusOK, w.Code)
}
