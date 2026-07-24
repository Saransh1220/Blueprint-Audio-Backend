package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/saransh1220/blueprint-audio/internal/modules/catalog/domain"
)

type PgSpecUploadRepository struct {
	db *sqlx.DB
}

func NewSpecUploadRepository(db *sqlx.DB) *PgSpecUploadRepository {
	return &PgSpecUploadRepository{db: db}
}

func (r *PgSpecUploadRepository) CreateSession(ctx context.Context, session *domain.SpecUploadSession) error {
	now := time.Now().UTC()
	if session.CreatedAt.IsZero() {
		session.CreatedAt = now
	}
	session.UpdatedAt = now
	if session.Status == "" {
		session.Status = domain.UploadStatusUploading
	}

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.NamedExecContext(ctx, `
		INSERT INTO spec_upload_sessions (
			id, spec_id, producer_id, metadata, status, error_message,
			expires_at, created_at, updated_at, completed_at
		) VALUES (
			:id, :spec_id, :producer_id, :metadata, :status, :error_message,
			:expires_at, :created_at, :updated_at, :completed_at
		)`, session)
	if err != nil {
		return err
	}

	for i := range session.Assets {
		asset := &session.Assets[i]
		if asset.ID == uuid.Nil {
			asset.ID, err = uuid.NewV7()
			if err != nil {
				return err
			}
		}
		asset.SessionID = session.ID
		if asset.CreatedAt.IsZero() {
			asset.CreatedAt = now
		}
		asset.UpdatedAt = now
		_, err = tx.NamedExecContext(ctx, `
			INSERT INTO spec_upload_assets (
				id, session_id, kind, file_name, object_key, final_object_key,
				declared_content_type, actual_content_type, expected_size,
				actual_size, etag, created_at, updated_at
			) VALUES (
				:id, :session_id, :kind, :file_name, :object_key, :final_object_key,
				:declared_content_type, :actual_content_type, :expected_size,
				:actual_size, :etag, :created_at, :updated_at
			)`, asset)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *PgSpecUploadRepository) GetSession(ctx context.Context, uploadID, producerID uuid.UUID) (*domain.SpecUploadSession, error) {
	session := &domain.SpecUploadSession{}
	err := r.db.GetContext(ctx, session, `
		SELECT id, spec_id, producer_id, metadata, status, error_message,
		       expires_at, created_at, updated_at, completed_at
		FROM spec_upload_sessions
		WHERE id = $1`, uploadID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrUploadNotFound
	}
	if err != nil {
		return nil, err
	}
	if session.ProducerID != producerID {
		return nil, domain.ErrUploadForbidden
	}
	if err := r.db.SelectContext(ctx, &session.Assets, `
		SELECT id, session_id, kind, file_name, object_key, final_object_key,
		       declared_content_type, actual_content_type, expected_size,
		       actual_size, etag, created_at, updated_at
		FROM spec_upload_assets
		WHERE session_id = $1
		ORDER BY kind`, uploadID); err != nil {
		return nil, err
	}
	return session, nil
}

func (r *PgSpecUploadRepository) UpdateSessionMetadata(
	ctx context.Context,
	uploadID, producerID uuid.UUID,
	metadata json.RawMessage,
) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE spec_upload_sessions
		SET metadata = $3, error_message = NULL, updated_at = NOW()
		WHERE id = $1
		  AND producer_id = $2
		  AND status = 'uploading'
		  AND expires_at > NOW()`,
		uploadID, producerID, metadata)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return domain.ErrUploadState
	}
	return nil
}

func (r *PgSpecUploadRepository) ReplaceAsset(
	ctx context.Context,
	uploadID, producerID uuid.UUID,
	asset *domain.SpecUploadAsset,
) (*domain.SpecUploadAsset, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var locked struct {
		ProducerID uuid.UUID           `db:"producer_id"`
		Status     domain.UploadStatus `db:"status"`
		ExpiresAt  time.Time           `db:"expires_at"`
	}
	err = tx.GetContext(ctx, &locked, `
		SELECT producer_id, status, expires_at
		FROM spec_upload_sessions
		WHERE id = $1
		FOR UPDATE`, uploadID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrUploadNotFound
	}
	if err != nil {
		return nil, err
	}
	if locked.ProducerID != producerID {
		return nil, domain.ErrUploadForbidden
	}
	if locked.Status != domain.UploadStatusUploading {
		return nil, domain.ErrUploadState
	}
	if time.Now().UTC().After(locked.ExpiresAt) {
		return nil, domain.ErrUploadExpired
	}

	var previous domain.SpecUploadAsset
	err = tx.GetContext(ctx, &previous, `
		SELECT id, session_id, kind, file_name, object_key, final_object_key,
		       declared_content_type, actual_content_type, expected_size,
		       actual_size, etag, created_at, updated_at
		FROM spec_upload_assets
		WHERE session_id = $1 AND kind = $2
		FOR UPDATE`, uploadID, asset.Kind)
	hasPrevious := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if hasPrevious {
		if _, err := tx.ExecContext(ctx, `
			DELETE FROM spec_upload_assets
			WHERE id = $1 AND session_id = $2`, previous.ID, uploadID); err != nil {
			return nil, err
		}
	}

	now := time.Now().UTC()
	asset.SessionID = uploadID
	if asset.CreatedAt.IsZero() {
		asset.CreatedAt = now
	}
	asset.UpdatedAt = now
	_, err = tx.NamedExecContext(ctx, `
		INSERT INTO spec_upload_assets (
			id, session_id, kind, file_name, object_key, final_object_key,
			declared_content_type, actual_content_type, expected_size,
			actual_size, etag, created_at, updated_at
		) VALUES (
			:id, :session_id, :kind, :file_name, :object_key, :final_object_key,
			:declared_content_type, :actual_content_type, :expected_size,
			:actual_size, :etag, :created_at, :updated_at
		)`, asset)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE spec_upload_sessions
		SET error_message = NULL, updated_at = NOW()
		WHERE id = $1`, uploadID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	if !hasPrevious {
		return nil, nil
	}
	return &previous, nil
}

func (r *PgSpecUploadRepository) VerifyAsset(
	ctx context.Context,
	uploadID, producerID, assetID uuid.UUID,
	actualSize int64,
	actualContentType, etag string,
) error {
	result, err := r.db.ExecContext(ctx, `
		WITH verified AS (
			UPDATE spec_upload_assets asset
			SET actual_size = $4,
			    actual_content_type = $5,
			    etag = $6,
			    updated_at = NOW()
			FROM spec_upload_sessions session
			WHERE asset.id = $3
			  AND asset.session_id = $1
			  AND session.id = asset.session_id
			  AND session.producer_id = $2
			  AND session.status = 'uploading'
			  AND session.expires_at > NOW()
			RETURNING asset.session_id
		)
		UPDATE spec_upload_sessions
		SET error_message = NULL, updated_at = NOW()
		WHERE id IN (SELECT session_id FROM verified)`,
		uploadID, producerID, assetID, actualSize, actualContentType, etag)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return domain.ErrUploadState
	}
	return nil
}

func (r *PgSpecUploadRepository) GetStatus(ctx context.Context, uploadID, producerID uuid.UUID) (*domain.SpecUploadStatus, error) {
	var row struct {
		UploadID         uuid.UUID               `db:"upload_id"`
		SpecID           uuid.UUID               `db:"spec_id"`
		ProducerID       uuid.UUID               `db:"producer_id"`
		Status           domain.UploadStatus     `db:"status"`
		ProcessingStatus domain.ProcessingStatus `db:"processing_status"`
		ErrorMessage     *string                 `db:"error_message"`
		CreatedAt        time.Time               `db:"created_at"`
		UpdatedAt        time.Time               `db:"updated_at"`
		ExpiresAt        time.Time               `db:"expires_at"`
	}
	err := r.db.GetContext(ctx, &row, `
		SELECT us.id AS upload_id, us.spec_id, us.producer_id, us.status,
		       COALESCE(s.processing_status, 'pending') AS processing_status,
		       COALESCE(us.error_message, j.error_message) AS error_message,
		       us.created_at, us.updated_at, us.expires_at
		FROM spec_upload_sessions us
		LEFT JOIN specs s ON s.id = us.spec_id
		LEFT JOIN spec_processing_jobs j ON j.session_id = us.id
		WHERE us.id = $1`, uploadID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrUploadNotFound
	}
	if err != nil {
		return nil, err
	}
	if row.ProducerID != producerID {
		return nil, domain.ErrUploadForbidden
	}
	return &domain.SpecUploadStatus{
		UploadID:         row.UploadID,
		SpecID:           row.SpecID,
		ProducerID:       row.ProducerID,
		Status:           row.Status,
		ProcessingStatus: row.ProcessingStatus,
		ErrorMessage:     row.ErrorMessage,
		CreatedAt:        row.CreatedAt,
		UpdatedAt:        row.UpdatedAt,
		ExpiresAt:        row.ExpiresAt,
	}, nil
}

func (r *PgSpecUploadRepository) MarkSessionFailed(ctx context.Context, uploadID uuid.UUID, reason string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE spec_upload_sessions
		SET status = 'failed', error_message = $2, updated_at = NOW()
		WHERE id = $1 AND status = 'uploading'`, uploadID, reason)
	return err
}

func (r *PgSpecUploadRepository) FinalizeUpload(
	ctx context.Context,
	session *domain.SpecUploadSession,
	spec *domain.Spec,
	job *domain.SpecProcessingJob,
) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var locked struct {
		ProducerID uuid.UUID           `db:"producer_id"`
		SpecID     uuid.UUID           `db:"spec_id"`
		Status     domain.UploadStatus `db:"status"`
		ExpiresAt  time.Time           `db:"expires_at"`
	}
	err = tx.GetContext(ctx, &locked, `
		SELECT producer_id, spec_id, status, expires_at
		FROM spec_upload_sessions
		WHERE id = $1
		FOR UPDATE`, session.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ErrUploadNotFound
	}
	if err != nil {
		return err
	}
	if locked.ProducerID != session.ProducerID {
		return domain.ErrUploadForbidden
	}
	if locked.SpecID != spec.ID || locked.Status != domain.UploadStatusUploading {
		return domain.ErrUploadState
	}
	if time.Now().UTC().After(locked.ExpiresAt) {
		return domain.ErrUploadExpired
	}

	for _, asset := range session.Assets {
		if asset.ActualSize == nil {
			return fmt.Errorf("%w: asset %s was not verified", domain.ErrInvalidUpload, asset.Kind)
		}
		result, err := tx.ExecContext(ctx, `
			UPDATE spec_upload_assets
			SET actual_size = $3, actual_content_type = $4, etag = $5, updated_at = NOW()
			WHERE id = $1 AND session_id = $2`,
			asset.ID, session.ID, *asset.ActualSize, asset.ActualContentType, asset.ETag)
		if err != nil {
			return err
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if rows != 1 {
			return domain.ErrUploadState
		}
	}

	if err := createSpecTx(ctx, tx, spec); err != nil {
		return err
	}

	now := time.Now().UTC()
	if job.ID == uuid.Nil {
		job.ID, err = uuid.NewV7()
		if err != nil {
			return err
		}
	}
	job.SpecID = spec.ID
	job.SessionID = session.ID
	job.Status = domain.ProcessingJobQueued
	job.CreatedAt = now
	job.UpdatedAt = now
	_, err = tx.NamedExecContext(ctx, `
		INSERT INTO spec_processing_jobs (
			id, spec_id, session_id, status, worker_id, locked_at, started_at,
			completed_at, error_message, created_at, updated_at
		) VALUES (
			:id, :spec_id, :session_id, :status, :worker_id, :locked_at, :started_at,
			:completed_at, :error_message, :created_at, :updated_at
		)`, job)
	if err != nil {
		return err
	}

	result, err := tx.ExecContext(ctx, `
		UPDATE spec_upload_sessions
		SET status = 'queued', error_message = NULL, updated_at = NOW()
		WHERE id = $1 AND status = 'uploading'`, session.ID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return domain.ErrUploadState
	}
	return tx.Commit()
}

func (r *PgSpecUploadRepository) ExpireUploadSessions(
	ctx context.Context,
	expiredBefore time.Time,
) (int64, error) {
	result, err := r.db.ExecContext(ctx, `
		UPDATE spec_upload_sessions
		SET status = 'expired',
		    error_message = 'upload session expired',
		    completed_at = NOW(),
		    updated_at = NOW()
		WHERE status = 'uploading' AND expires_at < $1`, expiredBefore)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (r *PgSpecUploadRepository) RequeueStaleJobs(ctx context.Context, staleBefore time.Time) (int64, error) {
	result, err := r.db.ExecContext(ctx, `
		WITH stale AS (
			UPDATE spec_processing_jobs
			SET status = 'queued', worker_id = NULL, locked_at = NULL, updated_at = NOW()
			WHERE status = 'processing' AND locked_at < $1
			RETURNING session_id
		)
		UPDATE spec_upload_sessions
		SET status = 'queued', updated_at = NOW()
		WHERE id IN (SELECT session_id FROM stale)`, staleBefore)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (r *PgSpecUploadRepository) ClaimNextJob(ctx context.Context, workerID string) (*domain.ProcessingBundle, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	job := domain.SpecProcessingJob{}
	err = tx.GetContext(ctx, &job, `
		WITH candidate AS (
			SELECT id
			FROM spec_processing_jobs
			WHERE status = 'queued'
			ORDER BY created_at
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		UPDATE spec_processing_jobs job
		SET status = 'processing',
		    worker_id = $1,
		    locked_at = NOW(),
		    started_at = COALESCE(job.started_at, NOW()),
		    updated_at = NOW(),
		    error_message = NULL
		FROM candidate
		WHERE job.id = candidate.id
		RETURNING job.*`, workerID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNoProcessingJob
	}
	if err != nil {
		return nil, err
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE spec_upload_sessions
		SET status = 'processing', updated_at = NOW()
		WHERE id = $1`, job.SessionID); err != nil {
		return nil, err
	}

	bundle := &domain.ProcessingBundle{Job: job}
	if err := tx.GetContext(ctx, &bundle.Session, `
		SELECT id, spec_id, producer_id, metadata, status, error_message,
		       expires_at, created_at, updated_at, completed_at
		FROM spec_upload_sessions WHERE id = $1`, job.SessionID); err != nil {
		return nil, err
	}
	if err := tx.SelectContext(ctx, &bundle.Assets, `
		SELECT id, session_id, kind, file_name, object_key, final_object_key,
		       declared_content_type, actual_content_type, expected_size,
		       actual_size, etag, created_at, updated_at
		FROM spec_upload_assets
		WHERE session_id = $1
		ORDER BY kind`, job.SessionID); err != nil {
		return nil, err
	}
	if err := tx.GetContext(ctx, &bundle.Spec, `SELECT * FROM specs WHERE id = $1`, job.SpecID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return bundle, nil
}

func (r *PgSpecUploadRepository) HeartbeatJob(ctx context.Context, jobID uuid.UUID, workerID string) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE spec_processing_jobs
		SET locked_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND status = 'processing' AND worker_id = $2`,
		jobID, workerID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return domain.ErrUploadState
	}
	return nil
}

func (r *PgSpecUploadRepository) CompleteJob(
	ctx context.Context,
	jobID uuid.UUID,
	workerID string,
	result domain.ProcessedSpecFiles,
) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var ids struct {
		SpecID    uuid.UUID `db:"spec_id"`
		SessionID uuid.UUID `db:"session_id"`
	}
	err = tx.GetContext(ctx, &ids, `
		SELECT spec_id, session_id
		FROM spec_processing_jobs
		WHERE id = $1 AND status = 'processing' AND worker_id = $2
		FOR UPDATE`, jobID, workerID)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ErrUploadState
	}
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE specs
		SET image_url = $2,
		    preview_url = $3,
		    wav_url = $4,
		    stems_url = $5,
		    duration = $6,
		    waveform_peaks = $7,
		    processing_status = 'completed',
		    updated_at = NOW()
		WHERE id = $1`,
		ids.SpecID, result.ImageURL, result.PreviewURL, result.WAVURL,
		result.StemsURL, result.Duration, pq.Array(result.WaveformPeaks))
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		UPDATE spec_processing_jobs
		SET status = 'completed', completed_at = NOW(), locked_at = NULL,
		    error_message = NULL, updated_at = NOW()
		WHERE id = $1`, jobID)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		UPDATE spec_upload_sessions
		SET status = 'completed', completed_at = NOW(), error_message = NULL, updated_at = NOW()
		WHERE id = $1`, ids.SessionID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (r *PgSpecUploadRepository) FailJob(
	ctx context.Context,
	jobID uuid.UUID,
	workerID, reason string,
) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var ids struct {
		SpecID    uuid.UUID `db:"spec_id"`
		SessionID uuid.UUID `db:"session_id"`
	}
	err = tx.GetContext(ctx, &ids, `
		SELECT spec_id, session_id
		FROM spec_processing_jobs
		WHERE id = $1 AND status = 'processing' AND worker_id = $2
		FOR UPDATE`, jobID, workerID)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ErrUploadState
	}
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE specs
		SET processing_status = 'failed', updated_at = NOW()
		WHERE id = $1`, ids.SpecID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE spec_processing_jobs
		SET status = 'failed', completed_at = NOW(), locked_at = NULL,
		    error_message = $2, updated_at = NOW()
		WHERE id = $1`, jobID, reason); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE spec_upload_sessions
		SET status = 'failed', completed_at = NOW(), error_message = $2, updated_at = NOW()
		WHERE id = $1`, ids.SessionID, reason); err != nil {
		return err
	}
	return tx.Commit()
}

var _ domain.SpecUploadRepository = (*PgSpecUploadRepository)(nil)
