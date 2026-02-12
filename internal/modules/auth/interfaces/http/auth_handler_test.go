package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/modules/auth/application"
	"github.com/saransh1220/blueprint-audio/internal/modules/auth/domain"
	auth_http "github.com/saransh1220/blueprint-audio/internal/modules/auth/interfaces/http"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// Mock AuthService
type MockAuthService struct {
	mock.Mock
}

func (m *MockAuthService) Register(ctx context.Context, req application.RegisterRequest) (*domain.User, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.User), args.Error(1)
}

func (m *MockAuthService) Login(ctx context.Context, req application.LoginRequest) (string, error) {
	args := m.Called(ctx, req)
	return args.String(0), args.Error(1)
}

func (m *MockAuthService) GetUser(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.User), args.Error(1)
}

// Mock FileService
type MockFileService struct {
	mock.Mock
}

func (m *MockFileService) GetKeyFromUrl(fileUrl string) (string, error) {
	args := m.Called(fileUrl)
	return args.String(0), args.Error(1)
}

func (m *MockFileService) GetPresignedURL(ctx context.Context, objectName string, expiry time.Duration) (string, error) {
	args := m.Called(ctx, objectName, expiry)
	return args.String(0), args.Error(1)
}

func TestRegisterHandler_Success(t *testing.T) {
	mockService := new(MockAuthService)
	mockFileService := new(MockFileService)
	h := auth_http.NewAuthHandler(mockService, mockFileService)

	reqBody := application.RegisterRequest{
		Email:    "test@example.com",
		Password: "password123",
		Name:     "Test User",
		Role:     "artist",
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/register", bytes.NewBuffer(body))
	w := httptest.NewRecorder()

	expectedUser := &domain.User{
		ID:    uuid.New(),
		Email: reqBody.Email,
		Role:  domain.RoleArtist,
	}
	mockService.On("Register", mock.Anything, reqBody).Return(expectedUser, nil)

	h.Register(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	mockService.AssertExpectations(t)
}

func TestRegisterHandler_Conflict(t *testing.T) {
	mockService := new(MockAuthService)
	mockFileService := new(MockFileService)
	h := auth_http.NewAuthHandler(mockService, mockFileService)

	reqBody := application.RegisterRequest{
		Email: "existing@example.com",
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/register", bytes.NewBuffer(body))
	w := httptest.NewRecorder()

	mockService.On("Register", mock.Anything, mock.Anything).Return(nil, domain.ErrUserAlreadyExists)

	h.Register(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)
}

func TestRegisterHandler_BadRequest(t *testing.T) {
	mockService := new(MockAuthService)
	mockFileService := new(MockFileService)
	h := auth_http.NewAuthHandler(mockService, mockFileService)

	reqBody := application.RegisterRequest{Email: ""}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/register", bytes.NewBuffer(body))
	w := httptest.NewRecorder()

	mockService.On("Register", mock.Anything, mock.Anything).Return(nil, errors.New("Display name required"))

	h.Register(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}
