package middleware

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestWebSocketTicketLifecycle(t *testing.T) {
	auth := NewAuthMiddleware("test-secret")
	tickets := NewWebSocketTickets(auth)
	now := time.Now()
	tickets.now = func() time.Time { return now }
	user := uuid.New()
	issue := func() string {
		req := httptest.NewRequest(http.MethodPost, "/ws/ticket", nil)
		req = req.WithContext(context.WithValue(req.Context(), ContextKeyUserId, user))
		rec := httptest.NewRecorder()
		tickets.Issue(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		require.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
		var body struct {
			Ticket string `json:"ticket"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		return body.Ticket
	}
	endpoint := tickets.Authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, user, r.Context().Value(ContextKeyUserId))
		require.Empty(t, r.URL.Query().Get("ticket"))
		w.WriteHeader(http.StatusNoContent)
	}))
	request := func(key string) int {
		rec := httptest.NewRecorder()
		endpoint.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ws?ticket="+key, nil))
		return rec.Code
	}
	ticket := issue()
	require.Equal(t, http.StatusNoContent, request(ticket))
	require.Equal(t, http.StatusUnauthorized, request(ticket), "tickets cannot be replayed")
	expired := issue()
	now = now.Add(31 * time.Second)
	require.Equal(t, http.StatusUnauthorized, request(expired), "expired tickets cannot connect")
	require.Equal(t, http.StatusUnauthorized, request("arbitrary"))
	unauth := httptest.NewRecorder()
	tickets.Issue(unauth, httptest.NewRequest(http.MethodPost, "/ws/ticket", nil))
	require.Equal(t, http.StatusUnauthorized, unauth.Code)
}

func TestRateLimitDoesNotTrustSpoofedForwardingHeaders(t *testing.T) {
	t.Setenv("TRUST_PROXY_HEADERS", "false")
	handler := RateLimitMiddleware(1, time.Minute)(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	for i, address := range []string{"1.1.1.1", "2.2.2.2"} {
		r := httptest.NewRequest(http.MethodPost, "/login", nil)
		r.RemoteAddr = "[2001:db8::1]:1234"
		r.Header.Set("X-Forwarded-For", address)
		w := httptest.NewRecorder()
		handler(w, r)
		if i == 0 {
			require.Equal(t, http.StatusNoContent, w.Code)
		} else {
			require.Equal(t, http.StatusTooManyRequests, w.Code)
		}
	}
}
