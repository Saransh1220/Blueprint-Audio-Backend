package middleware

import (
	"net/http"
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
			ip := r.Header.Get("X-Forwarded-For")
			if ip == "" {
				ip = r.RemoteAddr
			} else {
				ip = strings.Split(ip, ",")[0]
			}
			
			// Quick strip of port if it exists (like 127.0.0.1:12345)
			if idx := strings.LastIndex(ip, ":"); idx != -1 {
				ip = ip[:idx]
			}

			if !limiter.Allow(ip) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				w.Write([]byte(`{"error":"too many requests"}`))
				return
			}
			next(w, r)
		}
	}
}
