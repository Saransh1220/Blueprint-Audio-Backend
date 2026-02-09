package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/middleware"
	"github.com/saransh1220/blueprint-audio/internal/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequireAuth(t *testing.T) {
	newReq := func(auth string) *http.Request {
		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		return req
	}

	hitNext := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitNext = true
		_, okUser := r.Context().Value(middleware.ContextKeyUserId).(uuid.UUID)
		role, okRole := r.Context().Value(middleware.ContextKeyRole).(string)
		if !okUser || !okRole || role == "" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	mw := middleware.NewAuthMiddleware("secret")

	rr := httptest.NewRecorder()
	mw.RequireAuth(next).ServeHTTP(rr, newReq(""))
	assert.Equal(t, http.StatusUnauthorized, rr.Code)
	assert.False(t, hitNext)

	rr = httptest.NewRecorder()
	mw.RequireAuth(next).ServeHTTP(rr, newReq("BearerOnly"))
	assert.Equal(t, http.StatusUnauthorized, rr.Code)
	assert.False(t, hitNext)

	rr = httptest.NewRecorder()
	mw.RequireAuth(next).ServeHTTP(rr, newReq("Bearer bad-token"))
	assert.Equal(t, http.StatusUnauthorized, rr.Code)
	assert.False(t, hitNext)

	token, err := utils.GenerateToken("secret", time.Hour, uuid.New(), "artist")
	require.NoError(t, err)

	rr = httptest.NewRecorder()
	mw.RequireAuth(next).ServeHTTP(rr, newReq("Bearer "+token))
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.True(t, hitNext)
}

func TestFlexibleAuth(t *testing.T) {
	mw := middleware.NewAuthMiddleware("secret")
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/public", nil)
	rr := httptest.NewRecorder()
	mw.FlexibleAuth(next).ServeHTTP(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)

	req = httptest.NewRequest(http.MethodGet, "/public", nil)
	req.Header.Set("Authorization", "Bearer invalid")
	rr = httptest.NewRecorder()
	mw.FlexibleAuth(next).ServeHTTP(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)

	token, err := utils.GenerateToken("secret", time.Hour, uuid.New(), "producer")
	require.NoError(t, err)
	req = httptest.NewRequest(http.MethodGet, "/public", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr = httptest.NewRecorder()
	mw.FlexibleAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, ok := r.Context().Value(middleware.ContextKeyUserId).(uuid.UUID)
		assert.True(t, ok)
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)
}
