package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/modules/analytics/domain"
	catalogDomain "github.com/saransh1220/blueprint-audio/internal/modules/catalog/domain"
)

type AnalyticsService interface {
	TrackPlay(ctx context.Context, specID uuid.UUID) error
	TrackFreeDownload(ctx context.Context, specID uuid.UUID) error

	ToggleFavorite(ctx context.Context, userID, specID uuid.UUID) (bool, error)
	IsFavorited(ctx context.Context, userID, specID uuid.UUID) (bool, error)

	GetPublicAnalytics(ctx context.Context, specID uuid.UUID, userID *uuid.UUID) (*domain.PublicAnalytics, error)
	GetProducerAnalytics(ctx context.Context, specID, producerID uuid.UUID) (*domain.ProducerAnalytics, error)
	GetStatsOverview(ctx context.Context, producerID uuid.UUID, days int, sortBy string) (*domain.AnalyticsOverviewResponse, error)
	GetTopSpecs(ctx context.Context, producerID uuid.UUID, limit int, sortBy string) ([]domain.TopSpecStat, error)
}

type analyticsService struct {
	repo     domain.AnalyticsRepository
	specRepo catalogDomain.SpecRepository // Dependency on Catalog module
}

func NewAnalyticsService(repo domain.AnalyticsRepository, specRepo catalogDomain.SpecRepository) AnalyticsService {
	return &analyticsService{
		repo:     repo,
		specRepo: specRepo,
	}
}

func (s *analyticsService) TrackPlay(ctx context.Context, specID uuid.UUID) error {
	return s.repo.IncrementPlayCount(ctx, specID)
}

func (s *analyticsService) TrackFreeDownload(ctx context.Context, specID uuid.UUID) error {
	return s.repo.IncrementFreeDownloadCount(ctx, specID)
}

func (s *analyticsService) ToggleFavorite(ctx context.Context, userID, specID uuid.UUID) (bool, error) {
	isFavorited, err := s.repo.IsFavorited(ctx, userID, specID)
	if err != nil {
		return false, err
	}

	if isFavorited {
		if err := s.repo.RemoveFavorite(ctx, userID, specID); err != nil {
			return true, err // Still favorited if remove failed
		}
		return false, nil
	}

	if err := s.repo.AddFavorite(ctx, userID, specID); err != nil {
		return false, err
	}
	return true, nil
}

func (s *analyticsService) IsFavorited(ctx context.Context, userID, specID uuid.UUID) (bool, error) {
	return s.repo.IsFavorited(ctx, userID, specID)
}

func (s *analyticsService) GetPublicAnalytics(ctx context.Context, specID uuid.UUID, userID *uuid.UUID) (*domain.PublicAnalytics, error) {
	analytics, err := s.repo.GetSpecAnalytics(ctx, specID)
	if err != nil {
		return nil, err
	}

	isFavorited := false
	if userID != nil {
		isFavorited, err = s.repo.IsFavorited(ctx, *userID, specID)
		if err != nil {
			// Log error but continue
			fmt.Printf("Error checking favorite: %v\n", err)
		}
	}

	return &domain.PublicAnalytics{
		PlayCount:          analytics.PlayCount,
		FavoriteCount:      analytics.FavoriteCount,
		TotalDownloadCount: analytics.FreeDownloadCount,
		IsFavorited:        isFavorited,
	}, nil
}

func (s *analyticsService) GetProducerAnalytics(ctx context.Context, specID, producerID uuid.UUID) (*domain.ProducerAnalytics, error) {
	// Verify ownership using SpecRepo
	spec, err := s.specRepo.GetByID(ctx, specID)
	if err != nil {
		return nil, err
	}
	if spec == nil {
		return nil, errors.New("spec not found")
	}
	if spec.ProducerID != producerID {
		return nil, errors.New("unauthorized")
	}

	analytics, err := s.repo.GetSpecAnalytics(ctx, specID)
	if err != nil {
		return nil, err
	}

	licensePurchases, err := s.repo.GetLicensePurchaseCounts(ctx, specID)
	if err != nil {
		return nil, err
	}
	analytics.LicensePurchases = licensePurchases

	return &domain.ProducerAnalytics{
		PlayCount:          analytics.PlayCount,
		FavoriteCount:      analytics.FavoriteCount,
		FreeDownloadCount:  analytics.FreeDownloadCount,
		TotalPurchaseCount: analytics.TotalPurchaseCount,
		LicensePurchases:   analytics.LicensePurchases,
	}, nil
}

func (s *analyticsService) GetStatsOverview(ctx context.Context, producerID uuid.UUID, days int, sortBy string) (*domain.AnalyticsOverviewResponse, error) {
	totalPlays, err := s.repo.GetTotalPlays(ctx, producerID)
	if err != nil {
		return nil, err
	}
	totalFavorites, err := s.repo.GetTotalFavorites(ctx, producerID)
	if err != nil {
		return nil, err
	}
	totalDownloads, err := s.repo.GetTotalDownloads(ctx, producerID)
	if err != nil {
		return nil, err
	}
	totalRevenue, err := s.repo.GetTotalRevenue(ctx, producerID)
	if err != nil {
		return nil, err
	}

	playsByDay, err := s.repo.GetPlaysByDay(ctx, producerID, days)
	if err != nil {
		return nil, err
	}
	downloadsByDay, err := s.repo.GetDownloadsByDay(ctx, producerID, days)
	if err != nil {
		return nil, err
	}
	revenueByDay, err := s.repo.GetRevenueByDay(ctx, producerID, days)
	if err != nil {
		return nil, err
	}

	topSpecs, err := s.repo.GetTopSpecs(ctx, producerID, 5, sortBy)
	if err != nil {
		return nil, err
	}

	revenueByLicense, err := s.repo.GetRevenueByLicenseGlobal(ctx, producerID)
	if err != nil {
		return nil, err
	}

	return &domain.AnalyticsOverviewResponse{
		TotalPlays:       totalPlays,
		TotalFavorites:   totalFavorites,
		TotalRevenue:     totalRevenue,
		TotalDownloads:   totalDownloads,
		PlaysByDay:       playsByDay,
		DownloadsByDay:   downloadsByDay,
		RevenueByDay:     revenueByDay,
		TopSpecs:         topSpecs,
		RevenueByLicense: revenueByLicense,
	}, nil
}

func (s *analyticsService) GetTopSpecs(ctx context.Context, producerID uuid.UUID, limit int, sortBy string) ([]domain.TopSpecStat, error) {
	return s.repo.GetTopSpecs(ctx, producerID, limit, sortBy)
}
