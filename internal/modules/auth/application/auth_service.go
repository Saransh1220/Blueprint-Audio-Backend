package application

import (
	"context"
	"errors"
	"fmt"
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
	userRepo             domain.UserRepository
	sessionRepo          domain.SessionRepository
	jwtSecret            string
	jwtExpiry            time.Duration
	jwtRefreshExpiry     time.Duration
	googleTokenValidator func(ctx context.Context, token string, audience string) (*idtoken.Payload, error)
}
type GoogleLoginRequest struct {
	Token string `json:"token"`
}
type TokenPair struct {
	AccessToken  string
	RefreshToken string
}

// NewAuthService creates a new auth service
func NewAuthService(userRepo domain.UserRepository, sessionRepo domain.SessionRepository, jwtSecret string, jwtExpiry time.Duration, jwtRefreshExpiry time.Duration) *AuthService {
	return &AuthService{
		userRepo:             userRepo,
		sessionRepo:          sessionRepo,
		jwtSecret:            jwtSecret,
		jwtExpiry:            jwtExpiry,
		jwtRefreshExpiry:     jwtRefreshExpiry,
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

	userID, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("failed to generate uuid: %w", err)
	}

	user := &domain.User{
		ID:           userID,
		Email:        req.Email,
		PasswordHash: string(hashedPass),
		Name:         req.Name,
		DisplayName:  displayName,
		Role:         role,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := s.userRepo.Create(ctx, user); err != nil {
		return nil, err
	}

	return user, nil
}

// Login authenticates a user and returns a JWT token
func (s *AuthService) Login(ctx context.Context, req LoginRequest) (*TokenPair, error) {
	if req.Email == "" || req.Password == "" {
		return nil, errors.New("missing email or password")
	}

	user, err := s.userRepo.GetByEmail(ctx, req.Email)
	if err != nil {
		if err == domain.ErrUserNotFound {
			return nil, domain.ErrInvalidCredentials // Don't reveal user existence
		}
		return nil, err
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return nil, domain.ErrInvalidCredentials
	}

	// Generate session (NEW)
	tokens, err := s.generateSession(ctx, user)
	if err != nil {
		return nil, err
	}

	return tokens, nil
}

// GetUser retrieves a user by ID
func (s *AuthService) GetUser(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	return s.userRepo.GetByID(ctx, id)
}

// ValidateToken validates a JWT token and returns the claims
func (s *AuthService) ValidateToken(tokenStr string) (*jwt.CustomClaims, error) {
	return jwt.ValidateToken(tokenStr, s.jwtSecret)
}

func (s *AuthService) GoogleLogin(ctx context.Context, googleClientID string, req GoogleLoginRequest) (*TokenPair, error) {
	log.Printf("AuthService.GoogleLogin started. ClientID length: %d", len(googleClientID))

	validate := s.googleTokenValidator
	if validate == nil {
		validate = idtoken.Validate
	}

	payload, err := validate(ctx, req.Token, googleClientID)
	if err != nil {
		log.Printf("AuthService.GoogleLogin token validate failed: %v", err)
		return nil, errors.New("invalid google token")
	}

	email, _ := payload.Claims["email"].(string)
	name, _ := payload.Claims["name"].(string)
	picture, _ := payload.Claims["picture"].(string)

	log.Printf("AuthService.GoogleLogin token valid. Email: %s, Name: %s", email, name)

	if email == "" {
		log.Printf("AuthService.GoogleLogin missing email in token")
		return nil, errors.New("email not provided by google")
	}

	user, err := s.userRepo.GetByEmail(ctx, email)
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
			if createErr := s.userRepo.Create(ctx, user); createErr != nil {
				log.Printf("AuthService.GoogleLogin failed to create user: %v", createErr)
				return nil, createErr
			}
		} else {
			log.Printf("AuthService.GoogleLogin repo error: %v", err)
			return nil, err
		}
	} else {
		log.Printf("AuthService.GoogleLogin user found for %s", email)
	}

	// 5. Generate Session
	tokens, err := s.generateSession(ctx, user)
	if err != nil {
		log.Printf("AuthService.GoogleLogin failed to generate session: %v", err)
		return nil, err
	}

	log.Printf("AuthService.GoogleLogin returning success tokens")
	return tokens, nil
}

func (s *AuthService) generateSession(ctx context.Context, user *domain.User) (*TokenPair, error) {
	// 1. Generate Access Token (JWT)
	accessToken, err := jwt.GenerateToken(s.jwtSecret, s.jwtExpiry, user.ID, string(user.Role))
	if err != nil {
		return nil, err
	}

	// 2. Generate Refresh Token
	refreshTokenString, err := jwt.GenerateRefreshToken()
	if err != nil {
		return nil, err
	}

	// 3. Save session in DB
	expiresAt := time.Now().Add(s.jwtRefreshExpiry)
	session := &domain.UserSession{
		ID:           uuid.New(),
		UserID:       user.ID,
		RefreshToken: refreshTokenString,
		IsRevoked:    false,
		ExpiresAt:    expiresAt,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := s.sessionRepo.Create(ctx, session); err != nil {
		return nil, err
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshTokenString,
	}, nil
}

// RefreshSession validates a refresh token and issues a new access token
func (s *AuthService) RefreshSession(ctx context.Context, refreshToken string) (string, error) {
	if refreshToken == "" {
		return "", errors.New("refresh token is required")
	}

	// 1. Get session from DB
	session, err := s.sessionRepo.GetByToken(ctx, refreshToken)
	if err != nil {
		return "", err
	}
	if session == nil {
		return "", errors.New("invalid refresh token")
	}

	// 2. Validate session
	if session.IsRevoked {
		return "", errors.New("session has been revoked")
	}
	if time.Now().After(session.ExpiresAt) {
		return "", errors.New("refresh token expired")
	}

	// 3. Get user to encode into the new JWT
	user, err := s.userRepo.GetByID(ctx, session.UserID)
	if err != nil {
		return "", err
	}

	// 4. Generate NEW Access Token
	newAccessToken, err := jwt.GenerateToken(s.jwtSecret, s.jwtExpiry, user.ID, string(user.Role))
	if err != nil {
		return "", err
	}

	// Note: We are currently keeping the same refresh token until it expires.
	// You could implement "Refresh Token Rotation" here by generating a new one
	// and invalidating the old one if desired for extra security!

	return newAccessToken, nil
}

// Logout revokes a specific session
func (s *AuthService) Logout(ctx context.Context, refreshToken string) error {
	if refreshToken == "" {
		return nil // Nothing to revoke
	}

	// Mark the session as revoked in the database
	return s.sessionRepo.Revoke(ctx, refreshToken)
}
