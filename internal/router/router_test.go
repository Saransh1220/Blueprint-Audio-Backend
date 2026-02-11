package router_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/saransh1220/blueprint-audio/internal/handler"
	"github.com/saransh1220/blueprint-audio/internal/middleware"
	"github.com/saransh1220/blueprint-audio/internal/router"
	"github.com/stretchr/testify/assert"
)

func TestRouter_HealthRoute(t *testing.T) {
	r := router.NewRouter(
		&handler.AuthHandler{},
		middleware.NewAuthMiddleware("secret"),
		&handler.SpecHandler{},
		&handler.UserHandler{},
		&handler.PaymentHandler{},
		&handler.AnalyticsHandler{},
	).Setup()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "OK", w.Body.String())
}

func TestRouter_NewProtectedRoutesRegistered(t *testing.T) {
	r := router.NewRouter(
		&handler.AuthHandler{},
		middleware.NewAuthMiddleware("secret"),
		&handler.SpecHandler{},
		&handler.UserHandler{},
		&handler.PaymentHandler{},
		&handler.AnalyticsHandler{},
	).Setup()

	req := httptest.NewRequest(http.MethodGet, "/orders/producer", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	req = httptest.NewRequest(http.MethodGet, "/analytics/top-specs", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
