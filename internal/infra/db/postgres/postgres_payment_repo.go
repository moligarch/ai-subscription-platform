package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"

	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/repository"
)

var _ repository.PaymentRepository = (*paymentRepo)(nil)

type paymentRepo struct{ pool *pgxpool.Pool }

func NewPaymentRepo(pool *pgxpool.Pool) *paymentRepo {
	return &paymentRepo{pool: pool}
}

func (r *paymentRepo) Save(ctx context.Context, tx repository.Tx, p *model.Payment) error {
	const q = `
INSERT INTO payments (
  id, user_id, plan_id, provider, amount, currency, authority, ref_id, status, created_at, updated_at, paid_at, callback, description, meta, subscription_id, activation_code, activation_expires_at
) VALUES (
  $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18
) ON CONFLICT (id) DO UPDATE SET
  user_id=$2, plan_id=$3, provider=$4, amount=$5, currency=$6, authority=$7, ref_id=$8, status=$9, updated_at=$11, paid_at=$12, callback=$13, description=$14, meta=$15, subscription_id=$16, activation_code=$17, activation_expires_at=$18;`

	_, err := execSQL(ctx, r.pool, tx, q, p.ID, p.UserID, p.PlanID, p.Provider, p.Amount, p.Currency, p.Authority, p.RefID, p.Status, p.CreatedAt, p.UpdatedAt, p.PaidAt, p.Callback, p.Description, p.Meta, p.SubscriptionID, p.ActivationCode, p.ActivationExpiresAt)
	if err != nil {
		if err == domain.ErrInvalidArgument || err == domain.ErrInvalidExecContext {
			return err
		}
		return domain.ErrOperationFailed
	}
	return nil
}

func (r *paymentRepo) FindByID(ctx context.Context, tx repository.Tx, id string) (*model.Payment, error) {
	q := `SELECT id, user_id, plan_id, provider, amount, currency, authority, ref_id, status, created_at, updated_at, paid_at, callback, description, meta, subscription_id, activation_code, activation_expires_at FROM payments WHERE id=$1`
	if _, ok := tx.(pgx.Tx); ok {
		q += " FOR UPDATE"
	}
	q += ";"
	row, err := pickRow(ctx, r.pool, nil, q, id)
	if err != nil {
		return nil, err
	}

	p := &model.Payment{}
	if err := row.Scan(&p.ID, &p.UserID, &p.PlanID, &p.Provider, &p.Amount, &p.Currency, &p.Authority, &p.RefID, &p.Status, &p.CreatedAt, &p.UpdatedAt, &p.PaidAt, &p.Callback, &p.Description, &p.Meta, &p.SubscriptionID, &p.ActivationCode, &p.ActivationExpiresAt); err != nil {
		return nil, domain.ErrReadDatabaseRow
	}

	return p, nil
}

func (r *paymentRepo) FindByAuthority(ctx context.Context, tx repository.Tx, authority string) (*model.Payment, error) {
	q := `SELECT id, user_id, plan_id, provider, amount, currency, authority, ref_id, status, created_at, updated_at, paid_at, callback, description, meta, subscription_id, activation_code, activation_expires_at FROM payments WHERE authority=$1 LIMIT 1`
	if _, ok := tx.(pgx.Tx); ok {
		q += " FOR UPDATE"
	}
	q += ";"
	row, err := pickRow(ctx, r.pool, nil, q, authority)
	if err != nil {
		return nil, err
	}

	p := &model.Payment{}
	if err := row.Scan(&p.ID, &p.UserID, &p.PlanID, &p.Provider, &p.Amount, &p.Currency, &p.Authority, &p.RefID, &p.Status, &p.CreatedAt, &p.UpdatedAt, &p.PaidAt, &p.Callback, &p.Description, &p.Meta, &p.SubscriptionID, &p.ActivationCode, &p.ActivationExpiresAt); err != nil {
		return nil, domain.ErrReadDatabaseRow
	}

	return p, nil
}

func (r *paymentRepo) UpdateStatus(ctx context.Context, tx repository.Tx, id string, status model.PaymentStatus, refID *string, paidAt *time.Time) error {
	const q = `UPDATE payments SET status=$2, ref_id=COALESCE($3, ref_id), paid_at=COALESCE($4, paid_at), updated_at=NOW() WHERE id=$1;`
	_, err := execSQL(ctx, r.pool, tx, q, id, status, refID, paidAt)
	if err != nil {
		if err == domain.ErrInvalidArgument || err == domain.ErrInvalidExecContext {
			return err
		}
		return domain.ErrOperationFailed
	}
	return nil
}

func (r *paymentRepo) SumByPeriod(ctx context.Context, tx repository.Tx, period string) (int64, error) {
	const q = `SELECT COALESCE(SUM(amount),0) FROM payments WHERE status='succeeded' AND paid_at >= DATE_TRUNC($1, NOW());`
	row, err := pickRow(ctx, r.pool, nil, q, period)
	if err != nil {
		return 0, err
	}

	var sum int64
	if err := row.Scan(&sum); err != nil {
		return 0, domain.ErrReadDatabaseRow
	}

	return sum, nil
}

func (r *paymentRepo) SetActivationCode(ctx context.Context, tx repository.Tx, paymentID string, code string, expiresAt time.Time) error {
	const q = `UPDATE payments SET activation_code=$2, activation_expires_at=$3, updated_at=NOW() WHERE id=$1;`
	_, err := execSQL(ctx, r.pool, tx, q, paymentID, code, expiresAt)
	if err != nil {
		if err == domain.ErrInvalidArgument || err == domain.ErrInvalidExecContext {
			return err
		}
		return domain.ErrOperationFailed
	}
	return nil
}

func (r *paymentRepo) FindByActivationCode(ctx context.Context, tx repository.Tx, code string) (*model.Payment, error) {
	const q = `SELECT id, user_id, plan_id, provider, amount, currency, authority, ref_id, status, created_at, updated_at, paid_at, callback, description, meta, subscription_id, activation_code, activation_expires_at FROM payments WHERE activation_code=$1 LIMIT 1;`
	row, err := pickRow(ctx, r.pool, nil, q, code)
	if err != nil {
		return nil, err
	}

	p := &model.Payment{}
	if err := row.Scan(&p.ID, &p.UserID, &p.PlanID, &p.Provider, &p.Amount, &p.Currency, &p.Authority, &p.RefID, &p.Status, &p.CreatedAt, &p.UpdatedAt, &p.PaidAt, &p.Callback, &p.Description, &p.Meta, &p.SubscriptionID, &p.ActivationCode, &p.ActivationExpiresAt); err != nil {
		return nil, domain.ErrReadDatabaseRow
	}

	return p, nil
}

func (r *paymentRepo) ListPendingOlderThan(ctx context.Context, tx repository.Tx, olderThan time.Time, limit int) ([]*model.Payment, error) {
	if limit <= 0 {
		limit = 100
	}
	const q = `SELECT id, user_id, plan_id, provider, amount, currency, authority, ref_id, status, created_at, updated_at, paid_at, callback, description, meta, subscription_id, activation_code, activation_expires_at FROM payments WHERE status='pending' AND created_at < $1 ORDER BY created_at ASC LIMIT $2;`
	rows, err := queryRows(ctx, r.pool, nil, q, olderThan, limit)
	if err != nil {
		switch err {
		case pgx.ErrNoRows:
			return nil, domain.ErrNotFound
		case domain.ErrInvalidArgument, domain.ErrInvalidExecContext:
			return nil, err
		default:
			return nil, domain.ErrOperationFailed
		}
	}
	defer rows.Close()

	var out []*model.Payment
	for rows.Next() {
		p := new(model.Payment)
		if err := rows.Scan(&p.ID, &p.UserID, &p.PlanID, &p.Provider, &p.Amount, &p.Currency, &p.Authority, &p.RefID, &p.Status, &p.CreatedAt, &p.UpdatedAt, &p.PaidAt, &p.Callback, &p.Description, &p.Meta, &p.SubscriptionID, &p.ActivationCode, &p.ActivationExpiresAt); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, domain.ErrNotFound
			}
			return nil, domain.ErrReadDatabaseRow
		}
		out = append(out, p)
	}
	return out, nil
}

// UpdateStatusIfPending atomically updates status only when current status is 'pending' or 'initiated'.
func (r *paymentRepo) UpdateStatusIfPending(
	ctx context.Context, tx repository.Tx, id string, status model.PaymentStatus, refID *string, paidAt *time.Time,
) (bool, error) {
	query := `
    UPDATE payments
       SET status = $2,
           ref_id = $3,
           paid_at = $4,
           updated_at = NOW()
     WHERE id = $1
       AND status IN ('pending','initiated')`

	cmd, err := execSQL(ctx, r.pool, tx, query, id, string(status), refID, paidAt)
	if err != nil {
		if err == domain.ErrInvalidArgument || err == domain.ErrInvalidExecContext {
			return false, err
		}
		return false, domain.ErrOperationFailed
	}
	return cmd.RowsAffected() >= 1, nil
}
