package http_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/gateway/middleware"
	"github.com/saransh1220/blueprint-audio/internal/modules/auth/application"
	"github.com/saransh1220/blueprint-audio/internal/modules/auth/domain"
	auth_http "github.com/saransh1220/blueprint-audio/internal/modules/auth/interfaces/http"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestAuthHandler_LoginAndMeBranches(t *testing.T) {
	mockService := new(MockAuthService)
	mockFileService := new(MockFileService)
	const refreshTTL = 12 * time.Hour
	h := auth_http.NewAuthHandler(mockService, mockFileService, "test-client-id", refreshTTL)
	t.Cleanup(func() {
		mockService.AssertExpectations(t)
		mockFileService.AssertExpectations(t)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewBufferString("bad"))
	h.Login(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/login", bytes.NewBufferString(`{"email":"x","password":"y"}`))
	mockService.On("Login", mock.Anything, application.LoginRequest{Email: "x", Password: "y"}).Return((*application.TokenPair)(nil), domain.ErrInvalidCredentials).Once()
	h.Login(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/login", bytes.NewBufferString(`{"email":"x","password":"y"}`))
	mockService.On("Login", mock.Anything, application.LoginRequest{Email: "x", Password: "y"}).Return((*application.TokenPair)(nil), errors.New("db")).Once()
	h.Login(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/login", bytes.NewBufferString(`{"email":"x","password":"y"}`))
	mockService.On("Login", mock.Anything, application.LoginRequest{Email: "x", Password: "y"}).Return(&application.TokenPair{AccessToken: "token", RefreshToken: "refresh-1"}, nil).Once()
	h.Login(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	cookies := w.Result().Cookies()
	if assert.Len(t, cookies, 1) {
		assert.Equal(t, "refresh_token", cookies[0].Name)
		assert.Equal(t, "refresh-1", cookies[0].Value)
		assert.True(t, cookies[0].HttpOnly)
		assert.True(t, cookies[0].Secure)
		assert.WithinDuration(t, time.Now().Add(refreshTTL), cookies[0].Expires, time.Minute)
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/me", nil)
	h.Me(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	userID := uuid.New()
	ctx := context.WithValue(context.Background(), middleware.ContextKeyUserId, userID)
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/me", nil).WithContext(ctx)
	mockService.On("GetUser", mock.Anything, userID).Return(nil, domain.ErrUserNotFound).Once()
	h.Me(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/me", nil).WithContext(ctx)
	mockService.On("GetUser", mock.Anything, userID).Return(nil, errors.New("db")).Once()
	h.Me(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)

	avatar := "http://storage/u/a.jpg"
	user := &domain.User{ID: userID, Email: "a@a.com", AvatarUrl: &avatar}
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/me", nil).WithContext(ctx)
	mockService.On("GetUser", mock.Anything, userID).Return(user, nil).Once()
	mockFileService.On("GetKeyFromUrl", avatar).Return("u/a.jpg", nil).Once()
	mockFileService.On("GetPresignedURL", mock.Anything, "u/a.jpg", mock.Anything).Return("signed", nil).Once()
	h.Me(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAuthHandler_RegisterMethodAndDecode(t *testing.T) {
	mockService := new(MockAuthService)
	mockFileService := new(MockFileService)
	h := auth_http.NewAuthHandler(mockService, mockFileService, "test-client-id", time.Hour*720)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/register", nil)
	h.Register(w, req)
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/register", bytes.NewBufferString("bad"))
	h.Register(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAuthHandler_GoogleLoginBranches(t *testing.T) {
	mockService := new(MockAuthService)
	mockFileService := new(MockFileService)
	const refreshTTL = 12 * time.Hour
	h := auth_http.NewAuthHandler(mockService, mockFileService, "test-client-id", refreshTTL)
	t.Cleanup(func() {
		mockService.AssertExpectations(t)
		mockFileService.AssertExpectations(t)
	})

	// bad json
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/auth/google", bytes.NewBufferString("bad"))
	h.GoogleLogin(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// auth failure — wrapped ErrGoogleAuthFailed so handler returns 401
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/auth/google", bytes.NewBufferString(`{"token":"x"}`))
	mockService.On("GoogleLogin", mock.Anything, "test-client-id", application.GoogleLoginRequest{Token: "x"}).Return((*application.TokenPair)(nil), fmt.Errorf("invalid google token: %w", application.ErrGoogleAuthFailed)).Once()
	h.GoogleLogin(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	// internal error — plain error (not ErrGoogleAuthFailed) → 500
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/auth/google", bytes.NewBufferString(`{"token":"z"}`))
	mockService.On("GoogleLogin", mock.Anything, "test-client-id", application.GoogleLoginRequest{Token: "z"}).Return((*application.TokenPair)(nil), errors.New("db down")).Once()
	h.GoogleLogin(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)

	// success
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/auth/google", bytes.NewBufferString(`{"token":"y"}`))
	mockService.On("GoogleLogin", mock.Anything, "test-client-id", application.GoogleLoginRequest{Token: "y"}).Return(&application.TokenPair{AccessToken: "jwt-token", RefreshToken: "refresh-google"}, nil).Once()
	h.GoogleLogin(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "jwt-token")
	cookies := w.Result().Cookies()
	if assert.Len(t, cookies, 1) {
		assert.Equal(t, "refresh_token", cookies[0].Name)
		assert.Equal(t, "refresh-google", cookies[0].Value)
		assert.True(t, cookies[0].HttpOnly)
		assert.True(t, cookies[0].Secure)
		assert.WithinDuration(t, time.Now().Add(refreshTTL), cookies[0].Expires, time.Minute)
	}
}
