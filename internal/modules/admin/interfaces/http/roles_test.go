package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/gateway/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testRoleStore() *middleware.RoleStore {
	return middleware.NewStaticRoleStore(
		middleware.RoleDef{ID: 1, Name: "artist", Kind: middleware.RoleKindBusiness},
		middleware.RoleDef{ID: 2, Name: "producer", Kind: middleware.RoleKindBusiness, Permissions: []middleware.Permission{middleware.PermSpecUpload}},
		middleware.RoleDef{ID: 3, Name: "user", Kind: middleware.RoleKindSystem},
		middleware.RoleDef{ID: 4, Name: "support", Kind: middleware.RoleKindSystem, Permissions: []middleware.Permission{middleware.PermAdminUsersRead}},
		middleware.RoleDef{ID: 5, Name: "super_admin", Kind: middleware.RoleKindSystem, Permissions: []middleware.Permission{middleware.PermAdminUsersRead, middleware.PermAdminRolesAssign}},
	)
}

func TestListRoles(t *testing.T) {
	h, _, closeDB := newHandler(t)
	defer closeDB()

	w := httptest.NewRecorder()
	h.ListRoles(w, httptest.NewRequest(http.MethodGet, "/admin/roles", nil))
	require.Equal(t, http.StatusOK, w.Code)

	var body struct {
		Roles []middleware.RoleDef `json:"roles"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Len(t, body.Roles, 5)
	assert.Equal(t, "super_admin", body.Roles[4].Name)

	t.Run("unavailable without a store", func(t *testing.T) {
		bare := NewAdminHandler(nil, nil)
		w := httptest.NewRecorder()
		bare.ListRoles(w, httptest.NewRequest(http.MethodGet, "/admin/roles", nil))
		assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	})
}

// A role added by migration (here "support") is assignable with no code change,
// but a business role or an unknown role is not a valid system role.
func TestUpdateUserSystemRole_AcceptsAnySystemRoleFromTable(t *testing.T) {
	repo := &configurableRepo{superAdmins: 2}
	h, mock := newRepoHandler(t, repo)
	id := uuid.New()
	actor := uuid.New()

	mock.ExpectQuery("FROM users WHERE id").WillReturnRows(userRow(id, "user", "active"))
	mock.ExpectQuery("FROM users WHERE id").WillReturnRows(userRow(id, "support", "active"))
	mock.ExpectExec("INSERT INTO admin_audit_logs").WillReturnResult(sqlmock.NewResult(0, 1))
	w := httptest.NewRecorder()
	h.UpdateUserSystemRole(w, patchReq(id, `{"system_role":"support"}`, &actor))
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "support", string(repo.gotRole))

	for _, bad := range []string{"producer", "nonexistent", ""} {
		w := httptest.NewRecorder()
		h.UpdateUserSystemRole(w, patchReq(id, `{"system_role":"`+bad+`"}`, &actor))
		assert.Equal(t, http.StatusBadRequest, w.Code, bad)
	}
}
