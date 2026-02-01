package mocks

import (
	"context"

	"github.com/saransh1220/blueprint-audio/internal/domain"
	"github.com/saransh1220/blueprint-audio/internal/service"
	"github.com/stretchr/testify/mock"
)

type MockAuthService struct {
	mock.Mock
}

func (m *MockAuthService) RegisterUser(ctx context.Context, req service.RegisterUserReq) (*domain.User, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.User), args.Error(1)
}

func (m *MockAuthService) LoginUser(ctx context.Context, req service.LoginUserReq) (string, error) {
	args := m.Called(ctx, req)
	return args.String(0), args.Error(1)
}
