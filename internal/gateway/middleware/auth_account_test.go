package middleware

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/modules/auth/domain"
	"github.com/saransh1220/blueprint-audio/internal/shared/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func tokenFor(t *testing.T, id uuid.UUID, systemRole string) string {
	t.Helper()
	tok, err := utils.GenerateToken(id, "a@b.c", "producer", systemRole, testSecret, time.Hour)
	require.NoError(t, err)
	return tok
}

func call(h http.Handler, target, bearer string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestRequireAuth_QueryToken(t *testing.T) {
	m := NewAuthMiddleware(testSecret)
	tok := tokenFor(t, uuid.New(), "user")
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })

	assert.Equal(t, http.StatusUnauthorized, call(m.RequireAuth(ok), "/x?token="+tok, "").Code, "RequireAuth must ignore ?token=")
	assert.Equal(t, http.StatusOK, call(m.RequireAuthAllowQueryToken(ok), "/ws?token="+tok, "").Code, "websocket variant accepts ?token=")
	assert.Equal(t, http.StatusOK, call(m.RequireAuth(ok), "/x", tok).Code)
}

func TestValidateToken_RejectsOtherAlgorithms(t *testing.T) {
	claims := &utils.Claims{UserID: uuid.New(), RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}}

	hs512, err := jwt.NewWithClaims(jwt.SigningMethodHS512, claims).SignedString([]byte(testSecret))
	require.NoError(t, err)
	_, err = utils.ValidateToken(hs512, testSecret)
	assert.ErrorIs(t, err, utils.ErrInvalidToken)

	none, err := jwt.NewWithClaims(jwt.SigningMethodNone, claims).SignedString(jwt.UnsafeAllowNoneSignatureType)
	require.NoError(t, err)
	_, err = utils.ValidateToken(none, testSecret)
	assert.ErrorIs(t, err, utils.ErrInvalidToken)
}

func TestAccountLookup(t *testing.T) {
	id := uuid.New()
	state := AccountState{Status: domain.UserStatusActive, SystemRole: domain.SystemRoleUser}
	var lookupErr error
	calls := 0
	m := NewAuthMiddleware(testSecret).WithAccountLookup(func(ctx context.Context, _ uuid.UUID) (AccountState, error) {
		calls++
		return state, lookupErr
	}, time.Minute)

	var gotRole any
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRole = r.Context().Value(ContextKeySystemRole)
	})
	// Token still claims super_admin, as it would after a demotion.
	tok := tokenFor(t, id, "super_admin")

	t.Run("system role comes from the database, not the token", func(t *testing.T) {
		rec := call(m.RequireAuth(next), "/x", tok)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "user", gotRole)
		// And RequirePermission therefore denies the demoted admin.
		rec = call(m.RequirePermission(PermissionSuperAdmin, next), "/admin", tok)
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("lookups are cached", func(t *testing.T) {
		before := calls
		call(m.RequireAuth(next), "/x", tok)
		call(m.RequireAuth(next), "/x", tok)
		assert.Equal(t, before, calls)
	})

	t.Run("suspended account is rejected", func(t *testing.T) {
		m2 := NewAuthMiddleware(testSecret).WithAccountLookup(func(context.Context, uuid.UUID) (AccountState, error) {
			return AccountState{Status: domain.UserStatusSuspended, SystemRole: domain.SystemRoleUser}, nil
		}, 0)
		rec := call(m2.RequireAuth(next), "/x", tok)
		assert.Equal(t, http.StatusForbidden, rec.Code)
		assert.Contains(t, rec.Body.String(), "account suspended")
	})

	t.Run("suspended account is a guest on flexible routes", func(t *testing.T) {
		m2 := NewAuthMiddleware(testSecret).WithAccountLookup(func(context.Context, uuid.UUID) (AccountState, error) {
			return AccountState{Status: domain.UserStatusSuspended}, nil
		}, 0)
		var sawID any
		rec := call(m2.FlexibleAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sawID = r.Context().Value(ContextKeyUserId)
		})), "/x", tok)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Nil(t, sawID)
	})

	t.Run("deleted user is unauthorized, db outage fails closed", func(t *testing.T) {
		m2 := NewAuthMiddleware(testSecret).WithAccountLookup(func(context.Context, uuid.UUID) (AccountState, error) {
			return AccountState{}, sql.ErrNoRows
		}, 0)
		assert.Equal(t, http.StatusUnauthorized, call(m2.RequireAuth(next), "/x", tok).Code)

		m3 := NewAuthMiddleware(testSecret).WithAccountLookup(func(context.Context, uuid.UUID) (AccountState, error) {
			return AccountState{}, errors.New("db down")
		}, 0)
		assert.Equal(t, http.StatusServiceUnavailable, call(m3.RequireAuth(next), "/x", tok).Code)
	})
}
