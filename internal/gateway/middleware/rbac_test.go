package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/modules/auth/domain"
	"github.com/saransh1220/blueprint-audio/internal/shared/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testRoles mirrors the roles seeded by migration 000039 closely enough to
// exercise Authorize; TestPermissionsMatchMigrations guards the real seed.
func testRoles() *RoleStore {
	return NewStaticRoleStore(
		RoleDef{ID: 1, Name: "artist", Kind: RoleKindBusiness},
		RoleDef{ID: 2, Name: "producer", Kind: RoleKindBusiness, Permissions: []Permission{PermSpecUpload, PermAnalyticsView}},
		RoleDef{ID: 3, Name: "user", Kind: RoleKindSystem},
		RoleDef{ID: 4, Name: "super_admin", Kind: RoleKindSystem, Permissions: []Permission{PermAdminUsersRead, PermAdminRolesAssign}},
	)
}

func authorizeCall(t *testing.T, m *AuthMiddleWare, role, systemRole string, perms ...Permission) (*httptest.ResponseRecorder, bool) {
	t.Helper()
	token, err := utils.GenerateToken(uuid.New(), "u@example.com", role, systemRole, testSecret, time.Hour)
	require.NoError(t, err)
	called := false
	h := m.Authorize(perms...)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	h.ServeHTTP(rec, req)
	return rec, called
}

func TestAuthorize(t *testing.T) {
	m := NewAuthMiddleware(testSecret).WithRoleStore(testRoles())

	t.Run("requires authentication", func(t *testing.T) {
		rec := httptest.NewRecorder()
		m.Authorize(PermSpecUpload)(http.NotFoundHandler()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	cases := []struct {
		name       string
		role       string
		systemRole string
		perms      []Permission
		allowed    bool
	}{
		{"producer may upload", "producer", "user", []Permission{PermSpecUpload}, true},
		{"artist may not upload", "artist", "user", []Permission{PermSpecUpload}, false},
		{"super admin alone has no producer powers", "artist", "super_admin", []Permission{PermSpecUpload}, false},
		{"producer who is also super admin gets the union", "producer", "super_admin", []Permission{PermSpecUpload, PermAdminUsersRead}, true},
		{"admin permission via system role", "artist", "super_admin", []Permission{PermAdminUsersRead}, true},
		{"producer may not use admin permission", "producer", "user", []Permission{PermAdminUsersRead}, false},
		{"all listed permissions are required", "producer", "user", []Permission{PermSpecUpload, PermAdminUsersRead}, false},
		{"unknown role holds nothing", "ghost", "ghost", []Permission{PermSpecUpload}, false},
		{"permission nobody holds is denied", "producer", "super_admin", []Permission{PermOrdersProducer}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec, called := authorizeCall(t, m, tc.role, tc.systemRole, tc.perms...)
			assert.Equal(t, tc.allowed, called)
			if tc.allowed {
				assert.Equal(t, http.StatusOK, rec.Code)
			} else {
				assert.Equal(t, http.StatusForbidden, rec.Code)
			}
		})
	}

	t.Run("fails closed without a role store", func(t *testing.T) {
		rec, called := authorizeCall(t, NewAuthMiddleware(testSecret), "producer", "super_admin", PermSpecUpload)
		assert.False(t, called)
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})
}

func TestAuthorize_UsesLiveRolesNotTokenClaims(t *testing.T) {
	live := AccountState{Status: domain.UserStatusActive, Role: domain.UserRole("artist"), SystemRole: domain.SystemRoleUser}
	m := NewAuthMiddleware(testSecret).WithRoleStore(testRoles()).WithAccountLookup(
		func(context.Context, uuid.UUID) (AccountState, error) { return live, nil }, 0)

	// The token still claims producer + super_admin, as it would right after a
	// demotion. The database says artist + user.
	rec, called := authorizeCall(t, m, "producer", "super_admin", PermSpecUpload)
	assert.False(t, called)
	assert.Equal(t, http.StatusForbidden, rec.Code)

	rec, called = authorizeCall(t, m, "producer", "super_admin", PermAdminUsersRead)
	assert.False(t, called)
	assert.Equal(t, http.StatusForbidden, rec.Code)

	live.Role = domain.UserRole("producer")
	_, called = authorizeCall(t, m, "artist", "user", PermSpecUpload)
	assert.True(t, called, "a promotion in the database applies without a new token")
}

func TestRoleStore(t *testing.T) {
	ctx := context.Background()

	t.Run("reload picks up a granted permission and failure keeps old data", func(t *testing.T) {
		defs := []RoleDef{{ID: 2, Name: "producer", Kind: RoleKindBusiness}}
		var loadErr error
		s := NewRoleStore(func(context.Context) ([]RoleDef, error) { return defs, loadErr })
		require.NoError(t, s.Reload(ctx))
		assert.False(t, s.Can(PermSpecUpload, "producer"))

		// "migration" grants the permission
		defs = []RoleDef{{ID: 2, Name: "producer", Kind: RoleKindBusiness, Permissions: []Permission{PermSpecUpload}}}
		require.NoError(t, s.Reload(ctx))
		assert.True(t, s.Can(PermSpecUpload, "producer"))

		loadErr = errors.New("db down")
		assert.Error(t, s.Reload(ctx))
		assert.True(t, s.Can(PermSpecUpload, "producer"), "previous roles stay in effect")
	})

	t.Run("refuses an empty roles table", func(t *testing.T) {
		s := NewRoleStore(func(context.Context) ([]RoleDef, error) { return nil, nil })
		assert.Error(t, s.Reload(ctx))
		assert.False(t, s.Can(PermSpecUpload, "producer"))
	})

	t.Run("HasRole checks name and kind", func(t *testing.T) {
		s := testRoles()
		assert.True(t, s.HasRole("super_admin", RoleKindSystem))
		assert.False(t, s.HasRole("super_admin", RoleKindBusiness))
		assert.False(t, s.HasRole("nope", RoleKindSystem))
	})

	t.Run("List is a sorted copy", func(t *testing.T) {
		s := NewStaticRoleStore(RoleDef{ID: 1, Name: "r", Kind: RoleKindSystem, Permissions: []Permission{PermSpecUpload, PermAdminUsersRead}})
		got := s.List()
		require.Len(t, got, 1)
		assert.Equal(t, []Permission{PermAdminUsersRead, PermSpecUpload}, got[0].Permissions)
		got[0].Permissions[0] = "mutated"
		assert.Equal(t, []Permission{PermAdminUsersRead, PermSpecUpload}, s.List()[0].Permissions)
	})

	t.Run("List gives empty roles a non-nil permissions slice", func(t *testing.T) {
		got := testRoles().List()
		require.NotEmpty(t, got)
		assert.NotNil(t, got[0].Permissions)
		assert.Empty(t, got[0].Permissions)
		b, err := json.Marshal(got[0])
		require.NoError(t, err)
		assert.Contains(t, string(b), `"permissions":[]`)
	})

	t.Run("Start reloads periodically until cancelled", func(t *testing.T) {
		loads := make(chan struct{}, 8)
		s := NewRoleStore(func(context.Context) ([]RoleDef, error) {
			loads <- struct{}{}
			return []RoleDef{{ID: 1, Name: "r", Kind: RoleKindSystem}}, nil
		})
		cctx, cancel := context.WithCancel(ctx)
		s.Start(cctx, 5*time.Millisecond)
		for i := 0; i < 2; i++ {
			select {
			case <-loads:
			case <-time.After(2 * time.Second):
				t.Fatal("store did not reload")
			}
		}
		cancel()
	})
}
