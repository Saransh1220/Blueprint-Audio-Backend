package router

import (
	"net/http"

	"github.com/saransh1220/blueprint-audio/internal/handler"
	"github.com/saransh1220/blueprint-audio/internal/middleware"
)

type Router struct {
	authHandler    *handler.AuthHandler
	authMiddleware *middleware.AuthMiddleWare
}

func NewRouter(authHandler *handler.AuthHandler, authMiddleware *middleware.AuthMiddleWare) *Router {
	return &Router{
		authHandler:    authHandler,
		authMiddleware: authMiddleware,
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

	return mux
}
