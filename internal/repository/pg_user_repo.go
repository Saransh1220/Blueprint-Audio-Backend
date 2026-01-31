package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/saransh1220/blueprint-audio/internal/domain"
)

type pgUserRepository struct {
	db *sqlx.DB
}

func NewUserRepository(db *sqlx.DB) domain.UserRepository {
	return &pgUserRepository{db: db}
}

func (r *pgUserRepository) CreateUser(ctx context.Context, user *domain.User) error {
	query := `INSERT INTO users (id, email, password_hash,role,created_at,updated_at) VALUES (:id,:email,:password_hash,:name,:role,:created_at,:updated_at)`

	if user.CreatedAt.IsZero() {
		user.CreatedAt = time.Now()
	}
	if user.UpdatedAt.IsZero() {
		user.UpdatedAt = time.Now()
	}

	_, err := r.db.NamedExecContext(ctx, query, user)
	return err
}

func (r *pgUserRepository) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	user := &domain.User{}
	query := `SELECT * FROM users WHERE email = $1`

	err := r.db.GetContext(ctx, user, query, email)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return user, nil

}

func (r *pgUserRepository) GetUserById(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	user := &domain.User{}
	query := `SELECT * FROM users WHERE id = $1`
	err := r.db.GetContext(ctx, user, query, id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return user, nil
}
