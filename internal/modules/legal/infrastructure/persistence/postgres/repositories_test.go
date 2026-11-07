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
	"github.com/saransh1220/blueprint-audio/internal/modules/legal/domain"
	"github.com/saransh1220/blueprint-audio/internal/modules/legal/infrastructure/persistence/postgres"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMockDB(t *testing.T) (*sqlx.DB, sqlmock.Sqlmock, func()) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	return sqlx.NewDb(sqlDB, "sqlmock"), mock, func() { _ = sqlDB.Close() }
}

func TestPgAcceptanceRepository(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()
	repo := postgres.NewAcceptanceRepository(db)
	ctx := context.Background()

	accID := uuid.New()
	userID := uuid.New()
	verID := uuid.New()
	now := time.Now().UTC()

	acc := &domain.Acceptance{
		ID:          accID,
		UserID:      userID,
		DocumentKey: "terms",
		VersionID:   verID,
		ContentHash: "hash123",
		Action:      "implicit_browse",
		Context:     "test",
		CreatedAt:   now,
	}

	t.Run("RecordAcceptance success and error", func(t *testing.T) {
		mock.ExpectExec("INSERT INTO legal_acceptances").
			WithArgs(acc.ID, acc.UserID, acc.DocumentKey, acc.VersionID, acc.ContentHash, acc.Action, acc.Context, acc.CreatedAt).
			WillReturnResult(sqlmock.NewResult(1, 1))

		err := repo.RecordAcceptance(ctx, acc)
		require.NoError(t, err)

		mock.ExpectExec("INSERT INTO legal_acceptances").
			WillReturnError(sql.ErrConnDone)
		err = repo.RecordAcceptance(ctx, acc)
		require.Error(t, err)
	})

	t.Run("GetLatestAcceptance found, not found, error", func(t *testing.T) {
		rows := sqlmock.NewRows([]string{"id", "user_id", "document_key", "version_id", "content_hash", "action", "context", "created_at"}).
			AddRow(accID, userID, "terms", verID, "hash123", "implicit_browse", "test", now)

		mock.ExpectQuery("SELECT id, user_id, document_key").
			WithArgs(userID, "terms").
			WillReturnRows(rows)

		got, err := repo.GetLatestAcceptance(ctx, userID, "terms")
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, accID, got.ID)

		mock.ExpectQuery("SELECT id, user_id, document_key").
			WithArgs(userID, "terms").
			WillReturnError(sql.ErrNoRows)

		got, err = repo.GetLatestAcceptance(ctx, userID, "terms")
		require.NoError(t, err)
		assert.Nil(t, got)

		mock.ExpectQuery("SELECT id, user_id, document_key").
			WithArgs(userID, "terms").
			WillReturnError(sql.ErrConnDone)

		_, err = repo.GetLatestAcceptance(ctx, userID, "terms")
		require.Error(t, err)
	})

	t.Run("HasAcceptedVersion true, false, error", func(t *testing.T) {
		mock.ExpectQuery("SELECT count").
			WithArgs(userID, verID).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

		has, err := repo.HasAcceptedVersion(ctx, userID, verID)
		require.NoError(t, err)
		assert.True(t, has)

		mock.ExpectQuery("SELECT count").
			WithArgs(userID, verID).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

		has, err = repo.HasAcceptedVersion(ctx, userID, verID)
		require.NoError(t, err)
		assert.False(t, has)

		mock.ExpectQuery("SELECT count").
			WithArgs(userID, verID).
			WillReturnError(sql.ErrConnDone)

		_, err = repo.HasAcceptedVersion(ctx, userID, verID)
		require.Error(t, err)
	})
}

func TestPgDocumentRepository(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()
	repo := postgres.NewDocumentRepository(db)
	ctx := context.Background()

	verID := uuid.New()
	creatorID := uuid.New()
	now := time.Now().UTC()

	sectionsJSON, _ := json.Marshal([]domain.Section{
		{ID: "intro", Title: "Introduction", Content: "Welcome", Order: 1},
	})

	versionCols := []string{
		"id", "document_key", "display_version", "schema_version", "title", "summary",
		"sections", "content_hash", "update_classification", "status",
		"published_at", "effective_at", "superseded_at", "created_by", "created_at",
	}

	t.Run("GetDocument found, not found, error", func(t *testing.T) {
		docRows := sqlmock.NewRows([]string{"key", "audience", "locale", "title", "created_at", "updated_at"}).
			AddRow("terms", "general", "en", "Terms of Service", now, now)

		mock.ExpectQuery("SELECT key, audience, locale, title").
			WithArgs("terms", "en").
			WillReturnRows(docRows)

		doc, err := repo.GetDocument(ctx, "terms", "en")
		require.NoError(t, err)
		assert.Equal(t, "terms", doc.Key)

		mock.ExpectQuery("SELECT key, audience, locale, title").
			WithArgs("terms", "en").
			WillReturnError(sql.ErrNoRows)

		_, err = repo.GetDocument(ctx, "terms", "en")
		assert.ErrorIs(t, err, domain.ErrDocumentNotFound)

		mock.ExpectQuery("SELECT key, audience, locale, title").
			WithArgs("terms", "en").
			WillReturnError(sql.ErrConnDone)

		_, err = repo.GetDocument(ctx, "terms", "en")
		require.Error(t, err)
	})

	t.Run("GetEffectiveVersion found, not found", func(t *testing.T) {
		rows := sqlmock.NewRows(versionCols).
			AddRow(verID, "terms", "2026.1", 1, "Terms", "Summary", sectionsJSON, "hash", "major", "published", now, now, nil, creatorID, now)

		mock.ExpectQuery("SELECT v.id, v.document_key").
			WithArgs("terms", "en").
			WillReturnRows(rows)

		v, err := repo.GetEffectiveVersion(ctx, "terms", "en")
		require.NoError(t, err)
		require.NotNil(t, v)
		assert.Equal(t, verID, v.ID)
		assert.Len(t, v.Sections, 1)

		mock.ExpectQuery("SELECT v.id, v.document_key").
			WithArgs("terms", "en").
			WillReturnError(sql.ErrNoRows)

		v, err = repo.GetEffectiveVersion(ctx, "terms", "en")
		assert.ErrorIs(t, err, domain.ErrVersionNotFound)
		assert.Nil(t, v)
	})

	t.Run("GetUpcomingVersion", func(t *testing.T) {
		rows := sqlmock.NewRows(versionCols).
			AddRow(verID, "terms", "2026.2", 1, "Terms", "Summary", sectionsJSON, "hash", "major", "scheduled", now, now.Add(24*time.Hour), nil, creatorID, now)

		mock.ExpectQuery("SELECT v.id, v.document_key").
			WithArgs("terms", "en").
			WillReturnRows(rows)

		v, err := repo.GetUpcomingVersion(ctx, "terms", "en")
		require.NoError(t, err)
		require.NotNil(t, v)
		assert.Equal(t, "2026.2", v.DisplayVersion)
	})

	t.Run("GetVersionByDisplay and GetVersionByID", func(t *testing.T) {
		rows := sqlmock.NewRows(versionCols).
			AddRow(verID, "terms", "2026.1", 1, "Terms", "Summary", sectionsJSON, "hash", "major", "published", now, now, nil, creatorID, now)

		mock.ExpectQuery("SELECT id, document_key, display_version").
			WithArgs("terms", "2026.1").
			WillReturnRows(rows)

		v, err := repo.GetVersionByDisplay(ctx, "terms", "2026.1")
		require.NoError(t, err)
		require.NotNil(t, v)

		rowsByID := sqlmock.NewRows(versionCols).
			AddRow(verID, "terms", "2026.1", 1, "Terms", "Summary", sectionsJSON, "hash", "major", "published", now, now, nil, creatorID, now)

		mock.ExpectQuery("SELECT id, document_key, display_version").
			WithArgs(verID).
			WillReturnRows(rowsByID)

		v, err = repo.GetVersionByID(ctx, verID)
		require.NoError(t, err)
		require.NotNil(t, v)
	})

	t.Run("ListPublishedVersions", func(t *testing.T) {
		rows := sqlmock.NewRows(versionCols).
			AddRow(verID, "terms", "2026.1", 1, "Terms", "Summary", sectionsJSON, "hash", "major", "published", now, now, nil, creatorID, now)

		mock.ExpectQuery("SELECT id, document_key, display_version").
			WithArgs("terms").
			WillReturnRows(rows)

		list, err := repo.ListPublishedVersions(ctx, "terms")
		require.NoError(t, err)
		require.Len(t, list, 1)

		mock.ExpectQuery("SELECT id, document_key, display_version").
			WithArgs("terms").
			WillReturnError(sql.ErrConnDone)

		_, err = repo.ListPublishedVersions(ctx, "terms")
		require.Error(t, err)
	})

	t.Run("CreateDraft", func(t *testing.T) {
		v := &domain.DocumentVersion{
			ID:                   verID,
			DocumentKey:          "terms",
			DisplayVersion:       "2026.3",
			SchemaVersion:        1,
			Title:                "Terms",
			Summary:              "Draft summary",
			Sections:             []domain.Section{{ID: "s1", Title: "Title", Content: "Body", Order: 1}},
			ContentHash:          "hash",
			UpdateClassification: "minor",
			Status:               domain.StatusDraft,
			CreatedBy:            &creatorID,
			CreatedAt:            now,
		}

		mock.ExpectExec("INSERT INTO legal_document_versions").
			WillReturnResult(sqlmock.NewResult(1, 1))

		err := repo.CreateDraft(ctx, v)
		require.NoError(t, err)
	})

	t.Run("PublishVersion success and tx error", func(t *testing.T) {
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT document_key FROM legal_document_versions").
			WithArgs(verID).
			WillReturnRows(sqlmock.NewRows([]string{"document_key"}).AddRow("terms"))
		mock.ExpectExec("UPDATE legal_document_versions SET status = 'archived'").
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec("UPDATE legal_document_versions SET status = 'published'").
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()

		err := repo.PublishVersion(ctx, verID)
		require.NoError(t, err)

		mock.ExpectBegin().WillReturnError(errors.New("tx fail"))
		err = repo.PublishVersion(ctx, verID)
		require.Error(t, err)
	})
}
