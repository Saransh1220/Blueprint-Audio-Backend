package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	analyticsDomain "github.com/saransh1220/blueprint-audio/internal/modules/analytics/domain"
	catalogDomain "github.com/saransh1220/blueprint-audio/internal/modules/catalog/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnalyticsService_ToggleFavorite_WriteErrors(t *testing.T) {
	ctx := context.Background()
	ar := new(mockAnalyticsRepository)
	svc := NewAnalyticsService(ar, new(mockSpecRepository))
	userID, specID := uuid.New(), uuid.New()

	ar.On("IsFavorited", ctx, userID, specID).Return(true, nil).Once()
	ar.On("RemoveFavorite", ctx, userID, specID).Return(errors.New("rm fail")).Once()
	val, err := svc.ToggleFavorite(ctx, userID, specID)
	assert.EqualError(t, err, "rm fail")
	assert.True(t, val, "still favorited when remove fails")

	ar.On("IsFavorited", ctx, userID, specID).Return(false, nil).Once()
	ar.On("AddFavorite", ctx, userID, specID).Return(errors.New("add fail")).Once()
	val, err = svc.ToggleFavorite(ctx, userID, specID)
	assert.EqualError(t, err, "add fail")
	assert.False(t, val)
}

func TestAnalyticsService_TrackAndIsFavorited(t *testing.T) {
	ctx := context.Background()
	ar := new(mockAnalyticsRepository)
	svc := NewAnalyticsService(ar, new(mockSpecRepository))
	userID, specID := uuid.New(), uuid.New()

	ar.On("IncrementPlayCount", ctx, specID).Return(errors.New("p")).Once()
	assert.EqualError(t, svc.TrackPlay(ctx, specID), "p")
	ar.On("IncrementFreeDownloadCount", ctx, specID).Return(errors.New("d")).Once()
	assert.EqualError(t, svc.TrackFreeDownload(ctx, specID), "d")
	ar.On("IsFavorited", ctx, userID, specID).Return(true, nil).Once()
	ok, err := svc.IsFavorited(ctx, userID, specID)
	assert.NoError(t, err)
	assert.True(t, ok)
}

func TestAnalyticsService_GetPublicAnalytics_Branches(t *testing.T) {
	ctx := context.Background()
	ar := new(mockAnalyticsRepository)
	svc := NewAnalyticsService(ar, new(mockSpecRepository))
	userID, specID := uuid.New(), uuid.New()

	ar.On("GetSpecAnalytics", ctx, specID).Return(nil, errors.New("db")).Once()
	_, err := svc.GetPublicAnalytics(ctx, specID, &userID)
	assert.EqualError(t, err, "db")

	// IsFavorited error is swallowed
	ar.On("GetSpecAnalytics", ctx, specID).Return(&analyticsDomain.SpecAnalytics{PlayCount: 3, FavoriteCount: 2, FreeDownloadCount: 1}, nil).Once()
	ar.On("IsFavorited", ctx, userID, specID).Return(false, errors.New("fav err")).Once()
	res, err := svc.GetPublicAnalytics(ctx, specID, &userID)
	require.NoError(t, err)
	assert.False(t, res.IsFavorited)
	assert.Equal(t, 3, res.PlayCount)
	assert.Equal(t, 1, res.TotalDownloadCount)
}

func TestAnalyticsService_GetProducerAnalytics_RepoErrors(t *testing.T) {
	ctx := context.Background()
	ar := new(mockAnalyticsRepository)
	sr := new(mockSpecRepository)
	svc := NewAnalyticsService(ar, sr)
	specID, producerID := uuid.New(), uuid.New()
	spec := &catalogDomain.Spec{ID: specID, ProducerID: producerID}

	sr.On("GetByID", ctx, specID).Return(spec, nil).Twice()
	ar.On("GetSpecAnalytics", ctx, specID).Return(nil, errors.New("analytics err")).Once()
	_, err := svc.GetProducerAnalytics(ctx, specID, producerID)
	assert.EqualError(t, err, "analytics err")

	ar.On("GetSpecAnalytics", ctx, specID).Return(&analyticsDomain.SpecAnalytics{}, nil).Once()
	ar.On("GetLicensePurchaseCounts", ctx, specID).Return(nil, errors.New("counts err")).Once()
	_, err = svc.GetProducerAnalytics(ctx, specID, producerID)
	assert.EqualError(t, err, "counts err")
}

func TestAnalyticsService_GetStatsOverview_ErrorAtEachStep(t *testing.T) {
	ctx := context.Background()
	pid := uuid.New()
	boom := errors.New("boom")

	// steps in call order; each setter registers success for that step.
	setups := []func(ar *mockAnalyticsRepository, fail bool){
		func(ar *mockAnalyticsRepository, fail bool) {
			if fail {
				ar.On("GetTotalPlays", ctx, pid, 7).Return(0, boom)
			} else {
				ar.On("GetTotalPlays", ctx, pid, 7).Return(10, nil)
			}
		},
		func(ar *mockAnalyticsRepository, fail bool) {
			if fail {
				ar.On("GetTotalFavorites", ctx, pid, 7).Return(0, boom)
			} else {
				ar.On("GetTotalFavorites", ctx, pid, 7).Return(5, nil)
			}
		},
		func(ar *mockAnalyticsRepository, fail bool) {
			if fail {
				ar.On("GetTotalDownloads", ctx, pid, 7).Return(0, boom)
			} else {
				ar.On("GetTotalDownloads", ctx, pid, 7).Return(4, nil)
			}
		},
		func(ar *mockAnalyticsRepository, fail bool) {
			if fail {
				ar.On("GetTotalRevenue", ctx, pid, 7).Return(0.0, boom)
			} else {
				ar.On("GetTotalRevenue", ctx, pid, 7).Return(99.5, nil)
			}
		},
		func(ar *mockAnalyticsRepository, fail bool) {
			if fail {
				ar.On("GetPlaysByDay", ctx, pid, 7).Return(nil, boom)
			} else {
				ar.On("GetPlaysByDay", ctx, pid, 7).Return([]analyticsDomain.DailyStat{{Date: "d", Count: 1}}, nil)
			}
		},
		func(ar *mockAnalyticsRepository, fail bool) {
			if fail {
				ar.On("GetDownloadsByDay", ctx, pid, 7).Return(nil, boom)
			} else {
				ar.On("GetDownloadsByDay", ctx, pid, 7).Return([]analyticsDomain.DailyStat{{Date: "d", Count: 2}}, nil)
			}
		},
		func(ar *mockAnalyticsRepository, fail bool) {
			if fail {
				ar.On("GetRevenueByDay", ctx, pid, 7).Return(nil, boom)
			} else {
				ar.On("GetRevenueByDay", ctx, pid, 7).Return([]analyticsDomain.DailyRevenueStat{{Date: "d", Revenue: 3}}, nil)
			}
		},
		func(ar *mockAnalyticsRepository, fail bool) {
			if fail {
				ar.On("GetTopSpecs", ctx, pid, 5, "plays").Return(nil, boom)
			} else {
				ar.On("GetTopSpecs", ctx, pid, 5, "plays").Return([]analyticsDomain.TopSpecStat{{Title: "t"}}, nil)
			}
		},
		func(ar *mockAnalyticsRepository, fail bool) {
			if fail {
				ar.On("GetRevenueByLicenseGlobal", ctx, pid, 7).Return(nil, boom)
			} else {
				ar.On("GetRevenueByLicenseGlobal", ctx, pid, 7).Return(map[string]float64{"basic": 9}, nil)
			}
		},
	}

	for failAt := range setups {
		ar := new(mockAnalyticsRepository)
		svc := NewAnalyticsService(ar, new(mockSpecRepository))
		for i := 0; i <= failAt; i++ {
			setups[i](ar, i == failAt)
		}
		res, err := svc.GetStatsOverview(ctx, pid, 7, "plays")
		assert.Nil(t, res, "step %d", failAt)
		assert.ErrorIs(t, err, boom, "step %d", failAt)
	}

	// full success
	ar := new(mockAnalyticsRepository)
	svc := NewAnalyticsService(ar, new(mockSpecRepository))
	for _, s := range setups {
		s(ar, false)
	}
	res, err := svc.GetStatsOverview(ctx, pid, 7, "plays")
	require.NoError(t, err)
	assert.Equal(t, 10, res.TotalPlays)
	assert.Equal(t, 5, res.TotalFavorites)
	assert.Equal(t, 4, res.TotalDownloads)
	assert.Equal(t, 99.5, res.TotalRevenue)
	assert.Len(t, res.PlaysByDay, 1)
	assert.Len(t, res.DownloadsByDay, 1)
	assert.Len(t, res.RevenueByDay, 1)
	assert.Len(t, res.TopSpecs, 1)
	assert.Equal(t, 9.0, res.RevenueByLicense["basic"])
}

func TestAnalyticsService_GetStatsOverview_DaysFloor(t *testing.T) {
	ctx := context.Background()
	ar := new(mockAnalyticsRepository)
	svc := NewAnalyticsService(ar, new(mockSpecRepository))
	pid := uuid.New()
	ar.On("GetTotalPlays", ctx, pid, 1).Return(0, errors.New("x")).Once()
	_, err := svc.GetStatsOverview(ctx, pid, -4, "")
	assert.EqualError(t, err, "x")
}

func TestAnalyticsService_GetTopSpecs(t *testing.T) {
	ctx := context.Background()
	ar := new(mockAnalyticsRepository)
	svc := NewAnalyticsService(ar, new(mockSpecRepository))
	pid := uuid.New()
	ar.On("GetTopSpecs", ctx, pid, 3, "plays").Return([]analyticsDomain.TopSpecStat{{Title: "a"}}, nil).Once()
	got, err := svc.GetTopSpecs(ctx, pid, 3, "plays")
	require.NoError(t, err)
	assert.Len(t, got, 1)
}

func TestFavoriteCursor_RoundTripAndErrors(t *testing.T) {
	c := &analyticsDomain.FavoriteCursor{FavoritedAt: time.Now().UTC().Truncate(time.Second), SpecID: uuid.New()}
	enc, err := EncodeFavoriteCursor(c)
	require.NoError(t, err)
	dec, err := DecodeFavoriteCursor(enc)
	require.NoError(t, err)
	assert.True(t, c.FavoritedAt.Equal(dec.FavoritedAt))
	assert.Equal(t, c.SpecID, dec.SpecID)

	_, err = DecodeFavoriteCursor("!!!not base64!!!")
	assert.EqualError(t, err, "malformed cursor")
	// valid base64 but not JSON
	_, err = DecodeFavoriteCursor("bm90LWpzb24")
	assert.EqualError(t, err, "malformed cursor")
}

func TestAnalyticsService_ListMyFavorites(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	page := &analyticsDomain.FavoritePage{HasMore: true}

	t.Run("limit defaults and clamps, no cursor", func(t *testing.T) {
		ar := new(mockAnalyticsRepository)
		svc := NewAnalyticsService(ar, new(mockSpecRepository))
		var nilCursor *analyticsDomain.FavoriteCursor
		ar.On("ListUserFavorites", ctx, userID, 20, nilCursor).Return(page, nil).Once()
		ar.On("ListUserFavorites", ctx, userID, 50, nilCursor).Return(page, nil).Once()
		empty := ""
		got, err := svc.ListMyFavorites(ctx, userID, 0, &empty)
		require.NoError(t, err)
		assert.Same(t, page, got)
		_, err = svc.ListMyFavorites(ctx, userID, 999, nil)
		require.NoError(t, err)
		ar.AssertExpectations(t)
	})

	t.Run("valid cursor decoded", func(t *testing.T) {
		ar := new(mockAnalyticsRepository)
		svc := NewAnalyticsService(ar, new(mockSpecRepository))
		c := &analyticsDomain.FavoriteCursor{FavoritedAt: time.Now().UTC(), SpecID: uuid.New()}
		enc, err := EncodeFavoriteCursor(c)
		require.NoError(t, err)
		ar.On("ListUserFavorites", ctx, userID, 10, c).Return(page, nil).Once()
		_, err = svc.ListMyFavorites(ctx, userID, 10, &enc)
		require.NoError(t, err)
		ar.AssertExpectations(t)
	})

	t.Run("invalid cursor", func(t *testing.T) {
		ar := new(mockAnalyticsRepository)
		svc := NewAnalyticsService(ar, new(mockSpecRepository))
		bad := "%%%"
		_, err := svc.ListMyFavorites(ctx, userID, 10, &bad)
		assert.ErrorIs(t, err, ErrInvalidCursor)
		ar.AssertNotCalled(t, "ListUserFavorites")
	})

	t.Run("repo error wrapped", func(t *testing.T) {
		ar := new(mockAnalyticsRepository)
		svc := NewAnalyticsService(ar, new(mockSpecRepository))
		boom := errors.New("db")
		var nilCursor *analyticsDomain.FavoriteCursor
		ar.On("ListUserFavorites", ctx, userID, 20, nilCursor).Return(nil, boom).Once()
		got, err := svc.ListMyFavorites(ctx, userID, 20, nil)
		assert.Nil(t, got)
		assert.ErrorIs(t, err, boom)
		assert.Contains(t, err.Error(), "ListMyFavorites")
	})
}
