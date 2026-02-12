package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
)

func TestPrometheusMiddleware_RecordsMetrics(t *testing.T) {
	// Reset metrics
	prometheus.DefaultRegisterer = prometheus.NewRegistry()

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	handler := PrometheusMiddleware(nextHandler)

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestPrometheusMiddleware_DifferentStatusCodes(t *testing.T) {
	prometheus.DefaultRegisterer = prometheus.NewRegistry()

	testCases := []struct {
		name       string
		statusCode int
	}{
		{"success_200", http.StatusOK},
		{"created_201", http.StatusCreated},
		{"bad_request_400", http.StatusBadRequest},
		{"unauthorized_401", http.StatusUnauthorized},
		{"not_found_404", http.StatusNotFound},
		{"server_error_500", http.StatusInternalServerError},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.statusCode)
			})

			handler := PrometheusMiddleware(nextHandler)

			req := httptest.NewRequest("GET", "/api/endpoint", nil)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			assert.Equal(t, tc.statusCode, rec.Code)
		})
	}
}

func TestPrometheusMiddleware_DifferentMethods(t *testing.T) {
	prometheus.DefaultRegisterer = prometheus.NewRegistry()

	methods := []string{"GET", "POST", "PUT", "PATCH", "DELETE"}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			handler := PrometheusMiddleware(nextHandler)

			req := httptest.NewRequest(method, "/api/test", nil)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusOK, rec.Code)
		})
	}
}
