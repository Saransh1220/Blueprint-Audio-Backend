package gateway

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewServer(t *testing.T) {
	mux := http.NewServeMux()
	server := NewServer("8080", mux)

	assert.NotNil(t, server)
	assert.Equal(t, "8080", server.port)
	assert.NotNil(t, server.httpServer)
	assert.Equal(t, ":8080", server.httpServer.Addr)
}

func TestServer_Handler(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test"))
	})

	server := NewServer("8080", handler)

	assert.NotNil(t, server.httpServer.Handler)
}

func TestServer_Timeouts(t *testing.T) {
	mux := http.NewServeMux()
	server := NewServer("8080", mux)

	// Verify timeouts are set (check actual values from server.go)
	assert.Equal(t, 15*time.Second, server.httpServer.ReadTimeout)
	assert.Equal(t, 15*time.Second, server.httpServer.WriteTimeout)
	assert.Equal(t, 60*time.Second, server.httpServer.IdleTimeout)
}
