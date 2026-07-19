package http

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	auth "github.com/saransh1220/blueprint-audio/internal/modules/auth/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type userRepoStub struct{}

func (userRepoStub) Create(context.Context, *auth.User) error                { return nil }
func (userRepoStub) GetByEmail(context.Context, string) (*auth.User, error)  { return nil, nil }
func (userRepoStub) GetByID(context.Context, uuid.UUID) (*auth.User, error)  { return nil, nil }
func (userRepoStub) MarkEmailVerified(context.Context, uuid.UUID) error      { return nil }
func (userRepoStub) UpdatePassword(context.Context, uuid.UUID, string) error { return nil }
func (userRepoStub) UpdateProfile(context.Context, uuid.UUID, *string, *string, *string, *string, *string, *string, *string, *string, *string) error {
	return nil
}
func (userRepoStub) UpdateSystemRole(context.Context, uuid.UUID, auth.SystemRole) error { return nil }
func (userRepoStub) UpdateStatus(context.Context, uuid.UUID, auth.UserStatus) error     { return nil }
func (userRepoStub) CountBySystemRole(context.Context, auth.SystemRole) (int, error)    { return 2, nil }
func (userRepoStub) BootstrapSuperAdmin(context.Context, string) error                  { return nil }

func newHandler(t *testing.T) (*AdminHandler, sqlmock.Sqlmock, func()) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	return NewAdminHandler(sqlx.NewDb(db, "sqlmock"), userRepoStub{}), mock, func() { db.Close() }
}

func TestAdminHandlerInvalidIDs(t *testing.T) {
	h, _, closeDB := newHandler(t)
	defer closeDB()
	for name, fn := range map[string]func(http.ResponseWriter, *http.Request){"get": h.GetUser, "role": h.UpdateUserSystemRole, "status": h.UpdateUserStatus, "update_spec": h.UpdateSpec, "delete_spec": h.DeleteSpec} {
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPatch, "/", nil)
			r.SetPathValue("id", "bad")
			w := httptest.NewRecorder()
			fn(w, r)
			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

func TestListUsersAndPagination(t *testing.T) {
	h, mock, closeDB := newHandler(t)
	defer closeDB()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	id := uuid.New()
	mock.ExpectQuery(`SELECT id, email, name, display_name, role, system_role, status, email_verified, created_at, updated_at FROM users ORDER BY created_at DESC LIMIT \$1 OFFSET \$2`).WithArgs(100, 100).WillReturnRows(sqlmock.NewRows([]string{"id", "email", "name", "display_name", "role", "system_role", "status", "email_verified", "created_at", "updated_at"}).AddRow(id, "a@b.com", "A", nil, "artist", "user", "active", true, now, now))
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM users").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	w := httptest.NewRecorder()
	h.ListUsers(w, httptest.NewRequest(http.MethodGet, "/?page=2&limit=999", nil))
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "a@b.com")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestListEndpointsAndOverview(t *testing.T) {
	h, mock, closeDB := newHandler(t)
	defer closeDB()
	for _, fn := range []func(http.ResponseWriter, *http.Request){h.ListSpecs, h.ListOrders, h.ListLicenses} {
		mock.ExpectQuery("SELECT ").WillReturnRows(sqlmock.NewRows([]string{"id"}))
		mock.ExpectQuery("SELECT COUNT\\(\\*\\)").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
		w := httptest.NewRecorder()
		fn(w, httptest.NewRequest(http.MethodGet, "/", nil))
		assert.Equal(t, http.StatusOK, w.Code)
	}
	for i := 0; i < 5; i++ {
		mock.ExpectQuery("SELECT COUNT\\(\\*\\)").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(i))
	}
	mock.ExpectQuery("SELECT COALESCE").WillReturnRows(sqlmock.NewRows([]string{"coalesce"}).AddRow(12))
	w := httptest.NewRecorder()
	h.AnalyticsOverview(w, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "revenue")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAdminHelpers(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/?page=3&limit=200", nil)
	limit, offset := pagination(req, 50, 100)
	assert.Equal(t, 100, limit)
	assert.Equal(t, 200, offset)
	assert.Equal(t, "1.2.3.4", clientIP(func() *http.Request {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("X-Forwarded-For", "1.2.3.4, 5.6.7.8")
		return r
	}()))
	row := map[string]any{"value": []byte("text")}
	normalizeMap(row)
	assert.Equal(t, "text", row["value"])
	page := pageResponse([]string{}, 2, 10, 10)
	assert.Equal(t, 2, page["metadata"].(map[string]any)["page"])
}

func TestAdminHandlerValidationFailures(t *testing.T) {
	h, _, closeDB := newHandler(t)
	defer closeDB()
	id := uuid.New().String()
	for name, fn := range map[string]func(http.ResponseWriter, *http.Request){"role": h.UpdateUserSystemRole, "status": h.UpdateUserStatus, "spec": h.UpdateSpec} {
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPatch, "/", nil)
			r.SetPathValue("id", id)
			w := httptest.NewRecorder()
			fn(w, r)
			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
	for _, body := range []string{`{"system_role":"bad"}`, `{"status":"bad"}`} {
		r := httptest.NewRequest(http.MethodPatch, "/", strings.NewReader(body))
		r.SetPathValue("id", id)
		w := httptest.NewRecorder()
		if strings.Contains(body, "system_role") {
			h.UpdateUserSystemRole(w, r)
		} else {
			h.UpdateUserStatus(w, r)
		}
		assert.Equal(t, http.StatusBadRequest, w.Code)
	}
}

func TestListAuditLog(t *testing.T) {
	h, mock, closeDB := newHandler(t)
	defer closeDB()
	mock.ExpectQuery("SELECT a.id").WillReturnRows(sqlmock.NewRows([]string{"id", "actor_id", "actor_email", "action", "resource_type", "resource_id", "before_state", "after_state", "ip_address", "user_agent", "created_at"}))
	mock.ExpectQuery("SELECT COUNT\\(\\*\\)").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	w := httptest.NewRecorder()
	h.ListAuditLog(w, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, http.StatusOK, w.Code)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateAndDeleteSpec(t *testing.T) {
	h, mock, closeDB := newHandler(t)
	defer closeDB()
	id := uuid.New()
	state := func() *sqlmock.Rows {
		return sqlmock.NewRows([]string{"id", "producer_id", "title", "category", "base_price", "processing_status", "is_deleted", "deleted_at", "updated_at"}).AddRow(id, uuid.New(), "old", "beat", 10, "ready", false, nil, time.Now())
	}
	mock.ExpectQuery("SELECT id, producer_id").WillReturnRows(state())
	mock.ExpectExec("UPDATE specs").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT id, producer_id").WillReturnRows(state())
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPatch, "/", strings.NewReader(`{"title":"new"}`))
	r.SetPathValue("id", id.String())
	h.UpdateSpec(w, r)
	assert.Equal(t, http.StatusOK, w.Code)
	mock.ExpectQuery("SELECT id, producer_id").WillReturnRows(state())
	mock.ExpectExec("UPDATE specs SET is_deleted").WillReturnResult(sqlmock.NewResult(0, 1))
	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodDelete, "/", nil)
	r.SetPathValue("id", id.String())
	h.DeleteSpec(w, r)
	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAdminQueryFailures(t *testing.T) {
	h, mock, closeDB := newHandler(t)
	defer closeDB()
	mock.ExpectQuery("SELECT id, email").WillReturnError(errors.New("db"))
	w := httptest.NewRecorder()
	h.ListUsers(w, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	for _, fn := range []func(http.ResponseWriter, *http.Request){h.ListSpecs, h.ListOrders, h.ListLicenses, h.ListAuditLog} {
		mock.ExpectQuery("SELECT ").WillReturnError(errors.New("db"))
		w = httptest.NewRecorder()
		fn(w, httptest.NewRequest(http.MethodGet, "/", nil))
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	}
	id := uuid.New()
	mock.ExpectQuery("SELECT id, email").WithArgs(id).WillReturnError(sql.ErrNoRows)
	w = httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.SetPathValue("id", id.String())
	h.GetUser(w, r)
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.NoError(t, mock.ExpectationsWereMet())
}
