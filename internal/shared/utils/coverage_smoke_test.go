package utils_test

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/saransh1220/blueprint-audio/internal/gateway"
	"github.com/saransh1220/blueprint-audio/internal/gateway/middleware"
	analytics_http "github.com/saransh1220/blueprint-audio/internal/modules/analytics/interfaces/http"
	"github.com/saransh1220/blueprint-audio/internal/modules/auth"
	auth_http "github.com/saransh1220/blueprint-audio/internal/modules/auth/interfaces/http"
	catalog_http "github.com/saransh1220/blueprint-audio/internal/modules/catalog/interfaces/http"
	fileapp "github.com/saransh1220/blueprint-audio/internal/modules/filestorage/application"
	filedomain "github.com/saransh1220/blueprint-audio/internal/modules/filestorage/domain"
	notification_http "github.com/saransh1220/blueprint-audio/internal/modules/notification/interfaces/http"
	payment_http "github.com/saransh1220/blueprint-audio/internal/modules/payment/interfaces/http"
	user_http "github.com/saransh1220/blueprint-audio/internal/modules/user/interfaces/http"
	"github.com/stretchr/testify/require"
)

type noopStorage struct{}

func (noopStorage) UploadFile(_ context.Context, _ string, _ io.Reader, _ string) (string, error) {
	return "", nil
}
func (noopStorage) DeleteFile(_ context.Context, _ string) error { return nil }
func (noopStorage) GetPresignedURL(_ context.Context, _ string, _ time.Duration) (string, error) {
	return "", nil
}
func (noopStorage) GetPresignedDownloadURL(_ context.Context, _ string, _ string, _ time.Duration) (string, error) {
	return "", nil
}
func (noopStorage) GetKeyFromURL(_ string) (string, error) { return "", nil }

var _ filedomain.FileStorage = noopStorage{}

func TestCoverageSmoke_AuthModuleAndRoutes(t *testing.T) {
	fs := fileapp.NewFileService(noopStorage{})
	m, err := auth.NewModule(&sqlx.DB{}, "secret", time.Hour, fs, "google-client-id")
	require.NoError(t, err)
	require.NotNil(t, m.Service())
	require.NotNil(t, m.UserFinder())
	require.NotNil(t, m.UserRepository())
	require.NotNil(t, m.HTTPHandler())

	cfg := gateway.RouterConfig{
		AuthHandler:         &auth_http.AuthHandler{},
		AuthMiddleware:      middleware.NewAuthMiddleware("secret"),
		SpecHandler:         &catalog_http.SpecHandler{},
		UserHandler:         &user_http.UserHandler{},
		PaymentHandler:      &payment_http.PaymentHandler{},
		AnalyticsHandler:    &analytics_http.AnalyticsHandler{},
		NotificationHandler: &notification_http.NotificationHandler{},
	}

	mux := gateway.SetupRoutes(cfg)
	require.NotNil(t, mux)
}
