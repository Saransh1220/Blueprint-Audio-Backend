package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/saransh1220/blueprint-audio/internal/modules/legal/domain"
)

type PgDocumentRepository struct {
	db *sqlx.DB
}

func NewDocumentRepository(db *sqlx.DB) *PgDocumentRepository {
	return &PgDocumentRepository{db: db}
}

func (r *PgDocumentRepository) GetDocument(ctx context.Context, key, locale string) (*domain.Document, error) {
	var doc domain.Document
	query := `
		SELECT key, audience, locale, title, created_at, updated_at
		FROM legal_documents
		WHERE key = $1 AND locale = $2
	`
	err := r.db.GetContext(ctx, &doc, query, key, locale)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.ErrDocumentNotFound
		}
		return nil, fmt.Errorf("failed to get legal document: %w", err)
	}
	return &doc, nil
}

func (r *PgDocumentRepository) GetEffectiveVersion(ctx context.Context, key, locale string) (*domain.DocumentVersion, error) {
	query := `
		SELECT v.id, v.document_key, v.display_version, v.schema_version, v.title, v.summary,
		       v.sections, v.content_hash, v.update_classification, v.status,
		       v.published_at, v.effective_at, v.superseded_at, v.created_by, v.created_at
		FROM legal_document_versions v
		JOIN legal_documents d ON d.key = v.document_key
		WHERE v.document_key = $1 AND d.locale = $2
		  AND v.status = 'published' AND v.effective_at <= NOW()
		ORDER BY v.effective_at DESC
		LIMIT 1
	`
	return r.scanVersion(ctx, query, key, locale)
}

func (r *PgDocumentRepository) GetUpcomingVersion(ctx context.Context, key, locale string) (*domain.DocumentVersion, error) {
	query := `
		SELECT v.id, v.document_key, v.display_version, v.schema_version, v.title, v.summary,
		       v.sections, v.content_hash, v.update_classification, v.status,
		       v.published_at, v.effective_at, v.superseded_at, v.created_by, v.created_at
		FROM legal_document_versions v
		JOIN legal_documents d ON d.key = v.document_key
		WHERE v.document_key = $1 AND d.locale = $2
		  AND v.status IN ('published', 'scheduled') AND v.effective_at > NOW()
		ORDER BY v.effective_at ASC
		LIMIT 1
	`
	return r.scanVersion(ctx, query, key, locale)
}

func (r *PgDocumentRepository) GetVersionByDisplay(ctx context.Context, key, displayVersion string) (*domain.DocumentVersion, error) {
	query := `
		SELECT id, document_key, display_version, schema_version, title, summary,
		       sections, content_hash, update_classification, status,
		       published_at, effective_at, superseded_at, created_by, created_at
		FROM legal_document_versions
		WHERE document_key = $1 AND display_version = $2
	`
	return r.scanVersion(ctx, query, key, displayVersion)
}

func (r *PgDocumentRepository) GetVersionByID(ctx context.Context, id uuid.UUID) (*domain.DocumentVersion, error) {
	query := `
		SELECT id, document_key, display_version, schema_version, title, summary,
		       sections, content_hash, update_classification, status,
		       published_at, effective_at, superseded_at, created_by, created_at
		FROM legal_document_versions
		WHERE id = $1
	`
	return r.scanVersion(ctx, query, id)
}

func (r *PgDocumentRepository) ListPublishedVersions(ctx context.Context, key string) ([]domain.DocumentVersion, error) {
	query := `
		SELECT id, document_key, display_version, schema_version, title, summary,
		       sections, content_hash, update_classification, status,
		       published_at, effective_at, superseded_at, created_by, created_at
		FROM legal_document_versions
		WHERE document_key = $1 AND status IN ('published', 'archived')
		ORDER BY effective_at DESC
	`
	rows, err := r.db.QueryContext(ctx, query, key)
	if err != nil {
		return nil, fmt.Errorf("failed to list published versions: %w", err)
	}
	defer rows.Close()

	var versions []domain.DocumentVersion
	for rows.Next() {
		var v domain.DocumentVersion
		var sectionsRaw []byte
		err := rows.Scan(
			&v.ID, &v.DocumentKey, &v.DisplayVersion, &v.SchemaVersion, &v.Title, &v.Summary,
			&sectionsRaw, &v.ContentHash, &v.UpdateClassification, &v.Status,
			&v.PublishedAt, &v.EffectiveAt, &v.SupersededAt, &v.CreatedBy, &v.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan version row: %w", err)
		}
		if len(sectionsRaw) > 0 {
			_ = json.Unmarshal(sectionsRaw, &v.Sections)
		}
		versions = append(versions, v)
	}
	return versions, nil
}

func (r *PgDocumentRepository) CreateDraft(ctx context.Context, version *domain.DocumentVersion) error {
	sectionsRaw, err := json.Marshal(version.Sections)
	if err != nil {
		return fmt.Errorf("failed to marshal sections: %w", err)
	}

	query := `
		INSERT INTO legal_document_versions (
			id, document_key, display_version, schema_version, title, summary,
			sections, content_hash, update_classification, status,
			published_at, effective_at, created_by, created_at
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10,
			$11, $12, $13, $14
		)
	`
	_, err = r.db.ExecContext(
		ctx, query,
		version.ID, version.DocumentKey, version.DisplayVersion, version.SchemaVersion, version.Title, version.Summary,
		sectionsRaw, version.ContentHash, version.UpdateClassification, version.Status,
		version.PublishedAt, version.EffectiveAt, version.CreatedBy, version.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert draft version: %w", err)
	}
	return nil
}

func (r *PgDocumentRepository) PublishVersion(ctx context.Context, id uuid.UUID) error {
	now := time.Now().UTC()
	// Atomically supersedes older active versions for this document
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var docKey string
	err = tx.GetContext(ctx, &docKey, "SELECT document_key FROM legal_document_versions WHERE id = $1", id)
	if err != nil {
		return err
	}

	// Archive old published versions
	_, err = tx.ExecContext(ctx, `
		UPDATE legal_document_versions
		SET status = 'archived', superseded_at = $1
		WHERE document_key = $2 AND status = 'published' AND id != $3
	`, now, docKey, id)
	if err != nil {
		return err
	}

	// Set target version to published
	_, err = tx.ExecContext(ctx, `
		UPDATE legal_document_versions
		SET status = 'published', published_at = $1, effective_at = $1
		WHERE id = $2
	`, now, id)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (r *PgDocumentRepository) RecordAudit(ctx context.Context, audit *domain.PublicationAudit) error {
	query := `
		INSERT INTO legal_publication_audits (
			id, document_key, version_id, action, actor_id, notes, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
	`
	_, err := r.db.ExecContext(ctx, query, audit.ID, audit.DocumentKey, audit.VersionID, audit.Action, audit.ActorID, audit.Notes, audit.CreatedAt)
	return err
}

func (r *PgDocumentRepository) ListAudits(ctx context.Context, limit int) ([]domain.PublicationAudit, error) {
	query := `
		SELECT id, document_key, version_id, action, actor_id, notes, created_at
		FROM legal_publication_audits
		ORDER BY created_at DESC
		LIMIT $1
	`
	var audits []domain.PublicationAudit
	err := r.db.SelectContext(ctx, &audits, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list audits: %w", err)
	}
	return audits, nil
}

func (r *PgDocumentRepository) scanVersion(ctx context.Context, query string, args ...any) (*domain.DocumentVersion, error) {
	var v domain.DocumentVersion
	var sectionsRaw []byte

	row := r.db.QueryRowContext(ctx, query, args...)
	err := row.Scan(
		&v.ID, &v.DocumentKey, &v.DisplayVersion, &v.SchemaVersion, &v.Title, &v.Summary,
		&sectionsRaw, &v.ContentHash, &v.UpdateClassification, &v.Status,
		&v.PublishedAt, &v.EffectiveAt, &v.SupersededAt, &v.CreatedBy, &v.CreatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.ErrVersionNotFound
		}
		return nil, fmt.Errorf("failed to scan version: %w", err)
	}

	if len(sectionsRaw) > 0 {
		_ = json.Unmarshal(sectionsRaw, &v.Sections)
	}
	return &v, nil
}
