package dto

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/saransh1220/blueprint-audio/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestToSpecResponse_InternalPackage(t *testing.T) {
	now := time.Now()
	spec := &domain.Spec{
		ID:           uuid.New(),
		ProducerID:   uuid.New(),
		ProducerName: "Producer",
		Title:        "Track",
		Category:     domain.CategoryBeat,
		Type:         "WAV",
		BPM:          120,
		Key:          "C",
		Tags:         pq.StringArray{"trap"},
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

	resp := ToSpecResponse(spec)
	require.NotNil(t, resp)
	require.Equal(t, "Producer", resp.ProducerName)
	require.NotNil(t, resp.Analytics)
	require.Equal(t, 1, len(resp.Licenses))
	require.Equal(t, 1, len(resp.Genres))
}

