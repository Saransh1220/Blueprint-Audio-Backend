package http_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/middleware"
	"github.com/saransh1220/blueprint-audio/internal/modules/auth/application"
	"github.com/saransh1220/blueprint-audio/internal/modules/auth/domain"
	auth_http "github.com/saransh1220/blueprint-audio/internal/modules/auth/interfaces/http"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestAuthHandler_LoginAndMeBranches(t *testing.T) {
	mockService := new(MockAuthService)
	mockFileService := new(MockFileService)
	h := auth_http.NewAuthHandler(mockService, mockFileService)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewBufferString("bad"))
	h.Login(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/login", bytes.NewBufferString(`{"email":"x","password":"y"}`))
	mockService.On("Login", mock.Anything, application.LoginRequest{Email: "x", Password: "y"}).Return("", domain.ErrInvalidCredentials).Once()
	h.Login(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/login", bytes.NewBufferString(`{"email":"x","password":"y"}`))
	mockService.On("Login", mock.Anything, application.LoginRequest{Email: "x", Password: "y"}).Return("", errors.New("db")).Once()
	h.Login(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/login", bytes.NewBufferString(`{"email":"x","password":"y"}`))
	mockService.On("Login", mock.Anything, application.LoginRequest{Email: "x", Password: "y"}).Return("token", nil).Once()
	h.Login(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

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
	h := auth_http.NewAuthHandler(mockService, mockFileService)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/register", nil)
	h.Register(w, req)
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/register", bytes.NewBufferString("bad"))
	h.Register(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}
