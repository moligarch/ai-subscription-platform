package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"

	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/repository"
)

// Ensure implementation satisfies the interface.
var _ repository.ActivationCodeRepository = (*activationCodeRepo)(nil)

type activationCodeRepo struct {
	pool *pgxpool.Pool
}

func NewActivationCodeRepo(pool *pgxpool.Pool) repository.ActivationCodeRepository {
	return &activationCodeRepo{pool: pool}
}

// Save creates or updates an activation code. The logic uses ON CONFLICT
// to handle both creating a new code and marking an existing one as redeemed.
func (r *activationCodeRepo) Save(ctx context.Context, tx repository.Tx, code *model.ActivationCode) error {
	if code.ID == "" {
		code.ID = uuid.NewString()
	}

	const q = `
INSERT INTO activation_codes (id, code, plan_id, is_redeemed, redeemed_by_user_id, redeemed_at, created_at, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (id) DO UPDATE SET
  is_redeemed = EXCLUDED.is_redeemed,
  redeemed_by_user_id = EXCLUDED.redeemed_by_user_id,
  redeemed_at = EXCLUDED.redeemed_at;
`
	_, err := execSQL(ctx, r.pool, tx, q,
		code.ID, code.Code, code.PlanID, code.IsRedeemed, code.RedeemedByUserID, code.RedeemedAt, code.CreatedAt, code.ExpiresAt,
	)
	return err
}

// FindByCode finds a single, unredeemed activation code.
// This is the primary method used during the redemption flow.
func (r *activationCodeRepo) FindByCode(ctx context.Context, tx repository.Tx, code string) (*model.ActivationCode, error) {
	const q = `
SELECT id, code, plan_id, is_redeemed, redeemed_by_user_id, redeemed_at, created_at, expires_at
  FROM activation_codes
 WHERE code = $1 AND is_redeemed = FALSE;
`
	row, err := pickRow(ctx, r.pool, tx, q, code)
	if err != nil {
		return nil, err
	}

	var ac model.ActivationCode
	err = row.Scan(
		&ac.ID, &ac.Code, &ac.PlanID, &ac.IsRedeemed, &ac.RedeemedByUserID, &ac.RedeemedAt, &ac.CreatedAt, &ac.ExpiresAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, domain.ErrReadDatabaseRow
	}
	return &ac, nil
}
