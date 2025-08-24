package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"

	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/repository"
)

var _ repository.PaymentRepository = (*PostgresPaymentRepo)(nil)

type PostgresPaymentRepo struct{ pool *pgxpool.Pool }

func NewPostgresPaymentRepo(pool *pgxpool.Pool) *PostgresPaymentRepo {
	return &PostgresPaymentRepo{pool: pool}
}

func (r *PostgresPaymentRepo) Save(ctx context.Context, qx any, p *model.Payment) error {
	const q = `
INSERT INTO payments (
  id, user_id, plan_id, provider, amount, currency, authority, ref_id, status, created_at, updated_at, paid_at, callback, description, meta, subscription_id, activation_code, activation_expires_at
) VALUES (
  $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18
) ON CONFLICT (id) DO UPDATE SET
  user_id=$2, plan_id=$3, provider=$4, amount=$5, currency=$6, authority=$7, ref_id=$8, status=$9, updated_at=$11, paid_at=$12, callback=$13, description=$14, meta=$15, subscription_id=$16, activation_code=$17, activation_expires_at=$18;`
	switch v := qx.(type) {
	case pgx.Tx:
		_, err := v.Exec(ctx, q, p.ID, p.UserID, p.PlanID, p.Provider, p.Amount, p.Currency, p.Authority, p.RefID, p.Status, p.CreatedAt, p.UpdatedAt, p.PaidAt, p.Callback, p.Description, p.Meta, p.SubscriptionID, p.ActivationCode, p.ActivationExpiresAt)
		return err
	case *pgxpool.Conn:
		_, err := v.Exec(ctx, q, p.ID, p.UserID, p.PlanID, p.Provider, p.Amount, p.Currency, p.Authority, p.RefID, p.Status, p.CreatedAt, p.UpdatedAt, p.PaidAt, p.Callback, p.Description, p.Meta, p.SubscriptionID, p.ActivationCode, p.ActivationExpiresAt)
		return err
	default:
		_, err := r.pool.Exec(ctx, q, p.ID, p.UserID, p.PlanID, p.Provider, p.Amount, p.Currency, p.Authority, p.RefID, p.Status, p.CreatedAt, p.UpdatedAt, p.PaidAt, p.Callback, p.Description, p.Meta, p.SubscriptionID, p.ActivationCode, p.ActivationExpiresAt)
		return err
	}
}

func (r *PostgresPaymentRepo) FindByID(ctx context.Context, qx any, id string) (*model.Payment, error) {
	q := `SELECT id, user_id, plan_id, provider, amount, currency, authority, ref_id, status, created_at, updated_at, paid_at, callback, description, meta, subscription_id, activation_code, activation_expires_at FROM payments WHERE id=$1`
	if _, ok := qx.(pgx.Tx); ok {
		q += " FOR UPDATE"
	}
	q += ";"
	row := pickRow(r.pool, qx, q, id)
	p := &model.Payment{}
	if err := row.Scan(&p.ID, &p.UserID, &p.PlanID, &p.Provider, &p.Amount, &p.Currency, &p.Authority, &p.RefID, &p.Status, &p.CreatedAt, &p.UpdatedAt, &p.PaidAt, &p.Callback, &p.Description, &p.Meta, &p.SubscriptionID, &p.ActivationCode, &p.ActivationExpiresAt); err != nil {
		if err == pgx.ErrNoRows {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return p, nil
}

func (r *PostgresPaymentRepo) FindByAuthority(ctx context.Context, qx any, authority string) (*model.Payment, error) {
	q := `SELECT id, user_id, plan_id, provider, amount, currency, authority, ref_id, status, created_at, updated_at, paid_at, callback, description, meta, subscription_id, activation_code, activation_expires_at FROM payments WHERE authority=$1 LIMIT 1`
	if _, ok := qx.(pgx.Tx); ok {
		q += " FOR UPDATE"
	}
	q += ";"
	row := pickRow(r.pool, qx, q, authority)
	p := &model.Payment{}
	if err := row.Scan(&p.ID, &p.UserID, &p.PlanID, &p.Provider, &p.Amount, &p.Currency, &p.Authority, &p.RefID, &p.Status, &p.CreatedAt, &p.UpdatedAt, &p.PaidAt, &p.Callback, &p.Description, &p.Meta, &p.SubscriptionID, &p.ActivationCode, &p.ActivationExpiresAt); err != nil {
		if err == pgx.ErrNoRows {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return p, nil
}

func (r *PostgresPaymentRepo) UpdateStatus(ctx context.Context, qx any, id string, status model.PaymentStatus, refID *string, paidAt *time.Time) error {
	const q = `UPDATE payments SET status=$2, ref_id=COALESCE($3, ref_id), paid_at=COALESCE($4, paid_at), updated_at=NOW() WHERE id=$1;`
	switch v := qx.(type) {
	case pgx.Tx:
		_, err := v.Exec(ctx, q, id, status, refID, paidAt)
		return err
	case *pgxpool.Conn:
		_, err := v.Exec(ctx, q, id, status, refID, paidAt)
		return err
	default:
		_, err := r.pool.Exec(ctx, q, id, status, refID, paidAt)
		return err
	}
}

func (r *PostgresPaymentRepo) SumByPeriod(ctx context.Context, qx any, period string) (int64, error) {
	const q = `SELECT COALESCE(SUM(amount),0) FROM payments WHERE status='succeeded' AND paid_at >= DATE_TRUNC($1, NOW());`
	row := pickRow(r.pool, qx, q, period)
	var sum int64
	if err := row.Scan(&sum); err != nil {
		return 0, fmt.Errorf("sum payments: %w", err)
	}
	return sum, nil
}

func (r *PostgresPaymentRepo) SetActivationCode(ctx context.Context, qx any, paymentID string, code string, expiresAt time.Time) error {
	const q = `UPDATE payments SET activation_code=$2, activation_expires_at=$3, updated_at=NOW() WHERE id=$1;`
	switch v := qx.(type) {
	case pgx.Tx:
		_, err := v.Exec(ctx, q, paymentID, code, expiresAt)
		return err
	case *pgxpool.Conn:
		_, err := v.Exec(ctx, q, paymentID, code, expiresAt)
		return err
	default:
		_, err := r.pool.Exec(ctx, q, paymentID, code, expiresAt)
		return err
	}
}

func (r *PostgresPaymentRepo) FindByActivationCode(ctx context.Context, qx any, code string) (*model.Payment, error) {
	const q = `SELECT id, user_id, plan_id, provider, amount, currency, authority, ref_id, status, created_at, updated_at, paid_at, callback, description, meta, subscription_id, activation_code, activation_expires_at FROM payments WHERE activation_code=$1 LIMIT 1;`
	row := pickRow(r.pool, qx, q, code)
	p := &model.Payment{}
	if err := row.Scan(&p.ID, &p.UserID, &p.PlanID, &p.Provider, &p.Amount, &p.Currency, &p.Authority, &p.RefID, &p.Status, &p.CreatedAt, &p.UpdatedAt, &p.PaidAt, &p.Callback, &p.Description, &p.Meta, &p.SubscriptionID, &p.ActivationCode, &p.ActivationExpiresAt); err != nil {
		if err == pgx.ErrNoRows {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return p, nil
}

func (r *PostgresPaymentRepo) ListPendingOlderThan(ctx context.Context, qx any, olderThan time.Time, limit int) ([]*model.Payment, error) {
	if limit <= 0 {
		limit = 100
	}
	const q = `SELECT id, user_id, plan_id, provider, amount, currency, authority, ref_id, status, created_at, updated_at, paid_at, callback, description, meta, subscription_id, activation_code, activation_expires_at FROM payments WHERE status='pending' AND created_at < $1 ORDER BY created_at ASC LIMIT $2;`
	var rows pgx.Rows
	var err error
	switch v := qx.(type) {
	case pgx.Tx:
		rows, err = v.Query(ctx, q, olderThan, limit)
	case *pgxpool.Conn:
		rows, err = v.Query(ctx, q, olderThan, limit)
	default:
		rows, err = r.pool.Query(ctx, q, olderThan, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.Payment
	for rows.Next() {
		p := new(model.Payment)
		if err := rows.Scan(&p.ID, &p.UserID, &p.PlanID, &p.Provider, &p.Amount, &p.Currency, &p.Authority, &p.RefID, &p.Status, &p.CreatedAt, &p.UpdatedAt, &p.PaidAt, &p.Callback, &p.Description, &p.Meta, &p.SubscriptionID, &p.ActivationCode, &p.ActivationExpiresAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

// pickRow is a small helper used across repos to select the proper executor.
// Implemented here for completeness; if you already have one in this package, remove this and reuse that implementation.

type rowScanner interface {
	Scan(dest ...interface{}) error
}
