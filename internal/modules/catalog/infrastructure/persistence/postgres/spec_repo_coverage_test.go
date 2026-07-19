package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/saransh1220/blueprint-audio/internal/modules/catalog/domain"
	"github.com/stretchr/testify/require"
)

func coverageSpecRepository(t *testing.T) (*PgSpecRepository, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mockDB, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return NewSpecRepository(sqlx.NewDb(sqlDB, "sqlmock")), mockDB
}

func TestRankingIntervalsAndUnsupportedSections(t *testing.T) {
	tests := []struct {
		period string
		want   string
	}{
		{domain.HomePeriod24H, "24 hours"},
		{domain.HomePeriod7D, "7 days"},
		{domain.HomePeriod30D, "30 days"},
	}
	for _, tt := range tests {
		interval, err := rankingInterval(tt.period)
		require.NoError(t, err)
		require.Equal(t, tt.want, interval)
	}
	_, err := rankingInterval("year")
	require.EqualError(t, err, "unsupported ranking period: year")

	repo := &PgSpecRepository{}
	require.EqualError(t, repo.RecalculateBeatRankings(context.Background(), "trending", "year"), "unsupported ranking period: year")
	require.EqualError(t, repo.RecalculateBeatRankings(context.Background(), "unknown", domain.HomePeriod24H), "unsupported ranking section: unknown")
}

func TestUpdateFilesAndStatusBranches(t *testing.T) {
	ctx := context.Background()
	specID := uuid.New()
	imageURL := "image"
	previewURL := "preview"
	wavURL := "wav"
	stemsURL := "stems"
	peaks := "[8,25,100]"
	files := map[string]*string{
		"image_url":      &imageURL,
		"preview_url":    &previewURL,
		"wav_url":        &wavURL,
		"stems_url":      &stemsURL,
		"waveform_peaks": &peaks,
	}

	t.Run("success", func(t *testing.T) {
		repo, mockDB := coverageSpecRepository(t)
		mockDB.ExpectExec("UPDATE specs SET processing_status").WillReturnResult(sqlmock.NewResult(0, 1))
		require.NoError(t, repo.UpdateFilesAndStatus(ctx, specID, files, domain.ProcessingStatusCompleted))
		require.NoError(t, mockDB.ExpectationsWereMet())
	})

	t.Run("invalid waveform", func(t *testing.T) {
		repo, _ := coverageSpecRepository(t)
		bad := "not-json"
		err := repo.UpdateFilesAndStatus(ctx, specID, map[string]*string{"waveform_peaks": &bad}, domain.ProcessingStatusFailed)
		require.ErrorContains(t, err, "decode waveform peaks")
	})

	t.Run("database error", func(t *testing.T) {
		repo, mockDB := coverageSpecRepository(t)
		mockDB.ExpectExec("UPDATE specs SET processing_status").WillReturnError(errors.New("write failed"))
		require.EqualError(t, repo.UpdateFilesAndStatus(ctx, specID, nil, domain.ProcessingStatusFailed), "write failed")
	})

	t.Run("missing spec", func(t *testing.T) {
		repo, mockDB := coverageSpecRepository(t)
		mockDB.ExpectExec("UPDATE specs SET processing_status").WillReturnResult(sqlmock.NewResult(0, 0))
		require.ErrorIs(t, repo.UpdateFilesAndStatus(ctx, specID, nil, domain.ProcessingStatusFailed), domain.ErrSpecNotFound)
	})
}
