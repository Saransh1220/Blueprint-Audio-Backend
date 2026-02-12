package gateway

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// Server represents the HTTP server
type Server struct {
	httpServer *http.Server
	port       string
}

// NewServer creates a new HTTP server
func NewServer(port string, handler http.Handler) *Server {
	return &Server{
		httpServer: &http.Server{
			Addr:         ":" + port,
			Handler:      handler,
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 15 * time.Second,
			IdleTimeout:  60 * time.Second,
		},
		port: port,
	}
}

// Start starts the HTTP server with graceful shutdown
func (s *Server) Start() error {
	// Channel to listen for errors from the HTTP server
	serverErrors := make(chan error, 1)

	// Start the server
	go func() {
		log.Printf("Server starting on port %s", s.port)
		serverErrors <- s.httpServer.ListenAndServe()
	}()

	// Channel to listen for interrupt signals
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	// Block until we receive an error or shutdown signal
	select {
	case err := <-serverErrors:
		return fmt.Errorf("server error: %w", err)

	case sig := <-shutdown:
		log.Printf("Server shutting down: %v", sig)

		// Give ongoing requests 20 seconds to complete
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		if err := s.httpServer.Shutdown(ctx); err != nil {
			s.httpServer.Close()
			return fmt.Errorf("could not gracefully shutdown server: %w", err)
		}

		log.Println("Server stopped gracefully")
	}

	return nil
}
