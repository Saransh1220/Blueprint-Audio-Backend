package http

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/saransh1220/blueprint-audio/internal/gateway/middleware"
	auth "github.com/saransh1220/blueprint-audio/internal/modules/auth/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// configurableRepo overrides the pieces of userRepoStub that the role/status
// handlers depend on.
type configurableRepo struct {
	userRepoStub
	superAdmins   int
	countErr      error
	updateRoleErr error
	updateStatErr error
	gotRole       auth.SystemRole
	gotStatus     auth.UserStatus
	roleCalled    bool
	statusCalled  bool
}

func (c *configurableRepo) CountBySystemRole(context.Context, auth.SystemRole) (int, error) {
	return c.superAdmins, c.countErr
}

func (c *configurableRepo) UpdateSystemRole(_ context.Context, _ uuid.UUID, role auth.SystemRole) error {
	c.roleCalled, c.gotRole = true, role
	return c.updateRoleErr
}

func (c *configurableRepo) UpdateStatus(_ context.Context, _ uuid.UUID, status auth.UserStatus) error {
	c.statusCalled, c.gotStatus = true, status
	return c.updateStatErr
}

var adminUserCols = []string{"id", "email", "name", "display_name", "role", "system_role", "status", "email_verified", "created_at", "updated_at"}

func userRow(id uuid.UUID, systemRole, status string) *sqlmock.Rows {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	return sqlmock.NewRows(adminUserCols).AddRow(id, "u@x.com", "U", nil, "artist", systemRole, status, true, now, now)
}

func newRepoHandler(t *testing.T, repo *configurableRepo) (*AdminHandler, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return NewAdminHandler(sqlx.NewDb(db, "sqlmock"), repo).WithRoleStore(testRoleStore()), mock
}

func patchReq(id uuid.UUID, body string, actor *uuid.UUID) *http.Request {
	r := httptest.NewRequest(http.MethodPatch, "/", strings.NewReader(body))
	r.SetPathValue("id", id.String())
	if actor != nil {
		r = r.WithContext(context.WithValue(r.Context(), middleware.ContextKeyUserId, *actor))
	}
	return r
}

func TestListUsers_SearchAndCountFailure(t *testing.T) {
	t.Run("search adds ILIKE filter to both queries", func(t *testing.T) {
		h, mock, closeDB := newHandler(t)
		defer closeDB()
		mock.ExpectQuery(`FROM users WHERE email ILIKE \$1 .* LIMIT \$2 OFFSET \$3`).
			WithArgs("%bob%", 50, 0).WillReturnRows(userRow(uuid.New(), "user", "active"))
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM users WHERE email ILIKE \$1`).
			WithArgs("%bob%").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
		w := httptest.NewRecorder()
		h.ListUsers(w, httptest.NewRequest(http.MethodGet, "/?q=%20bob%20", nil))
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `"total":1`)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("count failure", func(t *testing.T) {
		h, mock, closeDB := newHandler(t)
		defer closeDB()
		mock.ExpectQuery("SELECT id, email").WillReturnRows(userRow(uuid.New(), "user", "active"))
		mock.ExpectQuery("SELECT COUNT").WillReturnError(errors.New("db"))
		w := httptest.NewRecorder()
		h.ListUsers(w, httptest.NewRequest(http.MethodGet, "/", nil))
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Contains(t, w.Body.String(), "failed to count users")
	})
}

func TestGetUser_SuccessAndError(t *testing.T) {
	h, mock, closeDB := newHandler(t)
	defer closeDB()
	id := uuid.New()
	mock.ExpectQuery("FROM users WHERE id").WithArgs(id).WillReturnRows(userRow(id, "user", "active"))
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.SetPathValue("id", id.String())
	w := httptest.NewRecorder()
	h.GetUser(w, r)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"email":"u@x.com"`)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	mock.ExpectQuery("FROM users WHERE id").WillReturnError(errors.New("db"))
	w = httptest.NewRecorder()
	h.GetUser(w, r)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "failed to fetch user")
}

func TestUpdateUserSystemRole_Flows(t *testing.T) {
	id, actor := uuid.New(), uuid.New()

	t.Run("promote succeeds and writes audit log", func(t *testing.T) {
		repo := &configurableRepo{}
		h, mock := newRepoHandler(t, repo)
		mock.ExpectQuery("FROM users WHERE id").WillReturnRows(userRow(id, "user", "active"))
		mock.ExpectQuery("FROM users WHERE id").WillReturnRows(userRow(id, "super_admin", "active"))
		mock.ExpectExec("INSERT INTO admin_audit_logs").
			WithArgs(actor, "users.set_system_role", "user", &id, sqlmock.AnyArg(), sqlmock.AnyArg(), "9.9.9.9", "ua/1").
			WillReturnResult(sqlmock.NewResult(0, 1))
		r := patchReq(id, `{"system_role":"super_admin"}`, &actor)
		r.Header.Set("X-Forwarded-For", "9.9.9.9, 1.1.1.1")
		r.Header.Set("User-Agent", "ua/1")
		w := httptest.NewRecorder()
		h.UpdateUserSystemRole(w, r)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `"system_role":"super_admin"`)
		assert.True(t, repo.roleCalled)
		assert.Equal(t, auth.SystemRoleSuperAdmin, repo.gotRole)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("invalid json", func(t *testing.T) {
		h, _ := newRepoHandler(t, &configurableRepo{})
		w := httptest.NewRecorder()
		h.UpdateUserSystemRole(w, patchReq(id, `{bad`, nil))
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "invalid request body")
	})

	t.Run("user not found", func(t *testing.T) {
		repo := &configurableRepo{}
		h, mock := newRepoHandler(t, repo)
		mock.ExpectQuery("FROM users WHERE id").WillReturnError(errors.New("no rows"))
		w := httptest.NewRecorder()
		h.UpdateUserSystemRole(w, patchReq(id, `{"system_role":"user"}`, nil))
		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.False(t, repo.roleCalled)
	})

	t.Run("cannot demote last super admin", func(t *testing.T) {
		repo := &configurableRepo{superAdmins: 1}
		h, mock := newRepoHandler(t, repo)
		mock.ExpectQuery("FROM users WHERE id").WillReturnRows(userRow(id, "super_admin", "active"))
		w := httptest.NewRecorder()
		h.UpdateUserSystemRole(w, patchReq(id, `{"system_role":"user"}`, nil))
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "cannot remove the last super admin")
		assert.False(t, repo.roleCalled)
	})

	t.Run("count failure blocks demotion", func(t *testing.T) {
		repo := &configurableRepo{superAdmins: 5, countErr: errors.New("db")}
		h, mock := newRepoHandler(t, repo)
		mock.ExpectQuery("FROM users WHERE id").WillReturnRows(userRow(id, "super_admin", "active"))
		w := httptest.NewRecorder()
		h.UpdateUserSystemRole(w, patchReq(id, `{"system_role":"user"}`, nil))
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.False(t, repo.roleCalled)
	})

	t.Run("demotion allowed when another super admin exists", func(t *testing.T) {
		repo := &configurableRepo{superAdmins: 2}
		h, mock := newRepoHandler(t, repo)
		mock.ExpectQuery("FROM users WHERE id").WillReturnRows(userRow(id, "super_admin", "active"))
		mock.ExpectQuery("FROM users WHERE id").WillReturnRows(userRow(id, "user", "active"))
		w := httptest.NewRecorder()
		h.UpdateUserSystemRole(w, patchReq(id, `{"system_role":"user"}`, nil))
		assert.Equal(t, http.StatusOK, w.Code)
		assert.True(t, repo.roleCalled)
		assert.Equal(t, auth.SystemRoleUser, repo.gotRole)
	})

	t.Run("repository failure", func(t *testing.T) {
		repo := &configurableRepo{updateRoleErr: errors.New("db")}
		h, mock := newRepoHandler(t, repo)
		mock.ExpectQuery("FROM users WHERE id").WillReturnRows(userRow(id, "user", "active"))
		w := httptest.NewRecorder()
		h.UpdateUserSystemRole(w, patchReq(id, `{"system_role":"super_admin"}`, nil))
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Contains(t, w.Body.String(), "failed to update system role")
	})
}

func TestUpdateUserStatus_Flows(t *testing.T) {
	id, actor := uuid.New(), uuid.New()

	t.Run("suspend succeeds and writes audit log", func(t *testing.T) {
		repo := &configurableRepo{}
		h, mock := newRepoHandler(t, repo)
		mock.ExpectQuery("FROM users WHERE id").WillReturnRows(userRow(id, "user", "active"))
		mock.ExpectQuery("FROM users WHERE id").WillReturnRows(userRow(id, "user", "suspended"))
		mock.ExpectExec("INSERT INTO admin_audit_logs").
			WithArgs(actor, "users.set_status", "user", &id, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
			WillReturnResult(sqlmock.NewResult(0, 1))
		w := httptest.NewRecorder()
		h.UpdateUserStatus(w, patchReq(id, `{"status":"suspended"}`, &actor))
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `"status":"suspended"`)
		assert.Equal(t, auth.UserStatusSuspended, repo.gotStatus)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("invalid json", func(t *testing.T) {
		h, _ := newRepoHandler(t, &configurableRepo{})
		w := httptest.NewRecorder()
		h.UpdateUserStatus(w, patchReq(id, `nope`, nil))
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "invalid request body")
	})

	t.Run("user not found", func(t *testing.T) {
		h, mock := newRepoHandler(t, &configurableRepo{})
		mock.ExpectQuery("FROM users WHERE id").WillReturnError(errors.New("no rows"))
		w := httptest.NewRecorder()
		h.UpdateUserStatus(w, patchReq(id, `{"status":"active"}`, nil))
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("cannot suspend last super admin", func(t *testing.T) {
		repo := &configurableRepo{superAdmins: 1}
		h, mock := newRepoHandler(t, repo)
		mock.ExpectQuery("FROM users WHERE id").WillReturnRows(userRow(id, "super_admin", "active"))
		w := httptest.NewRecorder()
		h.UpdateUserStatus(w, patchReq(id, `{"status":"suspended"}`, nil))
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "cannot suspend the last super admin")
		assert.False(t, repo.statusCalled)
	})

	t.Run("repository failure", func(t *testing.T) {
		repo := &configurableRepo{updateStatErr: errors.New("db")}
		h, mock := newRepoHandler(t, repo)
		mock.ExpectQuery("FROM users WHERE id").WillReturnRows(userRow(id, "user", "active"))
		w := httptest.NewRecorder()
		h.UpdateUserStatus(w, patchReq(id, `{"status":"suspended"}`, nil))
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Contains(t, w.Body.String(), "failed to update status")
	})
}

func TestListSpecs_ReturnsRowsWithNormalizedValues(t *testing.T) {
	h, mock, closeDB := newHandler(t)
	defer closeDB()
	mock.ExpectQuery("FROM specs s JOIN users u").WithArgs(50, 0).
		WillReturnRows(sqlmock.NewRows([]string{"id", "title"}).AddRow([]byte("abc"), "Beat"))
	mock.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	w := httptest.NewRecorder()
	h.ListSpecs(w, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"id":"abc"`)
	assert.Contains(t, w.Body.String(), `"title":"Beat"`)
	assert.Contains(t, w.Body.String(), `"total":1`)
}

func specStateRows(id uuid.UUID, title string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "title", "is_deleted"}).AddRow(id, title, false)
}

func TestUpdateSpec_Branches(t *testing.T) {
	id, actor := uuid.New(), uuid.New()

	t.Run("spec not found before update", func(t *testing.T) {
		h, mock, closeDB := newHandler(t)
		defer closeDB()
		mock.ExpectQuery("SELECT id, producer_id").WillReturnRows(sqlmock.NewRows([]string{"id"}))
		w := httptest.NewRecorder()
		h.UpdateSpec(w, patchReq(id, `{"title":"x"}`, nil))
		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Contains(t, w.Body.String(), "spec not found")
	})

	t.Run("all fields are passed and audited", func(t *testing.T) {
		h, mock, closeDB := newHandler(t)
		defer closeDB()
		mock.ExpectQuery("SELECT id, producer_id").WillReturnRows(specStateRows(id, "old"))
		mock.ExpectExec("UPDATE specs").WithArgs("new", 12.5, true, id).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectQuery("SELECT id, producer_id").WillReturnRows(specStateRows(id, "new"))
		mock.ExpectExec("INSERT INTO admin_audit_logs").
			WithArgs(actor, "specs.update", "spec", &id, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
			WillReturnResult(sqlmock.NewResult(0, 1))
		w := httptest.NewRecorder()
		h.UpdateSpec(w, patchReq(id, `{"title":"new","base_price":12.5,"is_deleted":true}`, &actor))
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `"title":"new"`)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("exec failure", func(t *testing.T) {
		h, mock, closeDB := newHandler(t)
		defer closeDB()
		mock.ExpectQuery("SELECT id, producer_id").WillReturnRows(specStateRows(id, "old"))
		mock.ExpectExec("UPDATE specs").WillReturnError(errors.New("db"))
		w := httptest.NewRecorder()
		h.UpdateSpec(w, patchReq(id, `{}`, nil))
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Contains(t, w.Body.String(), "failed to update spec")
	})

	t.Run("rows affected error", func(t *testing.T) {
		h, mock, closeDB := newHandler(t)
		defer closeDB()
		mock.ExpectQuery("SELECT id, producer_id").WillReturnRows(specStateRows(id, "old"))
		mock.ExpectExec("UPDATE specs").WillReturnResult(sqlmock.NewErrorResult(errors.New("driver")))
		w := httptest.NewRecorder()
		h.UpdateSpec(w, patchReq(id, `{}`, nil))
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Contains(t, w.Body.String(), "failed to confirm spec update")
	})

	t.Run("no rows affected", func(t *testing.T) {
		h, mock, closeDB := newHandler(t)
		defer closeDB()
		mock.ExpectQuery("SELECT id, producer_id").WillReturnRows(specStateRows(id, "old"))
		mock.ExpectExec("UPDATE specs").WillReturnResult(sqlmock.NewResult(0, 0))
		w := httptest.NewRecorder()
		h.UpdateSpec(w, patchReq(id, `{}`, nil))
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestDeleteSpec_Branches(t *testing.T) {
	id, actor := uuid.New(), uuid.New()
	delReq := func(a *uuid.UUID) *http.Request {
		r := patchReq(id, "", a)
		r.Method = http.MethodDelete
		return r
	}

	t.Run("exec failure", func(t *testing.T) {
		h, mock, closeDB := newHandler(t)
		defer closeDB()
		mock.ExpectQuery("SELECT id, producer_id").WillReturnRows(specStateRows(id, "t"))
		mock.ExpectExec("UPDATE specs SET is_deleted").WillReturnError(errors.New("db"))
		w := httptest.NewRecorder()
		h.DeleteSpec(w, delReq(nil))
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Contains(t, w.Body.String(), "failed to delete spec")
	})

	t.Run("not found", func(t *testing.T) {
		h, mock, closeDB := newHandler(t)
		defer closeDB()
		mock.ExpectQuery("SELECT id, producer_id").WillReturnRows(sqlmock.NewRows([]string{"id"}))
		mock.ExpectExec("UPDATE specs SET is_deleted").WillReturnResult(sqlmock.NewResult(0, 0))
		w := httptest.NewRecorder()
		h.DeleteSpec(w, delReq(nil))
		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Contains(t, w.Body.String(), "spec not found")
	})

	t.Run("success records audit entry", func(t *testing.T) {
		h, mock, closeDB := newHandler(t)
		defer closeDB()
		mock.ExpectQuery("SELECT id, producer_id").WillReturnRows(specStateRows(id, "t"))
		mock.ExpectExec("UPDATE specs SET is_deleted").WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectQuery("SELECT id, producer_id").WillReturnRows(specStateRows(id, "t"))
		mock.ExpectExec("INSERT INTO admin_audit_logs").
			WithArgs(actor, "specs.delete", "spec", &id, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
			WillReturnResult(sqlmock.NewResult(0, 1))
		w := httptest.NewRecorder()
		h.DeleteSpec(w, delReq(&actor))
		assert.Equal(t, http.StatusNoContent, w.Code)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestSelectMapsRowError(t *testing.T) {
	h, mock, closeDB := newHandler(t)
	defer closeDB()
	mock.ExpectQuery("SELECT ").WillReturnRows(
		sqlmock.NewRows([]string{"id"}).AddRow(1).AddRow(2).RowError(1, errors.New("broken row")))
	w := httptest.NewRecorder()
	h.ListOrders(w, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "failed to list orders")
}

func TestClientIPFallsBackToRemoteAddr(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "10.0.0.5:1234"
	assert.Equal(t, "10.0.0.5:1234", clientIP(r))
	r.Header.Set("X-Forwarded-For", "  ")
	assert.Equal(t, "10.0.0.5:1234", clientIP(r))
}
