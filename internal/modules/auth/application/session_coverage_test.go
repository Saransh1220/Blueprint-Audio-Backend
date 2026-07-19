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
	"github.com/stretchr/testify/require"
)

func TestResendVerificationBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("missing email", func(t *testing.T) {
		service, _, _, _, _ := newAuthServiceHarness(t)
		require.EqualError(t, service.ResendVerification(ctx, ResendVerificationRequest{}), "email is required")
	})

	t.Run("unknown email is intentionally silent", func(t *testing.T) {
		service, users, _, _, _ := newAuthServiceHarness(t)
		users.On("GetByEmail", ctx, "missing@example.com").Return(nil, domain.ErrUserNotFound).Once()
		require.NoError(t, service.ResendVerification(ctx, ResendVerificationRequest{Email: " missing@example.com "}))
	})

	t.Run("repository error", func(t *testing.T) {
		service, users, _, _, _ := newAuthServiceHarness(t)
		users.On("GetByEmail", ctx, "error@example.com").Return(nil, errors.New("read failed")).Once()
		require.EqualError(t, service.ResendVerification(ctx, ResendVerificationRequest{Email: "error@example.com"}), "read failed")
	})

	t.Run("verified user", func(t *testing.T) {
		service, users, _, _, _ := newAuthServiceHarness(t)
		user := &domain.User{ID: uuid.New(), Email: "verified@example.com", EmailVerified: true}
		users.On("GetByEmail", ctx, user.Email).Return(user, nil).Once()
		require.NoError(t, service.ResendVerification(ctx, ResendVerificationRequest{Email: user.Email}))
	})

	t.Run("unverified user receives a new code", func(t *testing.T) {
		service, users, _, tokens, email := newAuthServiceHarness(t)
		displayName := "Display Name"
		user := &domain.User{ID: uuid.New(), Email: "new@example.com", Name: "Name", DisplayName: &displayName}
		users.On("GetByEmail", ctx, user.Email).Return(user, nil).Once()
		tokens.On("InvalidateActive", ctx, user.ID, domain.TokenPurposeVerifyEmail).Return(nil).Once()
		tokens.On("Create", ctx, mock.MatchedBy(func(token *domain.EmailActionToken) bool {
			return token.UserID == user.ID && token.Purpose == domain.TokenPurposeVerifyEmail
		})).Return(nil).Once()
		email.On("Send", ctx, mock.Anything).Return(nil).Once()
		require.NoError(t, service.ResendVerification(ctx, ResendVerificationRequest{Email: user.Email}))
	})
}

func TestRefreshSessionAndLogoutBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("missing token", func(t *testing.T) {
		service, _, _, _, _ := newAuthServiceHarness(t)
		_, err := service.RefreshSession(ctx, "")
		require.EqualError(t, err, "refresh token is required")
		require.NoError(t, service.Logout(ctx, ""))
	})

	t.Run("repository error", func(t *testing.T) {
		service, _, sessions, _, _ := newAuthServiceHarness(t)
		sessions.On("GetByToken", ctx, "error").Return(nil, errors.New("read failed")).Once()
		_, err := service.RefreshSession(ctx, "error")
		require.EqualError(t, err, "read failed")
	})

	t.Run("unknown token", func(t *testing.T) {
		service, _, sessions, _, _ := newAuthServiceHarness(t)
		sessions.On("GetByToken", ctx, "unknown").Return(nil, nil).Once()
		_, err := service.RefreshSession(ctx, "unknown")
		require.EqualError(t, err, "invalid refresh token")
	})

	t.Run("revoked token", func(t *testing.T) {
		service, _, sessions, _, _ := newAuthServiceHarness(t)
		sessions.On("GetByToken", ctx, "revoked").Return(&domain.UserSession{IsRevoked: true}, nil).Once()
		_, err := service.RefreshSession(ctx, "revoked")
		require.EqualError(t, err, "session has been revoked")
	})

	t.Run("expired token", func(t *testing.T) {
		service, _, sessions, _, _ := newAuthServiceHarness(t)
		sessions.On("GetByToken", ctx, "expired").Return(&domain.UserSession{ExpiresAt: time.Now().Add(-time.Minute)}, nil).Once()
		_, err := service.RefreshSession(ctx, "expired")
		require.EqualError(t, err, "refresh token expired")
	})

	t.Run("user lookup error", func(t *testing.T) {
		service, users, sessions, _, _ := newAuthServiceHarness(t)
		userID := uuid.New()
		sessions.On("GetByToken", ctx, "lookup").Return(&domain.UserSession{UserID: userID, ExpiresAt: time.Now().Add(time.Hour)}, nil).Once()
		users.On("GetByID", ctx, userID).Return(nil, errors.New("user read failed")).Once()
		_, err := service.RefreshSession(ctx, "lookup")
		require.EqualError(t, err, "user read failed")
	})

	t.Run("success and logout", func(t *testing.T) {
		service, users, sessions, _, _ := newAuthServiceHarness(t)
		userID := uuid.New()
		sessions.On("GetByToken", ctx, "valid").Return(&domain.UserSession{UserID: userID, ExpiresAt: time.Now().Add(time.Hour)}, nil).Once()
		users.On("GetByID", ctx, userID).Return(&domain.User{ID: userID, Role: domain.RoleProducer, SystemRole: domain.SystemRoleUser}, nil).Once()
		accessToken, err := service.RefreshSession(ctx, "valid")
		require.NoError(t, err)
		assert.NotEmpty(t, accessToken)

		sessions.On("Revoke", ctx, "valid").Return(nil).Once()
		require.NoError(t, service.Logout(ctx, "valid"))
	})
}
