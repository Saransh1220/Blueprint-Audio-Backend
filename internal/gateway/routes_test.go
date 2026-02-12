package gateway

import (
	"net/http"
	"testing"

	"github.com/saransh1220/blueprint-audio/internal/gateway/middleware"
	analytics_http "github.com/saransh1220/blueprint-audio/internal/modules/analytics/interfaces/http"
	auth_http "github.com/saransh1220/blueprint-audio/internal/modules/auth/interfaces/http"
	catalog_http "github.com/saransh1220/blueprint-audio/internal/modules/catalog/interfaces/http"
	payment_http "github.com/saransh1220/blueprint-audio/internal/modules/payment/interfaces/http"
	user_http "github.com/saransh1220/blueprint-audio/internal/modules/user/interfaces/http"
)

func TestSetupRoutes(t *testing.T) {
	// Create mock handlers
	authHandler := &auth_http.AuthHandler{}
	authMiddleware := middleware.NewAuthMiddleware("test-secret")
	specHandler := &catalog_http.SpecHandler{}
	userHandler := &user_http.UserHandler{}
	paymentHandler := &payment_http.PaymentHandler{}
	analyticsHandler := &analytics_http.AnalyticsHandler{}

	config := RouterConfig{
		AuthHandler:      authHandler,
		AuthMiddleware:   authMiddleware,
		SpecHandler:      specHandler,
		UserHandler:      userHandler,
		PaymentHandler:   paymentHandler,
		AnalyticsHandler: analyticsHandler,
	}

	mux := SetupRoutes(config)

	// Test that mux is created
	if mux == nil {
		t.Fatal("Expected mux to be created, got nil")
	}
}

func TestSetupRoutes_HealthCheck(t *testing.T) {
	config := RouterConfig{
		AuthHandler:      &auth_http.AuthHandler{},
		AuthMiddleware:   middleware.NewAuthMiddleware("test-secret"),
		SpecHandler:      &catalog_http.SpecHandler{},
		UserHandler:      &user_http.UserHandler{},
		PaymentHandler:   &payment_http.PaymentHandler{},
		AnalyticsHandler: &analytics_http.AnalyticsHandler{},
	}

	mux := SetupRoutes(config)

	// Create a test request to /health
	req, err := http.NewRequest("GET", "/health", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Create a response recorder
	rr := &responseRecorder{}
	mux.ServeHTTP(rr, req)

	// Check status code
	if rr.statusCode != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rr.statusCode)
	}

	// Check body
	if rr.body != "OK" {
		t.Errorf("Expected body 'OK', got '%s'", rr.body)
	}
}

// responseRecorder is a helper to capture HTTP responses
type responseRecorder struct {
	statusCode int
	body       string
}

func (rr *responseRecorder) Header() http.Header {
	return http.Header{}
}

func (rr *responseRecorder) WriteHeader(statusCode int) {
	rr.statusCode = statusCode
}

func (rr *responseRecorder) Write(b []byte) (int, error) {
	rr.body = string(b)
	return len(b), nil
}
