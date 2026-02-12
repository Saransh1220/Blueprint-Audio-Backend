package application

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/modules/auth/domain"
	"github.com/saransh1220/blueprint-audio/internal/modules/auth/infrastructure/jwt"
	"github.com/saransh1220/blueprint-audio/internal/shared/utils"
	"golang.org/x/crypto/bcrypt"
)

// DTOs for registration and login
type RegisterRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// AuthService provides authentication operations
type AuthService struct {
	repo      domain.UserRepository
	jwtSecret string
	jwtExpiry time.Duration
}

// NewAuthService creates a new auth service
func NewAuthService(repo domain.UserRepository, jwtSecret string, jwtExpiry time.Duration) *AuthService {
	return &AuthService{
		repo:      repo,
		jwtSecret: jwtSecret,
		jwtExpiry: jwtExpiry,
	}
}

// Register creates a new user account
func (s *AuthService) Register(ctx context.Context, req RegisterRequest) (*domain.User, error) {
	// Validation
	if req.Email == "" {
		return nil, errors.New("email is required")
	}

	if req.DisplayName == "" {
		return nil, errors.New("display name is required")
	}

	if len(req.Password) < 8 {
		return nil, errors.New("password must be at least 8 characters")
	}

	if !utils.IsValidEmail(req.Email) {
		return nil, errors.New("invalid email format")
	}

	// Hash password
	hashedPass, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	// Validate role
	role := domain.UserRole(req.Role)
	if role != domain.RoleArtist && role != domain.RoleProducer {
		return nil, errors.New("invalid role")
	}

	// Create user
	var displayName *string
	if req.DisplayName != "" {
		displayName = &req.DisplayName
	}

	user := &domain.User{
		ID:           uuid.New(),
		Email:        req.Email,
		PasswordHash: string(hashedPass),
		Name:         req.Name,
		DisplayName:  displayName,
		Role:         role,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := s.repo.Create(ctx, user); err != nil {
		return nil, err
	}

	return user, nil
}

// Login authenticates a user and returns a JWT token
func (s *AuthService) Login(ctx context.Context, req LoginRequest) (string, error) {
	if req.Email == "" || req.Password == "" {
		return "", errors.New("missing email or password")
	}

	user, err := s.repo.GetByEmail(ctx, req.Email)
	if err != nil {
		if err == domain.ErrUserNotFound {
			return "", domain.ErrInvalidCredentials // Don't reveal user existence
		}
		return "", err
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return "", domain.ErrInvalidCredentials
	}

	// Generate token
	token, err := jwt.GenerateToken(s.jwtSecret, s.jwtExpiry, user.ID, string(user.Role))
	if err != nil {
		return "", err
	}

	return token, nil
}

// GetUser retrieves a user by ID
func (s *AuthService) GetUser(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	return s.repo.GetByID(ctx, id)
}

// ValidateToken validates a JWT token and returns the claims
func (s *AuthService) ValidateToken(tokenStr string) (*jwt.CustomClaims, error) {
	return jwt.ValidateToken(tokenStr, s.jwtSecret)
}
