package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type TokenPurpose string

const (
	TokenPurposeVerifyEmail   TokenPurpose = "verify_email"
	TokenPurposeResetPassword TokenPurpose = "reset_password"
)

type EmailActionToken struct {
	ID         uuid.UUID    `json:"id" db:"id"`
	UserID     uuid.UUID    `json:"user_id" db:"user_id"`
	Email      string       `json:"email" db:"email"`
	Purpose    TokenPurpose `json:"purpose" db:"purpose"`
	Code       string       `json:"-" db:"-"`
	CodeDigest string       `json:"-" db:"code_digest"`
	ExpiresAt  time.Time    `json:"expires_at" db:"expires_at"`
	ConsumedAt *time.Time   `json:"consumed_at,omitempty" db:"consumed_at"`
	CreatedAt  time.Time    `json:"created_at" db:"created_at"`
	UpdatedAt  time.Time    `json:"updated_at" db:"updated_at"`
}

type EmailActionTokenRepository interface {
	Create(ctx context.Context, token *EmailActionToken) error
	Consume(ctx context.Context, email string, purpose TokenPurpose, code string) (*EmailActionToken, error)
	InvalidateActive(ctx context.Context, userID uuid.UUID, purpose TokenPurpose) error
}
