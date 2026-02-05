package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/saransh1220/blueprint-audio/internal/domain"
)

type pgOrderRepository struct {
	db *sqlx.DB
}

func NewOrderRepository(db *sqlx.DB) domain.OrderRepository {
	return &pgOrderRepository{db: db}
}

func (r *pgOrderRepository) Create(ctx context.Context, order *domain.Order) error {

	if order.ID == uuid.Nil {
		order.ID = uuid.New()
	}
	if order.CreatedAt.IsZero() {
		order.CreatedAt = time.Now()
	}
	order.UpdatedAt = time.Now()

	// Marshal notes to JSON for JSONB column
	notesJSON, err := json.Marshal(order.Notes)
	if err != nil {
		return err
	}

	query := `
		INSERT INTO orders (
			id, user_id, spec_id, license_type, amount, currency,
			razorpay_order_id, status, notes, created_at, updated_at, expires_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
		)`

	_, err = r.db.ExecContext(ctx, query,
		order.ID,
		order.UserID,
		order.SpecID,
		order.LicenseType,
		order.Amount,
		order.Currency,
		order.RazorpayOrderID,
		order.Status,
		notesJSON, // Pass as JSON
		order.CreatedAt,
		order.UpdatedAt,
		order.ExpiresAt,
	)
	return err
}

func (r *pgOrderRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Order, error) {
	order := &domain.Order{}
	var notesJSON []byte

	query := `
		SELECT id, user_id, spec_id, license_type, amount, currency, 
		       razorpay_order_id, status, notes, created_at, updated_at, expires_at
		FROM orders 
		WHERE id = $1
	`

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&order.ID,
		&order.UserID,
		&order.SpecID,
		&order.LicenseType,
		&order.Amount,
		&order.Currency,
		&order.RazorpayOrderID,
		&order.Status,
		&notesJSON,
		&order.CreatedAt,
		&order.UpdatedAt,
		&order.ExpiresAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, err
		}
		return nil, err
	}

	// Unmarshal JSONB notes
	if len(notesJSON) > 0 {
		if err := json.Unmarshal(notesJSON, &order.Notes); err != nil {
			return nil, err
		}
	}

	return order, nil
}

func (r *pgOrderRepository) GetByRazorpayID(ctx context.Context, razorpayOrderID string) (*domain.Order, error) {
	order := &domain.Order{}
	query := `SELECT * FROM orders WHERE razorpay_order_id = $1`
	err := r.db.GetContext(ctx, order, query, razorpayOrderID)
	return order, err
}

func (r *pgOrderRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status domain.OrderStatus) error {
	query := `UPDATE orders SET status = $1, updated_at = NOW() WHERE id = $2`
	_, err := r.db.ExecContext(ctx, query, status, id)
	return err
}

func (r *pgOrderRepository) ListByUser(ctx context.Context, userID uuid.UUID, limit, offset int) ([]domain.Order, error) {
	var orders []domain.Order
	query := `SELECT * FROM orders WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`
	err := r.db.SelectContext(ctx, &orders, query, userID, limit, offset)
	return orders, err
}
