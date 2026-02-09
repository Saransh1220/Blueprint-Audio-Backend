package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/domain"
	"github.com/saransh1220/blueprint-audio/internal/handler"
	"github.com/saransh1220/blueprint-audio/internal/middleware"
	"github.com/saransh1220/blueprint-audio/internal/mocks"
	"github.com/saransh1220/blueprint-audio/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestAuthHandler_Login(t *testing.T) {
	mockService := new(mocks.MockAuthService)
	mockFileService := new(mocks.MockFileService)
	h := handler.NewAuthHandler(mockService, mockFileService)

	req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewBufferString("{invalid json"))
	w := httptest.NewRecorder()
	h.Login(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	body, _ := json.Marshal(service.LoginUserReq{Email: "x@example.com", Password: "password123"})
	req = httptest.NewRequest(http.MethodPost, "/login", bytes.NewBuffer(body))
	w = httptest.NewRecorder()
	mockService.On("LoginUser", mock.Anything, mock.Anything).Return("", domain.ErrInvalidCredentials).Once()
	h.Login(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	req = httptest.NewRequest(http.MethodPost, "/login", bytes.NewBuffer(body))
	w = httptest.NewRecorder()
	mockService.On("LoginUser", mock.Anything, mock.Anything).Return("token", nil).Once()
	h.Login(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAuthHandler_Me(t *testing.T) {
	mockService := new(mocks.MockAuthService)
	mockFileService := new(mocks.MockFileService)
	h := handler.NewAuthHandler(mockService, mockFileService)

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	w := httptest.NewRecorder()
	h.Me(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	userID := uuid.New()
	ctx := context.WithValue(req.Context(), middleware.ContextKeyUserId, userID)
	req = req.WithContext(ctx)
	mockService.On("GetUser", mock.Anything, userID).Return(nil, errors.New("not found")).Once()
	w = httptest.NewRecorder()
	h.Me(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)

	avatar := "http://storage/bucket/avatar.jpg"
	user := &domain.User{ID: userID, AvatarUrl: &avatar}
	req = httptest.NewRequest(http.MethodGet, "/me", nil).WithContext(ctx)
	mockService.On("GetUser", mock.Anything, userID).Return(user, nil).Once()
	mockFileService.On("GetKeyFromUrl", avatar).Return("avatar.jpg", nil).Once()
	mockFileService.On("GetPresignedURL", mock.Anything, "avatar.jpg", mock.Anything).Return("signed-avatar", nil).Once()

	w = httptest.NewRecorder()
	h.Me(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "signed-avatar")
}
