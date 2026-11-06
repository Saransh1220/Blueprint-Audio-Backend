package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTraceMiddleware_GeneratesNewIDWhenAbsent(t *testing.T) {
	var capturedID string
	handler := TraceMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedID = GetRequestID(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if capturedID == "" {
		t.Fatal("expected non-empty trace ID to be injected in context")
	}
	respHeader := rr.Header().Get(HeaderRequestID)
	if respHeader != capturedID {
		t.Fatalf("expected response header %s to equal context id %s", respHeader, capturedID)
	}
}

func TestTraceMiddleware_PropagatesExistingID(t *testing.T) {
	customID := "client-trace-12345"
	var capturedID string
	handler := TraceMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedID = GetRequestID(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set(HeaderRequestID, customID)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if capturedID != customID {
		t.Fatalf("expected propagated ID %q, got %q", customID, capturedID)
	}
	if rr.Header().Get(HeaderRequestID) != customID {
		t.Fatalf("expected response header %q, got %q", customID, rr.Header().Get(HeaderRequestID))
	}
}

func TestTraceMiddleware_PropagatesCorrelationID(t *testing.T) {
	customID := "correlation-67890"
	var capturedID string
	handler := TraceMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedID = GetRequestID(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set(HeaderCorrelationID, customID)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if capturedID != customID {
		t.Fatalf("expected propagated ID %q, got %q", customID, capturedID)
	}
}

func TestGetRequestID_EmptyOrNilContext(t *testing.T) {
	if id := GetRequestID(nil); id != "" {
		t.Fatalf("expected empty string for nil context, got %q", id)
	}
	if id := GetRequestID(context.Background()); id != "" {
		t.Fatalf("expected empty string for background context, got %q", id)
	}
}
