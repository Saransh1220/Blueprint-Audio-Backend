package repository

import (
	"context"
	"fmt"
	"time"

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
            tags, 
            created_at, updated_at
        ) VALUES (
            :id, :producer_id, :title, :category, :type, :bpm, :key, 
            :base_price, :image_url, :preview_url, :wav_url, :stems_url,
            :tags,
            :created_at, :updated_at
        )`

	_, err = tx.NamedExecContext(ctx, query, spec)
	if err != nil {
		return err
	}

	// 4. Insert Genres (Many-to-Many)
	for _, genre := range spec.Genres {
		genreQuery := `INSERT INTO spec_genres (spec_id, genre_id) VALUES ($1, $2)`
		_, err = tx.ExecContext(ctx, genreQuery, spec.ID, genre.ID)
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

func (r *pgSpecRepository) List(ctx context.Context, category domain.Category, genres []string, tags []string, limit, offset int) ([]domain.Spec, error) {
	var specs []domain.Spec

	query := `SELECT * FROM specs WHERE 1=1`
	args := []interface{}{}
	argId := 1

	if category != "" {
		query += fmt.Sprintf(" AND category = $%d", argId)

		args = append(args, category)
		argId++
	}

	if len(genres) > 0 {
		query += fmt.Sprintf(` AND id IN (
            SELECT spec_id FROM spec_genres sg 
            JOIN genres g ON sg.genre_id = g.id 
            WHERE g.slug = ANY($%d) OR g.name = ANY($%d)
        )`, argId, argId)

		args = append(args, pq.Array(genres))
		argId++
	}

	if len(tags) > 0 {
		query += fmt.Sprintf(" AND tags @> $%d", argId)
		args = append(args, pq.Array(tags))
		argId++
	}

	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", argId, argId+1)
	args = append(args, limit, offset)
	err := r.db.SelectContext(ctx, &specs, query, args...)

	if err != nil {
		return nil, err
	}

	return specs, nil
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
