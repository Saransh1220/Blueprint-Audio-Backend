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

	t.Run("suspended user is refused and all sessions revoked", func(t *testing.T) {
		service, users, sessions, _, _ := newAuthServiceHarness(t)
		userID := uuid.New()
		sessions.On("GetByToken", ctx, "banned").Return(&domain.UserSession{UserID: userID, ExpiresAt: time.Now().Add(time.Hour)}, nil).Once()
		users.On("GetByID", ctx, userID).Return(&domain.User{ID: userID, Status: domain.UserStatusSuspended}, nil).Once()
		sessions.On("RevokeAllForUser", ctx, userID).Return(nil).Once()
		tokens, err := service.RefreshSession(ctx, "banned")
		require.ErrorIs(t, err, ErrAccountSuspended)
		assert.Nil(t, tokens)
	})

	t.Run("rotation write failure discards the unused session", func(t *testing.T) {
		service, users, sessions, _, _ := newAuthServiceHarness(t)
		userID := uuid.New()
		sessions.On("GetByToken", ctx, "old").Return(&domain.UserSession{UserID: userID, ExpiresAt: time.Now().Add(time.Hour)}, nil).Once()
		users.On("GetByID", ctx, userID).Return(&domain.User{ID: userID, Status: domain.UserStatusActive}, nil).Once()
		var newToken string
		sessions.On("Create", ctx, mock.Anything).Run(func(a mock.Arguments) { newToken = a.Get(1).(*domain.UserSession).RefreshToken }).Return(nil).Once()
		sessions.On("MarkRotated", ctx, "old", mock.Anything).Return(false, errors.New("write failed")).Once()
		sessions.On("Revoke", ctx, mock.MatchedBy(func(tok string) bool { return tok == newToken })).Return(nil).Once()
		_, err := service.RefreshSession(ctx, "old")
		require.ErrorContains(t, err, "write failed")
	})

	t.Run("success rotates the refresh token, then logout", func(t *testing.T) {
		service, users, sessions, _, _ := newAuthServiceHarness(t)
		userID := uuid.New()
		sessions.On("GetByToken", ctx, "valid").Return(&domain.UserSession{UserID: userID, ExpiresAt: time.Now().Add(time.Hour)}, nil).Once()
		users.On("GetByID", ctx, userID).Return(&domain.User{ID: userID, Role: domain.RoleProducer, SystemRole: domain.SystemRoleUser, Status: domain.UserStatusActive}, nil).Once()
		var newID uuid.UUID
		sessions.On("Create", ctx, mock.MatchedBy(func(s *domain.UserSession) bool {
			newID = s.ID
			return s.UserID == userID && s.RefreshToken != "" && s.RefreshToken != "valid"
		})).Return(nil).Once()
		sessions.On("MarkRotated", ctx, "valid", mock.MatchedBy(func(id uuid.UUID) bool { return id == newID })).Return(true, nil).Once()
		tokens, err := service.RefreshSession(ctx, "valid")
		require.NoError(t, err)
		assert.NotEmpty(t, tokens.AccessToken)
		assert.NotEmpty(t, tokens.RefreshToken)
		assert.NotEqual(t, "valid", tokens.RefreshToken)

		sessions.On("Revoke", ctx, "valid").Return(nil).Once()
		require.NoError(t, service.Logout(ctx, "valid"))
	})
}

func TestRefreshSessionGraceWindow(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	successor := uuid.New()
	activeUser := &domain.User{ID: userID, Role: domain.RoleArtist, SystemRole: domain.SystemRoleUser, Status: domain.UserStatusActive}
	rotated := func(ago time.Duration) *domain.UserSession {
		at := time.Now().Add(-ago)
		return &domain.UserSession{UserID: userID, IsRevoked: true, RotatedAt: &at, ReplacedBy: &successor, ExpiresAt: time.Now().Add(time.Hour)}
	}

	t.Run("recently rotated token gets an access token only", func(t *testing.T) {
		service, users, sessions, _, _ := newAuthServiceHarness(t)
		sessions.On("GetByToken", ctx, "old").Return(rotated(2*time.Second), nil).Once()
		users.On("GetByID", ctx, userID).Return(activeUser, nil).Once()
		sessions.On("IsActive", ctx, successor).Return(true, nil).Once()
		tokens, err := service.RefreshSession(ctx, "old")
		require.NoError(t, err)
		assert.NotEmpty(t, tokens.AccessToken)
		assert.Empty(t, tokens.RefreshToken, "no new refresh token inside the grace window")
	})

	t.Run("outside the window is refused", func(t *testing.T) {
		service, users, sessions, _, _ := newAuthServiceHarness(t)
		sessions.On("GetByToken", ctx, "old").Return(rotated(time.Minute), nil).Once()
		users.On("GetByID", ctx, userID).Return(activeUser, nil).Once()
		_, err := service.RefreshSession(ctx, "old")
		require.EqualError(t, err, "session has been revoked")
	})

	t.Run("logout of the successor closes the window", func(t *testing.T) {
		service, users, sessions, _, _ := newAuthServiceHarness(t)
		sessions.On("GetByToken", ctx, "old").Return(rotated(time.Second), nil).Once()
		users.On("GetByID", ctx, userID).Return(activeUser, nil).Once()
		sessions.On("IsActive", ctx, successor).Return(false, nil).Once()
		_, err := service.RefreshSession(ctx, "old")
		require.EqualError(t, err, "session has been revoked")
	})

	t.Run("a session revoked by logout has no rotation marker", func(t *testing.T) {
		service, _, sessions, _, _ := newAuthServiceHarness(t)
		sessions.On("GetByToken", ctx, "logged-out").Return(&domain.UserSession{UserID: userID, IsRevoked: true, ExpiresAt: time.Now().Add(time.Hour)}, nil).Once()
		_, err := service.RefreshSession(ctx, "logged-out")
		require.EqualError(t, err, "session has been revoked")
	})

	t.Run("suspension beats the grace window", func(t *testing.T) {
		service, users, sessions, _, _ := newAuthServiceHarness(t)
		sessions.On("GetByToken", ctx, "old").Return(rotated(time.Second), nil).Once()
		users.On("GetByID", ctx, userID).Return(&domain.User{ID: userID, Status: domain.UserStatusSuspended}, nil).Once()
		sessions.On("RevokeAllForUser", ctx, userID).Return(nil).Once()
		_, err := service.RefreshSession(ctx, "old")
		require.ErrorIs(t, err, ErrAccountSuspended)
	})

	t.Run("losing a concurrent rotation race falls back to the grace path", func(t *testing.T) {
		service, users, sessions, _, _ := newAuthServiceHarness(t)
		live := &domain.UserSession{UserID: userID, ExpiresAt: time.Now().Add(time.Hour)}
		sessions.On("GetByToken", ctx, "old").Return(live, nil).Once()
		users.On("GetByID", ctx, userID).Return(activeUser, nil).Once()
		var orphan string
		sessions.On("Create", ctx, mock.Anything).Run(func(a mock.Arguments) { orphan = a.Get(1).(*domain.UserSession).RefreshToken }).Return(nil).Once()
		sessions.On("MarkRotated", ctx, "old", mock.Anything).Return(false, nil).Once()
		sessions.On("Revoke", ctx, mock.MatchedBy(func(tok string) bool { return tok == orphan })).Return(nil).Once()
		sessions.On("GetByToken", ctx, "old").Return(rotated(time.Second), nil).Once()
		sessions.On("IsActive", ctx, successor).Return(true, nil).Once()
		tokens, err := service.RefreshSession(ctx, "old")
		require.NoError(t, err)
		assert.Empty(t, tokens.RefreshToken)
		assert.NotEmpty(t, tokens.AccessToken)
	})
}
