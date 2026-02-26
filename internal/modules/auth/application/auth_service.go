package application

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/modules/auth/domain"
	"github.com/saransh1220/blueprint-audio/internal/modules/auth/infrastructure/jwt"
	"github.com/saransh1220/blueprint-audio/internal/shared/utils"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/api/idtoken"
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
	repo                 domain.UserRepository
	jwtSecret            string
	jwtExpiry            time.Duration
	googleTokenValidator func(ctx context.Context, token string, audience string) (*idtoken.Payload, error)
}
type GoogleLoginRequest struct {
	Token string `json:"token"`
}

// NewAuthService creates a new auth service
func NewAuthService(repo domain.UserRepository, jwtSecret string, jwtExpiry time.Duration) *AuthService {
	return &AuthService{
		repo:                 repo,
		jwtSecret:            jwtSecret,
		jwtExpiry:            jwtExpiry,
		googleTokenValidator: idtoken.Validate,
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

func (s *AuthService) GoogleLogin(ctx context.Context, googleClientID string, req GoogleLoginRequest) (string, error) {
	log.Printf("AuthService.GoogleLogin started. ClientID length: %d", len(googleClientID))

	validate := s.googleTokenValidator
	if validate == nil {
		validate = idtoken.Validate
	}

	payload, err := validate(ctx, req.Token, googleClientID)
	if err != nil {
		log.Printf("AuthService.GoogleLogin token validate failed: %v", err)
		return "", errors.New("invalid google token")
	}

	email, _ := payload.Claims["email"].(string)
	name, _ := payload.Claims["name"].(string)
	picture, _ := payload.Claims["picture"].(string)

	log.Printf("AuthService.GoogleLogin token valid. Email: %s, Name: %s", email, name)

	if email == "" {
		log.Printf("AuthService.GoogleLogin missing email in token")
		return "", errors.New("email not provided by google")
	}

	user, err := s.repo.GetByEmail(ctx, email)
	if err != nil {
		if err == domain.ErrUserNotFound {
			log.Printf("AuthService.GoogleLogin user not found, creating new one for %s", email)
			// 4. Create new user if they don't exist
			user = &domain.User{
				ID:           uuid.New(),
				Email:        email,
				PasswordHash: "", // No password for OAuth users
				Name:         name,
				DisplayName:  &name,
				Role:         domain.RoleArtist, // Default role
				AvatarUrl:    &picture,
				CreatedAt:    time.Now(),
				UpdatedAt:    time.Now(),
			}
			if createErr := s.repo.Create(ctx, user); createErr != nil {
				log.Printf("AuthService.GoogleLogin failed to create user: %v", createErr)
				return "", createErr
			}
		} else {
			log.Printf("AuthService.GoogleLogin repo error: %v", err)
			return "", err
		}
	} else {
		log.Printf("AuthService.GoogleLogin user found for %s", email)
	}

	// 5. Generate our own Application JWT
	token, err := jwt.GenerateToken(s.jwtSecret, s.jwtExpiry, user.ID, string(user.Role))
	if err != nil {
		log.Printf("AuthService.GoogleLogin failed to generate JWT: %v", err)
		return "", err
	}

	log.Printf("AuthService.GoogleLogin returning success token")
	return token, nil
}
