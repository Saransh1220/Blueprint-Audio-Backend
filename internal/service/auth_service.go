package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/domain"
	"github.com/saransh1220/blueprint-audio/internal/utils"
	"golang.org/x/crypto/bcrypt"
)

type RegisterUserReq struct {
	Email    string
	Password string
	Name     string
	Role     string
}

type AuthServiceInterface interface {
	RegisterUser(ctx context.Context, req RegisterUserReq) (*domain.User, error)
	LoginUser(ctx context.Context, req LoginUserReq) (string, error)
}

type AuthService struct {
	repo      domain.UserRepository
	jwtSecret string
	jwtExpiry time.Duration
}

type LoginUserReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// NewAuthService creates and returns a new instance of AuthService.
// It takes a UserRepository as a dependency and initializes the service with it.
func NewAuthService(repo domain.UserRepository, jwtSecret string, jwtExpiry time.Duration) *AuthService {
	return &AuthService{repo: repo,
		jwtSecret: jwtSecret,
		jwtExpiry: jwtExpiry,
	}
}

// RegisterUser creates a new user account with the provided registration details.
// It hashes the password using bcrypt, validates that the role is either Artist or Producer,
// and persists the user to the repository. Returns the created user or an error if
// password hashing, role validation, or repository creation fails.
func (s *AuthService) RegisterUser(ctx context.Context, req RegisterUserReq) (*domain.User, error) {
	// Validation
	if req.Email == "" || req.Password == "" || req.Name == "" {
		return nil, errors.New("missing required fields")
	}
	if len(req.Password) < 8 {
		return nil, errors.New("password must be at least 8 characters")
	}

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

func (s *AuthService) LoginUser(ctx context.Context, req LoginUserReq) (string, error) {
	if req.Email == "" || req.Password == "" {
		return "", errors.New("Missing Email or Password")
	}

	user, err := s.repo.GetUserByEmail(ctx, req.Email)
	if err != nil {
		if err == domain.ErrUserNotFound {
			return "", domain.ErrInvalidCredentials // Don't reveal user existence
		}
		return "", err
	}

	// 3. Verify Password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return "", domain.ErrInvalidCredentials
	}

	// 4. Generate Token
	token, err := utils.GenerateToken(s.jwtSecret, s.jwtExpiry, user.ID, string(user.Role))
	if err != nil {
		return "", err
	}
	return token, nil
}
