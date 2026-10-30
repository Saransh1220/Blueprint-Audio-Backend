package middleware

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/modules/auth/domain"
	"github.com/saransh1220/blueprint-audio/internal/shared/utils"
)

type contextKey string

const (
	ContextKeyUserId     contextKey = "user_id"
	ContextKeyRole       contextKey = "role"
	ContextKeySystemRole contextKey = "system_role"
)

// AccountState is the live, database-backed view of an account. The JWT only
// proves who the caller was when the token was issued; this is what they are
// allowed to do right now.
type AccountState struct {
	Status     domain.UserStatus
	Role       domain.UserRole
	SystemRole domain.SystemRole
}

// AccountLookup loads the current AccountState for a user.
type AccountLookup func(ctx context.Context, userID uuid.UUID) (AccountState, error)

type cachedAccount struct {
	state   AccountState
	expires time.Time
}

type AuthMiddleWare struct {
	jwtSecret string
	roles     *RoleStore

	lookup   AccountLookup
	cacheTTL time.Duration
	mu       sync.Mutex
	cache    map[uuid.UUID]cachedAccount
}

const maxAccountCacheEntries = 10000

// NewAuthMiddleware creates and returns a new instance of AuthMiddleWare.
// It initializes the middleware with the provided JWT secret key used for token validation.
// The jwtSecret parameter should contain a secure secret key for signing and verifying JWT tokens.
func NewAuthMiddleware(jwtSecret string) *AuthMiddleWare {
	return &AuthMiddleWare{jwtSecret: jwtSecret}
}

// WithAccountLookup makes the middleware re-check the account on every
// request (cached for ttl). Suspended accounts are rejected and the system
// role in the context comes from the database rather than the token, so a
// suspension or demotion takes effect within ttl instead of when the access
// token expires. Without a lookup the middleware trusts the token claims.
func (m *AuthMiddleWare) WithAccountLookup(lookup AccountLookup, ttl time.Duration) *AuthMiddleWare {
	m.lookup = lookup
	m.cacheTTL = ttl
	m.cache = make(map[uuid.UUID]cachedAccount)
	return m
}

func (m *AuthMiddleWare) accountState(ctx context.Context, userID uuid.UUID) (AccountState, error) {
	if m.cacheTTL > 0 {
		m.mu.Lock()
		c, ok := m.cache[userID]
		m.mu.Unlock()
		if ok && time.Now().Before(c.expires) {
			return c.state, nil
		}
	}
	state, err := m.lookup(ctx, userID)
	if err != nil {
		return AccountState{}, err
	}
	if m.cacheTTL > 0 {
		m.mu.Lock()
		if len(m.cache) >= maxAccountCacheEntries {
			now := time.Now()
			for id, c := range m.cache {
				if now.After(c.expires) {
					delete(m.cache, id)
				}
			}
			if len(m.cache) >= maxAccountCacheEntries {
				m.cache = make(map[uuid.UUID]cachedAccount)
			}
		}
		m.cache[userID] = cachedAccount{state: state, expires: time.Now().Add(m.cacheTTL)}
		m.mu.Unlock()
	}
	return state, nil
}

type authFailure struct {
	status int
	body   string
}

// authenticate validates the token and returns a request context carrying the
// caller's identity. allowQueryToken additionally accepts ?token=, which is
// only meant for browser WebSockets that cannot set an Authorization header.
func (m *AuthMiddleWare) authenticate(r *http.Request, allowQueryToken bool) (context.Context, *authFailure) {
	tokenStr := ""
	authHeader := r.Header.Get("Authorization")

	if authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && parts[0] == "Bearer" {
			tokenStr = parts[1]
		}
	}

	if tokenStr == "" && allowQueryToken {
		tokenStr = r.URL.Query().Get("token")
	}

	if tokenStr == "" {
		return nil, &authFailure{http.StatusUnauthorized, `{"error": "missing or invalid authorization"}`}
	}

	// Note: ValidateToken will be moved to auth module's JWT provider later
	claims, err := utils.ValidateToken(tokenStr, m.jwtSecret)
	if err != nil {
		return nil, &authFailure{http.StatusUnauthorized, `{"error": "invalid or expired token"}`}
	}

	role := claims.Role
	systemRole := claims.SystemRole
	if m.lookup != nil {
		state, err := m.accountState(r.Context(), claims.UserID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, &authFailure{http.StatusUnauthorized, `{"error": "invalid or expired token"}`}
			}
			return nil, &authFailure{http.StatusServiceUnavailable, `{"error": "unable to verify account"}`}
		}
		if state.Status == domain.UserStatusSuspended {
			return nil, &authFailure{http.StatusForbidden, `{"error": "account suspended"}`}
		}
		role = string(state.Role)
		systemRole = string(state.SystemRole)
	}

	//  Inject Identity & Role into Context
	ctx := context.WithValue(r.Context(), ContextKeyUserId, claims.UserID)
	ctx = context.WithValue(ctx, ContextKeyRole, role)
	ctx = context.WithValue(ctx, ContextKeySystemRole, systemRole)
	return ctx, nil
}

// RequireAuth is a middleware function that enforces authentication on HTTP requests.
// It validates the presence and format of a Bearer token in the Authorization header,
// verifies the token's validity and expiration using the stored JWT secret, and injects
// the authenticated user's ID and role into the request context for downstream handlers.
// If authentication fails at any step, it returns a 401 Unauthorized response with a
// descriptive error message. On success, the request is passed to the next handler with
// the user context enriched with identity and role information for RBAC purposes.
// Tokens in the query string are NOT accepted; they leak into logs and history.
func (m *AuthMiddleWare) RequireAuth(next http.Handler) http.Handler {
	return m.require(next, false)
}

// RequireAuthAllowQueryToken is RequireAuth that also accepts ?token=. Use it
// only for endpoints a browser must open without custom headers (WebSocket).
func (m *AuthMiddleWare) RequireAuthAllowQueryToken(next http.Handler) http.Handler {
	return m.require(next, true)
}

func (m *AuthMiddleWare) require(next http.Handler, allowQueryToken bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, fail := m.authenticate(r, allowQueryToken)
		if fail != nil {
			http.Error(w, fail.body, fail.status)
			return
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// FlexibleAuth attempts to authenticate the user but proceeds even if no token is present.
// If a valid token is found, it injects the UserID and Role into the context.
// If no token, an invalid token, or a suspended account, it proceeds as a guest
// without injecting identity.
func (m *AuthMiddleWare) FlexibleAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			next.ServeHTTP(w, r)
			return
		}
		ctx, fail := m.authenticate(r, false)
		if fail != nil {
			next.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
