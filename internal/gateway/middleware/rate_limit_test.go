package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRateLimitMiddleware_Allow(t *testing.T) {
	// 2 requests per 100ms
	middleware := RateLimitMiddleware(2, 100*time.Millisecond)

	handler := middleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Req 1: allowed
	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	req1.RemoteAddr = "192.168.1.1:12345"
	rr1 := httptest.NewRecorder()
	handler(rr1, req1)
	assert.Equal(t, http.StatusOK, rr1.Code)

	// Req 2: allowed
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.RemoteAddr = "192.168.1.1:54321" // different port, same IP
	rr2 := httptest.NewRecorder()
	handler(rr2, req2)
	assert.Equal(t, http.StatusOK, rr2.Code)

	// Req 3: too many requests
	req3 := httptest.NewRequest(http.MethodGet, "/", nil)
	req3.RemoteAddr = "192.168.1.1:9999"
	rr3 := httptest.NewRecorder()
	handler(rr3, req3)
	assert.Equal(t, http.StatusTooManyRequests, rr3.Code)
	assert.Contains(t, rr3.Body.String(), "too many requests")

	// Wait for reset
	time.Sleep(150 * time.Millisecond)

	// Req 4: allowed again
	req4 := httptest.NewRequest(http.MethodGet, "/", nil)
	req4.RemoteAddr = "192.168.1.1:1111"
	rr4 := httptest.NewRecorder()
	handler(rr4, req4)
	assert.Equal(t, http.StatusOK, rr4.Code)
}

func TestRateLimitMiddleware_XForwardedFor(t *testing.T) {
	middleware := RateLimitMiddleware(1, time.Second)

	handler := middleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Req 1: allowed
	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	req1.Header.Set("X-Forwarded-For", "10.0.0.1, 10.0.0.2")
	rr1 := httptest.NewRecorder()
	handler(rr1, req1)
	assert.Equal(t, http.StatusOK, rr1.Code)

	// Req 2: blocked (same IP from header)
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.Header.Set("X-Forwarded-For", "10.0.0.1")
	rr2 := httptest.NewRecorder()
	handler(rr2, req2)
	assert.Equal(t, http.StatusTooManyRequests, rr2.Code)

	// Req 3: allowed (different IP from header)
	req3 := httptest.NewRequest(http.MethodGet, "/", nil)
	req3.Header.Set("X-Forwarded-For", "10.0.0.2")
	rr3 := httptest.NewRecorder()
	handler(rr3, req3)
	assert.Equal(t, http.StatusOK, rr3.Code)
}

func TestRateLimiter_Cleanup(t *testing.T) {
	rl := newRateLimiter(1, 10*time.Millisecond)
	rl.Allow("1.1.1.1")
	rl.Allow("2.2.2.2")

	assert.Equal(t, 2, len(rl.ipRequests))

	// Wait for expiration
	time.Sleep(20 * time.Millisecond)
	
	rl.cleanup()
	
	assert.Equal(t, 0, len(rl.ipRequests))
	assert.Equal(t, 0, len(rl.ipResets))
}
