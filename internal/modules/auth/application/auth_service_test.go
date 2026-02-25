package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/modules/auth/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"google.golang.org/api/idtoken"
	"golang.org/x/crypto/bcrypt"
)

type mockUserRepository struct {
	mock.Mock
}

func (m *mockUserRepository) Create(ctx context.Context, user *domain.User) error {
	args := m.Called(ctx, user)
	return args.Error(0)
}

func (m *mockUserRepository) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	args := m.Called(ctx, email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.User), args.Error(1)
}

func (m *mockUserRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.User), args.Error(1)
}

func (m *mockUserRepository) UpdateProfile(ctx context.Context, id uuid.UUID, bio *string, avatarUrl *string, displayName *string, instagramURL, twitterURL, youtubeURL, spotifyURL *string) error {
	args := m.Called(ctx, id, bio, avatarUrl, displayName, instagramURL, twitterURL, youtubeURL, spotifyURL)
	return args.Error(0)
}

func TestRegister_Success(t *testing.T) {
	repo := new(mockUserRepository)
	svc := NewAuthService(repo, "secret", time.Hour)
	ctx := context.Background()

	req := RegisterRequest{
		Email:       "test@example.com",
		Password:    "password123",
		Name:        "Test User",
		DisplayName: "Test User",
		Role:        "artist",
	}

	repo.On("Create", ctx, mock.AnythingOfType("*domain.User")).Return(nil).Once()
	user, err := svc.Register(ctx, req)
	assert.NoError(t, err)
	assert.NotNil(t, user)
	assert.Equal(t, req.Email, user.Email)
	assert.Equal(t, domain.RoleArtist, user.Role)
	assert.NotZero(t, user.ID)
}

func TestRegister_InvalidInput(t *testing.T) {
	repo := new(mockUserRepository)
	svc := NewAuthService(repo, "secret", time.Hour)
	ctx := context.Background()

	_, err := svc.Register(ctx, RegisterRequest{
		Password:    "password123",
		Name:        "Test",
		DisplayName: "Test",
	})
	assert.EqualError(t, err, "email is required")

	_, err = svc.Register(ctx, RegisterRequest{
		Email:    "test@example.com",
		Password: "password123",
		Name:     "Test",
	})
	assert.EqualError(t, err, "display name is required")

	_, err = svc.Register(ctx, RegisterRequest{
		Email:       "test@example.com",
		Password:    "short",
		Name:        "Test",
		DisplayName: "Test",
	})
	assert.EqualError(t, err, "password must be at least 8 characters")
}

func TestRegister_InvalidRoleAndEmail(t *testing.T) {
	repo := new(mockUserRepository)
	svc := NewAuthService(repo, "secret", time.Hour)
	ctx := context.Background()

	_, err := svc.Register(ctx, RegisterRequest{
		Email:       "invalid-email",
		Password:    "password123",
		Name:        "Test",
		DisplayName: "Test",
		Role:        "artist",
	})
	assert.EqualError(t, err, "invalid email format")

	_, err = svc.Register(ctx, RegisterRequest{
		Email:       "test@example.com",
		Password:    "password123",
		Name:        "Test",
		DisplayName: "Test",
		Role:        "invalid_role",
	})
	assert.EqualError(t, err, "invalid role")
}

func TestRegister_RepoError(t *testing.T) {
	repo := new(mockUserRepository)
	svc := NewAuthService(repo, "secret", time.Hour)
	ctx := context.Background()

	req := RegisterRequest{
		Email:       "repoerr@example.com",
		Password:    "password123",
		Name:        "Test",
		DisplayName: "Test",
		Role:        "producer",
	}
	repo.On("Create", ctx, mock.AnythingOfType("*domain.User")).Return(assert.AnError).Once()

	_, err := svc.Register(ctx, req)
	assert.ErrorIs(t, err, assert.AnError)
}

func TestLogin(t *testing.T) {
	ctx := context.Background()

	t.Run("missing fields", func(t *testing.T) {
		repo := new(mockUserRepository)
		svc := NewAuthService(repo, "secret", time.Hour)
		_, err := svc.Login(ctx, LoginRequest{})
		assert.EqualError(t, err, "missing email or password")
	})

	t.Run("repo user not found maps to invalid credentials", func(t *testing.T) {
		repo := new(mockUserRepository)
		svc := NewAuthService(repo, "secret", time.Hour)
		repo.On("GetByEmail", ctx, "missing@example.com").Return(nil, domain.ErrUserNotFound).Once()

		_, err := svc.Login(ctx, LoginRequest{Email: "missing@example.com", Password: "password123"})
		assert.ErrorIs(t, err, domain.ErrInvalidCredentials)
	})

	t.Run("wrong password", func(t *testing.T) {
		repo := new(mockUserRepository)
		svc := NewAuthService(repo, "secret", time.Hour)

		hash, err := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.DefaultCost)
		assert.NoError(t, err)
		user := &domain.User{ID: uuid.New(), Email: "a@a.com", PasswordHash: string(hash), Role: domain.RoleArtist}
		repo.On("GetByEmail", ctx, "a@a.com").Return(user, nil).Once()

		_, err = svc.Login(ctx, LoginRequest{Email: "a@a.com", Password: "wrong-password"})
		assert.ErrorIs(t, err, domain.ErrInvalidCredentials)
	})

	t.Run("success", func(t *testing.T) {
		repo := new(mockUserRepository)
		svc := NewAuthService(repo, "secret", time.Hour)

		hash, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)
		assert.NoError(t, err)
		user := &domain.User{ID: uuid.New(), Email: "a@a.com", PasswordHash: string(hash), Role: domain.RoleProducer}
		repo.On("GetByEmail", ctx, "a@a.com").Return(user, nil).Once()

		token, err := svc.Login(ctx, LoginRequest{Email: "a@a.com", Password: "password123"})
		assert.NoError(t, err)
		assert.NotEmpty(t, token)
	})

	t.Run("repo generic error", func(t *testing.T) {
		repo := new(mockUserRepository)
		svc := NewAuthService(repo, "secret", time.Hour)
		repo.On("GetByEmail", ctx, "x@example.com").Return(nil, errors.New("db down")).Once()

		_, err := svc.Login(ctx, LoginRequest{Email: "x@example.com", Password: "password123"})
		assert.EqualError(t, err, "db down")
	})
}

func TestGetUser(t *testing.T) {
	ctx := context.Background()
	repo := new(mockUserRepository)
	svc := NewAuthService(repo, "secret", time.Hour)
	id := uuid.New()
	expected := &domain.User{ID: id}
	repo.On("GetByID", ctx, id).Return(expected, nil).Once()

	user, err := svc.GetUser(ctx, id)
	assert.NoError(t, err)
	assert.Equal(t, expected, user)
}

func TestValidateToken_Invalid(t *testing.T) {
	repo := new(mockUserRepository)
	svc := NewAuthService(repo, "secret", time.Hour)

	claims, err := svc.ValidateToken("invalid-token")
	assert.Error(t, err)
	assert.Nil(t, claims)
}

func TestGoogleLogin_InvalidToken(t *testing.T) {
	repo := new(mockUserRepository)
	svc := NewAuthService(repo, "secret", time.Hour)
	ctx := context.Background()

	_, err := svc.GoogleLogin(ctx, "fake-google-client-id", GoogleLoginRequest{Token: "not-a-google-token"})
	assert.EqualError(t, err, "invalid google token")
}

func TestGoogleLogin_MissingEmail(t *testing.T) {
	repo := new(mockUserRepository)
	svc := NewAuthService(repo, "secret", time.Hour)
	svc.googleTokenValidator = func(ctx context.Context, token string, audience string) (*idtoken.Payload, error) {
		return &idtoken.Payload{Claims: map[string]interface{}{"name": "Tester"}}, nil
	}

	_, err := svc.GoogleLogin(context.Background(), "google-client", GoogleLoginRequest{Token: "token"})
	assert.EqualError(t, err, "email not provided by google")
}

func TestGoogleLogin_RepoError(t *testing.T) {
	repo := new(mockUserRepository)
	svc := NewAuthService(repo, "secret", time.Hour)
	svc.googleTokenValidator = func(ctx context.Context, token string, audience string) (*idtoken.Payload, error) {
		return &idtoken.Payload{Claims: map[string]interface{}{"email": "user@example.com", "name": "Tester"}}, nil
	}

	repo.On("GetByEmail", mock.Anything, "user@example.com").Return(nil, errors.New("repo down")).Once()

	_, err := svc.GoogleLogin(context.Background(), "google-client", GoogleLoginRequest{Token: "token"})
	assert.EqualError(t, err, "repo down")
}

func TestGoogleLogin_CreateUserError(t *testing.T) {
	repo := new(mockUserRepository)
	svc := NewAuthService(repo, "secret", time.Hour)
	svc.googleTokenValidator = func(ctx context.Context, token string, audience string) (*idtoken.Payload, error) {
		return &idtoken.Payload{Claims: map[string]interface{}{
			"email":   "new@example.com",
			"name":    "New User",
			"picture": "https://img.example.com/a.jpg",
		}}, nil
	}

	repo.On("GetByEmail", mock.Anything, "new@example.com").Return(nil, domain.ErrUserNotFound).Once()
	repo.On("Create", mock.Anything, mock.AnythingOfType("*domain.User")).Return(errors.New("create failed")).Once()

	_, err := svc.GoogleLogin(context.Background(), "google-client", GoogleLoginRequest{Token: "token"})
	assert.EqualError(t, err, "create failed")
}

func TestGoogleLogin_CreateUserSuccess(t *testing.T) {
	repo := new(mockUserRepository)
	svc := NewAuthService(repo, "secret", time.Hour)
	svc.googleTokenValidator = func(ctx context.Context, token string, audience string) (*idtoken.Payload, error) {
		return &idtoken.Payload{Claims: map[string]interface{}{
			"email":   "new2@example.com",
			"name":    "New User2",
			"picture": "https://img.example.com/b.jpg",
		}}, nil
	}

	repo.On("GetByEmail", mock.Anything, "new2@example.com").Return(nil, domain.ErrUserNotFound).Once()
	repo.On("Create", mock.Anything, mock.AnythingOfType("*domain.User")).Return(nil).Once()

	token, err := svc.GoogleLogin(context.Background(), "google-client", GoogleLoginRequest{Token: "token"})
	assert.NoError(t, err)
	assert.NotEmpty(t, token)
}

func TestGoogleLogin_ExistingUserSuccess(t *testing.T) {
	repo := new(mockUserRepository)
	svc := NewAuthService(repo, "secret", time.Hour)
	svc.googleTokenValidator = func(ctx context.Context, token string, audience string) (*idtoken.Payload, error) {
		return &idtoken.Payload{Claims: map[string]interface{}{
			"email": "existing@example.com",
			"name":  "Existing",
		}}, nil
	}

	existing := &domain.User{
		ID:    uuid.New(),
		Email: "existing@example.com",
		Role:  domain.RoleProducer,
	}
	repo.On("GetByEmail", mock.Anything, "existing@example.com").Return(existing, nil).Once()

	token, err := svc.GoogleLogin(context.Background(), "google-client", GoogleLoginRequest{Token: "token"})
	assert.NoError(t, err)
	assert.NotEmpty(t, token)
}

