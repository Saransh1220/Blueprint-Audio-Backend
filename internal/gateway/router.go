package gateway

import (
	"net/http"
)

// Router wraps http.ServeMux and provides route registration
type Router struct {
	mux *http.ServeMux
}

// NewRouter creates a new router
func NewRouter() *Router {
	return &Router{
		mux: http.NewServeMux(),
	}
}

// Mux returns the underlying http.ServeMux
func (r *Router) Mux() *http.ServeMux {
	return r.mux
}

// Handle registers a handler for the given pattern
func (r *Router) Handle(pattern string, handler http.Handler) {
	r.mux.Handle(pattern, handler)
}

// HandleFunc registers a handler function for the given pattern
func (r *Router) HandleFunc(pattern string, handler http.HandlerFunc) {
	r.mux.HandleFunc(pattern, handler)
}
