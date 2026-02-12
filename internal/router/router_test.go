package router_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/saransh1220/blueprint-audio/internal/middleware"
	analytics_http "github.com/saransh1220/blueprint-audio/internal/modules/analytics/interfaces/http"
	auth_http "github.com/saransh1220/blueprint-audio/internal/modules/auth/interfaces/http"
	catalog_http "github.com/saransh1220/blueprint-audio/internal/modules/catalog/interfaces/http"
	payment_http "github.com/saransh1220/blueprint-audio/internal/modules/payment/interfaces/http"
	user_http "github.com/saransh1220/blueprint-audio/internal/modules/user/interfaces/http"
	"github.com/saransh1220/blueprint-audio/internal/router"
	"github.com/stretchr/testify/assert"
)

func TestRouter_HealthRoute(t *testing.T) {
	r := router.NewRouter(
		&auth_http.AuthHandler{},
		middleware.NewAuthMiddleware("secret"),
		&catalog_http.SpecHandler{},
		&user_http.UserHandler{},
		&payment_http.PaymentHandler{},
		&analytics_http.AnalyticsHandler{},
	).Setup()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "OK", w.Body.String())
}

func TestRouter_NewProtectedRoutesRegistered(t *testing.T) {
	r := router.NewRouter(
		&auth_http.AuthHandler{},
		middleware.NewAuthMiddleware("secret"),
		&catalog_http.SpecHandler{},
		&user_http.UserHandler{},
		&payment_http.PaymentHandler{},
		&analytics_http.AnalyticsHandler{},
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
