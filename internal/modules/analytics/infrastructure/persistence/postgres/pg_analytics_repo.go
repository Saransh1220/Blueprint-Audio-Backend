package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/saransh1220/blueprint-audio/internal/modules/analytics/domain"
	catalogDomain "github.com/saransh1220/blueprint-audio/internal/modules/catalog/domain"
)

type PgAnalyticsRepository struct {
	db *sqlx.DB
}

func NewAnalyticsRepository(db *sqlx.DB) domain.AnalyticsRepository {
	return &PgAnalyticsRepository{db: db}
}

// GetSpecAnalytics retrieves analytics for a spec, creates record if missing
func (r *PgAnalyticsRepository) GetSpecAnalytics(ctx context.Context, specID uuid.UUID) (*domain.SpecAnalytics, error) {
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
func (r *PgAnalyticsRepository) IncrementPlayCount(ctx context.Context, specID uuid.UUID) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	eventID, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("failed to generate analytics event id: %w", err)
	}

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
	eventQuery := `INSERT INTO analytics_events (id, spec_id, event_type) VALUES ($1, $2, 'play')`
	_, err = tx.ExecContext(ctx, eventQuery, eventID, specID)
	if err != nil {
		return fmt.Errorf("failed to log play event: %w", err)
	}

	return tx.Commit()
}

// IncrementFreeDownloadCount atomically increments the free download count and logs an event
func (r *PgAnalyticsRepository) IncrementFreeDownloadCount(ctx context.Context, specID uuid.UUID) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	eventID, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("failed to generate analytics event id: %w", err)
	}

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
	eventQuery := `INSERT INTO analytics_events (id, spec_id, event_type) VALUES ($1, $2, 'download')`
	_, err = tx.ExecContext(ctx, eventQuery, eventID, specID)
	if err != nil {
		return fmt.Errorf("failed to log download event: %w", err)
	}

	return tx.Commit()
}

// AddFavorite adds a favorite and increments the count
func (r *PgAnalyticsRepository) AddFavorite(ctx context.Context, userID, specID uuid.UUID) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	eventID, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("failed to generate analytics event id: %w", err)
	}

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
	eventQuery := `INSERT INTO analytics_events (id, spec_id, event_type, user_id) VALUES ($1, $2, 'favorite', $3)`
	_, err = tx.ExecContext(ctx, eventQuery, eventID, specID, userID)
	if err != nil {
		return fmt.Errorf("failed to log favorite event: %w", err)
	}

	return tx.Commit()
}

// RemoveFavorite removes a favorite and decrements the count
func (r *PgAnalyticsRepository) RemoveFavorite(ctx context.Context, userID, specID uuid.UUID) error {
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
func (r *PgAnalyticsRepository) IsFavorited(ctx context.Context, userID, specID uuid.UUID) (bool, error) {
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM user_favorites WHERE user_id = $1 AND spec_id = $2)`

	err := r.db.GetContext(ctx, &exists, query, userID, specID)
	if err != nil {
		return false, fmt.Errorf("failed to check favorite status: %w", err)
	}

	return exists, nil
}

// ListUserFavorites returns a cursor-paginated page of the user's favorited specs.
//
// The cursor encodes (favorited_at DESC, spec_id DESC) so pages are stable even
// when concurrent favorites are added. We fetch limit+1 rows to detect whether
// another page exists, then trim the slice before returning.
func (r *PgAnalyticsRepository) ListUserFavorites(ctx context.Context, userID uuid.UUID, limit int, cursor *domain.FavoriteCursor) (*domain.FavoritePage, error) {
	const maxLimit = 100
	if limit <= 0 || limit > maxLimit {
		limit = 20
	}

	// Base query — fetch limit+1 to check for more pages.
	// We join specs, producers (users) and exclude soft-deleted specs.
	args := []interface{}{userID}
	argIdx := 2

	query := `
		SELECT
			uf.created_at  AS favorited_at,
			s.id, s.producer_id, s.title, s.category, s.type, s.bpm, s.key,
			s.image_url, s.preview_url, s.wav_url, s.stems_url,
			s.base_price, s.price_currency, s.description, s.duration,
			s.free_mp3_enabled, s.created_at, s.updated_at, s.deleted_at,
			s.is_deleted, s.moods, s.instruments, s.waveform_peaks,
			s.slug, s.short_code, s.processing_status,
			u.display_name AS producer_name,
			'' AS producer_handle,
			s.tags
		FROM user_favorites uf
		JOIN specs s ON s.id = uf.spec_id
		JOIN users u ON u.id = s.producer_id
		WHERE uf.user_id = $1
		  AND s.is_deleted = FALSE
`

	// Keyset: continue after last seen (favorited_at DESC, spec_id DESC)
	if cursor != nil {
		query += fmt.Sprintf(`
		  AND (uf.created_at, uf.spec_id) < ($%d, $%d)
`, argIdx, argIdx+1)
		args = append(args, cursor.FavoritedAt, cursor.SpecID)
		argIdx += 2
	}

	query += fmt.Sprintf(`
		ORDER BY uf.created_at DESC, uf.spec_id DESC
		LIMIT $%d
`, argIdx)
	args = append(args, limit+1)

	// rawRow embeds the Spec domain struct and adds the favorited_at column.
	// sqlx scans PostgreSQL timestamptz directly into time.Time.
	type rawRow struct {
		FavoritedAt time.Time `db:"favorited_at"`
		catalogDomain.Spec
	}

	rows, err := r.db.QueryxContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("ListUserFavorites query: %w", err)
	}
	defer rows.Close()

	var rawRows []rawRow
	for rows.Next() {
		var rr rawRow
		if err := rows.StructScan(&rr); err != nil {
			return nil, fmt.Errorf("ListUserFavorites scan: %w", err)
		}
		rawRows = append(rawRows, rr)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ListUserFavorites rows: %w", err)
	}

	hasMore := len(rawRows) > limit
	if hasMore {
		rawRows = rawRows[:limit]
	}

	// Enrich each spec with its licenses and genres (N+1 is acceptable for
	// small pages; a single query with JSON aggregation can replace this later).
	items := make([]domain.FavoriteItem, 0, len(rawRows))
	for _, rr := range rawRows {
		spec := rr.Spec

		// Licenses
		licenseQuery := `SELECT * FROM license_options WHERE spec_id = $1 AND is_deleted = FALSE`
		if err := r.db.SelectContext(ctx, &spec.Licenses, licenseQuery, spec.ID); err != nil {
			return nil, fmt.Errorf("ListUserFavorites licenses: %w", err)
		}

		// Genres
		genreQuery := `SELECT g.* FROM genres g JOIN spec_genres sg ON g.id = sg.genre_id WHERE sg.spec_id = $1`
		if err := r.db.SelectContext(ctx, &spec.Genres, genreQuery, spec.ID); err != nil {
			return nil, fmt.Errorf("ListUserFavorites genres: %w", err)
		}

		items = append(items, domain.FavoriteItem{
			Spec:        spec,
			FavoritedAt: rr.FavoritedAt,
		})
	}

	page := &domain.FavoritePage{
		Items:   items,
		HasMore: hasMore,
	}

	if hasMore && len(items) > 0 {
		last := items[len(items)-1]
		page.NextCursor = &domain.FavoriteCursor{
			FavoritedAt: last.FavoritedAt,
			SpecID:      last.Spec.ID,
		}
	}

	return page, nil
}

// GetLicensePurchaseCounts returns purchase counts grouped by license type
func (r *PgAnalyticsRepository) GetLicensePurchaseCounts(ctx context.Context, specID uuid.UUID) (map[string]int, error) {
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

func (r *PgAnalyticsRepository) GetTotalPlays(ctx context.Context, producerID uuid.UUID, days int) (int, error) {
	if days <= 0 {
		days = 30
	}
	var total int
	query := `
		SELECT COUNT(*)
		FROM analytics_events ae
		JOIN specs s ON ae.spec_id = s.id
		WHERE s.producer_id = $1
		  AND ae.event_type = 'play'
		  AND ae.created_at > NOW() - ($2 || ' days')::INTERVAL`
	err := r.db.GetContext(ctx, &total, query, producerID, days)
	return total, err
}

func (r *PgAnalyticsRepository) GetTotalFavorites(ctx context.Context, producerID uuid.UUID, days int) (int, error) {
	if days <= 0 {
		days = 30
	}
	var total int
	query := `
		SELECT COUNT(*)
		FROM analytics_events ae
		JOIN specs s ON ae.spec_id = s.id
		WHERE s.producer_id = $1
		  AND ae.event_type = 'favorite'
		  AND ae.created_at > NOW() - ($2 || ' days')::INTERVAL`
	err := r.db.GetContext(ctx, &total, query, producerID, days)
	return total, err
}

func (r *PgAnalyticsRepository) GetTotalDownloads(ctx context.Context, producerID uuid.UUID, days int) (int, error) {
	if days <= 0 {
		days = 30
	}
	var total int
	query := `
		SELECT COUNT(*)
		FROM analytics_events ae
		JOIN specs s ON ae.spec_id = s.id
		WHERE s.producer_id = $1
		  AND ae.event_type = 'download'
		  AND ae.created_at > NOW() - ($2 || ' days')::INTERVAL`
	err := r.db.GetContext(ctx, &total, query, producerID, days)
	return total, err
}

func (r *PgAnalyticsRepository) GetTotalRevenue(ctx context.Context, producerID uuid.UUID, days int) (float64, error) {
	if days <= 0 {
		days = 30
	}
	var total float64
	query := `
		SELECT COALESCE(SUM(amount), 0) / 100.0
		FROM orders o
		JOIN specs s ON o.spec_id = s.id
		WHERE s.producer_id = $1 
		  AND o.status = 'paid'
		  AND o.created_at > NOW() - ($2 || ' days')::INTERVAL`
	err := r.db.GetContext(ctx, &total, query, producerID, days)
	return total, err
}

func (r *PgAnalyticsRepository) GetRevenueByLicenseGlobal(ctx context.Context, producerID uuid.UUID, days int) (map[string]float64, error) {
	if days <= 0 {
		days = 30
	}
	type licenseRev struct {
		LicenseType string  `db:"license_type"`
		Revenue     float64 `db:"revenue"`
	}
	query := `
		SELECT o.license_type, COALESCE(SUM(o.amount), 0) / 100.0 as revenue
		FROM orders o
		JOIN specs s ON o.spec_id = s.id
		WHERE s.producer_id = $1 
		  AND o.status = 'paid'
		  AND o.created_at > NOW() - ($2 || ' days')::INTERVAL
		GROUP BY o.license_type`
	var rows []licenseRev
	err := r.db.SelectContext(ctx, &rows, query, producerID, days)
	if err != nil {
		return nil, fmt.Errorf("failed to get global revenue by license: %w", err)
	}

	result := make(map[string]float64)
	for _, row := range rows {
		result[row.LicenseType] = row.Revenue
	}
	return result, nil
}

func (r *PgAnalyticsRepository) GetPlaysByDay(ctx context.Context, producerID uuid.UUID, days int) ([]domain.DailyStat, error) {
	log.Printf("[Analytics Repo] GetPlaysByDay: ProducerID=%s, Days=%d (raw)", producerID, days)

	if days <= 0 {
		log.Printf("[Analytics Repo] GetPlaysByDay: Days <= 0, defaulting to 30")
		days = 30
	}

	log.Printf("[Analytics Repo] GetPlaysByDay: Using days=%d for query", days)
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
	log.Printf("[Analytics Repo] GetPlaysByDay: Executing query with producerID=%s, days=%d", producerID, days)
	var stats []domain.DailyStat
	err := r.db.SelectContext(ctx, &stats, query, producerID, days)
	if err != nil {
		log.Printf("[Analytics Repo] GetPlaysByDay: Query error: %v", err)
		return nil, fmt.Errorf("failed to get plays by day: %w", err)
	}
	log.Printf("[Analytics Repo] GetPlaysByDay: Query returned %d rows", len(stats))
	return stats, nil
}

func (r *PgAnalyticsRepository) GetDownloadsByDay(ctx context.Context, producerID uuid.UUID, days int) ([]domain.DailyStat, error) {
	log.Printf("[Analytics Repo] GetDownloadsByDay: ProducerID=%s, Days=%d (raw)", producerID, days)

	if days <= 0 {
		log.Printf("[Analytics Repo] GetDownloadsByDay: Days <= 0, defaulting to 30")
		days = 30
	}

	log.Printf("[Analytics Repo] GetDownloadsByDay: Using days=%d for query", days)
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
	log.Printf("[Analytics Repo] GetDownloadsByDay: Executing query with producerID=%s, days=%d", producerID, days)
	var stats []domain.DailyStat
	err := r.db.SelectContext(ctx, &stats, query, producerID, days)
	if err != nil {
		log.Printf("[Analytics Repo] GetDownloadsByDay: Query error: %v", err)
		return nil, fmt.Errorf("failed to get downloads by day: %w", err)
	}
	log.Printf("[Analytics Repo] GetDownloadsByDay: Query returned %d rows", len(stats))
	return stats, nil
}

func (r *PgAnalyticsRepository) GetRevenueByDay(ctx context.Context, producerID uuid.UUID, days int) ([]domain.DailyRevenueStat, error) {
	log.Printf("[Analytics Repo] GetRevenueByDay: ProducerID=%s, Days=%d (raw)", producerID, days)

	if days <= 0 {
		log.Printf("[Analytics Repo] GetRevenueByDay: Days <= 0, defaulting to 30")
		days = 30
	}

	log.Printf("[Analytics Repo] GetRevenueByDay: Using days=%d for query", days)
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
	log.Printf("[Analytics Repo] GetRevenueByDay: Executing query with producerID=%s, days=%d", producerID, days)
	var stats []domain.DailyRevenueStat
	err := r.db.SelectContext(ctx, &stats, query, producerID, days)
	if err != nil {
		log.Printf("[Analytics Repo] GetRevenueByDay: Query error: %v", err)
		return nil, fmt.Errorf("failed to get revenue by day: %w", err)
	}
	log.Printf("[Analytics Repo] GetRevenueByDay: Query returned %d rows", len(stats))
	return stats, nil
}

func (r *PgAnalyticsRepository) GetTopSpecs(ctx context.Context, producerID uuid.UUID, limit int, sortBy string) ([]domain.TopSpecStat, error) {
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
		left JOIN spec_analytics sa ON s.id = sa.spec_id
		left JOIN orders o ON s.id = o.spec_id AND o.status = 'paid'
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
