package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/saransh1220/blueprint-audio/internal/modules/auth/domain"
)

type PgEmailActionTokenRepository struct {
	db *sqlx.DB
}

func NewEmailActionTokenRepository(db *sqlx.DB) *PgEmailActionTokenRepository {
	return &PgEmailActionTokenRepository{db: db}
}

func digestEmailActionCode(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
}

func (r *PgEmailActionTokenRepository) Create(ctx context.Context, token *domain.EmailActionToken) error {
	if token.ID == uuid.Nil {
		token.ID = uuid.Must(uuid.NewV7())
	}
	if token.CreatedAt.IsZero() {
		token.CreatedAt = time.Now()
	}
	token.UpdatedAt = time.Now()
	token.CodeDigest = digestEmailActionCode(token.Code)

	query := `INSERT INTO email_action_tokens (id, user_id, email, purpose, code_digest, expires_at, consumed_at, created_at, updated_at)
	VALUES (:id, :user_id, :email, :purpose, :code_digest, :expires_at, :consumed_at, :created_at, :updated_at)`

	_, err := r.db.NamedExecContext(ctx, query, token)
	return err
}

func (r *PgEmailActionTokenRepository) Consume(ctx context.Context, email string, purpose domain.TokenPurpose, code string) (*domain.EmailActionToken, error) {
	token := &domain.EmailActionToken{}
	query := `
		UPDATE email_action_tokens
		SET consumed_at = NOW(), updated_at = NOW()
		WHERE lower(email) = lower($1)
		  AND purpose = $2
		  AND code_digest = $3
		  AND consumed_at IS NULL
		  AND expires_at > NOW()
		RETURNING id, user_id, email, purpose, code_digest, expires_at, consumed_at, created_at, updated_at
	`
	err := r.db.GetContext(ctx, token, query, email, purpose, digestEmailActionCode(code))
	if err == sql.ErrNoRows {
		return nil, domain.ErrInvalidOrExpiredCode
	}
	if err != nil {
		return nil, err
	}
	return token, nil
}

func (r *PgEmailActionTokenRepository) InvalidateActive(ctx context.Context, userID uuid.UUID, purpose domain.TokenPurpose) error {
	query := `
		UPDATE email_action_tokens
		SET consumed_at = NOW(), updated_at = NOW()
		WHERE user_id = $1
		  AND purpose = $2
		  AND consumed_at IS NULL
		  AND expires_at > NOW()
	`
	_, err := r.db.ExecContext(ctx, query, userID, purpose)
	return err
}
