package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/saransh1220/blueprint-audio/internal/modules/legal/domain"
)

type PgAcceptanceRepository struct {
	db *sqlx.DB
}

func NewAcceptanceRepository(db *sqlx.DB) *PgAcceptanceRepository {
	return &PgAcceptanceRepository{db: db}
}

func (r *PgAcceptanceRepository) RecordAcceptance(ctx context.Context, acceptance *domain.Acceptance) error {
	query := `
		INSERT INTO legal_acceptances (
			id, user_id, document_key, version_id, content_hash, action, context, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	_, err := r.db.ExecContext(
		ctx, query,
		acceptance.ID, acceptance.UserID, acceptance.DocumentKey,
		acceptance.VersionID, acceptance.ContentHash, acceptance.Action,
		acceptance.Context, acceptance.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to record legal acceptance: %w", err)
	}
	return nil
}

func (r *PgAcceptanceRepository) GetLatestAcceptance(ctx context.Context, userID uuid.UUID, documentKey string) (*domain.Acceptance, error) {
	var a domain.Acceptance
	query := `
		SELECT id, user_id, document_key, version_id, content_hash, action, context, created_at
		FROM legal_acceptances
		WHERE user_id = $1 AND document_key = $2
		ORDER BY created_at DESC
		LIMIT 1
	`
	err := r.db.GetContext(ctx, &a, query, userID, documentKey)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get latest acceptance: %w", err)
	}
	return &a, nil
}

func (r *PgAcceptanceRepository) HasAcceptedVersion(ctx context.Context, userID, versionID uuid.UUID) (bool, error) {
	var count int
	query := `
		SELECT count(*)
		FROM legal_acceptances
		WHERE user_id = $1 AND version_id = $2
	`
	err := r.db.GetContext(ctx, &count, query, userID, versionID)
	if err != nil {
		return false, fmt.Errorf("failed to check version acceptance: %w", err)
	}
	return count > 0, nil
}
