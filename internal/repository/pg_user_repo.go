package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/saransh1220/blueprint-audio/internal/domain"
)

type pgUserRepository struct {
	db *sqlx.DB
}

// NewUserRepository creates and returns a new PostgreSQL-based user repository.
// It takes a database connection and initializes a pgUserRepository instance
// that implements the domain.UserRepository interface.
func NewUserRepository(db *sqlx.DB) domain.UserRepository {
	return &pgUserRepository{db: db}
}

// CreateUser inserts a new user record into the database.
// It takes a context and a pointer to a domain.User struct.
// If the user's CreatedAt or UpdatedAt timestamps are zero values,
// they are automatically set to the current time before insertion.
// Returns an error if the database operation fails.
func (r *pgUserRepository) CreateUser(ctx context.Context, user *domain.User) error {
	query := `INSERT INTO users (id, email, password_hash, name, role, created_at, updated_at) VALUES (:id, :email, :password_hash, :name, :role, :created_at, :updated_at)`

	if user.CreatedAt.IsZero() {
		user.CreatedAt = time.Now()
	}
	if user.UpdatedAt.IsZero() {
		user.UpdatedAt = time.Now()
	}

	// NamedExecContext is a sqlx feature! It uses the structure fields directly.
	_, err := r.db.NamedExecContext(ctx, query, user)
	if err != nil {
		if pqErr, ok := err.(*pq.Error); ok {
			if pqErr.Code == "23505" { // Unique violation
				return domain.ErrUserAlreadyExists
			}
		}
		return err
	}
	return nil
}

// GetUserByEmail retrieves a user from the database by their email address.
// It returns a pointer to the User domain object if found, or nil if no user exists with the given email.
// Returns an error if the database query fails for any reason other than a missing record.
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

// GetUserById retrieves a user from the database by their unique identifier.
// It queries the users table for a user matching the provided UUID.
// Returns the user if found, nil if no user exists with the given ID,
// or an error if the database query fails.
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
