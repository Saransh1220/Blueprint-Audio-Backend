package postgres_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/saransh1220/blueprint-audio/internal/modules/catalog/domain"
	"github.com/saransh1220/blueprint-audio/internal/modules/catalog/infrastructure/persistence/postgres"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errUploadBoom = errors.New("boom")

func newUploadRepo(t *testing.T) (*postgres.PgSpecUploadRepository, sqlmock.Sqlmock, func()) {
	t.Helper()
	db, mock, cleanup := newMockDB(t)
	return postgres.NewSpecUploadRepository(db), mock, cleanup
}

func uploadSessionRows(id, specID, producerID uuid.UUID) *sqlmock.Rows {
	now := time.Now()
	return sqlmock.NewRows([]string{
		"id", "spec_id", "producer_id", "metadata", "status", "error_message",
		"expires_at", "created_at", "updated_at", "completed_at",
	}).AddRow(id, specID, producerID, []byte(`{"title":"x"}`), "uploading", nil,
		now.Add(time.Hour), now, now, nil)
}

var uploadAssetCols = []string{
	"id", "session_id", "kind", "file_name", "object_key", "final_object_key",
	"declared_content_type", "actual_content_type", "expected_size",
	"actual_size", "etag", "created_at", "updated_at",
}

func uploadAssetRow(rows *sqlmock.Rows, id, sessionID uuid.UUID, kind string) *sqlmock.Rows {
	now := time.Now()
	return rows.AddRow(id, sessionID, kind, kind+".bin", "tmp/"+kind, "final/"+kind,
		"audio/mpeg", nil, int64(10), nil, nil, now, now)
}

func TestUploadRepoCreateSession(t *testing.T) {
	ctx := context.Background()

	t.Run("success fills defaults for session and assets", func(t *testing.T) {
		repo, mock, cleanup := newUploadRepo(t)
		defer cleanup()
		session := &domain.SpecUploadSession{
			ID: uuid.New(), SpecID: uuid.New(), ProducerID: uuid.New(),
			Metadata:  json.RawMessage(`{}`),
			ExpiresAt: time.Now().Add(time.Hour),
			Assets: []domain.SpecUploadAsset{
				{Kind: "audio"},
				{ID: uuid.New(), Kind: "image", CreatedAt: time.Unix(100, 0)},
			},
		}
		mock.ExpectBegin()
		mock.ExpectExec(`INSERT INTO spec_upload_sessions`).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(`INSERT INTO spec_upload_assets`).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(`INSERT INTO spec_upload_assets`).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()

		require.NoError(t, repo.CreateSession(ctx, session))
		assert.Equal(t, domain.UploadStatusUploading, session.Status)
		assert.False(t, session.CreatedAt.IsZero())
		assert.NotEqual(t, uuid.Nil, session.Assets[0].ID)
		assert.Equal(t, session.ID, session.Assets[0].SessionID)
		assert.Equal(t, session.ID, session.Assets[1].SessionID)
		assert.Equal(t, time.Unix(100, 0), session.Assets[1].CreatedAt)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("begin error", func(t *testing.T) {
		repo, mock, cleanup := newUploadRepo(t)
		defer cleanup()
		mock.ExpectBegin().WillReturnError(errUploadBoom)
		require.ErrorIs(t, repo.CreateSession(ctx, &domain.SpecUploadSession{}), errUploadBoom)
	})

	t.Run("session insert error rolls back", func(t *testing.T) {
		repo, mock, cleanup := newUploadRepo(t)
		defer cleanup()
		mock.ExpectBegin()
		mock.ExpectExec(`INSERT INTO spec_upload_sessions`).WillReturnError(errUploadBoom)
		mock.ExpectRollback()
		require.ErrorIs(t, repo.CreateSession(ctx, &domain.SpecUploadSession{ID: uuid.New()}), errUploadBoom)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("asset insert error rolls back", func(t *testing.T) {
		repo, mock, cleanup := newUploadRepo(t)
		defer cleanup()
		mock.ExpectBegin()
		mock.ExpectExec(`INSERT INTO spec_upload_sessions`).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(`INSERT INTO spec_upload_assets`).WillReturnError(errUploadBoom)
		mock.ExpectRollback()
		session := &domain.SpecUploadSession{ID: uuid.New(), Assets: []domain.SpecUploadAsset{{Kind: "audio"}}}
		require.ErrorIs(t, repo.CreateSession(ctx, session), errUploadBoom)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("commit error", func(t *testing.T) {
		repo, mock, cleanup := newUploadRepo(t)
		defer cleanup()
		mock.ExpectBegin()
		mock.ExpectExec(`INSERT INTO spec_upload_sessions`).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit().WillReturnError(errUploadBoom)
		require.ErrorIs(t, repo.CreateSession(ctx, &domain.SpecUploadSession{ID: uuid.New()}), errUploadBoom)
	})
}

func TestUploadRepoGetSession(t *testing.T) {
	ctx := context.Background()
	uploadID, specID, producerID := uuid.New(), uuid.New(), uuid.New()
	sessionQuery := `FROM spec_upload_sessions\s+WHERE id = \$1`
	assetQuery := `FROM spec_upload_assets\s+WHERE session_id = \$1\s+ORDER BY kind`

	t.Run("success loads assets", func(t *testing.T) {
		repo, mock, cleanup := newUploadRepo(t)
		defer cleanup()
		mock.ExpectQuery(sessionQuery).WithArgs(uploadID).
			WillReturnRows(uploadSessionRows(uploadID, specID, producerID))
		assets := sqlmock.NewRows(uploadAssetCols)
		uploadAssetRow(assets, uuid.New(), uploadID, "audio")
		uploadAssetRow(assets, uuid.New(), uploadID, "image")
		mock.ExpectQuery(assetQuery).WithArgs(uploadID).WillReturnRows(assets)

		got, err := repo.GetSession(ctx, uploadID, producerID)
		require.NoError(t, err)
		assert.Equal(t, specID, got.SpecID)
		assert.Equal(t, domain.UploadStatusUploading, got.Status)
		require.Len(t, got.Assets, 2)
		assert.Equal(t, domain.UploadAssetKind("audio"), got.Assets[0].Kind)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("not found", func(t *testing.T) {
		repo, mock, cleanup := newUploadRepo(t)
		defer cleanup()
		mock.ExpectQuery(sessionQuery).WithArgs(uploadID).WillReturnError(sql.ErrNoRows)
		_, err := repo.GetSession(ctx, uploadID, producerID)
		require.ErrorIs(t, err, domain.ErrUploadNotFound)
	})

	t.Run("db error", func(t *testing.T) {
		repo, mock, cleanup := newUploadRepo(t)
		defer cleanup()
		mock.ExpectQuery(sessionQuery).WithArgs(uploadID).WillReturnError(errUploadBoom)
		_, err := repo.GetSession(ctx, uploadID, producerID)
		require.ErrorIs(t, err, errUploadBoom)
	})

	t.Run("forbidden for other producer", func(t *testing.T) {
		repo, mock, cleanup := newUploadRepo(t)
		defer cleanup()
		mock.ExpectQuery(sessionQuery).WithArgs(uploadID).
			WillReturnRows(uploadSessionRows(uploadID, specID, uuid.New()))
		_, err := repo.GetSession(ctx, uploadID, producerID)
		require.ErrorIs(t, err, domain.ErrUploadForbidden)
	})

	t.Run("asset query error", func(t *testing.T) {
		repo, mock, cleanup := newUploadRepo(t)
		defer cleanup()
		mock.ExpectQuery(sessionQuery).WithArgs(uploadID).
			WillReturnRows(uploadSessionRows(uploadID, specID, producerID))
		mock.ExpectQuery(assetQuery).WithArgs(uploadID).WillReturnError(errUploadBoom)
		_, err := repo.GetSession(ctx, uploadID, producerID)
		require.ErrorIs(t, err, errUploadBoom)
	})
}

func TestUploadRepoUpdateSessionMetadata(t *testing.T) {
	ctx := context.Background()
	uploadID, producerID := uuid.New(), uuid.New()
	meta := json.RawMessage(`{"a":1}`)
	q := `UPDATE spec_upload_sessions\s+SET metadata = \$3`

	t.Run("success", func(t *testing.T) {
		repo, mock, cleanup := newUploadRepo(t)
		defer cleanup()
		mock.ExpectExec(q).WithArgs(uploadID, producerID, meta).WillReturnResult(sqlmock.NewResult(0, 1))
		require.NoError(t, repo.UpdateSessionMetadata(ctx, uploadID, producerID, meta))
		require.NoError(t, mock.ExpectationsWereMet())
	})
	t.Run("no row matched", func(t *testing.T) {
		repo, mock, cleanup := newUploadRepo(t)
		defer cleanup()
		mock.ExpectExec(q).WillReturnResult(sqlmock.NewResult(0, 0))
		require.ErrorIs(t, repo.UpdateSessionMetadata(ctx, uploadID, producerID, meta), domain.ErrUploadState)
	})
	t.Run("exec error", func(t *testing.T) {
		repo, mock, cleanup := newUploadRepo(t)
		defer cleanup()
		mock.ExpectExec(q).WillReturnError(errUploadBoom)
		require.ErrorIs(t, repo.UpdateSessionMetadata(ctx, uploadID, producerID, meta), errUploadBoom)
	})
	t.Run("rows affected error", func(t *testing.T) {
		repo, mock, cleanup := newUploadRepo(t)
		defer cleanup()
		mock.ExpectExec(q).WillReturnResult(sqlmock.NewErrorResult(errUploadBoom))
		require.ErrorIs(t, repo.UpdateSessionMetadata(ctx, uploadID, producerID, meta), errUploadBoom)
	})
}

func TestUploadRepoReplaceAsset(t *testing.T) {
	ctx := context.Background()
	uploadID, producerID := uuid.New(), uuid.New()
	lockQ := `SELECT producer_id, status, expires_at\s+FROM spec_upload_sessions`
	prevQ := `FROM spec_upload_assets\s+WHERE session_id = \$1 AND kind = \$2\s+FOR UPDATE`
	lockRows := func(owner uuid.UUID, status string, exp time.Time) *sqlmock.Rows {
		return sqlmock.NewRows([]string{"producer_id", "status", "expires_at"}).AddRow(owner, status, exp)
	}
	newAsset := func() *domain.SpecUploadAsset {
		return &domain.SpecUploadAsset{ID: uuid.New(), Kind: "audio", FileName: "a.mp3"}
	}
	future := time.Now().Add(time.Hour)

	t.Run("replaces previous asset", func(t *testing.T) {
		repo, mock, cleanup := newUploadRepo(t)
		defer cleanup()
		prevID := uuid.New()
		mock.ExpectBegin()
		mock.ExpectQuery(lockQ).WithArgs(uploadID).WillReturnRows(lockRows(producerID, "uploading", future))
		mock.ExpectQuery(prevQ).WithArgs(uploadID, domain.UploadAssetKind("audio")).
			WillReturnRows(uploadAssetRow(sqlmock.NewRows(uploadAssetCols), prevID, uploadID, "audio"))
		mock.ExpectExec(`DELETE FROM spec_upload_assets`).WithArgs(prevID, uploadID).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(`INSERT INTO spec_upload_assets`).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(`UPDATE spec_upload_sessions\s+SET error_message = NULL`).WithArgs(uploadID).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()

		asset := newAsset()
		prev, err := repo.ReplaceAsset(ctx, uploadID, producerID, asset)
		require.NoError(t, err)
		require.NotNil(t, prev)
		assert.Equal(t, prevID, prev.ID)
		assert.Equal(t, uploadID, asset.SessionID)
		assert.False(t, asset.CreatedAt.IsZero())
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("no previous asset returns nil", func(t *testing.T) {
		repo, mock, cleanup := newUploadRepo(t)
		defer cleanup()
		mock.ExpectBegin()
		mock.ExpectQuery(lockQ).WillReturnRows(lockRows(producerID, "uploading", future))
		mock.ExpectQuery(prevQ).WillReturnError(sql.ErrNoRows)
		mock.ExpectExec(`INSERT INTO spec_upload_assets`).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(`UPDATE spec_upload_sessions`).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()
		prev, err := repo.ReplaceAsset(ctx, uploadID, producerID, newAsset())
		require.NoError(t, err)
		assert.Nil(t, prev)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("begin error", func(t *testing.T) {
		repo, mock, cleanup := newUploadRepo(t)
		defer cleanup()
		mock.ExpectBegin().WillReturnError(errUploadBoom)
		_, err := repo.ReplaceAsset(ctx, uploadID, producerID, newAsset())
		require.ErrorIs(t, err, errUploadBoom)
	})

	guardCases := []struct {
		name string
		rows func() *sqlmock.Rows
		err  error
		want error
	}{
		{name: "session not found", err: sql.ErrNoRows, want: domain.ErrUploadNotFound},
		{name: "lock db error", err: errUploadBoom, want: errUploadBoom},
		{name: "forbidden", rows: func() *sqlmock.Rows { return lockRows(uuid.New(), "uploading", future) }, want: domain.ErrUploadForbidden},
		{name: "wrong status", rows: func() *sqlmock.Rows { return lockRows(producerID, "queued", future) }, want: domain.ErrUploadState},
		{name: "expired", rows: func() *sqlmock.Rows { return lockRows(producerID, "uploading", time.Now().Add(-time.Hour)) }, want: domain.ErrUploadExpired},
	}
	for _, tc := range guardCases {
		t.Run(tc.name, func(t *testing.T) {
			repo, mock, cleanup := newUploadRepo(t)
			defer cleanup()
			mock.ExpectBegin()
			q := mock.ExpectQuery(lockQ)
			if tc.err != nil {
				q.WillReturnError(tc.err)
			} else {
				q.WillReturnRows(tc.rows())
			}
			mock.ExpectRollback()
			_, err := repo.ReplaceAsset(ctx, uploadID, producerID, newAsset())
			require.ErrorIs(t, err, tc.want)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}

	// Failure at each later step.
	steps := []struct {
		name  string
		setup func(mock sqlmock.Sqlmock)
	}{
		{"previous lookup error", func(m sqlmock.Sqlmock) {
			m.ExpectQuery(prevQ).WillReturnError(errUploadBoom)
		}},
		{"delete previous error", func(m sqlmock.Sqlmock) {
			m.ExpectQuery(prevQ).WillReturnRows(uploadAssetRow(sqlmock.NewRows(uploadAssetCols), uuid.New(), uploadID, "audio"))
			m.ExpectExec(`DELETE FROM spec_upload_assets`).WillReturnError(errUploadBoom)
		}},
		{"insert error", func(m sqlmock.Sqlmock) {
			m.ExpectQuery(prevQ).WillReturnError(sql.ErrNoRows)
			m.ExpectExec(`INSERT INTO spec_upload_assets`).WillReturnError(errUploadBoom)
		}},
		{"session touch error", func(m sqlmock.Sqlmock) {
			m.ExpectQuery(prevQ).WillReturnError(sql.ErrNoRows)
			m.ExpectExec(`INSERT INTO spec_upload_assets`).WillReturnResult(sqlmock.NewResult(0, 1))
			m.ExpectExec(`UPDATE spec_upload_sessions`).WillReturnError(errUploadBoom)
		}},
		{"commit error", func(m sqlmock.Sqlmock) {
			m.ExpectQuery(prevQ).WillReturnError(sql.ErrNoRows)
			m.ExpectExec(`INSERT INTO spec_upload_assets`).WillReturnResult(sqlmock.NewResult(0, 1))
			m.ExpectExec(`UPDATE spec_upload_sessions`).WillReturnResult(sqlmock.NewResult(0, 1))
			m.ExpectCommit().WillReturnError(errUploadBoom)
		}},
	}
	for _, st := range steps {
		t.Run(st.name, func(t *testing.T) {
			repo, mock, cleanup := newUploadRepo(t)
			defer cleanup()
			mock.ExpectBegin()
			mock.ExpectQuery(lockQ).WillReturnRows(lockRows(producerID, "uploading", future))
			st.setup(mock)
			_, err := repo.ReplaceAsset(ctx, uploadID, producerID, newAsset())
			require.ErrorIs(t, err, errUploadBoom)
		})
	}
}

func TestUploadRepoVerifyAsset(t *testing.T) {
	ctx := context.Background()
	uploadID, producerID, assetID := uuid.New(), uuid.New(), uuid.New()
	q := `WITH verified AS`

	t.Run("success passes all arguments", func(t *testing.T) {
		repo, mock, cleanup := newUploadRepo(t)
		defer cleanup()
		mock.ExpectExec(q).WithArgs(uploadID, producerID, assetID, int64(42), "audio/mpeg", "etag1").
			WillReturnResult(sqlmock.NewResult(0, 1))
		require.NoError(t, repo.VerifyAsset(ctx, uploadID, producerID, assetID, 42, "audio/mpeg", "etag1"))
		require.NoError(t, mock.ExpectationsWereMet())
	})
	t.Run("no row", func(t *testing.T) {
		repo, mock, cleanup := newUploadRepo(t)
		defer cleanup()
		mock.ExpectExec(q).WillReturnResult(sqlmock.NewResult(0, 0))
		require.ErrorIs(t, repo.VerifyAsset(ctx, uploadID, producerID, assetID, 1, "x", "y"), domain.ErrUploadState)
	})
	t.Run("exec error", func(t *testing.T) {
		repo, mock, cleanup := newUploadRepo(t)
		defer cleanup()
		mock.ExpectExec(q).WillReturnError(errUploadBoom)
		require.ErrorIs(t, repo.VerifyAsset(ctx, uploadID, producerID, assetID, 1, "x", "y"), errUploadBoom)
	})
	t.Run("rows affected error", func(t *testing.T) {
		repo, mock, cleanup := newUploadRepo(t)
		defer cleanup()
		mock.ExpectExec(q).WillReturnResult(sqlmock.NewErrorResult(errUploadBoom))
		require.ErrorIs(t, repo.VerifyAsset(ctx, uploadID, producerID, assetID, 1, "x", "y"), errUploadBoom)
	})
}

func TestUploadRepoGetStatus(t *testing.T) {
	ctx := context.Background()
	uploadID, specID, producerID := uuid.New(), uuid.New(), uuid.New()
	q := `SELECT us.id AS upload_id`
	cols := []string{"upload_id", "spec_id", "producer_id", "status", "processing_status",
		"error_message", "created_at", "updated_at", "expires_at"}
	now := time.Now()

	t.Run("success", func(t *testing.T) {
		repo, mock, cleanup := newUploadRepo(t)
		defer cleanup()
		mock.ExpectQuery(q).WithArgs(uploadID).WillReturnRows(
			sqlmock.NewRows(cols).AddRow(uploadID, specID, producerID, "processing", "pending", "oops", now, now, now))
		got, err := repo.GetStatus(ctx, uploadID, producerID)
		require.NoError(t, err)
		assert.Equal(t, uploadID, got.UploadID)
		assert.Equal(t, specID, got.SpecID)
		assert.Equal(t, domain.UploadStatus("processing"), got.Status)
		assert.Equal(t, domain.ProcessingStatus("pending"), got.ProcessingStatus)
		require.NotNil(t, got.ErrorMessage)
		assert.Equal(t, "oops", *got.ErrorMessage)
	})
	t.Run("not found", func(t *testing.T) {
		repo, mock, cleanup := newUploadRepo(t)
		defer cleanup()
		mock.ExpectQuery(q).WillReturnError(sql.ErrNoRows)
		_, err := repo.GetStatus(ctx, uploadID, producerID)
		require.ErrorIs(t, err, domain.ErrUploadNotFound)
	})
	t.Run("db error", func(t *testing.T) {
		repo, mock, cleanup := newUploadRepo(t)
		defer cleanup()
		mock.ExpectQuery(q).WillReturnError(errUploadBoom)
		_, err := repo.GetStatus(ctx, uploadID, producerID)
		require.ErrorIs(t, err, errUploadBoom)
	})
	t.Run("forbidden", func(t *testing.T) {
		repo, mock, cleanup := newUploadRepo(t)
		defer cleanup()
		mock.ExpectQuery(q).WillReturnRows(
			sqlmock.NewRows(cols).AddRow(uploadID, specID, uuid.New(), "uploading", "pending", nil, now, now, now))
		_, err := repo.GetStatus(ctx, uploadID, producerID)
		require.ErrorIs(t, err, domain.ErrUploadForbidden)
	})
}

func TestUploadRepoMarkSessionFailed(t *testing.T) {
	ctx := context.Background()
	uploadID := uuid.New()
	repo, mock, cleanup := newUploadRepo(t)
	defer cleanup()
	mock.ExpectExec(`SET status = 'failed', error_message = \$2`).WithArgs(uploadID, "bad").
		WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, repo.MarkSessionFailed(ctx, uploadID, "bad"))
	mock.ExpectExec(`SET status = 'failed'`).WillReturnError(errUploadBoom)
	require.ErrorIs(t, repo.MarkSessionFailed(ctx, uploadID, "bad"), errUploadBoom)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUploadRepoExpireAndRequeue(t *testing.T) {
	ctx := context.Background()
	cutoff := time.Now()

	t.Run("expire counts rows", func(t *testing.T) {
		repo, mock, cleanup := newUploadRepo(t)
		defer cleanup()
		mock.ExpectExec(`SET status = 'expired'`).WithArgs(cutoff).WillReturnResult(sqlmock.NewResult(0, 3))
		n, err := repo.ExpireUploadSessions(ctx, cutoff)
		require.NoError(t, err)
		assert.EqualValues(t, 3, n)
	})
	t.Run("expire rows affected error", func(t *testing.T) {
		repo, mock, cleanup := newUploadRepo(t)
		defer cleanup()
		mock.ExpectExec(`SET status = 'expired'`).WillReturnResult(sqlmock.NewErrorResult(errUploadBoom))
		_, err := repo.ExpireUploadSessions(ctx, cutoff)
		require.ErrorIs(t, err, errUploadBoom)
	})
	t.Run("requeue counts rows", func(t *testing.T) {
		repo, mock, cleanup := newUploadRepo(t)
		defer cleanup()
		mock.ExpectExec(`WITH stale AS`).WithArgs(cutoff).WillReturnResult(sqlmock.NewResult(0, 2))
		n, err := repo.RequeueStaleJobs(ctx, cutoff)
		require.NoError(t, err)
		assert.EqualValues(t, 2, n)
	})
	t.Run("requeue exec error", func(t *testing.T) {
		repo, mock, cleanup := newUploadRepo(t)
		defer cleanup()
		mock.ExpectExec(`WITH stale AS`).WillReturnError(errUploadBoom)
		n, err := repo.RequeueStaleJobs(ctx, cutoff)
		require.ErrorIs(t, err, errUploadBoom)
		assert.Zero(t, n)
	})
}

func TestUploadRepoFinalizeUpload(t *testing.T) {
	ctx := context.Background()
	producerID, specID, sessionID := uuid.New(), uuid.New(), uuid.New()
	lockQ := `SELECT producer_id, spec_id, status, expires_at`
	future := time.Now().Add(time.Hour)
	lockRows := func(owner, spec uuid.UUID, status string, exp time.Time) *sqlmock.Rows {
		return sqlmock.NewRows([]string{"producer_id", "spec_id", "status", "expires_at"}).
			AddRow(owner, spec, status, exp)
	}
	size := int64(99)
	ctype, etag := "audio/mpeg", "e"
	mk := func() (*domain.SpecUploadSession, *domain.Spec, *domain.SpecProcessingJob) {
		return &domain.SpecUploadSession{
				ID: sessionID, ProducerID: producerID,
				Assets: []domain.SpecUploadAsset{{ID: uuid.New(), Kind: "audio", ActualSize: &size, ActualContentType: &ctype, ETag: &etag}},
			},
			&domain.Spec{ID: specID, ProducerID: producerID, Title: "t"},
			&domain.SpecProcessingJob{}
	}

	t.Run("success", func(t *testing.T) {
		repo, mock, cleanup := newUploadRepo(t)
		defer cleanup()
		session, spec, job := mk()
		mock.ExpectBegin()
		mock.ExpectQuery(lockQ).WithArgs(sessionID).WillReturnRows(lockRows(producerID, specID, "uploading", future))
		mock.ExpectExec(`UPDATE spec_upload_assets`).
			WithArgs(session.Assets[0].ID, sessionID, size, &ctype, &etag).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(`INSERT INTO specs`).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(`INSERT INTO spec_analytics`).WithArgs(specID).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(`INSERT INTO spec_processing_jobs`).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(`SET status = 'queued'`).WithArgs(sessionID).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()

		require.NoError(t, repo.FinalizeUpload(ctx, session, spec, job))
		assert.NotEqual(t, uuid.Nil, job.ID)
		assert.Equal(t, specID, job.SpecID)
		assert.Equal(t, sessionID, job.SessionID)
		assert.Equal(t, domain.ProcessingJobQueued, job.Status)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("begin error", func(t *testing.T) {
		repo, mock, cleanup := newUploadRepo(t)
		defer cleanup()
		s, sp, j := mk()
		mock.ExpectBegin().WillReturnError(errUploadBoom)
		require.ErrorIs(t, repo.FinalizeUpload(ctx, s, sp, j), errUploadBoom)
	})

	guards := []struct {
		name string
		err  error
		rows func() *sqlmock.Rows
		want error
	}{
		{name: "session not found", err: sql.ErrNoRows, want: domain.ErrUploadNotFound},
		{name: "lock error", err: errUploadBoom, want: errUploadBoom},
		{name: "forbidden", rows: func() *sqlmock.Rows { return lockRows(uuid.New(), specID, "uploading", future) }, want: domain.ErrUploadForbidden},
		{name: "spec mismatch", rows: func() *sqlmock.Rows { return lockRows(producerID, uuid.New(), "uploading", future) }, want: domain.ErrUploadState},
		{name: "status not uploading", rows: func() *sqlmock.Rows { return lockRows(producerID, specID, "queued", future) }, want: domain.ErrUploadState},
		{name: "expired", rows: func() *sqlmock.Rows {
			return lockRows(producerID, specID, "uploading", time.Now().Add(-time.Minute))
		}, want: domain.ErrUploadExpired},
	}
	for _, tc := range guards {
		t.Run(tc.name, func(t *testing.T) {
			repo, mock, cleanup := newUploadRepo(t)
			defer cleanup()
			s, sp, j := mk()
			mock.ExpectBegin()
			q := mock.ExpectQuery(lockQ)
			if tc.err != nil {
				q.WillReturnError(tc.err)
			} else {
				q.WillReturnRows(tc.rows())
			}
			mock.ExpectRollback()
			require.ErrorIs(t, repo.FinalizeUpload(ctx, s, sp, j), tc.want)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}

	t.Run("unverified asset", func(t *testing.T) {
		repo, mock, cleanup := newUploadRepo(t)
		defer cleanup()
		s, sp, j := mk()
		s.Assets[0].ActualSize = nil
		mock.ExpectBegin()
		mock.ExpectQuery(lockQ).WillReturnRows(lockRows(producerID, specID, "uploading", future))
		mock.ExpectRollback()
		err := repo.FinalizeUpload(ctx, s, sp, j)
		require.ErrorIs(t, err, domain.ErrInvalidUpload)
		assert.Contains(t, err.Error(), "was not verified")
	})

	later := []struct {
		name  string
		setup func(m sqlmock.Sqlmock)
		want  error
	}{
		{"asset update error", func(m sqlmock.Sqlmock) {
			m.ExpectExec(`UPDATE spec_upload_assets`).WillReturnError(errUploadBoom)
		}, errUploadBoom},
		{"asset rows affected error", func(m sqlmock.Sqlmock) {
			m.ExpectExec(`UPDATE spec_upload_assets`).WillReturnResult(sqlmock.NewErrorResult(errUploadBoom))
		}, errUploadBoom},
		{"asset not matched", func(m sqlmock.Sqlmock) {
			m.ExpectExec(`UPDATE spec_upload_assets`).WillReturnResult(sqlmock.NewResult(0, 0))
		}, domain.ErrUploadState},
		{"create spec error", func(m sqlmock.Sqlmock) {
			m.ExpectExec(`UPDATE spec_upload_assets`).WillReturnResult(sqlmock.NewResult(0, 1))
			m.ExpectExec(`INSERT INTO specs`).WillReturnError(errUploadBoom)
		}, errUploadBoom},
		{"job insert error", func(m sqlmock.Sqlmock) {
			m.ExpectExec(`UPDATE spec_upload_assets`).WillReturnResult(sqlmock.NewResult(0, 1))
			m.ExpectExec(`INSERT INTO specs`).WillReturnResult(sqlmock.NewResult(0, 1))
			m.ExpectExec(`INSERT INTO spec_analytics`).WillReturnResult(sqlmock.NewResult(0, 1))
			m.ExpectExec(`INSERT INTO spec_processing_jobs`).WillReturnError(errUploadBoom)
		}, errUploadBoom},
		{"queue update error", func(m sqlmock.Sqlmock) {
			m.ExpectExec(`UPDATE spec_upload_assets`).WillReturnResult(sqlmock.NewResult(0, 1))
			m.ExpectExec(`INSERT INTO specs`).WillReturnResult(sqlmock.NewResult(0, 1))
			m.ExpectExec(`INSERT INTO spec_analytics`).WillReturnResult(sqlmock.NewResult(0, 1))
			m.ExpectExec(`INSERT INTO spec_processing_jobs`).WillReturnResult(sqlmock.NewResult(0, 1))
			m.ExpectExec(`SET status = 'queued'`).WillReturnError(errUploadBoom)
		}, errUploadBoom},
		{"queue rows affected error", func(m sqlmock.Sqlmock) {
			m.ExpectExec(`UPDATE spec_upload_assets`).WillReturnResult(sqlmock.NewResult(0, 1))
			m.ExpectExec(`INSERT INTO specs`).WillReturnResult(sqlmock.NewResult(0, 1))
			m.ExpectExec(`INSERT INTO spec_analytics`).WillReturnResult(sqlmock.NewResult(0, 1))
			m.ExpectExec(`INSERT INTO spec_processing_jobs`).WillReturnResult(sqlmock.NewResult(0, 1))
			m.ExpectExec(`SET status = 'queued'`).WillReturnResult(sqlmock.NewErrorResult(errUploadBoom))
		}, errUploadBoom},
		{"queue not matched", func(m sqlmock.Sqlmock) {
			m.ExpectExec(`UPDATE spec_upload_assets`).WillReturnResult(sqlmock.NewResult(0, 1))
			m.ExpectExec(`INSERT INTO specs`).WillReturnResult(sqlmock.NewResult(0, 1))
			m.ExpectExec(`INSERT INTO spec_analytics`).WillReturnResult(sqlmock.NewResult(0, 1))
			m.ExpectExec(`INSERT INTO spec_processing_jobs`).WillReturnResult(sqlmock.NewResult(0, 1))
			m.ExpectExec(`SET status = 'queued'`).WillReturnResult(sqlmock.NewResult(0, 0))
		}, domain.ErrUploadState},
	}
	for _, tc := range later {
		t.Run(tc.name, func(t *testing.T) {
			repo, mock, cleanup := newUploadRepo(t)
			defer cleanup()
			s, sp, j := mk()
			mock.ExpectBegin()
			mock.ExpectQuery(lockQ).WillReturnRows(lockRows(producerID, specID, "uploading", future))
			tc.setup(mock)
			require.ErrorIs(t, repo.FinalizeUpload(ctx, s, sp, j), tc.want)
		})
	}
}

func TestUploadRepoHeartbeatErrors(t *testing.T) {
	ctx := context.Background()
	jobID := uuid.New()
	repo, mock, cleanup := newUploadRepo(t)
	defer cleanup()
	mock.ExpectExec(`UPDATE spec_processing_jobs`).WillReturnError(errUploadBoom)
	require.ErrorIs(t, repo.HeartbeatJob(ctx, jobID, "w"), errUploadBoom)
	mock.ExpectExec(`UPDATE spec_processing_jobs`).WillReturnResult(sqlmock.NewErrorResult(errUploadBoom))
	require.ErrorIs(t, repo.HeartbeatJob(ctx, jobID, "w"), errUploadBoom)
}

func TestUploadRepoClaimNextJob(t *testing.T) {
	ctx := context.Background()
	jobID, specID, sessionID, producerID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	claimQ := `WITH candidate AS`
	touchQ := `SET status = 'processing', updated_at = NOW\(\)`
	sessQ := `FROM spec_upload_sessions WHERE id = \$1`
	assetQ := `FROM spec_upload_assets\s+WHERE session_id = \$1\s+ORDER BY kind`
	specQ := `SELECT \* FROM specs WHERE id = \$1`
	now := time.Now()
	jobRows := func() *sqlmock.Rows {
		return sqlmock.NewRows([]string{"id", "spec_id", "session_id", "status", "worker_id", "created_at", "updated_at"}).
			AddRow(jobID, specID, sessionID, "processing", "w1", now, now)
	}

	t.Run("success builds bundle", func(t *testing.T) {
		repo, mock, cleanup := newUploadRepo(t)
		defer cleanup()
		mock.ExpectBegin()
		mock.ExpectQuery(claimQ).WithArgs("w1").WillReturnRows(jobRows())
		mock.ExpectExec(touchQ).WithArgs(sessionID).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectQuery(sessQ).WithArgs(sessionID).WillReturnRows(uploadSessionRows(sessionID, specID, producerID))
		assets := uploadAssetRow(sqlmock.NewRows(uploadAssetCols), uuid.New(), sessionID, "audio")
		mock.ExpectQuery(assetQ).WithArgs(sessionID).WillReturnRows(assets)
		mock.ExpectQuery(specQ).WithArgs(specID).
			WillReturnRows(sqlmock.NewRows([]string{"id", "title"}).AddRow(specID, "Beat"))
		mock.ExpectCommit()

		bundle, err := repo.ClaimNextJob(ctx, "w1")
		require.NoError(t, err)
		assert.Equal(t, jobID, bundle.Job.ID)
		assert.Equal(t, sessionID, bundle.Session.ID)
		assert.Equal(t, producerID, bundle.Session.ProducerID)
		require.Len(t, bundle.Assets, 1)
		assert.Equal(t, specID, bundle.Spec.ID)
		assert.Equal(t, "Beat", bundle.Spec.Title)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("begin error", func(t *testing.T) {
		repo, mock, cleanup := newUploadRepo(t)
		defer cleanup()
		mock.ExpectBegin().WillReturnError(errUploadBoom)
		_, err := repo.ClaimNextJob(ctx, "w1")
		require.ErrorIs(t, err, errUploadBoom)
	})

	t.Run("no job queued", func(t *testing.T) {
		repo, mock, cleanup := newUploadRepo(t)
		defer cleanup()
		mock.ExpectBegin()
		mock.ExpectQuery(claimQ).WillReturnError(sql.ErrNoRows)
		mock.ExpectRollback()
		_, err := repo.ClaimNextJob(ctx, "w1")
		require.ErrorIs(t, err, domain.ErrNoProcessingJob)
	})

	t.Run("claim query error", func(t *testing.T) {
		repo, mock, cleanup := newUploadRepo(t)
		defer cleanup()
		mock.ExpectBegin()
		mock.ExpectQuery(claimQ).WillReturnError(errUploadBoom)
		mock.ExpectRollback()
		_, err := repo.ClaimNextJob(ctx, "w1")
		require.ErrorIs(t, err, errUploadBoom)
	})

	later := []struct {
		name  string
		setup func(m sqlmock.Sqlmock)
	}{
		{"session touch error", func(m sqlmock.Sqlmock) {
			m.ExpectExec(touchQ).WillReturnError(errUploadBoom)
		}},
		{"session load error", func(m sqlmock.Sqlmock) {
			m.ExpectExec(touchQ).WillReturnResult(sqlmock.NewResult(0, 1))
			m.ExpectQuery(sessQ).WillReturnError(errUploadBoom)
		}},
		{"asset load error", func(m sqlmock.Sqlmock) {
			m.ExpectExec(touchQ).WillReturnResult(sqlmock.NewResult(0, 1))
			m.ExpectQuery(sessQ).WillReturnRows(uploadSessionRows(sessionID, specID, producerID))
			m.ExpectQuery(assetQ).WillReturnError(errUploadBoom)
		}},
		{"spec load error", func(m sqlmock.Sqlmock) {
			m.ExpectExec(touchQ).WillReturnResult(sqlmock.NewResult(0, 1))
			m.ExpectQuery(sessQ).WillReturnRows(uploadSessionRows(sessionID, specID, producerID))
			m.ExpectQuery(assetQ).WillReturnRows(sqlmock.NewRows(uploadAssetCols))
			m.ExpectQuery(specQ).WillReturnError(errUploadBoom)
		}},
		{"commit error", func(m sqlmock.Sqlmock) {
			m.ExpectExec(touchQ).WillReturnResult(sqlmock.NewResult(0, 1))
			m.ExpectQuery(sessQ).WillReturnRows(uploadSessionRows(sessionID, specID, producerID))
			m.ExpectQuery(assetQ).WillReturnRows(sqlmock.NewRows(uploadAssetCols))
			m.ExpectQuery(specQ).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(specID))
			m.ExpectCommit().WillReturnError(errUploadBoom)
		}},
	}
	for _, tc := range later {
		t.Run(tc.name, func(t *testing.T) {
			repo, mock, cleanup := newUploadRepo(t)
			defer cleanup()
			mock.ExpectBegin()
			mock.ExpectQuery(claimQ).WillReturnRows(jobRows())
			tc.setup(mock)
			bundle, err := repo.ClaimNextJob(ctx, "w1")
			require.ErrorIs(t, err, errUploadBoom)
			assert.Nil(t, bundle)
		})
	}
}

func TestUploadRepoCompleteJob(t *testing.T) {
	ctx := context.Background()
	jobID, specID, sessionID := uuid.New(), uuid.New(), uuid.New()
	selQ := `SELECT spec_id, session_id\s+FROM spec_processing_jobs`
	specQ := `UPDATE specs\s+SET image_url = \$2`
	jobQ := `UPDATE spec_processing_jobs\s+SET status = 'completed'`
	sessQ := `UPDATE spec_upload_sessions\s+SET status = 'completed'`
	wavURL, stemsURL := "wav", "stems"
	files := domain.ProcessedSpecFiles{ImageURL: "img", PreviewURL: "prev", WAVURL: &wavURL, StemsURL: &stemsURL, Duration: 12}
	idRows := func() *sqlmock.Rows {
		return sqlmock.NewRows([]string{"spec_id", "session_id"}).AddRow(specID, sessionID)
	}

	t.Run("success passes file urls", func(t *testing.T) {
		repo, mock, cleanup := newUploadRepo(t)
		defer cleanup()
		mock.ExpectBegin()
		mock.ExpectQuery(selQ).WithArgs(jobID, "w1").WillReturnRows(idRows())
		mock.ExpectExec(specQ).
			WithArgs(specID, "img", "prev", "wav", "stems", sqlmock.AnyArg(), sqlmock.AnyArg()).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(jobQ).WithArgs(jobID).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(sessQ).WithArgs(sessionID).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()
		require.NoError(t, repo.CompleteJob(ctx, jobID, "w1", files))
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("begin error", func(t *testing.T) {
		repo, mock, cleanup := newUploadRepo(t)
		defer cleanup()
		mock.ExpectBegin().WillReturnError(errUploadBoom)
		require.ErrorIs(t, repo.CompleteJob(ctx, jobID, "w1", files), errUploadBoom)
	})

	cases := []struct {
		name  string
		setup func(m sqlmock.Sqlmock)
		want  error
	}{
		{"select error", func(m sqlmock.Sqlmock) { m.ExpectQuery(selQ).WillReturnError(errUploadBoom) }, errUploadBoom},
		{"spec update error", func(m sqlmock.Sqlmock) {
			m.ExpectQuery(selQ).WillReturnRows(idRows())
			m.ExpectExec(specQ).WillReturnError(errUploadBoom)
		}, errUploadBoom},
		{"spec rows affected error", func(m sqlmock.Sqlmock) {
			m.ExpectQuery(selQ).WillReturnRows(idRows())
			m.ExpectExec(specQ).WillReturnResult(sqlmock.NewErrorResult(errUploadBoom))
		}, errUploadBoom},
		{"spec not found", func(m sqlmock.Sqlmock) {
			m.ExpectQuery(selQ).WillReturnRows(idRows())
			m.ExpectExec(specQ).WillReturnResult(sqlmock.NewResult(0, 0))
		}, domain.ErrUploadState},
		{"job update error", func(m sqlmock.Sqlmock) {
			m.ExpectQuery(selQ).WillReturnRows(idRows())
			m.ExpectExec(specQ).WillReturnResult(sqlmock.NewResult(0, 1))
			m.ExpectExec(jobQ).WillReturnError(errUploadBoom)
		}, errUploadBoom},
		{"session update error", func(m sqlmock.Sqlmock) {
			m.ExpectQuery(selQ).WillReturnRows(idRows())
			m.ExpectExec(specQ).WillReturnResult(sqlmock.NewResult(0, 1))
			m.ExpectExec(jobQ).WillReturnResult(sqlmock.NewResult(0, 1))
			m.ExpectExec(sessQ).WillReturnError(errUploadBoom)
		}, errUploadBoom},
		{"commit error", func(m sqlmock.Sqlmock) {
			m.ExpectQuery(selQ).WillReturnRows(idRows())
			m.ExpectExec(specQ).WillReturnResult(sqlmock.NewResult(0, 1))
			m.ExpectExec(jobQ).WillReturnResult(sqlmock.NewResult(0, 1))
			m.ExpectExec(sessQ).WillReturnResult(sqlmock.NewResult(0, 1))
			m.ExpectCommit().WillReturnError(errUploadBoom)
		}, errUploadBoom},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo, mock, cleanup := newUploadRepo(t)
			defer cleanup()
			mock.ExpectBegin()
			tc.setup(mock)
			require.ErrorIs(t, repo.CompleteJob(ctx, jobID, "w1", files), tc.want)
		})
	}
}

func TestUploadRepoFailJob(t *testing.T) {
	ctx := context.Background()
	jobID, specID, sessionID := uuid.New(), uuid.New(), uuid.New()
	selQ := `SELECT spec_id, session_id\s+FROM spec_processing_jobs`
	specQ := `SET processing_status = 'failed'`
	jobQ := `UPDATE spec_processing_jobs\s+SET status = 'failed'`
	sessQ := `UPDATE spec_upload_sessions\s+SET status = 'failed'`
	idRows := func() *sqlmock.Rows {
		return sqlmock.NewRows([]string{"spec_id", "session_id"}).AddRow(specID, sessionID)
	}

	t.Run("success", func(t *testing.T) {
		repo, mock, cleanup := newUploadRepo(t)
		defer cleanup()
		mock.ExpectBegin()
		mock.ExpectQuery(selQ).WithArgs(jobID, "w1").WillReturnRows(idRows())
		mock.ExpectExec(specQ).WithArgs(specID).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(jobQ).WithArgs(jobID, "why").WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(sessQ).WithArgs(sessionID, "why").WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()
		require.NoError(t, repo.FailJob(ctx, jobID, "w1", "why"))
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("begin error", func(t *testing.T) {
		repo, mock, cleanup := newUploadRepo(t)
		defer cleanup()
		mock.ExpectBegin().WillReturnError(errUploadBoom)
		require.ErrorIs(t, repo.FailJob(ctx, jobID, "w1", "why"), errUploadBoom)
	})

	cases := []struct {
		name  string
		setup func(m sqlmock.Sqlmock)
	}{
		{"select error", func(m sqlmock.Sqlmock) { m.ExpectQuery(selQ).WillReturnError(errUploadBoom) }},
		{"spec update error", func(m sqlmock.Sqlmock) {
			m.ExpectQuery(selQ).WillReturnRows(idRows())
			m.ExpectExec(specQ).WillReturnError(errUploadBoom)
		}},
		{"job update error", func(m sqlmock.Sqlmock) {
			m.ExpectQuery(selQ).WillReturnRows(idRows())
			m.ExpectExec(specQ).WillReturnResult(sqlmock.NewResult(0, 1))
			m.ExpectExec(jobQ).WillReturnError(errUploadBoom)
		}},
		{"session update error", func(m sqlmock.Sqlmock) {
			m.ExpectQuery(selQ).WillReturnRows(idRows())
			m.ExpectExec(specQ).WillReturnResult(sqlmock.NewResult(0, 1))
			m.ExpectExec(jobQ).WillReturnResult(sqlmock.NewResult(0, 1))
			m.ExpectExec(sessQ).WillReturnError(errUploadBoom)
		}},
		{"commit error", func(m sqlmock.Sqlmock) {
			m.ExpectQuery(selQ).WillReturnRows(idRows())
			m.ExpectExec(specQ).WillReturnResult(sqlmock.NewResult(0, 1))
			m.ExpectExec(jobQ).WillReturnResult(sqlmock.NewResult(0, 1))
			m.ExpectExec(sessQ).WillReturnResult(sqlmock.NewResult(0, 1))
			m.ExpectCommit().WillReturnError(errUploadBoom)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo, mock, cleanup := newUploadRepo(t)
			defer cleanup()
			mock.ExpectBegin()
			tc.setup(mock)
			require.ErrorIs(t, repo.FailJob(ctx, jobID, "w1", "why"), errUploadBoom)
		})
	}
}

var _ = sqlx.DB{}
