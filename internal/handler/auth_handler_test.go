package handler_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/domain"
	"github.com/saransh1220/blueprint-audio/internal/handler"
	"github.com/saransh1220/blueprint-audio/internal/mocks"
	"github.com/saransh1220/blueprint-audio/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestRegisterHandler_Success(t *testing.T) {
	// Setup
	mockService := new(mocks.MockAuthService)
	mockFileService := new(mocks.MockFileService)
	h := handler.NewAuthHandler(mockService, mockFileService)

	reqBody := service.RegisterUserReq{
		Email:    "test@example.com",
		Password: "password123",
		Name:     "Test User",
		Role:     "artist",
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/register", bytes.NewBuffer(body))
	w := httptest.NewRecorder()

	// Expectation
	expectedUser := &domain.User{
		ID:    uuid.New(),
		Email: reqBody.Email,
		Role:  domain.RoleArtist,
	}
	mockService.On("RegisterUser", mock.Anything, reqBody).Return(expectedUser, nil)

	// Execute
	h.Register(w, req)

	// Assert
	assert.Equal(t, http.StatusCreated, w.Code)
	mockService.AssertExpectations(t)
}

func TestRegisterHandler_Conflict(t *testing.T) {
	mockService := new(mocks.MockAuthService)
	mockFileService := new(mocks.MockFileService)
	h := handler.NewAuthHandler(mockService, mockFileService)

	reqBody := service.RegisterUserReq{
		Email: "existing@example.com",
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/register", bytes.NewBuffer(body))
	w := httptest.NewRecorder()

	// Expectation
	mockService.On("RegisterUser", mock.Anything, mock.Anything).Return(nil, domain.ErrUserAlreadyExists)

	// Execute
	h.Register(w, req)

	// Assert
	assert.Equal(t, http.StatusConflict, w.Code)
}

func TestRegisterHandler_BadRequest(t *testing.T) {
	mockService := new(mocks.MockAuthService)
	mockFileService := new(mocks.MockFileService)
	h := handler.NewAuthHandler(mockService, mockFileService)

	// Validation Error
	reqBody := service.RegisterUserReq{Email: ""}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/register", bytes.NewBuffer(body))
	w := httptest.NewRecorder()

	mockService.On("RegisterUser", mock.Anything, mock.Anything).Return(nil, errors.New("missing fields"))

	h.Register(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}
