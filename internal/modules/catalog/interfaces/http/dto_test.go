package http

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/saransh1220/blueprint-audio/internal/modules/catalog/domain"
	"github.com/stretchr/testify/require"
)

func TestToSpecResponse_WithRelations(t *testing.T) {
	now := time.Now()
	spec := &domain.Spec{
		ID: uuid.New(), ProducerID: uuid.New(), ProducerName: "p", Title: "t", Category: domain.CategoryBeat, Type: "wav", BPM: 120, Key: "C",
		ImageUrl: "img", PreviewUrl: "prev", BasePrice: 10, Duration: 100, FreeMp3Enabled: true, CreatedAt: now, UpdatedAt: now,
		Tags: pq.StringArray{"trap"},
		Licenses: []domain.LicenseOption{{ID: uuid.New(), SpecID: uuid.New(), LicenseType: domain.LicenseBasic, Name: "Basic", Price: 1, Features: pq.StringArray{"a"}, FileTypes: pq.StringArray{"mp3"}, CreatedAt: now, UpdatedAt: now}},
		Genres:   []domain.Genre{{ID: uuid.New(), Name: "HipHop", Slug: "hiphop", CreatedAt: now}},
	}
	res := ToSpecResponse(spec)
	require.Equal(t, spec.Title, res.Title)
	require.Len(t, res.Licenses, 1)
	require.Len(t, res.Genres, 1)
	require.Equal(t, "Basic", res.Licenses[0].Type)
}

func TestToSpecResponse_WithoutRelations(t *testing.T) {
	spec := &domain.Spec{ID: uuid.New(), ProducerID: uuid.New(), Title: "t", Category: domain.CategorySample}
	res := ToSpecResponse(spec)
	require.NotNil(t, res)
	require.Nil(t, res.Licenses)
	require.Nil(t, res.Genres)
}
