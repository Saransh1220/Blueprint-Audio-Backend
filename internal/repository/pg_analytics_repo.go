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

// IncrementPlayCount atomically increments the play count and logs an event
func (r *pgAnalyticsRepository) IncrementPlayCount(ctx context.Context, specID uuid.UUID) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 1. Update totals
	query := `
		INSERT INTO spec_analytics (spec_id, play_count)
		VALUES ($1, 1)
		ON CONFLICT (spec_id) 
		DO UPDATE SET 
			play_count = spec_analytics.play_count + 1,
			updated_at = NOW()`

	_, err = tx.ExecContext(ctx, query, specID)
	if err != nil {
		return fmt.Errorf("failed to increment play count: %w", err)
	}

	// 2. Log event
	eventQuery := `INSERT INTO analytics_events (spec_id, event_type) VALUES ($1, 'play')`
	_, err = tx.ExecContext(ctx, eventQuery, specID)
	if err != nil {
		return fmt.Errorf("failed to log play event: %w", err)
	}

	return tx.Commit()
}

// IncrementFreeDownloadCount atomically increments the free download count and logs an event
func (r *pgAnalyticsRepository) IncrementFreeDownloadCount(ctx context.Context, specID uuid.UUID) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 1. Update totals
	query := `
		INSERT INTO spec_analytics (spec_id, free_download_count)
		VALUES ($1, 1)
		ON CONFLICT (spec_id) 
		DO UPDATE SET 
			free_download_count = spec_analytics.free_download_count + 1,
			updated_at = NOW()`

	_, err = tx.ExecContext(ctx, query, specID)
	if err != nil {
		return fmt.Errorf("failed to increment free download count: %w", err)
	}

	// 2. Log event
	eventQuery := `INSERT INTO analytics_events (spec_id, event_type) VALUES ($1, 'download')`
	_, err = tx.ExecContext(ctx, eventQuery, specID)
	if err != nil {
		return fmt.Errorf("failed to log download event: %w", err)
	}

	return tx.Commit()
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

	// 3. Log event
	eventQuery := `INSERT INTO analytics_events (spec_id, event_type, user_id) VALUES ($1, 'favorite', $2)`
	_, err = tx.ExecContext(ctx, eventQuery, specID, userID)
	if err != nil {
		return fmt.Errorf("failed to log favorite event: %w", err)
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
	if days <= 0 {
		days = 30
	}

	query := `
		SELECT 
			to_char(date_trunc('day', ae.created_at), 'YYYY-MM-DD') as date,
			COUNT(*) as count
		FROM analytics_events ae
		JOIN specs s ON ae.spec_id = s.id
		WHERE s.producer_id = $1 
		  AND ae.event_type = 'play'
		  AND ae.created_at > NOW() - ($2 || ' days')::INTERVAL
		GROUP BY 1
		ORDER BY 1 ASC
	`
	var stats []domain.DailyStat
	err := r.db.SelectContext(ctx, &stats, query, producerID, days)
	if err != nil {
		return nil, fmt.Errorf("failed to get plays by day: %w", err)
	}
	return stats, nil
}

func (r *pgAnalyticsRepository) GetDownloadsByDay(ctx context.Context, producerID uuid.UUID, days int) ([]domain.DailyStat, error) {
	if days <= 0 {
		days = 30
	}

	query := `
		SELECT 
			to_char(date_trunc('day', ae.created_at), 'YYYY-MM-DD') as date,
			COUNT(*) as count
		FROM analytics_events ae
		JOIN specs s ON ae.spec_id = s.id
		WHERE s.producer_id = $1 
		  AND ae.event_type = 'download'
		  AND ae.created_at > NOW() - ($2 || ' days')::INTERVAL
		GROUP BY 1
		ORDER BY 1 ASC
	`
	var stats []domain.DailyStat
	err := r.db.SelectContext(ctx, &stats, query, producerID, days)
	if err != nil {
		return nil, fmt.Errorf("failed to get downloads by day: %w", err)
	}
	return stats, nil
}

func (r *pgAnalyticsRepository) GetRevenueByDay(ctx context.Context, producerID uuid.UUID, days int) ([]domain.DailyRevenueStat, error) {
	if days <= 0 {
		days = 30
	}

	query := `
		SELECT 
			to_char(date_trunc('day', o.created_at), 'YYYY-MM-DD') as date,
			COALESCE(SUM(o.amount), 0) / 100.0 as revenue
		FROM orders o
		JOIN specs s ON o.spec_id = s.id
		WHERE s.producer_id = $1
		  AND o.status = 'paid'
		  AND o.created_at > NOW() - ($2 || ' days')::INTERVAL
		GROUP BY 1
		ORDER BY 1 ASC
	`
	var stats []domain.DailyRevenueStat
	err := r.db.SelectContext(ctx, &stats, query, producerID, days)
	if err != nil {
		return nil, fmt.Errorf("failed to get revenue by day: %w", err)
	}
	return stats, nil
}

func (r *pgAnalyticsRepository) GetTopSpecs(ctx context.Context, producerID uuid.UUID, limit int, sortBy string) ([]domain.TopSpecStat, error) {
	var stats []domain.TopSpecStat

	orderBy := "sa.play_count DESC"
	switch sortBy {
	case "revenue":
		orderBy = "revenue DESC"
	case "downloads":
		orderBy = "downloads DESC"
	case "plays":
		orderBy = "plays DESC"
	}

	query := fmt.Sprintf(`
		SELECT 
			s.id as spec_id, 
			s.title, 
			COALESCE(sa.play_count, 0) as plays,
			COALESCE(sa.free_download_count, 0) as downloads,
			COALESCE(SUM(o.amount), 0) / 100.0 as revenue
		FROM specs s
		LEFT JOIN spec_analytics sa ON s.id = sa.spec_id
		LEFT JOIN orders o ON s.id = o.spec_id AND o.status = 'paid'
		WHERE s.producer_id = $1
		GROUP BY s.id, s.title, sa.play_count, sa.free_download_count
		ORDER BY %s
		LIMIT $2`, orderBy)

	err := r.db.SelectContext(ctx, &stats, query, producerID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get top specs: %w", err)
	}
	return stats, nil
}
