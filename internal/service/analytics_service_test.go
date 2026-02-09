package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/domain"
	"github.com/saransh1220/blueprint-audio/internal/service"
	"github.com/stretchr/testify/assert"
)

func TestAnalyticsService_ToggleFavorite(t *testing.T) {
	ctx := context.Background()
	ar := new(mockAnalyticsRepository)
	sr := new(mockSpecRepository)
	svc := service.NewAnalyticsService(ar, sr)
	userID := uuid.New()
	specID := uuid.New()

	ar.On("IsFavorited", ctx, userID, specID).Return(true, nil).Once()
	ar.On("RemoveFavorite", ctx, userID, specID).Return(nil).Once()
	val, err := svc.ToggleFavorite(ctx, userID, specID)
	assert.NoError(t, err)
	assert.False(t, val)

	ar.On("IsFavorited", ctx, userID, specID).Return(false, nil).Once()
	ar.On("AddFavorite", ctx, userID, specID).Return(nil).Once()
	val, err = svc.ToggleFavorite(ctx, userID, specID)
	assert.NoError(t, err)
	assert.True(t, val)

	ar.On("IsFavorited", ctx, userID, specID).Return(false, errors.New("db")).Once()
	_, err = svc.ToggleFavorite(ctx, userID, specID)
	assert.EqualError(t, err, "db")
}

func TestAnalyticsService_GetPublicAnalytics(t *testing.T) {
	ctx := context.Background()
	ar := new(mockAnalyticsRepository)
	sr := new(mockSpecRepository)
	svc := service.NewAnalyticsService(ar, sr)
	specID := uuid.New()
	userID := uuid.New()
	base := &domain.SpecAnalytics{PlayCount: 10, FavoriteCount: 2, FreeDownloadCount: 3}

	ar.On("GetSpecAnalytics", ctx, specID).Return(base, nil).Once()
	ar.On("IsFavorited", ctx, userID, specID).Return(true, nil).Once()

	out, err := svc.GetPublicAnalytics(ctx, specID, &userID)
	assert.NoError(t, err)
	assert.Equal(t, 10, out.PlayCount)
	assert.True(t, out.IsFavorited)

	ar.On("GetSpecAnalytics", ctx, specID).Return(nil, errors.New("db")).Once()
	_, err = svc.GetPublicAnalytics(ctx, specID, nil)
	assert.EqualError(t, err, "db")

	ar.On("GetSpecAnalytics", ctx, specID).Return(base, nil).Once()
	ar.On("IsFavorited", ctx, userID, specID).Return(false, errors.New("ignore")).Once()
	out, err = svc.GetPublicAnalytics(ctx, specID, &userID)
	assert.NoError(t, err)
	assert.False(t, out.IsFavorited)
}

func TestAnalyticsService_GetProducerAnalyticsAndOverview(t *testing.T) {
	ctx := context.Background()
	ar := new(mockAnalyticsRepository)
	sr := new(mockSpecRepository)
	svc := service.NewAnalyticsService(ar, sr)

	specID := uuid.New()
	producerID := uuid.New()
	sr.On("GetByID", ctx, specID).Return(&domain.Spec{
		ID:         specID,
		ProducerID: producerID,
		Licenses: []domain.LicenseOption{
			{LicenseType: domain.LicenseBasic, Price: 100},
			{LicenseType: domain.LicensePremium, Price: 250},
		},
	}, nil).Once()
	ar.On("GetSpecAnalytics", ctx, specID).Return(&domain.SpecAnalytics{PlayCount: 5, FavoriteCount: 1, FreeDownloadCount: 2}, nil).Once()
	ar.On("GetLicensePurchaseCounts", ctx, specID).Return(map[string]int{"Basic": 2, "Premium": 1}, nil).Once()

	pa, err := svc.GetProducerAnalytics(ctx, specID, producerID)
	assert.NoError(t, err)
	assert.Equal(t, 3, pa.TotalPurchaseCount)
	assert.Equal(t, 450.0, pa.TotalRevenue)

	spec2 := uuid.New()
	sr.On("GetByID", ctx, spec2).Return(&domain.Spec{ID: spec2, ProducerID: uuid.New()}, nil).Once()
	_, err = svc.GetProducerAnalytics(ctx, spec2, producerID)
	assert.EqualError(t, err, "unauthorized: user is not the producer of this spec")

	ar.On("GetTotalPlays", ctx, producerID).Return(10, nil)
	ar.On("GetTotalFavorites", ctx, producerID).Return(5, nil)
	ar.On("GetTotalDownloads", ctx, producerID).Return(2, nil)
	ar.On("GetTotalRevenue", ctx, producerID).Return(33.5, nil)
	ar.On("GetRevenueByLicenseGlobal", ctx, producerID).Return(map[string]float64{"Basic": 10}, nil)
	ar.On("GetPlaysByDay", ctx, producerID, 30).Return([]domain.DailyStat{{Date: "2026-02-01", Count: 1}}, nil)
	ar.On("GetTopSpecs", ctx, producerID, 5).Return([]domain.TopSpecStat{{SpecID: specID.String(), Title: "A", Plays: 7}}, nil)

	overview, err := svc.GetStatsOverview(ctx, producerID, 0)
	assert.NoError(t, err)
	assert.Equal(t, 10, overview.TotalPlays)
	assert.Len(t, overview.PlaysByDay, 1)

	specErrID := uuid.New()
	sr.On("GetByID", ctx, specErrID).Return(nil, errors.New("spec err")).Once()
	_, err = svc.GetProducerAnalytics(ctx, specErrID, producerID)
	assert.EqualError(t, err, "spec err")

	spec3 := uuid.New()
	sr.On("GetByID", ctx, spec3).Return(&domain.Spec{ID: spec3, ProducerID: producerID}, nil).Once()
	ar.On("GetSpecAnalytics", ctx, spec3).Return(nil, errors.New("analytics err")).Once()
	_, err = svc.GetProducerAnalytics(ctx, spec3, producerID)
	assert.EqualError(t, err, "analytics err")

	spec4 := uuid.New()
	sr.On("GetByID", ctx, spec4).Return(&domain.Spec{ID: spec4, ProducerID: producerID}, nil).Once()
	ar.On("GetSpecAnalytics", ctx, spec4).Return(&domain.SpecAnalytics{}, nil).Once()
	ar.On("GetLicensePurchaseCounts", ctx, spec4).Return(nil, errors.New("license err")).Once()
	_, err = svc.GetProducerAnalytics(ctx, spec4, producerID)
	assert.EqualError(t, err, "license err")
}

func TestAnalyticsService_TrackAndIsFavorited(t *testing.T) {
	ctx := context.Background()
	ar := new(mockAnalyticsRepository)
	sr := new(mockSpecRepository)
	svc := service.NewAnalyticsService(ar, sr)
	userID := uuid.New()
	specID := uuid.New()

	ar.On("IncrementPlayCount", ctx, specID).Return(nil).Once()
	assert.NoError(t, svc.TrackPlay(ctx, specID))

	ar.On("IncrementFreeDownloadCount", ctx, specID).Return(nil).Once()
	assert.NoError(t, svc.TrackFreeDownload(ctx, specID))

	ar.On("IsFavorited", ctx, userID, specID).Return(true, nil).Once()
	fav, err := svc.IsFavorited(ctx, userID, specID)
	assert.NoError(t, err)
	assert.True(t, fav)
}
