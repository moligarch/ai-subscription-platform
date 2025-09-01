// File: internal/infra/db/postgres/postgres_model_pricing_repo.go
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"

	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/repository"
)

var _ repository.ModelPricingRepository = (*PostgresModelPricingRepo)(nil)

type PostgresModelPricingRepo struct {
	pool *pgxpool.Pool
}

func NewPostgresModelPricingRepo(pool *pgxpool.Pool) *PostgresModelPricingRepo {
	return &PostgresModelPricingRepo{pool: pool}
}

func (r *PostgresModelPricingRepo) GetByModelName(ctx context.Context, name string) (*model.ModelPricing, error) {
	const q = `
SELECT id, model_name, input_token_price_micros, output_token_price_micros, active, created_at, updated_at
  FROM model_pricing
 WHERE model_name=$1 AND active=TRUE
 LIMIT 1;`
	row := r.pool.QueryRow(ctx, q, name)
	var p model.ModelPricing
	if err := row.Scan(&p.ID, &p.ModelName, &p.InputTokenPriceMicros, &p.OutputTokenPriceMicros, &p.Active, &p.CreatedAt, &p.UpdatedAt); err != nil {
		// align with repo conventions
		if errors.Is(err, pgx.ErrNoRows) { // follows pattern used elsewhere
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("model_pricing.get: %w", err)
	}
	return &p, nil
}

func (r *PostgresModelPricingRepo) Save(ctx context.Context, p *model.ModelPricing) error {
	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	now := time.Now()
	p.UpdatedAt = now
	const q = `
INSERT INTO model_pricing (id, model_name, input_token_price_micros, output_token_price_micros, active, created_at, updated_at)
VALUES ($1,$2,$3,$4,$5,COALESCE($6,NOW()),$7)
ON CONFLICT (id) DO UPDATE SET
  model_name=EXCLUDED.model_name,
  input_token_price_micros=EXCLUDED.input_token_price_micros,
  output_token_price_micros=EXCLUDED.output_token_price_micros,
  active=EXCLUDED.active,
  updated_at=EXCLUDED.updated_at;`
	_, err := r.pool.Exec(ctx, q, p.ID, p.ModelName, p.InputTokenPriceMicros, p.OutputTokenPriceMicros, p.Active, p.CreatedAt, p.UpdatedAt)
	return err
}

func (r *PostgresModelPricingRepo) ListActive(ctx context.Context) ([]*model.ModelPricing, error) {
	const q = `
SELECT id, model_name, input_token_price_micros, output_token_price_micros, active, created_at, updated_at
  FROM model_pricing WHERE active=TRUE ORDER BY model_name ASC;`
	rows, err := r.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("model_pricing.list: %w", err)
	}
	defer rows.Close()
	var out []*model.ModelPricing
	for rows.Next() {
		var p model.ModelPricing
		if err := rows.Scan(&p.ID, &p.ModelName, &p.InputTokenPriceMicros, &p.OutputTokenPriceMicros, &p.Active, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("model_pricing.scan: %w", err)
		}
		out = append(out, &p)
	}
	return out, rows.Err()
}
