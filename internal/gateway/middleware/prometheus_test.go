package middleware

import (
	"bufio"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
)

type hijackableWriter struct {
	http.ResponseWriter
	hijackErr error
}

func (h hijackableWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h.hijackErr != nil {
		return nil, nil, h.hijackErr
	}
	c1, c2 := net.Pipe()
	_ = c2.Close()
	rw := bufio.NewReadWriter(bufio.NewReader(c1), bufio.NewWriter(c1))
	return c1, rw, nil
}

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

func TestResponseWriter_Hijack(t *testing.T) {
	t.Run("supported_success_sets_status", func(t *testing.T) {
		base := httptest.NewRecorder()
		rw := &responseWriter{
			ResponseWriter: hijackableWriter{ResponseWriter: base},
			status:         http.StatusOK,
		}

		conn, _, err := rw.Hijack()
		assert.NoError(t, err)
		assert.Equal(t, http.StatusSwitchingProtocols, rw.status)
		if conn != nil {
			_ = conn.Close()
		}
	})

	t.Run("supported_error_keeps_status", func(t *testing.T) {
		base := httptest.NewRecorder()
		rw := &responseWriter{
			ResponseWriter: hijackableWriter{ResponseWriter: base, hijackErr: errors.New("boom")},
			status:         http.StatusOK,
		}

		_, _, err := rw.Hijack()
		assert.EqualError(t, err, "boom")
		assert.Equal(t, http.StatusOK, rw.status)
	})

	t.Run("unsupported_hijack_returns_error", func(t *testing.T) {
		base := httptest.NewRecorder()
		rw := &responseWriter{
			ResponseWriter: base,
			status:         http.StatusOK,
		}

		_, _, err := rw.Hijack()
		assert.EqualError(t, err, "hijack not supported")
		assert.Equal(t, http.StatusOK, rw.status)
	})
}
