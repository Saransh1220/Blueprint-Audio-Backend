package gateway

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/saransh1220/blueprint-audio/internal/gateway/middleware"
	admin_http "github.com/saransh1220/blueprint-audio/internal/modules/admin/interfaces/http"
	analytics_http "github.com/saransh1220/blueprint-audio/internal/modules/analytics/interfaces/http"
	auth_http "github.com/saransh1220/blueprint-audio/internal/modules/auth/interfaces/http"
	catalog_http "github.com/saransh1220/blueprint-audio/internal/modules/catalog/interfaces/http"
	notification_http "github.com/saransh1220/blueprint-audio/internal/modules/notification/interfaces/http"
	payment_http "github.com/saransh1220/blueprint-audio/internal/modules/payment/interfaces/http"
	user_http "github.com/saransh1220/blueprint-audio/internal/modules/user/interfaces/http"
)

func TestSetupRoutesRegistersAdminRoutes(t *testing.T) {
	mux := SetupRoutes(RouterConfig{
		AuthHandler:         &auth_http.AuthHandler{},
		AuthMiddleware:      middleware.NewAuthMiddleware("test-secret"),
		SpecHandler:         &catalog_http.SpecHandler{},
		UserHandler:         &user_http.UserHandler{},
		PaymentHandler:      &payment_http.PaymentHandler{},
		AnalyticsHandler:    &analytics_http.AnalyticsHandler{},
		NotificationHandler: &notification_http.NotificationHandler{},
		AdminHandler:        admin_http.NewAdminHandler(nil, nil),
	})

	for _, path := range []string{
		"/admin/users",
		"/admin/specs",
		"/admin/orders",
		"/admin/licenses",
		"/admin/analytics/overview",
		"/admin/audit-log",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, req)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("expected %s to be registered and protected, got %d", path, response.Code)
		}
	}
}
