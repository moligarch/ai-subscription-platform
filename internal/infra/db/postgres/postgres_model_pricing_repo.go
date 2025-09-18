package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"

	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/repository"
)

var _ repository.ModelPricingRepository = (*modelPricingRepo)(nil)

type modelPricingRepo struct {
	pool *pgxpool.Pool
}

func NewModelPricingRepo(pool *pgxpool.Pool) *modelPricingRepo {
	return &modelPricingRepo{pool: pool}
}

func (r *modelPricingRepo) GetByModelName(ctx context.Context, tx repository.Tx, name string) (*model.ModelPricing, error) {
	const q = `
SELECT id, model_name, input_token_price_micros, output_token_price_micros, active, created_at, updated_at
  FROM model_pricing
 WHERE model_name=$1 AND active=TRUE
 LIMIT 1;`
	row, err := pickRow(ctx, r.pool, tx, q, name)
	if err != nil {
		return nil, domain.ErrOperationFailed
	}
	var p model.ModelPricing
	if err := row.Scan(&p.ID, &p.ModelName, &p.InputTokenPriceMicros, &p.OutputTokenPriceMicros, &p.Active, &p.CreatedAt, &p.UpdatedAt); err != nil {
		if err == pgx.ErrNoRows {
			return nil, domain.ErrNotFound
		}
		return nil, domain.ErrReadDatabaseRow
	}
	return &p, nil
}

func (r *modelPricingRepo) Create(ctx context.Context, tx repository.Tx, p *model.ModelPricing) error {
	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	now := time.Now()
	p.CreatedAt = now
	p.UpdatedAt = now
	const q = `
INSERT INTO model_pricing (id, model_name, input_token_price_micros, output_token_price_micros, active, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7);`
	_, err := execSQL(ctx, r.pool, tx, q, p.ID, p.ModelName, p.InputTokenPriceMicros, p.OutputTokenPriceMicros, p.Active, p.CreatedAt, p.UpdatedAt)
	return err
}

func (r *modelPricingRepo) Update(ctx context.Context, tx repository.Tx, p *model.ModelPricing) error {
	p.UpdatedAt = time.Now()
	const q = `
UPDATE model_pricing SET
  model_name = $2, -- Also allow updating the name
  input_token_price_micros = $3,
  output_token_price_micros = $4,
  active = $5,
  updated_at = $6
WHERE id = $1;`
	_, err := execSQL(ctx, r.pool, tx, q, p.ID, p.ModelName, p.InputTokenPriceMicros, p.OutputTokenPriceMicros, p.Active, p.UpdatedAt)
	return err
}

func (r *modelPricingRepo) ListActive(ctx context.Context, tx repository.Tx) ([]*model.ModelPricing, error) {
	const q = `
SELECT id, model_name, input_token_price_micros, output_token_price_micros, active, created_at, updated_at
  FROM model_pricing WHERE active=TRUE ORDER BY model_name ASC;`
	rows, err := queryRows(ctx, r.pool, tx, q)
	if err != nil {
		switch err {
		case pgx.ErrNoRows:
			return nil, domain.ErrNotFound
		default:
			return nil, domain.ErrOperationFailed
		}
	}
	defer rows.Close()

	var out []*model.ModelPricing
	for rows.Next() {
		var p model.ModelPricing
		if err := rows.Scan(&p.ID, &p.ModelName, &p.InputTokenPriceMicros, &p.OutputTokenPriceMicros, &p.Active, &p.CreatedAt, &p.UpdatedAt); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, domain.ErrNotFound
			}
			return nil, domain.ErrReadDatabaseRow
		}
		out = append(out, &p)
	}
	if rows.Err() != nil {
		return nil, domain.ErrReadDatabaseRow
	}
	return out, nil
}
