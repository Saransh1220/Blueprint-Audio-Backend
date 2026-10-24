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
	// RotatedAt and ReplacedBy are set only when the session was revoked because
	// it was exchanged for a new one. Logout, password reset and suspension leave
	// them nil, which is what keeps the refresh grace window from reviving them.
	RotatedAt  *time.Time `json:"-" db:"rotated_at"`
	ReplacedBy *uuid.UUID `json:"-" db:"replaced_by"`
}

// SessionRepository defines the contract for session data access
type SessionRepository interface {
	Create(ctx context.Context, session *UserSession) error
	GetByToken(ctx context.Context, token string) (*UserSession, error)
	Revoke(ctx context.Context, token string) error
	RevokeAllForUser(ctx context.Context, userID uuid.UUID) error
	// MarkRotated revokes the session behind token and records its successor. It
	// reports false when the session was already revoked, so exactly one of
	// several concurrent refreshes wins.
	MarkRotated(ctx context.Context, token string, successorID uuid.UUID) (bool, error)
	// IsActive reports whether the session exists, is not revoked and has not expired.
	IsActive(ctx context.Context, id uuid.UUID) (bool, error)
}
