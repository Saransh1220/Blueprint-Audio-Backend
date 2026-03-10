package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/saransh1220/blueprint-audio/internal/modules/auth/domain"
)

type PgSessionRepository struct {
	db *sqlx.DB
}

func NewSessionRepository(db *sqlx.DB) *PgSessionRepository {
	return &PgSessionRepository{db: db}
}

func digestRefreshToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (r *PgSessionRepository) Create(ctx context.Context, session *domain.UserSession) error {
	query := `INSERT INTO user_sessions (id, user_id, refresh_token_digest, is_revoked, expires_at, created_at, updated_at) 
			  VALUES (:id, :user_id, :refresh_token_digest, :is_revoked, :expires_at, :created_at, :updated_at)`

	if session.ID == uuid.Nil {
		sessionID, err := uuid.NewV7()
		if err != nil {
			return fmt.Errorf("failed to generate uuid: %w", err)
		}
		session.ID = sessionID
	}

	if session.CreatedAt.IsZero() {
		session.CreatedAt = time.Now()
	}
	if session.UpdatedAt.IsZero() {
		session.UpdatedAt = time.Now()
	}

	persistedSession := *session
	persistedSession.RefreshTokenDigest = digestRefreshToken(session.RefreshToken)
	persistedSession.RefreshToken = ""
	session.RefreshTokenDigest = persistedSession.RefreshTokenDigest

	_, err := r.db.NamedExecContext(ctx, query, &persistedSession)
	return err
}

func (r *PgSessionRepository) GetByToken(ctx context.Context, token string) (*domain.UserSession, error) {
	session := &domain.UserSession{}
	query := `SELECT * FROM user_sessions WHERE refresh_token_digest = $1`

	err := r.db.GetContext(ctx, session, query, digestRefreshToken(token))
	if err == sql.ErrNoRows {
		return nil, nil // Return nil, nil when not found to easily distinguish from db errors
	}
	if err != nil {
		return nil, err
	}
	return session, nil
}

func (r *PgSessionRepository) Revoke(ctx context.Context, token string) error {
	query := `UPDATE user_sessions SET is_revoked = true, updated_at = $1 WHERE refresh_token_digest = $2`
	_, err := r.db.ExecContext(ctx, query, time.Now(), digestRefreshToken(token))
	return err
}

func (r *PgSessionRepository) RevokeAllForUser(ctx context.Context, userID uuid.UUID) error {
	query := `UPDATE user_sessions SET is_revoked = true, updated_at = $1 WHERE user_id = $2 AND is_revoked = false`
	_, err := r.db.ExecContext(ctx, query, time.Now(), userID)
	return err
}
