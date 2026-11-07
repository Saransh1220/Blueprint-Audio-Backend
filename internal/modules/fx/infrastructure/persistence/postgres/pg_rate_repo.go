package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/saransh1220/blueprint-audio/internal/modules/fx/domain"
	"github.com/saransh1220/blueprint-audio/internal/shared/money"
)

type PgRateRepository struct {
	db *sqlx.DB
}

func NewRateRepository(db *sqlx.DB) *PgRateRepository {
	return &PgRateRepository{db: db}
}

func (r *PgRateRepository) Append(ctx context.Context, rate *domain.Rate) error {
	if rate == nil {
		return errors.New("rate cannot be nil")
	}
	if rate.RateDecimal == "" && rate.Rate != nil {
		rate.RateDecimal = money.FormatRateDecimal(rate.Rate)
	}
	if rate.Status == "" {
		rate.Status = domain.RateStatusAccepted
	}

	query := `
		INSERT INTO fx_rates (base, quote, rate, source, as_of, status)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, fetched_at
	`

	var id int64
	var fetchedAt = rate.FetchedAt
	row := r.db.QueryRowContext(ctx, query,
		rate.Base,
		rate.Quote,
		rate.RateDecimal,
		rate.Source,
		rate.AsOf,
		string(rate.Status),
	)
	if err := row.Scan(&id, &fetchedAt); err != nil {
		return fmt.Errorf("failed to append fx rate: %w", err)
	}
	rate.ID = id
	rate.FetchedAt = fetchedAt
	return nil
}

func (r *PgRateRepository) AppendBatch(ctx context.Context, rates []*domain.Rate) error {
	if len(rates) == 0 {
		return nil
	}

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO fx_rates (base, quote, rate, source, as_of, status)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, fetched_at
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare insert statement: %w", err)
	}
	defer stmt.Close()

	for _, rate := range rates {
		if rate.RateDecimal == "" && rate.Rate != nil {
			rate.RateDecimal = money.FormatRateDecimal(rate.Rate)
		}
		if rate.Status == "" {
			rate.Status = domain.RateStatusAccepted
		}

		var id int64
		var fetchedAt = rate.FetchedAt
		if err := stmt.QueryRowContext(ctx,
			rate.Base,
			rate.Quote,
			rate.RateDecimal,
			rate.Source,
			rate.AsOf,
			string(rate.Status),
		).Scan(&id, &fetchedAt); err != nil {
			return fmt.Errorf("failed to insert batch fx rate for %s/%s: %w", rate.Base, rate.Quote, err)
		}
		rate.ID = id
		rate.FetchedAt = fetchedAt
	}

	return tx.Commit()
}

func (r *PgRateRepository) Latest(ctx context.Context, base, quote string) (*domain.Rate, error) {
	base = strings.ToUpper(strings.TrimSpace(base))
	quote = strings.ToUpper(strings.TrimSpace(quote))

	query := `
		SELECT id, base, quote, rate::TEXT AS rate, source, as_of, fetched_at, status
		FROM fx_rates
		WHERE base = $1 AND quote = $2 AND status = 'accepted'
		ORDER BY fetched_at DESC, id DESC
		LIMIT 1
	`

	var item domain.Rate
	row := r.db.QueryRowContext(ctx, query, base, quote)
	if err := row.Scan(
		&item.ID,
		&item.Base,
		&item.Quote,
		&item.RateDecimal,
		&item.Source,
		&item.AsOf,
		&item.FetchedAt,
		&item.Status,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrNoRate
		}
		return nil, fmt.Errorf("failed to fetch latest fx rate: %w", err)
	}

	parsedRat, err := money.ParseRateDecimal(item.RateDecimal)
	if err != nil {
		return nil, fmt.Errorf("corrupt rate in database %q: %w", item.RateDecimal, err)
	}
	item.Rate = parsedRat
	return &item, nil
}

func (r *PgRateRepository) History(ctx context.Context, filter domain.HistoryFilter) ([]*domain.Rate, int, error) {
	clauses := []string{"1=1"}
	args := []any{}
	argIdx := 1

	if filter.Base != "" {
		clauses = append(clauses, fmt.Sprintf("base = $%d", argIdx))
		args = append(args, strings.ToUpper(strings.TrimSpace(filter.Base)))
		argIdx++
	}
	if filter.Quote != "" {
		clauses = append(clauses, fmt.Sprintf("quote = $%d", argIdx))
		args = append(args, strings.ToUpper(strings.TrimSpace(filter.Quote)))
		argIdx++
	}
	if filter.Status != "" {
		clauses = append(clauses, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, string(filter.Status))
		argIdx++
	}

	whereClause := strings.Join(clauses, " AND ")

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM fx_rates WHERE %s", whereClause)
	var total int
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count fx history: %w", err)
	}

	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	selectQuery := fmt.Sprintf(`
		SELECT id, base, quote, rate::TEXT AS rate, source, as_of, fetched_at, status
		FROM fx_rates
		WHERE %s
		ORDER BY fetched_at DESC, id DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argIdx, argIdx+1)

	queryArgs := append(args, limit, offset)
	rows, err := r.db.QueryContext(ctx, selectQuery, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query fx history: %w", err)
	}
	defer rows.Close()

	var results []*domain.Rate
	for rows.Next() {
		var item domain.Rate
		if err := rows.Scan(
			&item.ID,
			&item.Base,
			&item.Quote,
			&item.RateDecimal,
			&item.Source,
			&item.AsOf,
			&item.FetchedAt,
			&item.Status,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan fx history row: %w", err)
		}
		if rat, err := money.ParseRateDecimal(item.RateDecimal); err == nil {
			item.Rate = rat
		}
		results = append(results, &item)
	}

	return results, total, rows.Err()
}

func (r *PgRateRepository) LatestPerPair(ctx context.Context) ([]*domain.Rate, error) {
	query := `
		SELECT DISTINCT ON (base, quote)
			id, base, quote, rate::TEXT AS rate, source, as_of, fetched_at, status
		FROM fx_rates
		WHERE status = 'accepted'
		ORDER BY base, quote, fetched_at DESC, id DESC
	`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query latest rates per pair: %w", err)
	}
	defer rows.Close()

	var results []*domain.Rate
	for rows.Next() {
		var item domain.Rate
		if err := rows.Scan(
			&item.ID,
			&item.Base,
			&item.Quote,
			&item.RateDecimal,
			&item.Source,
			&item.AsOf,
			&item.FetchedAt,
			&item.Status,
		); err != nil {
			return nil, fmt.Errorf("failed to scan distinct rate row: %w", err)
		}
		if rat, err := money.ParseRateDecimal(item.RateDecimal); err == nil {
			item.Rate = rat
		}
		results = append(results, &item)
	}

	return results, rows.Err()
}

// TryLock attempts to acquire a Postgres advisory session lock on the given key.
// Returns an unlock callback function if the lock was acquired.
func (r *PgRateRepository) TryLock(ctx context.Context, key string) (func() error, bool, error) {
	conn, err := r.db.Conn(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("failed to acquire db connection for advisory lock: %w", err)
	}

	var acquired bool
	query := "SELECT pg_try_advisory_lock(hashtext($1))"
	if err := conn.QueryRowContext(ctx, query, key).Scan(&acquired); err != nil {
		_ = conn.Close()
		return nil, false, fmt.Errorf("advisory lock query failed: %w", err)
	}

	if !acquired {
		_ = conn.Close()
		return nil, false, nil
	}

	unlock := func() error {
		defer conn.Close()
		var unlocked bool
		return conn.QueryRowContext(context.Background(), "SELECT pg_advisory_unlock(hashtext($1))", key).Scan(&unlocked)
	}

	return unlock, true, nil
}
