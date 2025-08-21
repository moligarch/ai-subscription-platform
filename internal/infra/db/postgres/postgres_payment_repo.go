package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"

	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/repository"
)

var _ repository.PaymentRepository = (*PostgresPaymentRepo)(nil)

type PostgresPaymentRepo struct {
	db *pgxpool.Pool
}

func NewPostgresPaymentRepo(db *pgxpool.Pool) *PostgresPaymentRepo {
	return &PostgresPaymentRepo{db: db}
}

var _ repository.PaymentRepository = (*PostgresPaymentRepo)(nil)

func (r *PostgresPaymentRepo) Save(ctx context.Context, p *model.Payment) error {
	meta := []byte("null")
	if p.Meta != nil {
		b, err := json.Marshal(p.Meta)
		if err != nil {
			return err
		}
		meta = b
	}
	_, err := r.db.Exec(ctx, `
		INSERT INTO payments (id, user_id, plan_id, provider, amount, currency, authority, ref_id, status,
			created_at, updated_at, paid_at, callback, description, meta, subscription_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15::jsonb,$16)
	`, p.ID, p.UserID, p.PlanID, p.Provider, p.Amount, p.Currency, p.Authority, p.RefID, string(p.Status),
		p.CreatedAt, p.UpdatedAt, p.PaidAt, p.Callback, p.Description, string(meta), p.SubscriptionID)
	return err
}

func (r *PostgresPaymentRepo) Update(ctx context.Context, p *model.Payment) error {
	meta := []byte("null")
	if p.Meta != nil {
		b, err := json.Marshal(p.Meta)
		if err != nil {
			return err
		}
		meta = b
	}
	_, err := r.db.Exec(ctx, `
		UPDATE payments SET user_id=$2, plan_id=$3, provider=$4, amount=$5, currency=$6, authority=$7, ref_id=$8,
			status=$9, created_at=$10, updated_at=$11, paid_at=$12, callback=$13, description=$14, meta=$15::jsonb, subscription_id=$16
		WHERE id=$1
	`, p.ID, p.UserID, p.PlanID, p.Provider, p.Amount, p.Currency, p.Authority, p.RefID,
		string(p.Status), p.CreatedAt, p.UpdatedAt, p.PaidAt, p.Callback, p.Description, string(meta), p.SubscriptionID)
	return err
}

func (r *PostgresPaymentRepo) Get(ctx context.Context, id string) (*model.Payment, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, user_id, plan_id, provider, amount, currency, authority, ref_id, status,
		       created_at, updated_at, paid_at, callback, description, meta, subscription_id
		FROM payments WHERE id=$1
	`, id)
	return scanPayment(row)
}

func (r *PostgresPaymentRepo) GetByAuthority(ctx context.Context, authority string) (*model.Payment, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, user_id, plan_id, provider, amount, currency, authority, ref_id, status,
		       created_at, updated_at, paid_at, callback, description, meta, subscription_id
		FROM payments WHERE authority=$1
	`, authority)
	return scanPayment(row)
}

func (r *PostgresPaymentRepo) TotalPaymentsSince(ctx context.Context, since time.Time) (int64, error) {
	var sum int64
	err := r.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(amount),0) FROM payments
		WHERE status='succeeded' AND paid_at >= $1
	`, since).Scan(&sum)
	return sum, err
}

func (r *PostgresPaymentRepo) TotalPaymentsAll(ctx context.Context) (int64, error) {
	var sum int64
	err := r.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(amount),0) FROM payments
		WHERE status='succeeded'
	`).Scan(&sum)
	return sum, err
}

func scanPayment(row pgx.Row) (*model.Payment, error) {
	var p model.Payment
	var status string
	var metaRaw []byte
	err := row.Scan(&p.ID, &p.UserID, &p.PlanID, &p.Provider, &p.Amount, &p.Currency, &p.Authority, &p.RefID, &status,
		&p.CreatedAt, &p.UpdatedAt, &p.PaidAt, &p.Callback, &p.Description, &metaRaw, &p.SubscriptionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	p.Status = model.PaymentStatus(status)
	if len(metaRaw) > 0 && string(metaRaw) != "null" {
		_ = json.Unmarshal(metaRaw, &p.Meta)
	}
	return &p, nil
}
