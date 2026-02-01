package router

import (
	"net/http"

	"github.com/saransh1220/blueprint-audio/internal/handler"
)

type Router struct {
	authHandler *handler.AuthHandler
}

func NewRouter(authHandler *handler.AuthHandler) *Router {
	return &Router{
		authHandler: authHandler,
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

	return mux
}
