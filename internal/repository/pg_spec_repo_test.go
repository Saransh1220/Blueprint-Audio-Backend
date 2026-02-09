package repository_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/saransh1220/blueprint-audio/internal/domain"
	"github.com/saransh1220/blueprint-audio/internal/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPGSpecRepository_CreateAndListEmpty(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()
	repo := repository.NewSpecRepository(db)
	ctx := context.Background()
	producerID := uuid.New()

	spec := &domain.Spec{
		ID:             uuid.New(),
		ProducerID:     producerID,
		Title:          "Track",
		Category:       domain.CategoryBeat,
		Type:           "WAV",
		BPM:            120,
		Key:            "C",
		BasePrice:      100,
		ImageUrl:       "img",
		PreviewUrl:     "preview",
		Tags:           pq.StringArray{"trap"},
		Duration:       120,
		FreeMp3Enabled: true,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO specs").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO spec_analytics").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	require.NoError(t, repo.Create(ctx, spec))

	mock.ExpectQuery("SELECT \\*, COUNT\\(\\*\\) OVER\\(\\) as total_count FROM specs WHERE is_deleted = FALSE").
		WillReturnRows(sqlmock.NewRows([]string{"id", "producer_id", "title", "category", "type", "bpm", "key", "base_price", "image_url", "preview_url", "duration", "free_mp3_enabled", "total_count"}))
	out, total, err := repo.List(ctx, domain.SpecFilter{Limit: 20, Offset: 0, MinPrice: -1})
	require.NoError(t, err)
	assert.Len(t, out, 0)
	assert.Equal(t, 0, total)
}

func TestPGSpecRepository_DeletePaths(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()
	repo := repository.NewSpecRepository(db)
	ctx := context.Background()
	id := uuid.New()
	producerID := uuid.New()

	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM licenses WHERE spec_id = \\$1").
		WithArgs(id).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectExec("UPDATE specs SET is_deleted = TRUE, deleted_at = NOW\\(\\) WHERE id = \\$1 AND producer_id = \\$2").
		WithArgs(id, producerID).WillReturnResult(sqlmock.NewResult(0, 1))
	err := repo.Delete(ctx, id, producerID)
	assert.ErrorIs(t, err, domain.ErrSpecSoftDeleted)

	id2 := uuid.New()
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM licenses WHERE spec_id = \\$1").
		WithArgs(id2).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec("DELETE FROM specs WHERE id = \\$1 AND producer_id = \\$2").
		WithArgs(id2, producerID).WillReturnResult(sqlmock.NewResult(0, 0))
	err = repo.Delete(ctx, id2, producerID)
	assert.ErrorIs(t, err, domain.ErrSpecNotFound)
}

func TestPGSpecRepository_UpdateAndGetErrors(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()
	repo := repository.NewSpecRepository(db)
	ctx := context.Background()
	id := uuid.New()
	producerID := uuid.New()

	spec := &domain.Spec{
		ID:             id,
		ProducerID:     producerID,
		Title:          "Track",
		Category:       domain.CategorySample,
		Type:           "WAV",
		BPM:            120,
		Key:            "C",
		BasePrice:      100,
		ImageUrl:       "img",
		Description:    "desc",
		Tags:           pq.StringArray{"a"},
		Duration:       10,
		FreeMp3Enabled: false,
	}

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE specs").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	require.NoError(t, repo.Update(ctx, spec))

	mock.ExpectQuery("SELECT \\* FROM specs WHERE id = \\$1 AND is_deleted = FALSE").
		WithArgs(id).WillReturnError(errors.New("db"))
	_, err := repo.GetByID(ctx, id)
	assert.EqualError(t, err, "db")

	mock.ExpectQuery("SELECT \\* FROM specs WHERE id = \\$1").
		WithArgs(id).WillReturnError(errors.New("db"))
	_, err = repo.GetByIDSystem(ctx, id)
	assert.EqualError(t, err, "db")
}

func TestPGSpecRepository_GetByIDAndListByUserID(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()
	repo := repository.NewSpecRepository(db)
	ctx := context.Background()
	id := uuid.New()
	userID := uuid.New()

	rows := sqlmock.NewRows([]string{
		"id", "producer_id", "title", "category", "type", "bpm", "key", "base_price",
		"image_url", "preview_url", "duration", "free_mp3_enabled", "is_deleted",
	}).
		AddRow(id, userID, "Track", "beat", "WAV", 120, "C", 100, "img", "prev", 120, true, false)
	mock.ExpectQuery("SELECT \\* FROM specs WHERE id = \\$1 AND is_deleted = FALSE").WithArgs(id).WillReturnRows(rows)
	mock.ExpectQuery("SELECT \\* FROM license_options WHERE spec_id = \\$1 AND is_deleted = FALSE").WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{"id", "spec_id", "license_type", "name", "price", "features", "file_types", "is_deleted"}))
	mock.ExpectQuery("SELECT g\\.\\* FROM genres g JOIN spec_genres sg ON g.id = sg.genre_id WHERE sg.spec_id = \\$1").WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "slug", "created_at"}))
	_, err := repo.GetByID(ctx, id)
	require.NoError(t, err)

	mock.ExpectQuery("SELECT \\*, COUNT\\(\\*\\) OVER\\(\\) as total_count").
		WithArgs(userID, 20, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "producer_id", "title", "category", "type", "bpm", "key", "base_price",
			"image_url", "preview_url", "duration", "free_mp3_enabled", "is_deleted", "total_count",
		}))
	specs, total, err := repo.ListByUserID(ctx, userID, 20, 0)
	require.NoError(t, err)
	assert.Len(t, specs, 0)
	assert.Equal(t, 0, total)
}

func TestPGSpecRepository_ListWithRelationsAndUpdateNotFound(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()
	repo := repository.NewSpecRepository(db)
	ctx := context.Background()

	specID := uuid.New()
	genreID := uuid.New()
	licenseID := uuid.New()
	producerID := uuid.New()

	mainRows := sqlmock.NewRows([]string{
		"id", "producer_id", "title", "category", "type", "bpm", "key", "base_price", "image_url", "preview_url", "duration", "free_mp3_enabled", "total_count",
	}).AddRow(specID, producerID, "Track", "beat", "WAV", 120, "C", 10.0, "img", "prev", 120, true, 1)

	mock.ExpectQuery("SELECT \\*, COUNT\\(\\*\\) OVER\\(\\) as total_count FROM specs WHERE is_deleted = FALSE").
		WillReturnRows(mainRows)
	mock.ExpectQuery("SELECT sg.spec_id, g.\\*").
		WillReturnRows(sqlmock.NewRows([]string{"spec_id", "id", "name", "slug", "created_at"}).AddRow(specID, genreID, "Hip Hop", "hip-hop", time.Now()))
	mock.ExpectQuery("SELECT \\* FROM license_options WHERE spec_id IN").
		WillReturnRows(sqlmock.NewRows([]string{"id", "spec_id", "license_type", "name", "price", "features", "file_types", "is_deleted"}).
			AddRow(licenseID, specID, "Basic", "Basic", 10.0, pq.StringArray{"f1"}, pq.StringArray{"mp3"}, false))

	out, total, err := repo.List(ctx, domain.SpecFilter{Limit: 20, Offset: 0, MinPrice: -1, Sort: "price_asc"})
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, 1, total)
	assert.Len(t, out[0].Genres, 1)
	assert.Len(t, out[0].Licenses, 1)

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE specs").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()
	err = repo.Update(ctx, &domain.Spec{ID: specID, ProducerID: producerID})
	assert.ErrorIs(t, err, domain.ErrSpecNotFound)
}

func TestPGSpecRepository_UpdateLicenseSyncDeletionBranches(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()
	repo := repository.NewSpecRepository(db)
	ctx := context.Background()

	specID := uuid.New()
	producerID := uuid.New()
	usedLicenseID := uuid.New()
	unusedLicenseID := uuid.New()
	spec := &domain.Spec{
		ID:         specID,
		ProducerID: producerID,
		Title:      "Track",
		Category:   domain.CategoryBeat,
		Type:       "WAV",
		BPM:        120,
		Key:        "C",
		BasePrice:  10,
		ImageUrl:   "img",
		Tags:       pq.StringArray{"a"},
		Duration:   120,
		Licenses:   []domain.LicenseOption{},
	}

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE specs").
		WillReturnResult(sqlmock.NewResult(0, 1))
	existingRows := sqlmock.NewRows([]string{
		"id", "spec_id", "license_type", "name", "price", "features", "file_types", "is_deleted",
	}).
		AddRow(usedLicenseID, specID, "Basic", "Basic", 10, pq.StringArray{"f1"}, pq.StringArray{"mp3"}, false).
		AddRow(unusedLicenseID, specID, "Premium", "Premium", 20, pq.StringArray{"f2"}, pq.StringArray{"wav"}, false)
	mock.ExpectQuery("SELECT \\* FROM license_options WHERE spec_id = \\$1 AND is_deleted = FALSE").
		WithArgs(specID).WillReturnRows(existingRows)

	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM licenses WHERE license_option_id = \\$1").
		WithArgs(usedLicenseID).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectExec("UPDATE license_options SET is_deleted = TRUE, updated_at = NOW\\(\\) WHERE id = \\$1").
		WithArgs(usedLicenseID).WillReturnResult(sqlmock.NewResult(0, 1))

	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM licenses WHERE license_option_id = \\$1").
		WithArgs(unusedLicenseID).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec("DELETE FROM license_options WHERE id = \\$1").
		WithArgs(unusedLicenseID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	require.NoError(t, repo.Update(ctx, spec))
}

func TestPGSpecRepository_CreateWithGenresAndLicenses(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()
	repo := repository.NewSpecRepository(db)
	ctx := context.Background()

	specID := uuid.New()
	producerID := uuid.New()
	spec := &domain.Spec{
		ID:         specID,
		ProducerID: producerID,
		Title:      "Track",
		Category:   domain.CategoryBeat,
		Type:       "WAV",
		BPM:        120,
		Key:        "C",
		BasePrice:  10,
		ImageUrl:   "img",
		PreviewUrl: "prev",
		Genres: []domain.Genre{
			{Name: "Hip Hop", Slug: "hip-hop"},
		},
		Licenses: []domain.LicenseOption{
			{LicenseType: domain.LicenseBasic, Name: "Basic", Price: 10, Features: pq.StringArray{"f1"}, FileTypes: pq.StringArray{"mp3"}},
		},
	}

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO specs").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO spec_analytics").WithArgs(specID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT id FROM genres WHERE slug = \\$1").WithArgs("hip-hop").WillReturnError(errors.New("no rows"))
	mock.ExpectExec("INSERT INTO genres \\(id, name, slug, created_at\\)").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO spec_genres \\(spec_id, genre_id\\) VALUES \\(\\$1, \\$2\\)").WithArgs(specID, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO license_options").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	require.NoError(t, repo.Create(ctx, spec))
}

func TestPGSpecRepository_UpdateLicenseUpsertBranches(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()
	repo := repository.NewSpecRepository(db)
	ctx := context.Background()

	specID := uuid.New()
	producerID := uuid.New()
	existingID := uuid.New()
	spec := &domain.Spec{
		ID:         specID,
		ProducerID: producerID,
		Title:      "Track",
		Category:   domain.CategoryBeat,
		Type:       "WAV",
		BPM:        120,
		Key:        "C",
		BasePrice:  10,
		ImageUrl:   "img",
		Tags:       pq.StringArray{"a"},
		Duration:   120,
		Licenses: []domain.LicenseOption{
			{LicenseType: domain.LicenseBasic, Name: "Basic", Price: 12, Features: pq.StringArray{"f1"}, FileTypes: pq.StringArray{"mp3"}},
			{LicenseType: domain.LicensePremium, Name: "Premium", Price: 20, Features: pq.StringArray{"f2"}, FileTypes: pq.StringArray{"wav"}},
		},
	}

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE specs").WillReturnResult(sqlmock.NewResult(0, 1))
	rows := sqlmock.NewRows([]string{
		"id", "spec_id", "license_type", "name", "price", "features", "file_types", "is_deleted",
	}).AddRow(existingID, specID, "Basic", "Basic", 10, pq.StringArray{"f1"}, pq.StringArray{"mp3"}, false)
	mock.ExpectQuery("SELECT \\* FROM license_options WHERE spec_id = \\$1 AND is_deleted = FALSE").
		WithArgs(specID).WillReturnRows(rows)
	mock.ExpectExec("UPDATE license_options SET").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO license_options").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	require.NoError(t, repo.Update(ctx, spec))
}
