package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/saransh1220/blueprint-audio/internal/domain"
)

type pgAnalyticsRepository struct {
	db *sqlx.DB
}

func NewAnalyticsRepository(db *sqlx.DB) domain.AnalyticsRepository {
	return &pgAnalyticsRepository{db: db}
}

// GetSpecAnalytics retrieves analytics for a spec, creates record if missing
func (r *pgAnalyticsRepository) GetSpecAnalytics(ctx context.Context, specID uuid.UUID) (*domain.SpecAnalytics, error) {
	analytics := &domain.SpecAnalytics{}

	query := `SELECT * FROM spec_analytics WHERE spec_id = $1`
	err := r.db.GetContext(ctx, analytics, query, specID)

	if err == sql.ErrNoRows {
		// Create analytics record if it doesn't exist
		createQuery := `
			INSERT INTO spec_analytics (spec_id)
			VALUES ($1)
			RETURNING *`
		err = r.db.GetContext(ctx, analytics, createQuery, specID)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get spec analytics: %w", err)
	}

	return analytics, nil
}

// IncrementPlayCount atomically increments the play count
func (r *pgAnalyticsRepository) IncrementPlayCount(ctx context.Context, specID uuid.UUID) error {
	query := `
		INSERT INTO spec_analytics (spec_id, play_count)
		VALUES ($1, 1)
		ON CONFLICT (spec_id) 
		DO UPDATE SET 
			play_count = spec_analytics.play_count + 1,
			updated_at = NOW()`

	_, err := r.db.ExecContext(ctx, query, specID)
	if err != nil {
		return fmt.Errorf("failed to increment play count: %w", err)
	}

	return nil
}

// IncrementFreeDownloadCount atomically increments the free download count
func (r *pgAnalyticsRepository) IncrementFreeDownloadCount(ctx context.Context, specID uuid.UUID) error {
	query := `
		INSERT INTO spec_analytics (spec_id, free_download_count)
		VALUES ($1, 1)
		ON CONFLICT (spec_id) 
		DO UPDATE SET 
			free_download_count = spec_analytics.free_download_count + 1,
			updated_at = NOW()`

	_, err := r.db.ExecContext(ctx, query, specID)
	if err != nil {
		return fmt.Errorf("failed to increment free download count: %w", err)
	}

	return nil
}

// AddFavorite adds a favorite and increments the count
func (r *pgAnalyticsRepository) AddFavorite(ctx context.Context, userID, specID uuid.UUID) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Insert favorite
	favoriteQuery := `
		INSERT INTO user_favorites (user_id, spec_id)
		VALUES ($1, $2)
		ON CONFLICT (user_id, spec_id) DO NOTHING`

	result, err := tx.ExecContext(ctx, favoriteQuery, userID, specID)
	if err != nil {
		return fmt.Errorf("failed to add favorite: %w", err)
	}

	// Check if row was actually inserted
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		// Already favorited, nothing to do
		return nil
	}

	// Increment favorite count
	analyticsQuery := `
		INSERT INTO spec_analytics (spec_id, favorite_count)
		VALUES ($1, 1)
		ON CONFLICT (spec_id) 
		DO UPDATE SET 
			favorite_count = spec_analytics.favorite_count + 1,
			updated_at = NOW()`

	_, err = tx.ExecContext(ctx, analyticsQuery, specID)
	if err != nil {
		return fmt.Errorf("failed to increment favorite count: %w", err)
	}

	return tx.Commit()
}

// RemoveFavorite removes a favorite and decrements the count
func (r *pgAnalyticsRepository) RemoveFavorite(ctx context.Context, userID, specID uuid.UUID) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Delete favorite
	favoriteQuery := `DELETE FROM user_favorites WHERE user_id = $1 AND spec_id = $2`
	result, err := tx.ExecContext(ctx, favoriteQuery, userID, specID)
	if err != nil {
		return fmt.Errorf("failed to remove favorite: %w", err)
	}

	// Check if row was actually deleted
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		// Not favorited, nothing to do
		return nil
	}

	// Decrement favorite count
	analyticsQuery := `
		UPDATE spec_analytics 
		SET favorite_count = GREATEST(favorite_count - 1, 0),
		    updated_at = NOW()
		WHERE spec_id = $1`

	_, err = tx.ExecContext(ctx, analyticsQuery, specID)
	if err != nil {
		return fmt.Errorf("failed to decrement favorite count: %w", err)
	}

	return tx.Commit()
}

// IsFavorited checks if a user has favorited a spec
func (r *pgAnalyticsRepository) IsFavorited(ctx context.Context, userID, specID uuid.UUID) (bool, error) {
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM user_favorites WHERE user_id = $1 AND spec_id = $2)`

	err := r.db.GetContext(ctx, &exists, query, userID, specID)
	if err != nil {
		return false, fmt.Errorf("failed to check favorite status: %w", err)
	}

	return exists, nil
}

// GetLicensePurchaseCounts returns purchase counts grouped by license type
func (r *pgAnalyticsRepository) GetLicensePurchaseCounts(ctx context.Context, specID uuid.UUID) (map[string]int, error) {
	type licenseCount struct {
		LicenseType string `db:"license_type"`
		Count       int    `db:"count"`
	}

	query := `
		SELECT 
			o.license_type,
			COUNT(*) as count
		FROM orders o
		WHERE o.spec_id = $1 
		  AND o.status = 'paid'
		GROUP BY o.license_type`

	var results []licenseCount
	err := r.db.SelectContext(ctx, &results, query, specID)
	if err != nil {
		return nil, fmt.Errorf("failed to get license purchase counts: %w", err)
	}

	counts := make(map[string]int)
	for _, result := range results {
		counts[result.LicenseType] = result.Count
	}

	return counts, nil
}

// Overview Analytics Implementations

func (r *pgAnalyticsRepository) GetTotalPlays(ctx context.Context, producerID uuid.UUID) (int, error) {
	var total int
	query := `
		SELECT COALESCE(SUM(play_count), 0)
		FROM spec_analytics sa
		JOIN specs s ON sa.spec_id = s.id
		WHERE s.producer_id = $1`
	err := r.db.GetContext(ctx, &total, query, producerID)
	return total, err
}

func (r *pgAnalyticsRepository) GetTotalFavorites(ctx context.Context, producerID uuid.UUID) (int, error) {
	var total int
	query := `
		SELECT COALESCE(SUM(favorite_count), 0)
		FROM spec_analytics sa
		JOIN specs s ON sa.spec_id = s.id
		WHERE s.producer_id = $1`
	err := r.db.GetContext(ctx, &total, query, producerID)
	return total, err
}

func (r *pgAnalyticsRepository) GetTotalDownloads(ctx context.Context, producerID uuid.UUID) (int, error) {
	var total int
	query := `
		SELECT COALESCE(SUM(free_download_count), 0)
		FROM spec_analytics sa
		JOIN specs s ON sa.spec_id = s.id
		WHERE s.producer_id = $1`
	err := r.db.GetContext(ctx, &total, query, producerID)
	return total, err
}

func (r *pgAnalyticsRepository) GetTotalRevenue(ctx context.Context, producerID uuid.UUID) (float64, error) {
	var total float64
	query := `
		SELECT COALESCE(SUM(amount), 0) / 100.0
		FROM orders o
		JOIN specs s ON o.spec_id = s.id
		WHERE s.producer_id = $1 AND o.status = 'paid'`
	err := r.db.GetContext(ctx, &total, query, producerID)
	return total, err
}

func (r *pgAnalyticsRepository) GetRevenueByLicenseGlobal(ctx context.Context, producerID uuid.UUID) (map[string]float64, error) {
	type licenseRev struct {
		LicenseType string  `db:"license_type"`
		Revenue     float64 `db:"revenue"`
	}
	query := `
		SELECT o.license_type, COALESCE(SUM(o.amount), 0) / 100.0 as revenue
		FROM orders o
		JOIN specs s ON o.spec_id = s.id
		WHERE s.producer_id = $1 AND o.status = 'paid'
		GROUP BY o.license_type`
	var rows []licenseRev
	err := r.db.SelectContext(ctx, &rows, query, producerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get global revenue by license: %w", err)
	}

	result := make(map[string]float64)
	for _, row := range rows {
		result[row.LicenseType] = row.Revenue
	}
	return result, nil
}

func (r *pgAnalyticsRepository) GetPlaysByDay(ctx context.Context, producerID uuid.UUID, days int) ([]domain.DailyStat, error) {
	// TODO: Implement time-series tracking. Current schema only stores totals.
	// Returning empty list for now.
	return []domain.DailyStat{}, nil
}

func (r *pgAnalyticsRepository) GetTopSpecs(ctx context.Context, producerID uuid.UUID, limit int) ([]domain.TopSpecStat, error) {
	var stats []domain.TopSpecStat
	query := `
		SELECT s.id as spec_id, s.title, sa.play_count as plays
		FROM spec_analytics sa
		JOIN specs s ON sa.spec_id = s.id
		WHERE s.producer_id = $1
		ORDER BY sa.play_count DESC
		LIMIT $2`
	err := r.db.SelectContext(ctx, &stats, query, producerID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get top specs: %w", err)
	}
	return stats, nil
}
