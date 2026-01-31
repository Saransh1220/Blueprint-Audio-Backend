package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/domain"
	"golang.org/x/crypto/bcrypt"
)

type RegisterUserReq struct {
	Email    string
	Password string
	Name     string
	Role     string
}

type AuthService struct {
	repo domain.UserRepository
}

// NewAuthService creates and returns a new instance of AuthService.
// It takes a UserRepository as a dependency and initializes the service with it.
func NewAuthService(repo domain.UserRepository) *AuthService {
	return &AuthService{repo: repo}
}

// RegisterUser creates a new user account with the provided registration details.
// It hashes the password using bcrypt, validates that the role is either Artist or Producer,
// and persists the user to the repository. Returns the created user or an error if
// password hashing, role validation, or repository creation fails.
func (s *AuthService) RegisterUser(ctx context.Context, req RegisterUserReq) (*domain.User, error) {
	hashedPass, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	role := domain.UserRole(req.Role)
	if role != domain.RoleArtist && role != domain.RoleProducer {
		return nil, errors.New("Invalid role!")
	}

	user := &domain.User{
		ID:           uuid.New(),
		Email:        req.Email,
		PasswordHash: string(hashedPass),
		Name:         req.Name,
		Role:         role,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := s.repo.CreateUser(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}
