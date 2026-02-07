package router

import (
	"net/http"

	"github.com/saransh1220/blueprint-audio/internal/handler"
	"github.com/saransh1220/blueprint-audio/internal/middleware"
)

type Router struct {
	authHandler      *handler.AuthHandler
	authMiddleware   *middleware.AuthMiddleWare
	specHandler      *handler.SpecHandler
	userHandler      *handler.UserHandler
	paymentHandler   *handler.PaymentHandler
	analyticsHandler *handler.AnalyticsHandler
}

func NewRouter(authHandler *handler.AuthHandler, authMiddleware *middleware.AuthMiddleWare, specHandler *handler.SpecHandler, userHandler *handler.UserHandler, paymentHandler *handler.PaymentHandler, analyticsHandler *handler.AnalyticsHandler) *Router {
	return &Router{
		authHandler:      authHandler,
		authMiddleware:   authMiddleware,
		specHandler:      specHandler,
		userHandler:      userHandler,
		paymentHandler:   paymentHandler,
		analyticsHandler: analyticsHandler,
	}
}

func (r *Router) Setup() *http.ServeMux {
	mux := http.NewServeMux()

	// Health Check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// API Routes
	mux.HandleFunc("POST /register", r.authHandler.Register)
	mux.HandleFunc("POST /login", r.authHandler.Login)
	mux.Handle("GET /me", r.authMiddleware.RequireAuth(http.HandlerFunc(r.authHandler.Me)))

	//BEATS
	mux.HandleFunc("GET /specs", r.specHandler.List)
	mux.HandleFunc("GET /specs/{id}", r.specHandler.Get)

	mux.Handle("POST /specs", r.authMiddleware.RequireAuth(http.HandlerFunc(r.specHandler.Create)))
	mux.Handle("PATCH /specs/{id}", r.authMiddleware.RequireAuth(http.HandlerFunc(r.specHandler.Update)))
	mux.Handle("DELETE /specs/{id}", r.authMiddleware.RequireAuth(http.HandlerFunc(r.specHandler.Delete)))

	// User routes
	mux.Handle("PATCH /users/profile", r.authMiddleware.RequireAuth(http.HandlerFunc(r.userHandler.UpdateProfile)))
	mux.Handle("POST /users/profile/avatar", r.authMiddleware.RequireAuth(http.HandlerFunc(r.userHandler.UploadAvatar)))
	mux.HandleFunc("GET /users/{id}/public", r.userHandler.GetPublicProfile)
	mux.HandleFunc("GET /users/{id}/specs", r.specHandler.GetUserSpecs)

	// Payment routes (protected)
	mux.Handle("POST /orders", r.authMiddleware.RequireAuth(http.HandlerFunc(r.paymentHandler.CreateOrder)))
	mux.Handle("GET /orders", r.authMiddleware.RequireAuth(http.HandlerFunc(r.paymentHandler.GetUserOrders)))
	mux.Handle("GET /orders/{id}", r.authMiddleware.RequireAuth(http.HandlerFunc(r.paymentHandler.GetOrder)))
	mux.Handle("POST /payments/verify", r.authMiddleware.RequireAuth(http.HandlerFunc(r.paymentHandler.VerifyPayment)))
	mux.Handle("GET /licenses", r.authMiddleware.RequireAuth(http.HandlerFunc(r.paymentHandler.GetUserLicenses)))
	mux.Handle("GET /licenses/{id}/downloads", r.authMiddleware.RequireAuth(http.HandlerFunc(r.paymentHandler.GetLicenseDownloads)))

	// Analytics routes
	mux.HandleFunc("POST /specs/{id}/play", r.analyticsHandler.TrackPlay)                                                            // Public - track plays
	mux.HandleFunc("POST /specs/{id}/download-free", r.analyticsHandler.DownloadFreeMp3)                                             // Public - download free MP3
	mux.Handle("POST /specs/{id}/favorite", r.authMiddleware.RequireAuth(http.HandlerFunc(r.analyticsHandler.ToggleFavorite)))       // Protected - toggle favorite
	mux.Handle("GET /specs/{id}/analytics", r.authMiddleware.RequireAuth(http.HandlerFunc(r.analyticsHandler.GetProducerAnalytics))) // Protected - get producer analytics
	mux.Handle("GET /analytics/overview", r.authMiddleware.RequireAuth(http.HandlerFunc(r.analyticsHandler.GetOverview)))            // Protected - get dashboard overview

	return mux
}
