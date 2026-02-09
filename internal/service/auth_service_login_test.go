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
	"golang.org/x/crypto/bcrypt"
)

func TestLoginUser(t *testing.T) {
	ctx := context.Background()

	t.Run("missing fields", func(t *testing.T) {
		repo := new(mocks.MockUserRepository)
		svc := service.NewAuthService(repo, "secret", time.Hour)
		_, err := svc.LoginUser(ctx, service.LoginUserReq{})
		assert.EqualError(t, err, "Missing Email or Password")
	})

	t.Run("repo user not found maps to invalid credentials", func(t *testing.T) {
		repo := new(mocks.MockUserRepository)
		svc := service.NewAuthService(repo, "secret", time.Hour)
		repo.On("GetUserByEmail", ctx, "missing@example.com").Return(nil, domain.ErrUserNotFound)

		_, err := svc.LoginUser(ctx, service.LoginUserReq{Email: "missing@example.com", Password: "password123"})
		assert.ErrorIs(t, err, domain.ErrInvalidCredentials)
	})

	t.Run("wrong password", func(t *testing.T) {
		repo := new(mocks.MockUserRepository)
		svc := service.NewAuthService(repo, "secret", time.Hour)

		hash, err := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.DefaultCost)
		assert.NoError(t, err)
		user := &domain.User{ID: uuid.New(), Email: "a@a.com", PasswordHash: string(hash), Role: domain.RoleArtist}
		repo.On("GetUserByEmail", ctx, "a@a.com").Return(user, nil)

		_, err = svc.LoginUser(ctx, service.LoginUserReq{Email: "a@a.com", Password: "wrong-password"})
		assert.ErrorIs(t, err, domain.ErrInvalidCredentials)
	})

	t.Run("success", func(t *testing.T) {
		repo := new(mocks.MockUserRepository)
		svc := service.NewAuthService(repo, "secret", time.Hour)

		hash, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)
		assert.NoError(t, err)
		user := &domain.User{ID: uuid.New(), Email: "a@a.com", PasswordHash: string(hash), Role: domain.RoleProducer}
		repo.On("GetUserByEmail", ctx, "a@a.com").Return(user, nil)

		token, err := svc.LoginUser(ctx, service.LoginUserReq{Email: "a@a.com", Password: "password123"})
		assert.NoError(t, err)
		assert.NotEmpty(t, token)
	})

	t.Run("repo generic error", func(t *testing.T) {
		repo := new(mocks.MockUserRepository)
		svc := service.NewAuthService(repo, "secret", time.Hour)
		repo.On("GetUserByEmail", ctx, "x@example.com").Return(nil, errors.New("db down"))

		_, err := svc.LoginUser(ctx, service.LoginUserReq{Email: "x@example.com", Password: "password123"})
		assert.EqualError(t, err, "db down")
	})
}

func TestGetUser(t *testing.T) {
	ctx := context.Background()
	repo := new(mocks.MockUserRepository)
	svc := service.NewAuthService(repo, "secret", time.Hour)
	id := uuid.New()
	expected := &domain.User{ID: id}
	repo.On("GetUserById", ctx, id).Return(expected, nil)

	user, err := svc.GetUser(ctx, id)
	assert.NoError(t, err)
	assert.Equal(t, expected, user)
}
