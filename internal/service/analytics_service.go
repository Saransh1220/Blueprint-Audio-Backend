package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/domain"
	"github.com/saransh1220/blueprint-audio/internal/dto"
)

// PublicAnalytics contains analytics visible to all users
type PublicAnalytics struct {
	PlayCount          int
	FavoriteCount      int
	TotalDownloadCount int
	IsFavorited        bool // Only if user is authenticated
}

// ProducerAnalytics contains full analytics for producers
type ProducerAnalytics struct {
	PublicAnalytics
	TotalPurchaseCount int
	PurchasesByLicense map[string]int
	TotalRevenue       float64
}

type AnalyticsServiceInterface interface {
	TrackPlay(ctx context.Context, specID uuid.UUID) error
	TrackFreeDownload(ctx context.Context, specID uuid.UUID) error

	ToggleFavorite(ctx context.Context, userID, specID uuid.UUID) (bool, error)
	IsFavorited(ctx context.Context, userID, specID uuid.UUID) (bool, error)

	GetPublicAnalytics(ctx context.Context, specID uuid.UUID, userID *uuid.UUID) (*PublicAnalytics, error)
	GetProducerAnalytics(ctx context.Context, specID, producerID uuid.UUID) (*ProducerAnalytics, error)
	GetStatsOverview(ctx context.Context, producerID uuid.UUID, days int, sortBy string) (*dto.AnalyticsOverviewResponse, error)
	GetTopSpecs(ctx context.Context, producerID uuid.UUID, limit int, sortBy string) ([]dto.TopSpecStat, error)
}

type analyticsService struct {
	analyticsRepo domain.AnalyticsRepository
	specRepo      domain.SpecRepository
}

func NewAnalyticsService(analyticsRepo domain.AnalyticsRepository, specRepo domain.SpecRepository) AnalyticsServiceInterface {
	return &analyticsService{
		analyticsRepo: analyticsRepo,
		specRepo:      specRepo,
	}
}

func (s *analyticsService) TrackPlay(ctx context.Context, specID uuid.UUID) error {
	return s.analyticsRepo.IncrementPlayCount(ctx, specID)
}

func (s *analyticsService) TrackFreeDownload(ctx context.Context, specID uuid.UUID) error {
	return s.analyticsRepo.IncrementFreeDownloadCount(ctx, specID)
}

func (s *analyticsService) ToggleFavorite(ctx context.Context, userID, specID uuid.UUID) (bool, error) {
	// Check current status
	isFavorited, err := s.analyticsRepo.IsFavorited(ctx, userID, specID)
	if err != nil {
		return false, err
	}

	// Toggle
	if isFavorited {
		err = s.analyticsRepo.RemoveFavorite(ctx, userID, specID)
		return false, err
	} else {
		err = s.analyticsRepo.AddFavorite(ctx, userID, specID)
		return true, err
	}
}

func (s *analyticsService) IsFavorited(ctx context.Context, userID, specID uuid.UUID) (bool, error) {
	return s.analyticsRepo.IsFavorited(ctx, userID, specID)
}

func (s *analyticsService) GetPublicAnalytics(ctx context.Context, specID uuid.UUID, userID *uuid.UUID) (*PublicAnalytics, error) {
	analytics, err := s.analyticsRepo.GetSpecAnalytics(ctx, specID)
	if err != nil {
		return nil, err
	}

	publicAnalytics := &PublicAnalytics{
		PlayCount:          analytics.PlayCount,
		FavoriteCount:      analytics.FavoriteCount,
		TotalDownloadCount: analytics.FreeDownloadCount, // Will be updated later to include purchased downloads
		IsFavorited:        false,
	}

	// Check if user has favorited (if authenticated)
	if userID != nil {
		isFavorited, err := s.analyticsRepo.IsFavorited(ctx, *userID, specID)
		if err == nil {
			publicAnalytics.IsFavorited = isFavorited
		}
	}

	return publicAnalytics, nil
}

func (s *analyticsService) GetProducerAnalytics(ctx context.Context, specID, producerID uuid.UUID) (*ProducerAnalytics, error) {
	// Verify ownership
	spec, err := s.specRepo.GetByID(ctx, specID)
	if err != nil {
		return nil, err
	}

	if spec.ProducerID != producerID {
		return nil, fmt.Errorf("unauthorized: user is not the producer of this spec")
	}

	// Get base analytics
	analytics, err := s.analyticsRepo.GetSpecAnalytics(ctx, specID)
	if err != nil {
		return nil, err
	}

	// Get license purchase counts
	licensePurchases, err := s.analyticsRepo.GetLicensePurchaseCounts(ctx, specID)
	if err != nil {
		return nil, err
	}

	// Calculate total purchases and revenue
	totalPurchases := 0
	totalRevenue := 0.0

	for licenseType, count := range licensePurchases {
		totalPurchases += count

		// Find the license price (simplified - assumes license options are loaded)
		for _, license := range spec.Licenses {
			if string(license.LicenseType) == licenseType {
				totalRevenue += float64(count) * license.Price
				break
			}
		}
	}

	producerAnalytics := &ProducerAnalytics{
		PublicAnalytics: PublicAnalytics{
			PlayCount:          analytics.PlayCount,
			FavoriteCount:      analytics.FavoriteCount,
			TotalDownloadCount: analytics.FreeDownloadCount, // + purchased downloads
		},
		TotalPurchaseCount: totalPurchases,
		PurchasesByLicense: licensePurchases,
		TotalRevenue:       totalRevenue,
	}

	return producerAnalytics, nil
}

func (s *analyticsService) GetStatsOverview(ctx context.Context, producerID uuid.UUID, days int, sortBy string) (*dto.AnalyticsOverviewResponse, error) {
	plays, err := s.analyticsRepo.GetTotalPlays(ctx, producerID)
	if err != nil {
		return nil, err
	}

	favorites, err := s.analyticsRepo.GetTotalFavorites(ctx, producerID)
	if err != nil {
		return nil, err
	}

	downloads, err := s.analyticsRepo.GetTotalDownloads(ctx, producerID)
	if err != nil {
		return nil, err
	}

	revenue, err := s.analyticsRepo.GetTotalRevenue(ctx, producerID)
	if err != nil {
		return nil, err
	}

	revByLicense, err := s.analyticsRepo.GetRevenueByLicenseGlobal(ctx, producerID)
	if err != nil {
		return nil, err
	}

	// Daily stats
	if days <= 0 {
		days = 30
	}
	dailyStats, err := s.analyticsRepo.GetPlaysByDay(ctx, producerID, days)
	if err != nil {
		return nil, err
	}
	var dStats []dto.DailyStat
	for _, d := range dailyStats {
		dStats = append(dStats, dto.DailyStat{Date: d.Date, Count: d.Count})
	}

	downloadsByDay, err := s.analyticsRepo.GetDownloadsByDay(ctx, producerID, days)
	if err != nil {
		return nil, err
	}
	var dlStats []dto.DailyStat
	for _, d := range downloadsByDay {
		dlStats = append(dlStats, dto.DailyStat{Date: d.Date, Count: d.Count})
	}

	revenueByDay, err := s.analyticsRepo.GetRevenueByDay(ctx, producerID, days)
	if err != nil {
		return nil, err
	}
	var revStats []dto.DailyRevenueStat
	for _, r := range revenueByDay {
		revStats = append(revStats, dto.DailyRevenueStat{Date: r.Date, Revenue: r.Revenue})
	}

	// Top specs
	if sortBy == "" {
		sortBy = "plays"
	}
	topSpecs, err := s.analyticsRepo.GetTopSpecs(ctx, producerID, 5, sortBy)
	if err != nil {
		return nil, err
	}
	var tStats []dto.TopSpecStat
	for _, t := range topSpecs {
		tStats = append(tStats, dto.TopSpecStat{
			SpecID: t.SpecID,
			Title:  t.Title,
			Plays:  t.Plays,
		})
	}

	return &dto.AnalyticsOverviewResponse{
		TotalPlays:       plays,
		TotalFavorites:   favorites,
		TotalRevenue:     revenue,
		TotalDownloads:   downloads,
		RevenueByLicense: revByLicense,
		PlaysByDay:       dStats,
		DownloadsByDay:   dlStats,
		RevenueByDay:     revStats,
		TopSpecs:         tStats,
	}, nil
}

func (s *analyticsService) GetTopSpecs(ctx context.Context, producerID uuid.UUID, limit int, sortBy string) ([]dto.TopSpecStat, error) {
	// Top Specs
	topSpecs, err := s.analyticsRepo.GetTopSpecs(ctx, producerID, limit, sortBy)
	if err != nil {
		return nil, err
	}

	var topSpecDtos []dto.TopSpecStat
	for _, spec := range topSpecs {
		topSpecDtos = append(topSpecDtos, dto.TopSpecStat{
			SpecID:    spec.SpecID,
			Title:     spec.Title,
			Plays:     spec.Plays,
			Downloads: spec.Downloads,
			Revenue:   spec.Revenue,
		})
	}
	return topSpecDtos, nil
}
