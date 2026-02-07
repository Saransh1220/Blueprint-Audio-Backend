package repository

import (
	"context"
	"fmt"
	"time"

	"strings"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/saransh1220/blueprint-audio/internal/domain"
)

type pgSpecRepository struct {
	db *sqlx.DB
}

func NewSpecRepository(db *sqlx.DB) domain.SpecRepository {
	return &pgSpecRepository{db: db}
}

func (r *pgSpecRepository) Create(ctx context.Context, spec *domain.Spec) error {
	// 1. Initialize metadata
	if spec.ID == uuid.Nil {
		spec.ID = uuid.New()
	}
	if spec.CreatedAt.IsZero() {
		spec.CreatedAt = time.Now()
	}
	spec.UpdatedAt = time.Now()

	// 2. Start Transaction
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 3. Insert Main Spec
	query := `
        INSERT INTO specs (
            id, producer_id, title, category, type, bpm, key, 
            base_price, image_url, preview_url, wav_url, stems_url,
            tags, duration, free_mp3_enabled,
            created_at, updated_at
        ) VALUES (
            :id, :producer_id, :title, :category, :type, :bpm, :key, 
            :base_price, :image_url, :preview_url, :wav_url, :stems_url,
            :tags, :duration, :free_mp3_enabled,
            :created_at, :updated_at
        )`

	_, err = tx.NamedExecContext(ctx, query, spec)
	if err != nil {
		return err
	}

	// 3b. Create analytics record
	analyticsQuery := `INSERT INTO spec_analytics (spec_id) VALUES ($1) ON CONFLICT DO NOTHING`
	_, err = tx.ExecContext(ctx, analyticsQuery, spec.ID)
	if err != nil {
		return err
	}

	// 4. Insert Genres (Many-to-Many)
	for _, genre := range spec.Genres {
		var genreID uuid.UUID

		// Check if we have an ID, if not look it up or create
		if genre.ID != uuid.Nil {
			genreID = genre.ID
		} else {
			// Try to find by slug
			err = tx.GetContext(ctx, &genreID, "SELECT id FROM genres WHERE slug = $1", genre.Slug)
			if err != nil {
				// Not found, Create new Genre
				genreID = uuid.New()
				now := time.Now()
				createGenreQuery := `INSERT INTO genres (id, name, slug, created_at) VALUES ($1, $2, $3, $4)`
				_, err = tx.ExecContext(ctx, createGenreQuery, genreID, genre.Name, genre.Slug, now)
				if err != nil {
					return fmt.Errorf("failed to create genre %s: %w", genre.Name, err)
				}
			}
		}

		genreQuery := `INSERT INTO spec_genres (spec_id, genre_id) VALUES ($1, $2)`
		_, err = tx.ExecContext(ctx, genreQuery, spec.ID, genreID)
		if err != nil {
			return err
		}
	}

	// 5. Insert License Options
	for i := range spec.Licenses {
		license := &spec.Licenses[i]
		if license.ID == uuid.Nil {
			license.ID = uuid.New()
		}
		license.SpecID = spec.ID

		licenseQuery := `
            INSERT INTO license_options (
                id, spec_id, license_type, name, price, features, file_types
            ) VALUES (
                :id, :spec_id, :license_type, :name, :price, :features, :file_types
            )`
		_, err = tx.NamedExecContext(ctx, licenseQuery, license)
		if err != nil {
			return err
		}
	}

	// 6. Commit
	return tx.Commit()
}

func (r *pgSpecRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Spec, error) {
	spec := &domain.Spec{}

	query := `SELECT * FROM specs WHERE id = $1`
	err := r.db.GetContext(ctx, spec, query, id)
	if err != nil {
		return nil, err
	}

	//fetct licences
	//fetct licences
	licenseQuery := `SELECT * FROM license_options WHERE spec_id = $1`
	err = r.db.SelectContext(ctx, &spec.Licenses, licenseQuery, id)
	if err != nil {
		return nil, err
	}

	//fetch genres
	genreQuery := `SELECT g.* FROM genres g JOIN spec_genres sg ON g.id = sg.genre_id WHERE sg.spec_id = $1`

	err = r.db.SelectContext(ctx, &spec.Genres, genreQuery, id)
	if err != nil {
		return nil, err
	}
	return spec, nil
}

func (r *pgSpecRepository) List(ctx context.Context, filter domain.SpecFilter) ([]domain.Spec, int, error) {
	var results []struct {
		domain.Spec
		TotalCount int `db:"total_count"`
	}

	query := `SELECT *, COUNT(*) OVER() as total_count FROM specs WHERE 1=1`
	args := []interface{}{}
	argId := 1

	if filter.Category != "" {
		query += fmt.Sprintf(" AND category = $%d", argId)
		args = append(args, filter.Category)
		argId++
	}

	if len(filter.Genres) > 0 {
		query += fmt.Sprintf(` AND id IN (
            SELECT spec_id FROM spec_genres sg 
            JOIN genres g ON sg.genre_id = g.id 
            WHERE g.slug ILIKE ANY($%d) OR g.name ILIKE ANY($%d)
        )`, argId, argId)
		args = append(args, pq.Array(filter.Genres))
		argId++
	}

	if len(filter.Tags) > 0 {
		query += fmt.Sprintf(" AND tags @> $%d", argId)
		args = append(args, pq.Array(filter.Tags))
		argId++
	}

	if filter.Search != "" {
		searchTerm := "%" + filter.Search + "%"
		lowerSearch := strings.ToLower(filter.Search)
		query += fmt.Sprintf(" AND (title ILIKE $%d OR tags @> ARRAY[$%d])", argId, argId+1)
		args = append(args, searchTerm, lowerSearch)
		argId += 2
	}

	if filter.MinBPM > 0 {
		query += fmt.Sprintf(" AND bpm >= $%d", argId)
		args = append(args, filter.MinBPM)
		argId++
	}

	if filter.MaxBPM > 0 {
		query += fmt.Sprintf(" AND bpm <= $%d", argId)
		args = append(args, filter.MaxBPM)
		argId++
	}

	if filter.MinPrice >= 0 {
		query += fmt.Sprintf(" AND base_price >= $%d", argId)
		args = append(args, filter.MinPrice)
		argId++
	}

	if filter.MaxPrice > 0 {
		query += fmt.Sprintf(" AND base_price <= $%d", argId)
		args = append(args, filter.MaxPrice)
		argId++
	}

	if filter.Key != "" {
		query += fmt.Sprintf(" AND key = $%d", argId)
		args = append(args, filter.Key)
		argId++
	}

	// Dynamic Sorting
	orderBy := "created_at DESC" // Default
	switch filter.Sort {
	case "newest":
		orderBy = "created_at DESC"
	case "oldest":
		orderBy = "created_at ASC"
	case "price_asc":
		orderBy = "base_price ASC"
	case "price_desc":
		orderBy = "base_price DESC"
	case "bpm_asc":
		orderBy = "bpm ASC"
	case "bpm_desc":
		orderBy = "bpm DESC"
	}

	query += fmt.Sprintf(" ORDER BY %s LIMIT $%d OFFSET $%d", orderBy, argId, argId+1)
	args = append(args, filter.Limit, filter.Offset)

	err := r.db.SelectContext(ctx, &results, query, args...)
	if err != nil {
		return nil, 0, err
	}

	if len(results) == 0 {
		return []domain.Spec{}, 0, nil
	}

	total := results[0].TotalCount
	specs := make([]domain.Spec, len(results))
	for i, res := range results {
		specs[i] = res.Spec
	}

	// Fetch Relations (Genres, Licenses) for each spec
	// N+1 query pattern, acceptable for small pagination limits
	for i := range specs {
		// Fetch Genres
		genreQuery := `SELECT g.* FROM genres g JOIN spec_genres sg ON g.id = sg.genre_id WHERE sg.spec_id = $1`
		err = r.db.SelectContext(ctx, &specs[i].Genres, genreQuery, specs[i].ID)
		if err != nil {
			return nil, 0, err
		}

		// Fetch Licenses
		licenseQuery := `SELECT * FROM license_options WHERE spec_id = $1`
		err = r.db.SelectContext(ctx, &specs[i].Licenses, licenseQuery, specs[i].ID)
		if err != nil {
			return nil, 0, err
		}
	}

	return specs, total, nil
}

func (r *pgSpecRepository) Delete(ctx context.Context, id uuid.UUID, producerId uuid.UUID) error {
	query := `DELETE FROM specs WHERE id = $1 AND producer_id = $2`

	result, err := r.db.ExecContext(ctx, query, id, producerId)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return domain.ErrSpecNotFound
	}

	return nil
}

func (r *pgSpecRepository) Update(ctx context.Context, spec *domain.Spec) error {
	spec.UpdatedAt = time.Now()

	query := `
		UPDATE specs 
		SET title = :title,
		    category = :category,
		    type = :type,
		    bpm = :bpm,
		    key = :key,
		    base_price = :base_price,
		    tags = :tags,
		    duration = :duration,
		    free_mp3_enabled = :free_mp3_enabled,
		    updated_at = :updated_at
		WHERE id = :id AND producer_id = :producer_id
	`

	result, err := r.db.NamedExecContext(ctx, query, spec)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return domain.ErrSpecNotFound
	}

	return nil
}

// ListByUserID retrieves all specs for a specific producer with pagination.
func (r *pgSpecRepository) ListByUserID(ctx context.Context, producerID uuid.UUID, limit, offset int) ([]domain.Spec, int, error) {
	var results []struct {
		domain.Spec
		TotalCount int `db:"total_count"`
	}

	query := `
		SELECT *, COUNT(*) OVER() as total_count 
		FROM specs 
		WHERE producer_id = $1
		ORDER BY created_at DESC 
		LIMIT $2 OFFSET $3
	`

	err := r.db.SelectContext(ctx, &results, query, producerID, limit, offset)
	if err != nil {
		return nil, 0, err
	}

	if len(results) == 0 {
		return []domain.Spec{}, 0, nil
	}

	total := results[0].TotalCount
	specs := make([]domain.Spec, len(results))
	for i, res := range results {
		specs[i] = res.Spec
	}

	// Fetch Relations (Genres, Licenses) for each spec
	for i := range specs {
		// Fetch Genres
		genreQuery := `SELECT g.* FROM genres g JOIN spec_genres sg ON g.id = sg.genre_id WHERE sg.spec_id = $1`
		err = r.db.SelectContext(ctx, &specs[i].Genres, genreQuery, specs[i].ID)
		if err != nil {
			return nil, 0, err
		}

		// Fetch Licenses
		licenseQuery := `SELECT * FROM license_options WHERE spec_id = $1`
		err = r.db.SelectContext(ctx, &specs[i].Licenses, licenseQuery, specs[i].ID)
		if err != nil {
			return nil, 0, err
		}
	}

	return specs, total, nil
}
