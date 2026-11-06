package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

const (
	// ContextKeyRequestID holds the correlation/trace ID for the request.
	ContextKeyRequestID contextKey = "request_id"
	// HeaderRequestID is the standard request tracing header.
	HeaderRequestID = "X-Request-ID"
	// HeaderCorrelationID is an alternative accepted correlation header.
	HeaderCorrelationID = "X-Correlation-ID"
)

// TraceMiddleware attaches a unique trace/correlation ID to every request.
// If the client supplies an X-Request-ID or X-Correlation-ID header, it is
// validated and propagated; otherwise a new UUID is generated.
func TraceMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := strings.TrimSpace(r.Header.Get(HeaderRequestID))
		if reqID == "" {
			reqID = strings.TrimSpace(r.Header.Get(HeaderCorrelationID))
		}
		if reqID == "" || len(reqID) > 128 {
			reqID = uuid.New().String()
		}

		ctx := context.WithValue(r.Context(), ContextKeyRequestID, reqID)
		w.Header().Set(HeaderRequestID, reqID)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetRequestID returns the trace ID from the context, or an empty string if absent.
func GetRequestID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if id, ok := ctx.Value(ContextKeyRequestID).(string); ok {
		return id
	}
	return ""
}
