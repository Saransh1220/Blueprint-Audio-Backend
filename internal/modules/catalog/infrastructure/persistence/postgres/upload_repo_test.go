package postgres_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/modules/catalog/domain"
	"github.com/saransh1220/blueprint-audio/internal/modules/catalog/infrastructure/persistence/postgres"
	"github.com/stretchr/testify/require"
)

func TestSpecUploadRepositoryHeartbeatIsFencedByWorker(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()
	repository := postgres.NewSpecUploadRepository(db)
	jobID := uuid.New()

	mock.ExpectExec("UPDATE spec_processing_jobs").
		WithArgs(jobID, "worker-one").
		WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, repository.HeartbeatJob(
		context.Background(), jobID, "worker-one",
	))

	mock.ExpectExec("UPDATE spec_processing_jobs").
		WithArgs(jobID, "stale-worker").
		WillReturnResult(sqlmock.NewResult(0, 0))
	require.ErrorIs(t, repository.HeartbeatJob(
		context.Background(), jobID, "stale-worker",
	), domain.ErrUploadState)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSpecUploadRepositoryCompleteAndFailRejectStaleWorker(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()
	repository := postgres.NewSpecUploadRepository(db)
	jobID := uuid.New()

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT spec_id, session_id[\\s\\S]*worker_id = \\$2").
		WithArgs(jobID, "stale-worker").
		WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()
	require.ErrorIs(t, repository.CompleteJob(
		context.Background(), jobID, "stale-worker", domain.ProcessedSpecFiles{},
	), domain.ErrUploadState)

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT spec_id, session_id[\\s\\S]*worker_id = \\$2").
		WithArgs(jobID, "stale-worker").
		WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()
	require.ErrorIs(t, repository.FailJob(
		context.Background(), jobID, "stale-worker", "failed",
	), domain.ErrUploadState)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSpecUploadRepositoryExpiresAbandonedSessions(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()
	repository := postgres.NewSpecUploadRepository(db)
	cutoff := time.Now().UTC()

	mock.ExpectExec("UPDATE spec_upload_sessions[\\s\\S]*status = 'expired'").
		WithArgs(cutoff).
		WillReturnResult(sqlmock.NewResult(0, 3))
	count, err := repository.ExpireUploadSessions(context.Background(), cutoff)
	require.NoError(t, err)
	require.Equal(t, int64(3), count)
	require.NoError(t, mock.ExpectationsWereMet())
}
