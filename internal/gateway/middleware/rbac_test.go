package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/modules/auth/domain"
	"github.com/saransh1220/blueprint-audio/internal/shared/utils"
	"github.com/stretchr/testify/assert"
)

func TestHasPermission(t *testing.T) {
	assert.True(t, HasPermission("super_admin", PermissionSuperAdmin))
	assert.False(t, HasPermission("user", PermissionSuperAdmin))
	assert.False(t, HasPermission("super_admin", "unknown"))
}

func TestRequirePermission(t *testing.T) {
	m := NewAuthMiddleware(testSecret)
	for _, role := range []string{"super_admin", "user"} {
		t.Run(role, func(t *testing.T) {
			token, err := utils.GenerateToken(uuid.New(), "admin@example.com", "user", role, testSecret, time.Hour)
			assert.NoError(t, err)
			called := false
			h := m.RequirePermission(PermissionSuperAdmin, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
			r := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			h.ServeHTTP(r, req)
			assert.Equal(t, role == "super_admin", called)
			if role == "super_admin" {
				assert.Equal(t, http.StatusOK, r.Code)
			} else {
				assert.Equal(t, http.StatusForbidden, r.Code)
			}
		})
	}
}

func TestRequireSystemRole(t *testing.T) {
	m := NewAuthMiddleware(testSecret)
	token, err := utils.GenerateToken(uuid.New(), "user@example.com", "user", "user", testSecret, time.Hour)
	assert.NoError(t, err)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	m.RequireSystemRole([]string{"super_admin"}, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("next should not run") })).ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestRequireUserRole(t *testing.T) {
	m := NewAuthMiddleware(testSecret)
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := m.RequireUserRole([]domain.UserRole{domain.RoleProducer}, next)

	t.Run("requires authentication", func(t *testing.T) {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil))
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("rejects a different application role", func(t *testing.T) {
		token, err := utils.GenerateToken(
			uuid.New(), "artist@example.com", string(domain.RoleArtist), "user",
			testSecret, time.Hour,
		)
		assert.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("allows producer", func(t *testing.T) {
		token, err := utils.GenerateToken(
			uuid.New(), "producer@example.com", string(domain.RoleProducer), "user",
			testSecret, time.Hour,
		)
		assert.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusNoContent, rec.Code)
	})
}
