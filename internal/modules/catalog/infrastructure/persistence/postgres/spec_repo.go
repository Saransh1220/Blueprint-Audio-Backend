package postgres

import (
	"context"
	"fmt"
	"sort"
	"time"

	"strings"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/saransh1220/blueprint-audio/internal/modules/catalog/domain"
)

type PgSpecRepository struct {
	db *sqlx.DB
}

func NewSpecRepository(db *sqlx.DB) *PgSpecRepository {
	return &PgSpecRepository{db: db}
}

func (r *PgSpecRepository) Create(ctx context.Context, spec *domain.Spec) error {
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
            created_at, updated_at, processing_status
        ) VALUES (
            :id, :producer_id, :title, :category, :type, :bpm, :key, 
            :base_price, :image_url, :preview_url, :wav_url, :stems_url,
            :tags, :duration, :free_mp3_enabled,
            :created_at, :updated_at, :processing_status
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

func (r *PgSpecRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Spec, error) {
	spec := &domain.Spec{}

	query := `
		SELECT s.*, u.display_name as producer_name
		FROM specs s
		JOIN users u ON s.producer_id = u.id
		WHERE s.id = $1 AND s.is_deleted = FALSE
	`
	err := r.db.GetContext(ctx, spec, query, id)
	if err != nil {
		return nil, err
	}

	//fetct licences
	//fetct licences
	licenseQuery := `SELECT * FROM license_options WHERE spec_id = $1 AND is_deleted = FALSE`
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

func (r *PgSpecRepository) List(ctx context.Context, filter domain.SpecFilter) ([]domain.Spec, int, error) {
	// Use a struct to hold the result including the window function count
	var results []struct {
		domain.Spec
		TotalCount int `db:"total_count"`
	}

	query := `
		SELECT s.*, u.display_name as producer_name, COUNT(*) OVER() as total_count 
		FROM specs s
		JOIN users u ON s.producer_id = u.id
		WHERE s.is_deleted = FALSE
	`
	args := []interface{}{}
	argId := 1

	if filter.Category != "" {
		query += fmt.Sprintf(" AND category = $%d", argId)
		args = append(args, filter.Category)
		argId++
	}

	if len(filter.Genres) > 0 {
		query += fmt.Sprintf(` AND s.id IN (
            SELECT spec_id FROM spec_genres sg 
            JOIN genres g ON sg.genre_id = g.id 
            WHERE g.slug ILIKE ANY($%d::text[]) OR g.name ILIKE ANY($%d::text[])
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
		query += fmt.Sprintf(" AND (title ILIKE $%d OR array_to_string(tags, ',') ILIKE $%d)", argId, argId)
		args = append(args, searchTerm)
		argId++
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

	// Create a map for O(1) lookup to assign relations
	specMap := make(map[uuid.UUID]*domain.Spec, len(results))
	specIDs := make([]uuid.UUID, len(results))

	for i, res := range results {
		specs[i] = res.Spec
		specs[i].Genres = []domain.Genre{}           // Initialize empty slice
		specs[i].Licenses = []domain.LicenseOption{} // Initialize empty slice
		specMap[specs[i].ID] = &specs[i]
		specIDs[i] = specs[i].ID
	}

	// 1. Bulk Fetch Genres
	// Use sqlx.In to handle IN clause with slice
	genreQuery, args, err := sqlx.In(`
		SELECT sg.spec_id, g.* 
		FROM genres g 
		JOIN spec_genres sg ON g.id = sg.genre_id 
		WHERE sg.spec_id IN (?)`, specIDs)
	if err != nil {
		return nil, 0, err
	}
	genreQuery = r.db.Rebind(genreQuery)

	var genreRows []struct {
		SpecID uuid.UUID `db:"spec_id"`
		domain.Genre
	}

	err = r.db.SelectContext(ctx, &genreRows, genreQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to fetch genres: %w", err)
	}

	for _, row := range genreRows {
		if spec, ok := specMap[row.SpecID]; ok {
			spec.Genres = append(spec.Genres, row.Genre)
		}
	}

	// 2. Bulk Fetch Licenses
	licenseQuery, args, err := sqlx.In(`SELECT * FROM license_options WHERE spec_id IN (?) AND is_deleted = FALSE`, specIDs)
	if err != nil {
		return nil, 0, err
	}
	licenseQuery = r.db.Rebind(licenseQuery)

	var licenses []domain.LicenseOption
	err = r.db.SelectContext(ctx, &licenses, licenseQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to fetch licenses: %w", err)
	}

	for _, lic := range licenses {
		if spec, ok := specMap[lic.SpecID]; ok {
			spec.Licenses = append(spec.Licenses, lic)
		}
	}

	return specs, total, nil
}

func (r *PgSpecRepository) Delete(ctx context.Context, id uuid.UUID, producerId uuid.UUID) error {
	// 1. Check if ANY purchases exist (licenses table)
	// We check licenses table as it represents completed purchases granting access.
	// You could also check orders table if you want to be stricter (e.g. pending orders).
	var licenseCount int
	err := r.db.GetContext(ctx, &licenseCount, "SELECT COUNT(*) FROM licenses WHERE spec_id = $1", id)
	if err != nil {
		return fmt.Errorf("failed to check license existence: %w", err)
	}

	if licenseCount > 0 {
		// Soft Delete
		query := `UPDATE specs SET is_deleted = TRUE, deleted_at = NOW() WHERE id = $1 AND producer_id = $2`
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
		return domain.ErrSpecSoftDeleted
	}

	// Hard Delete (No purchases)
	query := `DELETE FROM specs WHERE id = $1 AND producer_id = $2`

	result, err := r.db.ExecContext(ctx, query, id, producerId)
	if err != nil {
		// Check for potential constraint violation just in case (e.g. from orders table)
		if strings.Contains(err.Error(), "violates foreign key constraint") {
			// Fallback to soft delete if constrained by something else (like an older order that didn't generate a license?)
			// But for now, let's treat it as an error or decide to soft delete.
			// Given the user request "maybe a user does want to delete", soft delete seems safer as fallback.
			// Let's retry with soft delete logic?
			// Simplified: Just return error for now to confirm behavior.
			return fmt.Errorf("cannot delete spec with existing dependencies: %w", err)
		}
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

func (r *PgSpecRepository) Update(ctx context.Context, spec *domain.Spec) error {
	spec.UpdatedAt = time.Now()

	// 1. Start Transaction
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 2. Update Spec Details
	query := `
		UPDATE specs 
		SET title = :title,
		    category = :category,
		    type = :type,
		    bpm = :bpm,
		    key = :key,
		    base_price = :base_price,
		    image_url = :image_url,
		    description = :description,
		    tags = :tags,
		    duration = :duration,
		    free_mp3_enabled = :free_mp3_enabled,
		    updated_at = :updated_at
		WHERE id = :id AND producer_id = :producer_id
	`

	result, err := tx.NamedExecContext(ctx, query, spec)
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

	// 3. Update Licenses (Smart Sync)
	if spec.Licenses != nil {
		// A. Fetch existing (active) licenses to compare
		var existingLicenses []domain.LicenseOption
		err = tx.SelectContext(ctx, &existingLicenses, "SELECT * FROM license_options WHERE spec_id = $1 AND is_deleted = FALSE", spec.ID)
		if err != nil {
			return err
		}

		existingMap := make(map[uuid.UUID]bool)
		existingByType := make(map[domain.LicenseType]domain.LicenseOption)
		for _, l := range existingLicenses {
			existingMap[l.ID] = true
			existingByType[l.LicenseType] = l
		}

		// Keep track of IDs processed in the update (to identify deletions)
		processedIDs := make(map[uuid.UUID]bool)

		// Define Queries
		insertQuery := `
            INSERT INTO license_options (
                id, spec_id, license_type, name, price, features, file_types
            ) VALUES (
                :id, :spec_id, :license_type, :name, :price, :features, :file_types
            )`

		updateQuery := `
			UPDATE license_options SET
				license_type = :license_type,
				name = :name,
				price = :price,
				features = :features,
				file_types = :file_types,
				is_deleted = FALSE,
				updated_at = NOW()
			WHERE id = :id
		`

		// B. Upsert (Insert or Update)
		for i := range spec.Licenses {
			license := &spec.Licenses[i]
			license.SpecID = spec.ID // Ensure SpecID is set

			// Try to match existing license by type if ID is missing (to prevent ID rotation)
			if license.ID == uuid.Nil {
				if existing, found := existingByType[license.LicenseType]; found {
					license.ID = existing.ID
				}
			}

			if license.ID == uuid.Nil {
				// New License (and no match found) -> INSERT
				license.ID = uuid.New()
				_, err = tx.NamedExecContext(ctx, insertQuery, license)
				if err != nil {
					return err
				}
			} else {
				// Existing ID (or matched)
				if existingMap[license.ID] {
					// Update existing
					_, err = tx.NamedExecContext(ctx, updateQuery, license)
					if err != nil {
						return err
					}
					processedIDs[license.ID] = true
				} else {
					// ID provided but not in DB (or soft deleted)
					// Verify it exists in DB (maybe soft deleted) to decide whether to INSERT with new ID or UPDATE
					// For simplicity and since we matched by Type if possible:
					// If we are here, it means ID is not in active existingMap.
					// It could be a soft-deleted ID.
					// Attempt UPDATE (which reactivates due to is_deleted=FALSE).
					result, err := tx.NamedExecContext(ctx, updateQuery, license)
					if err != nil {
						return err
					}
					rows, _ := result.RowsAffected()
					if rows == 0 {
						// ID doesn't exist at all -> Insert
						_, err = tx.NamedExecContext(ctx, insertQuery, license)
						if err != nil {
							return err
						}
					}
					processedIDs[license.ID] = true
				}
			}
		}

		// C. Hande Deletions
		// Collect IDs to delete
		var idsToDelete []uuid.UUID
		for existingID := range existingMap {
			if !processedIDs[existingID] {
				idsToDelete = append(idsToDelete, existingID)
			}
		}

		// Sort IDs to ensure deterministic execution order (crucial for tests and consistency)
		sort.Slice(idsToDelete, func(i, j int) bool {
			return idsToDelete[i].String() < idsToDelete[j].String()
		})

		for _, existingID := range idsToDelete {
			// This license was NOT in the update payload -> DELETE it.

			// 1. Check if used in any purchases
			var usageCount int
			err := tx.GetContext(ctx, &usageCount, "SELECT COUNT(*) FROM licenses WHERE license_option_id = $1", existingID)
			if err != nil {
				return err
			}

			if usageCount > 0 {
				// Used -> Soft Delete
				_, err = tx.ExecContext(ctx, "UPDATE license_options SET is_deleted = TRUE, updated_at = NOW() WHERE id = $1", existingID)
				if err != nil {
					return err
				}
			} else {
				// Not Used -> Hard Delete
				_, err = tx.ExecContext(ctx, "DELETE FROM license_options WHERE id = $1", existingID)
				if err != nil {
					// In case of race condition or other constraint, fall back to soft delete isn't safe if tx aborted,
					// but usageCount check minimizes this risk significantly.
					return err
				}
			}
		}
	}

	return tx.Commit()
}

// ListByUserID retrieves all specs for a specific producer with pagination.
func (r *PgSpecRepository) ListByUserID(ctx context.Context, producerID uuid.UUID, limit, offset int) ([]domain.Spec, int, error) {
	var results []struct {
		domain.Spec
		TotalCount int `db:"total_count"`
	}

	query := `
		SELECT s.*, u.display_name as producer_name, COUNT(*) OVER() as total_count 
		FROM specs s
		JOIN users u ON s.producer_id = u.id
		WHERE s.producer_id = $1 AND s.is_deleted = FALSE
		ORDER BY s.created_at DESC 
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

	specMap := make(map[uuid.UUID]*domain.Spec, len(results))
	specIDs := make([]uuid.UUID, len(results))

	for i, res := range results {
		specs[i] = res.Spec
		specs[i].Genres = []domain.Genre{}
		specs[i].Licenses = []domain.LicenseOption{}
		specMap[specs[i].ID] = &specs[i]
		specIDs[i] = specs[i].ID
	}

	// 1. Bulk Fetch Genres
	genreQuery, args, err := sqlx.In(`
		SELECT sg.spec_id, g.* 
		FROM genres g 
		JOIN spec_genres sg ON g.id = sg.genre_id 
		WHERE sg.spec_id IN (?)`, specIDs)
	if err != nil {
		return nil, 0, err
	}
	genreQuery = r.db.Rebind(genreQuery)

	var genreRows []struct {
		SpecID uuid.UUID `db:"spec_id"`
		domain.Genre
	}

	err = r.db.SelectContext(ctx, &genreRows, genreQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to fetch genres: %w", err)
	}

	for _, row := range genreRows {
		if spec, ok := specMap[row.SpecID]; ok {
			spec.Genres = append(spec.Genres, row.Genre)
		}
	}

	// 2. Bulk Fetch Licenses
	licenseQuery, args, err := sqlx.In(`SELECT * FROM license_options WHERE spec_id IN (?) AND is_deleted = FALSE`, specIDs)
	if err != nil {
		return nil, 0, err
	}
	licenseQuery = r.db.Rebind(licenseQuery)

	var licenses []domain.LicenseOption
	err = r.db.SelectContext(ctx, &licenses, licenseQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to fetch licenses: %w", err)
	}

	for _, lic := range licenses {
		if spec, ok := specMap[lic.SpecID]; ok {
			spec.Licenses = append(spec.Licenses, lic)
		}
	}

	return specs, total, nil
}

// GetByIDSystem retrieves a spec by ID without filtering deleted ones.
func (r *PgSpecRepository) GetByIDSystem(ctx context.Context, id uuid.UUID) (*domain.Spec, error) {
	spec := &domain.Spec{}

	query := `
		SELECT s.*, u.display_name as producer_name
		FROM specs s
		JOIN users u ON s.producer_id = u.id
		WHERE s.id = $1
	`
	err := r.db.GetContext(ctx, spec, query, id)
	if err != nil {
		return nil, err
	}

	// Fetch licenses
	licenseQuery := `SELECT * FROM license_options WHERE spec_id = $1 AND is_deleted = FALSE`
	err = r.db.SelectContext(ctx, &spec.Licenses, licenseQuery, id)
	if err != nil {
		return nil, err
	}

	// Fetch genres
	genreQuery := `SELECT g.* FROM genres g JOIN spec_genres sg ON g.id = sg.genre_id WHERE sg.spec_id = $1`

	err = r.db.SelectContext(ctx, &spec.Genres, genreQuery, id)
	if err != nil {
		return nil, err
	}
	return spec, nil
}

// FindByIDIncludingDeleted implements domain.SpecFinder interface
// Alias for GetByIDSystem - retrieves a spec even if it's soft-deleted
func (r *PgSpecRepository) FindByIDIncludingDeleted(ctx context.Context, id uuid.UUID) (*domain.Spec, error) {
	return r.GetByIDSystem(ctx, id)
}

func (r *PgSpecRepository) UpdateFilesAndStatus(ctx context.Context, id uuid.UUID, files map[string]*string, status domain.ProcessingStatus) error {
	// Build dynamic query
	query := "UPDATE specs SET processing_status = :status, updated_at = :updated_at"
	params := map[string]interface{}{
		"id":         id,
		"status":     status,
		"updated_at": time.Now(),
	}

	if val, ok := files["image_url"]; ok && val != nil {
		query += ", image_url = :image_url"
		params["image_url"] = *val
	}
	if val, ok := files["preview_url"]; ok && val != nil {
		query += ", preview_url = :preview_url"
		params["preview_url"] = *val
	}
	if val, ok := files["wav_url"]; ok && val != nil {
		query += ", wav_url = :wav_url"
		params["wav_url"] = *val
	}
	if val, ok := files["stems_url"]; ok && val != nil {
		query += ", stems_url = :stems_url"
		params["stems_url"] = *val
	}

	query += " WHERE id = :id"

	result, err := r.db.NamedExecContext(ctx, query, params)
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
