package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/domain"
	"github.com/saransh1220/blueprint-audio/internal/mocks"
	"github.com/saransh1220/blueprint-audio/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"golang.org/x/crypto/bcrypt"
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

func TestRegisterUser_DisplayNameAndErrors(t *testing.T) {
	mockRepo := new(mocks.MockUserRepository)
	authService := service.NewAuthService(mockRepo, "secret", time.Hour)
	ctx := context.Background()

	t.Run("invalid email format", func(t *testing.T) {
		_, err := authService.RegisterUser(ctx, service.RegisterUserReq{
			Email:    "invalid-email",
			Password: "password123",
			Name:     "Test",
			Role:     "artist",
		})
		assert.EqualError(t, err, "invalid email format")
	})

	t.Run("display name set", func(t *testing.T) {
		req := service.RegisterUserReq{
			Email:       "display@example.com",
			Password:    "password123",
			Name:        "Real Name",
			DisplayName: "DJ Display",
			Role:        "producer",
		}

		mockRepo.On("CreateUser", ctx, mock.MatchedBy(func(u *domain.User) bool {
			return u != nil && u.DisplayName != nil && *u.DisplayName == "DJ Display"
		})).Return(nil).Once()

		user, err := authService.RegisterUser(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, user)
		if assert.NotNil(t, user.DisplayName) {
			assert.Equal(t, "DJ Display", *user.DisplayName)
		}
	})

	t.Run("display name omitted", func(t *testing.T) {
		req := service.RegisterUserReq{
			Email:    "nodisplay@example.com",
			Password: "password123",
			Name:     "Real Name",
			Role:     "artist",
		}

		mockRepo.On("CreateUser", ctx, mock.MatchedBy(func(u *domain.User) bool {
			return u != nil && u.DisplayName == nil
		})).Return(nil).Once()

		user, err := authService.RegisterUser(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, user)
		assert.Nil(t, user.DisplayName)
	})

	t.Run("repo error propagates", func(t *testing.T) {
		req := service.RegisterUserReq{
			Email:    "repoerr@example.com",
			Password: "password123",
			Name:     "Test",
			Role:     "producer",
		}
		mockRepo.On("CreateUser", ctx, mock.AnythingOfType("*domain.User")).Return(assert.AnError).Once()

		_, err := authService.RegisterUser(ctx, req)
		assert.ErrorIs(t, err, assert.AnError)
	})
}

func TestLoginUser_Branches(t *testing.T) {
	mockRepo := new(mocks.MockUserRepository)
	authService := service.NewAuthService(mockRepo, "secret", time.Hour)
	ctx := context.Background()

	t.Run("missing email or password", func(t *testing.T) {
		_, err := authService.LoginUser(ctx, service.LoginUserReq{})
		assert.EqualError(t, err, "Missing Email or Password")
	})

	t.Run("user not found maps to invalid credentials", func(t *testing.T) {
		mockRepo.On("GetUserByEmail", ctx, "nouser@example.com").
			Return((*domain.User)(nil), domain.ErrUserNotFound).Once()

		_, err := authService.LoginUser(ctx, service.LoginUserReq{
			Email:    "nouser@example.com",
			Password: "password123",
		})
		assert.ErrorIs(t, err, domain.ErrInvalidCredentials)
	})

	t.Run("repo error passes through", func(t *testing.T) {
		dbErr := errors.New("db error")
		mockRepo.On("GetUserByEmail", ctx, "dberr@example.com").
			Return((*domain.User)(nil), dbErr).Once()

		_, err := authService.LoginUser(ctx, service.LoginUserReq{
			Email:    "dberr@example.com",
			Password: "password123",
		})
		assert.ErrorIs(t, err, dbErr)
	})

	t.Run("password mismatch", func(t *testing.T) {
		hash, err := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.DefaultCost)
		assert.NoError(t, err)

		mockRepo.On("GetUserByEmail", ctx, "wrongpass@example.com").
			Return(&domain.User{
				ID:           uuid.New(),
				Email:        "wrongpass@example.com",
				PasswordHash: string(hash),
				Role:         domain.RoleArtist,
			}, nil).Once()

		_, err = authService.LoginUser(ctx, service.LoginUserReq{
			Email:    "wrongpass@example.com",
			Password: "incorrect-password",
		})
		assert.ErrorIs(t, err, domain.ErrInvalidCredentials)
	})

	t.Run("success returns token", func(t *testing.T) {
		hash, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)
		assert.NoError(t, err)

		mockRepo.On("GetUserByEmail", ctx, "ok@example.com").
			Return(&domain.User{
				ID:           uuid.New(),
				Email:        "ok@example.com",
				PasswordHash: string(hash),
				Role:         domain.RoleProducer,
			}, nil).Once()

		token, err := authService.LoginUser(ctx, service.LoginUserReq{
			Email:    "ok@example.com",
			Password: "password123",
		})
		assert.NoError(t, err)
		assert.NotEmpty(t, token)
	})
}
