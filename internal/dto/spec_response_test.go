package dto_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/saransh1220/blueprint-audio/internal/domain"
	"github.com/saransh1220/blueprint-audio/internal/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToSpecResponse(t *testing.T) {
	now := time.Now()
	spec := &domain.Spec{
		ID:             uuid.New(),
		ProducerID:     uuid.New(),
		ProducerName:   "Display Producer",
		Title:          "Track",
		Category:       domain.CategoryBeat,
		Type:           "WAV",
		BPM:            120,
		Key:            "C",
		ImageUrl:       "img",
		PreviewUrl:     "preview",
		BasePrice:      10,
		Duration:       120,
		FreeMp3Enabled: true,
		CreatedAt:      now,
		UpdatedAt:      now,
		Tags:           pq.StringArray{"trap"},
		Licenses: []domain.LicenseOption{
			{ID: uuid.New(), SpecID: uuid.New(), LicenseType: domain.LicenseBasic, Name: "Basic", Price: 10},
		},
		Genres: []domain.Genre{
			{ID: uuid.New(), Name: "Hip Hop", Slug: "hip-hop", CreatedAt: now},
		},
		Analytics: &domain.SpecAnalytics{
			PlayCount:         5,
			FavoriteCount:     2,
			FreeDownloadCount: 1,
			IsFavorited:       true,
		},
	}

	resp := dto.ToSpecResponse(spec)
	assert.Equal(t, "Track", resp.Title)
	assert.Equal(t, "Display Producer", resp.ProducerName)
	assert.Equal(t, "beat", resp.Category)
	assert.Len(t, resp.Licenses, 1)
	assert.Len(t, resp.Genres, 1)
	assert.NotNil(t, resp.Analytics)
	assert.Equal(t, 5, resp.Analytics.PlayCount)
}

func TestToSpecResponse_Minimal(t *testing.T) {
	spec := &domain.Spec{
		ID:         uuid.New(),
		ProducerID: uuid.New(),
		Title:      "Minimal",
		Category:   domain.CategorySample,
		Type:       "MP3",
		BPM:        90,
		Key:        "Dm",
	}

	resp := dto.ToSpecResponse(spec)
	require.NotNil(t, resp)
	assert.Equal(t, "Minimal", resp.Title)
	assert.Equal(t, "sample", resp.Category)
	assert.Nil(t, resp.Analytics)
	assert.Empty(t, resp.Licenses)
	assert.Empty(t, resp.Genres)
}
