package middleware

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
)

// WebSocketTickets exchanges authenticated requests for single-use, short-lived
// credentials. Access tokens never enter browser WebSocket URLs. Tickets are
// process-local; multi-instance deployments must use a shared ticket store.
type WebSocketTickets struct {
	mu      sync.Mutex
	tickets map[string]socketTicket
	auth    *AuthMiddleWare
	now     func() time.Time
}

type socketTicket struct {
	userID  uuid.UUID
	expires time.Time
}

func NewWebSocketTickets(auth *AuthMiddleWare) *WebSocketTickets {
	return &WebSocketTickets{tickets: make(map[string]socketTicket), auth: auth, now: time.Now}
}

func (s *WebSocketTickets) Issue(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(ContextKeyUserId).(uuid.UUID)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		http.Error(w, "unable to issue ticket", http.StatusInternalServerError)
		return
	}
	ticket := base64.RawURLEncoding.EncodeToString(secret)
	s.mu.Lock()
	for key, value := range s.tickets {
		if !s.now().Before(value.expires) {
			delete(s.tickets, key)
		}
	}
	if len(s.tickets) >= 10000 {
		s.mu.Unlock()
		http.Error(w, "ticket capacity reached", http.StatusServiceUnavailable)
		return
	}
	s.tickets[ticket] = socketTicket{userID, s.now().Add(30 * time.Second)}
	s.mu.Unlock()
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ticket": ticket, "expires_in": 30})
}

func (s *WebSocketTickets) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Query().Get("ticket")
		s.mu.Lock()
		ticket, ok := s.tickets[key]
		delete(s.tickets, key)
		s.mu.Unlock()
		if !ok || !s.now().Before(ticket.expires) {
			http.Error(w, "invalid or expired ticket", http.StatusUnauthorized)
			return
		}
		// Recheck suspension at connection time, not merely when issuing a ticket.
		if s.auth.lookup != nil {
			state, err := s.auth.accountState(r.Context(), ticket.userID)
			if err != nil {
				http.Error(w, "unable to verify account", http.StatusServiceUnavailable)
				return
			}
			if state.Status == "suspended" {
				http.Error(w, "account suspended", http.StatusForbidden)
				return
			}
		}
		ctx := context.WithValue(r.Context(), ContextKeyUserId, ticket.userID)
		// Remove credentials before downstream logging or upgrade handlers.
		query := r.URL.Query()
		query.Del("ticket")
		r.URL.RawQuery = query.Encode()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
