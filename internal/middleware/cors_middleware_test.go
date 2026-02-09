package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/saransh1220/blueprint-audio/internal/middleware"
	"github.com/stretchr/testify/assert"
)

func TestCORSMiddleware_Wildcard(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})

	handler := middleware.CORSMiddleware(next, "*")
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Origin", "http://example.com")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusTeapot, rr.Code)
	assert.Equal(t, "*", rr.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "true", rr.Header().Get("Access-Control-Allow-Credentials"))
}

func TestCORSMiddleware_AllowListAndPreflight(t *testing.T) {
	nextHit := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextHit = true
		w.WriteHeader(http.StatusOK)
	})
	handler := middleware.CORSMiddleware(next, "http://a.com,http://b.com")

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Origin", "http://b.com")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	assert.True(t, nextHit)
	assert.Equal(t, "http://b.com", rr.Header().Get("Access-Control-Allow-Origin"))

	nextHit = false
	preflight := httptest.NewRequest(http.MethodOptions, "/x", nil)
	preflight.Header.Set("Origin", "http://b.com")
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, preflight)
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.False(t, nextHit)
}
