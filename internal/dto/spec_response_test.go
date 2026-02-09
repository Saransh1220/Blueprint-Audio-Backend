package dto_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/saransh1220/blueprint-audio/internal/domain"
	"github.com/saransh1220/blueprint-audio/internal/dto"
	"github.com/stretchr/testify/assert"
)

func TestToSpecResponse(t *testing.T) {
	now := time.Now()
	spec := &domain.Spec{
		ID:             uuid.New(),
		ProducerID:     uuid.New(),
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
	assert.Equal(t, "beat", resp.Category)
	assert.Len(t, resp.Licenses, 1)
	assert.Len(t, resp.Genres, 1)
	assert.NotNil(t, resp.Analytics)
	assert.Equal(t, 5, resp.Analytics.PlayCount)
}
