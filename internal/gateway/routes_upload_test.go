package gateway

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/gateway/middleware"
	admin_http "github.com/saransh1220/blueprint-audio/internal/modules/admin/interfaces/http"
	analytics_http "github.com/saransh1220/blueprint-audio/internal/modules/analytics/interfaces/http"
	auth_http "github.com/saransh1220/blueprint-audio/internal/modules/auth/interfaces/http"
	catalog_http "github.com/saransh1220/blueprint-audio/internal/modules/catalog/interfaces/http"
	notification_http "github.com/saransh1220/blueprint-audio/internal/modules/notification/interfaces/http"
	payment_http "github.com/saransh1220/blueprint-audio/internal/modules/payment/interfaces/http"
	user_http "github.com/saransh1220/blueprint-audio/internal/modules/user/interfaces/http"
	"github.com/saransh1220/blueprint-audio/internal/shared/utils"
	"github.com/stretchr/testify/require"
)

func TestSpecUploadRoutesRequireProducer(t *testing.T) {
	const secret = "upload-route-test-secret"
	mux := SetupRoutes(RouterConfig{
		AuthHandler:         &auth_http.AuthHandler{},
		AuthMiddleware:      middleware.NewAuthMiddleware(secret),
		SpecHandler:         &catalog_http.SpecHandler{},
		SpecUploadHandler:   &catalog_http.SpecUploadHandler{},
		UserHandler:         &user_http.UserHandler{},
		PaymentHandler:      &payment_http.PaymentHandler{},
		AnalyticsHandler:    &analytics_http.AnalyticsHandler{},
		NotificationHandler: &notification_http.NotificationHandler{},
		AdminHandler:        admin_http.NewAdminHandler(nil, nil),
	})

	artistToken, err := utils.GenerateToken(
		uuid.New(), "artist@example.com", "artist", "user", secret, time.Hour,
	)
	require.NoError(t, err)

	routes := []struct {
		method string
		path   string
	}{
		{method: http.MethodPost, path: "/spec-uploads"},
		{method: http.MethodPut, path: "/spec-uploads/" + uuid.NewString() + "/metadata"},
		{method: http.MethodPost, path: "/spec-uploads/" + uuid.NewString() + "/files"},
		{
			method: http.MethodPost,
			path:   "/spec-uploads/" + uuid.NewString() + "/files/" + uuid.NewString() + "/complete",
		},
		{method: http.MethodPost, path: "/spec-uploads/" + uuid.NewString() + "/complete"},
		{method: http.MethodGet, path: "/spec-uploads/" + uuid.NewString()},
	}
	for _, route := range routes {
		t.Run(route.method+" "+route.path+" requires authentication", func(t *testing.T) {
			req := httptest.NewRequest(route.method, route.path, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			require.Equal(t, http.StatusUnauthorized, rec.Code)
		})

		t.Run(route.method+" "+route.path+" rejects artist", func(t *testing.T) {
			req := httptest.NewRequest(route.method, route.path, nil)
			req.Header.Set("Authorization", "Bearer "+artistToken)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			require.Equal(t, http.StatusForbidden, rec.Code)
		})
	}
}
