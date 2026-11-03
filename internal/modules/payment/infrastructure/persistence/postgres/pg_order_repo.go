package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/saransh1220/blueprint-audio/internal/modules/payment/domain"
)

type PgOrderRepository struct {
	db *sqlx.DB
}

func NewOrderRepository(db *sqlx.DB) domain.OrderRepository {
	return &PgOrderRepository{db: db}
}

func (r *PgOrderRepository) Create(ctx context.Context, order *domain.Order) error {

	if order.ID == uuid.Nil {
		id, err := uuid.NewV7()
		if err != nil {
			return err
		}
		order.ID = id
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
			provider, provider_checkout_id, provider_payment_id,
			status, notes, created_at, updated_at, expires_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14
		)`

	_, err = r.db.ExecContext(ctx, query,
		order.ID,
		order.UserID,
		order.SpecID,
		order.LicenseType,
		order.Amount,
		order.Currency,
		order.Provider,
		order.ProviderCheckoutID,
		order.ProviderPaymentID,
		order.Status,
		notesJSON, // Pass as JSON
		order.CreatedAt,
		order.UpdatedAt,
		order.ExpiresAt,
	)
	return err
}

func (r *PgOrderRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Order, error) {
	order := &domain.Order{}
	var notesJSON []byte

	query := `
		SELECT id, user_id, spec_id, license_type, amount, currency,
		       provider, provider_checkout_id, provider_payment_id,
		       status, notes, created_at, updated_at, expires_at
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
		&order.Provider,
		&order.ProviderCheckoutID,
		&order.ProviderPaymentID,
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

// orderColumns lists the columns scanOrder reads, in order. notes is JSONB, so
// rows are scanned by hand rather than with SELECT * into the struct.
const orderColumns = `id, user_id, spec_id, license_type, amount, currency,
	provider, provider_checkout_id, provider_payment_id,
	status, notes, created_at, updated_at, expires_at`

type rowScanner interface{ Scan(dest ...any) error }

func scanOrder(row rowScanner) (*domain.Order, error) {
	order := &domain.Order{}
	var notesJSON []byte
	if err := row.Scan(
		&order.ID, &order.UserID, &order.SpecID, &order.LicenseType, &order.Amount, &order.Currency,
		&order.Provider, &order.ProviderCheckoutID, &order.ProviderPaymentID,
		&order.Status, &notesJSON, &order.CreatedAt, &order.UpdatedAt, &order.ExpiresAt,
	); err != nil {
		return nil, err
	}
	if len(notesJSON) > 0 {
		if err := json.Unmarshal(notesJSON, &order.Notes); err != nil {
			return nil, err
		}
	}
	return order, nil
}

func (r *PgOrderRepository) GetByProviderCheckoutID(ctx context.Context, provider, checkoutID string) (*domain.Order, error) {
	query := `SELECT ` + orderColumns + ` FROM orders WHERE provider = $1 AND provider_checkout_id = $2`
	return scanOrder(r.db.QueryRowContext(ctx, query, provider, checkoutID))
}

func (r *PgOrderRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status domain.OrderStatus) error {
	query := `UPDATE orders SET status = $1, updated_at = NOW() WHERE id = $2`
	_, err := r.db.ExecContext(ctx, query, status, id)
	return err
}

func (r *PgOrderRepository) ListByUser(ctx context.Context, userID uuid.UUID, limit, offset int) ([]domain.Order, error) {
	query := `SELECT ` + orderColumns + ` FROM orders WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`
	rows, err := r.db.QueryContext(ctx, query, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	orders := []domain.Order{}
	for rows.Next() {
		order, err := scanOrder(rows)
		if err != nil {
			return nil, err
		}
		orders = append(orders, *order)
	}
	return orders, rows.Err()
}

func (r *PgOrderRepository) ListByProducer(ctx context.Context, producerID uuid.UUID, limit, offset int) ([]domain.OrderWithBuyer, int, error) {
	var orders []domain.OrderWithBuyer

	// Get total count first
	var total int
	countQuery := `
		SELECT COUNT(*)
		FROM orders o
		JOIN specs s ON o.spec_id = s.id
		WHERE s.producer_id = $1
	`
	err := r.db.GetContext(ctx, &total, countQuery, producerID)
	if err != nil {
		return nil, 0, err
	}

	// Get orders with buyer details
	query := `
		SELECT 
			o.id,
			o.user_id,
			o.spec_id,
			o.license_type,
			o.amount,
			o.currency,
			o.provider,
			o.provider_checkout_id,
			o.provider_payment_id,
			o.status,
			o.notes,
			o.created_at,
			o.updated_at,
			o.expires_at,
			u.name as buyer_name,
			u.email as buyer_email,
			s.title as spec_title
		FROM orders o
		JOIN specs s ON o.spec_id = s.id
		JOIN users u ON o.user_id = u.id
		WHERE s.producer_id = $1
		ORDER BY o.created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.db.QueryxContext(ctx, query, producerID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	for rows.Next() {
		var order domain.OrderWithBuyer
		var notesJSON []byte

		// We need to scan manually because of the JSON field and embedded struct
		err := rows.Scan(
			&order.ID, &order.UserID, &order.SpecID, &order.LicenseType, &order.Amount, &order.Currency,
			&order.Provider, &order.ProviderCheckoutID, &order.ProviderPaymentID,
			&order.Status, &notesJSON, &order.CreatedAt, &order.UpdatedAt, &order.ExpiresAt,
			&order.BuyerName, &order.BuyerEmail, &order.SpecTitle,
		)
		if err != nil {
			return nil, 0, err
		}

		if len(notesJSON) > 0 {
			_ = json.Unmarshal(notesJSON, &order.Notes)
		}

		orders = append(orders, order)
	}

	return orders, total, nil
}
