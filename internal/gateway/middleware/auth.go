package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/saransh1220/blueprint-audio/internal/shared/utils"
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
		tokenStr := ""
		authHeader := r.Header.Get("Authorization")

		if authHeader != "" {
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) == 2 && parts[0] == "Bearer" {
				tokenStr = parts[1]
			}
		}

		if tokenStr == "" {
			tokenStr = r.URL.Query().Get("token")
		}

		if tokenStr == "" {
			http.Error(w, `{"error": "missing or invalid authorization"}`, http.StatusUnauthorized)
			return
		}

		// Note: ValidateToken will be moved to auth module's JWT provider later
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

// FlexibleAuth attempts to authenticate the user but proceeds even if no token is present.
// If a valid token is found, it injects the UserID and Role into the context.
// If no token or invalid token, it simply proceeds without injecting identity.
func (m *AuthMiddleWare) FlexibleAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			next.ServeHTTP(w, r)
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			next.ServeHTTP(w, r)
			return
		}

		tokenStr := parts[1]
		claims, err := utils.ValidateToken(tokenStr, m.jwtSecret)
		if err != nil {
			// Token invalid/expired - proceed as guest
			next.ServeHTTP(w, r)
			return
		}

		// Inject Identity & Role into Context
		ctx := context.WithValue(r.Context(), ContextKeyUserId, claims.UserID)
		ctx = context.WithValue(ctx, ContextKeyRole, claims.Role)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
