package middleware

import (
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type rateLimiter struct {
	sync.Mutex
	ipRequests map[string]int
	ipResets   map[string]time.Time
	maxReq     int
	window     time.Duration
}

func newRateLimiter(maxReq int, window time.Duration) *rateLimiter {
	rl := &rateLimiter{
		ipRequests: make(map[string]int),
		ipResets:   make(map[string]time.Time),
		maxReq:     maxReq,
		window:     window,
	}

	go func() {
		for {
			time.Sleep(time.Minute)
			rl.cleanup()
		}
	}()

	return rl
}

func (rl *rateLimiter) cleanup() {
	rl.Lock()
	defer rl.Unlock()
	now := time.Now()
	for ip, resetTime := range rl.ipResets {
		if now.After(resetTime) {
			delete(rl.ipRequests, ip)
			delete(rl.ipResets, ip)
		}
	}
}

func (rl *rateLimiter) Allow(ip string) bool {
	rl.Lock()
	defer rl.Unlock()

	now := time.Now()

	if resetTime, exists := rl.ipResets[ip]; !exists || now.After(resetTime) {
		rl.ipRequests[ip] = 1
		rl.ipResets[ip] = now.Add(rl.window)
		return true
	}

	rl.ipRequests[ip]++
	return rl.ipRequests[ip] <= rl.maxReq
}

func RateLimitMiddleware(maxReq int, window time.Duration) func(http.HandlerFunc) http.HandlerFunc {
	limiter := newRateLimiter(maxReq, window)

	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			ip := r.RemoteAddr
			if host, _, err := net.SplitHostPort(ip); err == nil {
				ip = host
			}
			// Enable only behind a trusted proxy that overwrites forwarded headers.
			if os.Getenv("TRUST_PROXY_HEADERS") == "true" {
				if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
					ip = strings.TrimSpace(strings.Split(forwarded, ",")[0])
				}
			}

			if !limiter.Allow(ip) {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Retry-After", strconv.Itoa(max(1, int(window.Seconds()))))
				w.WriteHeader(http.StatusTooManyRequests)
				w.Write([]byte(`{"error":"too many requests"}`))
				return
			}
			next(w, r)
		}
	}
}
