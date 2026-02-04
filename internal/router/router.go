package router

import (
	"net/http"

	"github.com/saransh1220/blueprint-audio/internal/handler"
	"github.com/saransh1220/blueprint-audio/internal/middleware"
)

type Router struct {
	authHandler    *handler.AuthHandler
	authMiddleware *middleware.AuthMiddleWare
	specHandler    *handler.SpecHandler
	paymentHandler *handler.PaymentHandler
}

func NewRouter(authHandler *handler.AuthHandler, authMiddleware *middleware.AuthMiddleWare, specHandler *handler.SpecHandler, paymentHandler *handler.PaymentHandler) *Router {
	return &Router{
		authHandler:    authHandler,
		authMiddleware: authMiddleware,
		specHandler:    specHandler,
		paymentHandler: paymentHandler,
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
	mux.Handle("DELETE /specs/{id}", r.authMiddleware.RequireAuth(http.HandlerFunc(r.specHandler.Delete)))

	// Payment routes (protected)
	mux.Handle("POST /orders", r.authMiddleware.RequireAuth(http.HandlerFunc(r.paymentHandler.CreateOrder)))
	mux.Handle("GET /orders", r.authMiddleware.RequireAuth(http.HandlerFunc(r.paymentHandler.GetUserOrders)))
	mux.Handle("GET /orders/{id}", r.authMiddleware.RequireAuth(http.HandlerFunc(r.paymentHandler.GetOrder)))
	mux.Handle("POST /payments/verify", r.authMiddleware.RequireAuth(http.HandlerFunc(r.paymentHandler.VerifyPayment)))
	mux.Handle("GET /licenses", r.authMiddleware.RequireAuth(http.HandlerFunc(r.paymentHandler.GetUserLicenses)))
	mux.Handle("GET /licenses/{id}/downloads", r.authMiddleware.RequireAuth(http.HandlerFunc(r.paymentHandler.GetLicenseDownloads)))

	return mux
}
