package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/saransh1220/blueprint-audio/internal/domain"
	"github.com/saransh1220/blueprint-audio/internal/mocks"
	"github.com/saransh1220/blueprint-audio/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestRegisterUser_Success(t *testing.T) {
	// Setup
	mockRepo := new(mocks.MockUserRepository)
	authService := service.NewAuthService(mockRepo, "secret", time.Hour)
	ctx := context.Background()

	req := service.RegisterUserReq{
		Email:    "test@example.com",
		Password: "password123",
		Name:     "Test User",
		Role:     "artist",
	}

	// Expectation
	mockRepo.On("CreateUser", ctx, mock.AnythingOfType("*domain.User")).Return(nil)

	// Execute
	user, err := authService.RegisterUser(ctx, req)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, user)
	assert.Equal(t, req.Email, user.Email)
	assert.Equal(t, domain.RoleArtist, user.Role)
	assert.NotZero(t, user.ID)

	mockRepo.AssertExpectations(t)
}

func TestRegisterUser_InvalidInput(t *testing.T) {
	mockRepo := new(mocks.MockUserRepository)
	authService := service.NewAuthService(mockRepo, "secret", time.Hour)
	ctx := context.Background()

	// Case 1: Empty Email
	_, err := authService.RegisterUser(ctx, service.RegisterUserReq{
		Email:    "",
		Password: "password123",
		Name:     "Test",
	})
	assert.Error(t, err)
	assert.Equal(t, "missing required fields", err.Error())

	// Case 2: Short Password
	_, err = authService.RegisterUser(ctx, service.RegisterUserReq{
		Email:    "test@example.com",
		Password: "short",
		Name:     "Test",
	})
	assert.Error(t, err)
	assert.Equal(t, "password must be at least 8 characters", err.Error())
}

func TestRegisterUser_InvalidRole(t *testing.T) {
	mockRepo := new(mocks.MockUserRepository)
	authService := service.NewAuthService(mockRepo, "secret", time.Hour)
	ctx := context.Background()

	_, err := authService.RegisterUser(ctx, service.RegisterUserReq{
		Email:    "test@example.com",
		Password: "password123",
		Name:     "Test",
		Role:     "invalid_role",
	})
	assert.Error(t, err)
	assert.Equal(t, "Invalid role!", err.Error())
}
