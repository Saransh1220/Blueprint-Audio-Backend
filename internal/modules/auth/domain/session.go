package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// UserSession represents a user's refresh token session
type UserSession struct {
	ID                 uuid.UUID `json:"id" db:"id"`
	UserID             uuid.UUID `json:"user_id" db:"user_id"`
	RefreshToken       string    `json:"-" db:"-"`
	RefreshTokenDigest string    `json:"-" db:"refresh_token_digest"`
	IsRevoked          bool      `json:"is_revoked" db:"is_revoked"`
	ExpiresAt          time.Time `json:"expires_at" db:"expires_at"`
	CreatedAt          time.Time `json:"created_at" db:"created_at"`
	UpdatedAt          time.Time `json:"updated_at" db:"updated_at"`
}

// SessionRepository defines the contract for session data access
type SessionRepository interface {
	Create(ctx context.Context, session *UserSession) error
	GetByToken(ctx context.Context, token string) (*UserSession, error)
	Revoke(ctx context.Context, token string) error
	RevokeAllForUser(ctx context.Context, userID uuid.UUID) error
}
