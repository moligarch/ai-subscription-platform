package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"

	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/domain/repository"
)

// PostgresPaymentRepo implements repository.PaymentRepository using Postgres.
type PostgresPaymentRepo struct {
	pool *pgxpool.Pool
}

// NewPostgresPaymentRepo constructs the repo.
func NewPostgresPaymentRepo(pool *pgxpool.Pool) *PostgresPaymentRepo {
	return &PostgresPaymentRepo{pool: pool}
}

// Save inserts or updates a payment record.
// DB columns assumed: id TEXT PRIMARY KEY, user_id TEXT, amount DOUBLE PRECISION, method TEXT, status TEXT, created_at TIMESTAMPTZ
func (r *PostgresPaymentRepo) Save(ctx context.Context, p *domain.Payment) error {
	const sql = `
INSERT INTO payments (id, user_id, amount, method, status, created_at)
VALUES ($1,$2,$3,$4,$5,$6)
ON CONFLICT (id) DO UPDATE SET
  user_id = EXCLUDED.user_id,
  amount = EXCLUDED.amount,
  method = EXCLUDED.method,
  status = EXCLUDED.status,
  created_at = EXCLUDED.created_at;
`
	_, err := r.pool.Exec(ctx, sql,
		p.ID,
		p.UserID,
		p.Amount,
		p.Method,
		string(p.Status),
		p.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("postgres Save payment: %w", err)
	}
	return nil
}

// FindByID loads a payment by id. Returns domain.ErrNotFound if missing.
func (r *PostgresPaymentRepo) FindByID(ctx context.Context, id string) (*domain.Payment, error) {
	const sql = `
SELECT id, user_id, amount, method, status, created_at
FROM payments
WHERE id = $1;
`
	var (
		payID   string
		userID  string
		amount  float64
		method  string
		status  string
		created time.Time
	)

	row := r.pool.QueryRow(ctx, sql, id)
	if err := row.Scan(&payID, &userID, &amount, &method, &status, &created); err != nil {
		if err == pgx.ErrNoRows {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("postgres FindByID payment scan: %w", err)
	}

	p := &domain.Payment{
		ID:        payID,
		UserID:    userID,
		Amount:    amount, // in Toman (float64)
		Method:    method,
		Status:    domain.PaymentStatus(status), // cast string -> PaymentStatus
		CreatedAt: created,
	}
	return p, nil
}

// TotalPaymentsInPeriod sums payments' amount between since (inclusive) and till (exclusive).
// Returns amount in Toman (float64).
func (r *PostgresPaymentRepo) TotalPaymentsInPeriod(ctx context.Context, since, till time.Time) (float64, error) {
	const sql = `
SELECT COALESCE(SUM(amount), 0) FROM payments
WHERE status = 'paid' AND created_at >= $1 AND created_at < $2;
`
	var sum float64
	row := r.pool.QueryRow(ctx, sql, since, till)
	if err := row.Scan(&sum); err != nil {
		return 0, fmt.Errorf("postgres TotalPaymentsInPeriod: %w", err)
	}
	return sum, nil
}

// Ensure interface compliance at compile time
var _ repository.PaymentRepository = (*PostgresPaymentRepo)(nil)
