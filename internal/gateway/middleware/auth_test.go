package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/shared/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testSecret = "test-jwt-secret"

func TestRequireAuth_Success(t *testing.T) {
	middleware := NewAuthMiddleware(testSecret)

	// Generate valid token
	userID := uuid.New()
	token, err := utils.GenerateToken(userID, "test@example.com", "user", testSecret, 1*time.Hour)
	require.NoError(t, err)

	// Create request
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	rec := httptest.NewRecorder()

	// Handler to verify context
	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		// Verify user ID in context
		ctxUserID := r.Context().Value(ContextKeyUserId)
		assert.Equal(t, userID, ctxUserID)
		// Verify role in context
		ctxRole := r.Context().Value(ContextKeyRole)
		assert.Equal(t, "user", ctxRole)
	})

	middleware.RequireAuth(next).ServeHTTP(rec, req)

	assert.True(t, nextCalled)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestRequireAuth_MissingHeader(t *testing.T) {
	middleware := NewAuthMiddleware(testSecret)

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
	})

	middleware.RequireAuth(next).ServeHTTP(rec, req)

	assert.False(t, nextCalled)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Contains(t, rec.Body.String(), "missing or invalid authorization")
}

func TestRequireAuth_InvalidFormat(t *testing.T) {
	middleware := NewAuthMiddleware(testSecret)

	tests := []struct {
		name   string
		header string
	}{
		{"no_bearer", "token123"},
		{"wrong_prefix", "Basic token123"},
		{"missing_token", "Bearer"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			req.Header.Set("Authorization", tt.header)
			rec := httptest.NewRecorder()

			nextCalled := false
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				nextCalled = true
			})

			middleware.RequireAuth(next).ServeHTTP(rec, req)

			assert.False(t, nextCalled)
			assert.Equal(t, http.StatusUnauthorized, rec.Code)
		})
	}
}

func TestRequireAuth_InvalidToken(t *testing.T) {
	middleware := NewAuthMiddleware(testSecret)

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	rec := httptest.NewRecorder()

	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
	})

	middleware.RequireAuth(next).ServeHTTP(rec, req)

	assert.False(t, nextCalled)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Contains(t, rec.Body.String(), "invalid or expired token")
}

func TestFlexibleAuth_WithValidToken(t *testing.T) {
	middleware := NewAuthMiddleware(testSecret)

	userID := uuid.New()
	token, err := utils.GenerateToken(userID, "test@example.com", "admin", testSecret, 1*time.Hour)
	require.NoError(t, err)

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		ctxUserID := r.Context().Value(ContextKeyUserId)
		assert.Equal(t, userID, ctxUserID)
		ctxRole := r.Context().Value(ContextKeyRole)
		assert.Equal(t, "admin", ctxRole)
	})

	middleware.FlexibleAuth(next).ServeHTTP(rec, req)

	assert.True(t, nextCalled)
}

func TestFlexibleAuth_WithoutToken(t *testing.T) {
	middleware := NewAuthMiddleware(testSecret)

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		// Verify no user context
		ctxUserID := r.Context().Value(ContextKeyUserId)
		assert.Nil(t, ctxUserID)
	})

	middleware.FlexibleAuth(next).ServeHTTP(rec, req)

	assert.True(t, nextCalled)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestFlexibleAuth_WithInvalidToken(t *testing.T) {
	middleware := NewAuthMiddleware(testSecret)

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer invalid")
	rec := httptest.NewRecorder()

	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		// Should proceed as guest
		ctxUserID := r.Context().Value(ContextKeyUserId)
		assert.Nil(t, ctxUserID)
	})

	middleware.FlexibleAuth(next).ServeHTTP(rec, req)

	assert.True(t, nextCalled)
	assert.Equal(t, http.StatusOK, rec.Code)
}
