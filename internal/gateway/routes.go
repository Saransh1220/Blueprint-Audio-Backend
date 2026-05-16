package gateway

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/saransh1220/blueprint-audio/internal/gateway/middleware"
	admin_http "github.com/saransh1220/blueprint-audio/internal/modules/admin/interfaces/http"
	analytics_http "github.com/saransh1220/blueprint-audio/internal/modules/analytics/interfaces/http"
	auth_http "github.com/saransh1220/blueprint-audio/internal/modules/auth/interfaces/http"
	catalog_http "github.com/saransh1220/blueprint-audio/internal/modules/catalog/interfaces/http"
	notification_http "github.com/saransh1220/blueprint-audio/internal/modules/notification/interfaces/http"
	payment_http "github.com/saransh1220/blueprint-audio/internal/modules/payment/interfaces/http"
	user_http "github.com/saransh1220/blueprint-audio/internal/modules/user/interfaces/http"
)

// RouterConfig holds all the handlers and middleware needed for routing
type RouterConfig struct {
	AuthHandler         *auth_http.AuthHandler
	AuthMiddleware      *middleware.AuthMiddleWare
	SpecHandler         *catalog_http.SpecHandler
	UserHandler         *user_http.UserHandler
	PaymentHandler      *payment_http.PaymentHandler
	AnalyticsHandler    *analytics_http.AnalyticsHandler
	NotificationHandler *notification_http.NotificationHandler
	AdminHandler        *admin_http.AdminHandler
}

// SetupRoutes creates and configures all application routes
func SetupRoutes(config RouterConfig) *http.ServeMux {
	mux := http.NewServeMux()

	// Health Check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Prometheus Metrics Endpoint
	mux.Handle("/metrics", promhttp.Handler())

	// Auth Routes
	emailActionLimiter := middleware.RateLimitMiddleware(3, 15*time.Minute)

	mux.HandleFunc("POST /register", config.AuthHandler.Register)
	mux.HandleFunc("POST /login", config.AuthHandler.Login)
	mux.HandleFunc("POST /auth/google", config.AuthHandler.GoogleLogin)
	mux.HandleFunc("POST /auth/refresh", config.AuthHandler.Refresh)
	mux.HandleFunc("POST /auth/logout", config.AuthHandler.Logout)
	mux.HandleFunc("POST /auth/verify-email", emailActionLimiter(config.AuthHandler.VerifyEmail))
	mux.HandleFunc("POST /auth/resend-verification", emailActionLimiter(config.AuthHandler.ResendVerification))
	mux.HandleFunc("POST /auth/forgot-password", emailActionLimiter(config.AuthHandler.ForgotPassword))
	mux.HandleFunc("POST /auth/reset-password", emailActionLimiter(config.AuthHandler.ResetPassword))
	mux.Handle("GET /me", config.AuthMiddleware.RequireAuth(http.HandlerFunc(config.AuthHandler.Me)))

	// Catalog/Spec Routes
	mux.Handle("GET /specs", config.AuthMiddleware.FlexibleAuth(http.HandlerFunc(config.SpecHandler.List)))
	mux.Handle("GET /specs/{id}", config.AuthMiddleware.FlexibleAuth(http.HandlerFunc(config.SpecHandler.Get)))
	mux.Handle("POST /specs", config.AuthMiddleware.RequireAuth(http.HandlerFunc(config.SpecHandler.Create)))
	mux.Handle("PATCH /specs/{id}", config.AuthMiddleware.RequireAuth(http.HandlerFunc(config.SpecHandler.Update)))
	mux.Handle("DELETE /specs/{id}", config.AuthMiddleware.RequireAuth(http.HandlerFunc(config.SpecHandler.Delete)))
	mux.Handle("POST /specs/{id}/download-free", config.AuthMiddleware.RequireAuth(http.HandlerFunc(config.SpecHandler.DownloadFree)))

	// User Routes
	mux.Handle("PATCH /users/profile", config.AuthMiddleware.RequireAuth(http.HandlerFunc(config.UserHandler.UpdateProfile)))
	mux.Handle("POST /users/profile/avatar", config.AuthMiddleware.RequireAuth(http.HandlerFunc(config.UserHandler.UploadAvatar)))
	mux.HandleFunc("GET /users/{id}/public", config.UserHandler.GetPublicProfile)
	mux.Handle("GET /users/{id}/specs", config.AuthMiddleware.FlexibleAuth(http.HandlerFunc(config.SpecHandler.GetUserSpecs)))

	// Payment Routes
	mux.Handle("POST /orders", config.AuthMiddleware.RequireAuth(http.HandlerFunc(config.PaymentHandler.CreateOrder)))
	mux.Handle("GET /orders", config.AuthMiddleware.RequireAuth(http.HandlerFunc(config.PaymentHandler.GetUserOrders)))
	mux.Handle("GET /orders/{id}", config.AuthMiddleware.RequireAuth(http.HandlerFunc(config.PaymentHandler.GetOrder)))
	mux.Handle("POST /payments/verify", config.AuthMiddleware.RequireAuth(http.HandlerFunc(config.PaymentHandler.VerifyPayment)))
	mux.Handle("GET /licenses", config.AuthMiddleware.RequireAuth(http.HandlerFunc(config.PaymentHandler.GetUserLicenses)))
	mux.Handle("GET /licenses/{id}/downloads", config.AuthMiddleware.RequireAuth(http.HandlerFunc(config.PaymentHandler.GetLicenseDownloads)))
	mux.Handle("GET /orders/producer", config.AuthMiddleware.RequireAuth(http.HandlerFunc(config.PaymentHandler.GetProducerOrders)))

	// Notification Routes
	mux.Handle("GET /notifications", config.AuthMiddleware.RequireAuth(http.HandlerFunc(config.NotificationHandler.ListNotifications)))
	mux.Handle("PATCH /notifications/{id}/read", config.AuthMiddleware.RequireAuth(http.HandlerFunc(config.NotificationHandler.MarkAsRead)))
	mux.Handle("PATCH /notifications/read-all", config.AuthMiddleware.RequireAuth(http.HandlerFunc(config.NotificationHandler.MarkAllAsRead)))
	mux.Handle("GET /notifications/unread-count", config.AuthMiddleware.RequireAuth(http.HandlerFunc(config.NotificationHandler.UnreadCount)))
	mux.Handle("GET /ws", config.AuthMiddleware.RequireAuth(http.HandlerFunc(config.NotificationHandler.Subscribe)))

	// Analytics Routes
	mux.HandleFunc("POST /specs/{id}/play", config.AnalyticsHandler.TrackPlay)
	mux.Handle("POST /specs/{id}/favorite", config.AuthMiddleware.RequireAuth(http.HandlerFunc(config.AnalyticsHandler.ToggleFavorite)))
	mux.Handle("GET /specs/{id}/analytics", config.AuthMiddleware.RequireAuth(http.HandlerFunc(config.AnalyticsHandler.GetProducerAnalytics)))
	mux.Handle("GET /analytics/overview", config.AuthMiddleware.RequireAuth(http.HandlerFunc(config.AnalyticsHandler.GetOverview)))
	mux.Handle("GET /analytics/top-specs", config.AuthMiddleware.RequireAuth(http.HandlerFunc(config.AnalyticsHandler.GetTopSpecs)))

	if config.AdminHandler != nil {
		// Super Admin Routes
		mux.Handle("GET /admin/users", config.AuthMiddleware.RequirePermission(middleware.PermissionSuperAdmin, http.HandlerFunc(config.AdminHandler.ListUsers)))
		mux.Handle("GET /admin/users/{id}", config.AuthMiddleware.RequirePermission(middleware.PermissionSuperAdmin, http.HandlerFunc(config.AdminHandler.GetUser)))
		mux.Handle("PATCH /admin/users/{id}/system-role", config.AuthMiddleware.RequirePermission(middleware.PermissionSuperAdmin, http.HandlerFunc(config.AdminHandler.UpdateUserSystemRole)))
		mux.Handle("PATCH /admin/users/{id}/status", config.AuthMiddleware.RequirePermission(middleware.PermissionSuperAdmin, http.HandlerFunc(config.AdminHandler.UpdateUserStatus)))
		mux.Handle("GET /admin/specs", config.AuthMiddleware.RequirePermission(middleware.PermissionSuperAdmin, http.HandlerFunc(config.AdminHandler.ListSpecs)))
		mux.Handle("PATCH /admin/specs/{id}", config.AuthMiddleware.RequirePermission(middleware.PermissionSuperAdmin, http.HandlerFunc(config.AdminHandler.UpdateSpec)))
		mux.Handle("DELETE /admin/specs/{id}", config.AuthMiddleware.RequirePermission(middleware.PermissionSuperAdmin, http.HandlerFunc(config.AdminHandler.DeleteSpec)))
		mux.Handle("GET /admin/orders", config.AuthMiddleware.RequirePermission(middleware.PermissionSuperAdmin, http.HandlerFunc(config.AdminHandler.ListOrders)))
		mux.Handle("GET /admin/licenses", config.AuthMiddleware.RequirePermission(middleware.PermissionSuperAdmin, http.HandlerFunc(config.AdminHandler.ListLicenses)))
		mux.Handle("GET /admin/analytics/overview", config.AuthMiddleware.RequirePermission(middleware.PermissionSuperAdmin, http.HandlerFunc(config.AdminHandler.AnalyticsOverview)))
		mux.Handle("GET /admin/audit-log", config.AuthMiddleware.RequirePermission(middleware.PermissionSuperAdmin, http.HandlerFunc(config.AdminHandler.ListAuditLog)))
	}

	return mux
}
