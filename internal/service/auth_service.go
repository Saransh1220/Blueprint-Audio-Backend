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
	Email       string
	Password    string
	Name        string
	DisplayName string
	Role        string
}

type AuthServiceInterface interface {
	RegisterUser(ctx context.Context, req RegisterUserReq) (*domain.User, error)
	LoginUser(ctx context.Context, req LoginUserReq) (string, error)
	GetUser(ctx context.Context, id uuid.UUID) (*domain.User, error)
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

	if !utils.IsValidEmail(req.Email) {
		return nil, errors.New("invalid email format")
	}

	hashedPass, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	role := domain.UserRole(req.Role)
	if role != domain.RoleArtist && role != domain.RoleProducer {
		return nil, errors.New("Invalid role!")
	}

	var displayName *string
	if req.DisplayName != "" {
		displayName = &req.DisplayName
	} else {
		// Default to Name if not provided? Or keep nil?
		// "Display Name" usually implies an override. If nil, frontend can fallback to Name.
		// But let's check the requirement. User wants to "set a custom name".
		// If they don't set it, it's nice to have it nil.
		// However, I declared it as string in Req, so empty string means not set.
		displayName = nil
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
	if err := s.repo.CreateUser(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

// LoginUser authenticates a user by verifying their email and password credentials.
// It takes a context and a LoginUserReq containing the user's email and password.
// Returns a JWT token string on successful authentication, or an error if authentication fails.
// Returns domain.ErrInvalidCredentials if the email is not found or password verification fails.
// Returns domain.ErrInvalidCredentials for missing email or password to avoid revealing user existence.
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

// GetUser retrieves a user by their ID.
func (s *AuthService) GetUser(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	return s.repo.GetUserById(ctx, id)
}
