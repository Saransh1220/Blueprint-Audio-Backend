package gateway

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewRouter(t *testing.T) {
	router := NewRouter()

	assert.NotNil(t, router)
	assert.NotNil(t, router.mux)
}

func TestRouter_Mux(t *testing.T) {
	router := NewRouter()

	// Verify mux is initialized
	assert.NotNil(t, router.mux)

	// Add a test handler
	router.mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {})

	assert.NotNil(t, router)
}

func TestRouter_HandleAndHandleFunc(t *testing.T) {
	router := NewRouter()

	router.Handle("/healthz", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	router.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	router.Mux().ServeHTTP(w, req)
	assert.Equal(t, http.StatusNoContent, w.Code)

	req = httptest.NewRequest(http.MethodGet, "/ping", nil)
	w = httptest.NewRecorder()
	router.Mux().ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}
