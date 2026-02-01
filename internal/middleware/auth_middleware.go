package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/saransh1220/blueprint-audio/internal/utils"
)

type contextKey string

const (
	ContextKeyUserId contextKey = "user_id"
	ContextKeyRole   contextKey = "role"
)

type AuthMiddleWare struct {
	jwtSecret string
}

// NewAuthMiddleware creates and returns a new instance of AuthMiddleWare.
// It initializes the middleware with the provided JWT secret key used for token validation.
// The jwtSecret parameter should contain a secure secret key for signing and verifying JWT tokens.
func NewAuthMiddleware(jwtSecret string) *AuthMiddleWare {
	return &AuthMiddleWare{jwtSecret: jwtSecret}
}

// RequireAuth is a middleware function that enforces authentication on HTTP requests.
// It validates the presence and format of a Bearer token in the Authorization header,
// verifies the token's validity and expiration using the stored JWT secret, and injects
// the authenticated user's ID and role into the request context for downstream handlers.
// If authentication fails at any step, it returns a 401 Unauthorized response with a
// descriptive error message. On success, the request is passed to the next handler with
// the user context enriched with identity and role information for RBAC purposes.
func (m *AuthMiddleWare) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, `{"error": "missing authorization header"}`, http.StatusUnauthorized)
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			http.Error(w, `{"error": "invalid authorization header format"}`, http.StatusUnauthorized)
			return
		}

		tokenStr := parts[1]
		claims, err := utils.ValidateToken(tokenStr, m.jwtSecret)
		if err != nil {
			http.Error(w, `{"error": "invalid or expired token"}`, http.StatusUnauthorized)
			return
		}
		//  Inject Identity & Role into Context
		ctx := context.WithValue(r.Context(), ContextKeyUserId, claims.UserID)
		ctx = context.WithValue(ctx, ContextKeyRole, claims.Role) // <--- Crucial for RBAC

		next.ServeHTTP(w, r.WithContext(ctx))

	})
}
