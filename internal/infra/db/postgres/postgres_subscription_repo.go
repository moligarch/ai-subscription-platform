package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"

	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/repository"
)

// Ensure PostgresSubscriptionRepo implements repository.SubscriptionRepository
var _ repository.SubscriptionRepository = (*PostgresSubscriptionRepo)(nil)

// PostgresSubscriptionRepo implements repository.SubscriptionRepository for Postgres.
type PostgresSubscriptionRepo struct {
	pool *pgxpool.Pool
}

// NewPostgresSubscriptionRepo constructs a PostgresSubscriptionRepo.
func NewPostgresSubscriptionRepo(pool *pgxpool.Pool) *PostgresSubscriptionRepo {
	return &PostgresSubscriptionRepo{pool: pool}
}

// ---------- Save / SaveTx ----------
func (r *PostgresSubscriptionRepo) Save(ctx context.Context, s *model.UserSubscription) error {
	conn, err := r.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	return r.saveWithExec(ctx, conn.Conn(), s)
}

func (r *PostgresSubscriptionRepo) SaveTx(ctx context.Context, tx pgx.Tx, s *model.UserSubscription) error {
	return r.saveWithExec(ctx, tx, s)
}

func (r *PostgresSubscriptionRepo) saveWithExec(ctx context.Context, execer interface{}, s *model.UserSubscription) error {
	const sqlStr = `
INSERT INTO user_subscriptions (
  id, user_id, plan_id, created_at, scheduled_start_at, start_at, expires_at, remaining_credits, status
)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
ON CONFLICT (id) DO UPDATE
  SET scheduled_start_at = EXCLUDED.scheduled_start_at,
      start_at = EXCLUDED.start_at,
      expires_at = EXCLUDED.expires_at,
      remaining_credits = EXCLUDED.remaining_credits,
      status = EXCLUDED.status;
`
	var err error
	switch e := execer.(type) {
	case pgx.Tx:
		_, err = e.Exec(ctx, sqlStr,
			s.ID, s.UserID, s.PlanID, s.CreatedAt, s.ScheduledStartAt, s.StartAt, s.ExpiresAt, s.RemainingCredits, string(s.Status),
		)
	case *pgxpool.Conn:
		_, err = e.Exec(ctx, sqlStr,
			s.ID, s.UserID, s.PlanID, s.CreatedAt, s.ScheduledStartAt, s.StartAt, s.ExpiresAt, s.RemainingCredits, string(s.Status),
		)
	default:
		// fallback: try pool exec
		_, err = r.pool.Exec(ctx, sqlStr,
			s.ID, s.UserID, s.PlanID, s.CreatedAt, s.ScheduledStartAt, s.StartAt, s.ExpiresAt, s.RemainingCredits, string(s.Status),
		)
	}
	if err != nil {
		return fmt.Errorf("postgres Save subscription: %w", err)
	}
	return nil
}

// ---------- FindActiveByUserAndPlan ----------
func (r *PostgresSubscriptionRepo) FindActiveByUserAndPlan(ctx context.Context, userID, planID string) (*model.UserSubscription, error) {
	conn, err := r.pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Release()
	return r.findActiveByUserAndPlanWithQuery(ctx, conn.Conn(), userID, planID)
}

func (r *PostgresSubscriptionRepo) FindActiveByUserAndPlanTx(ctx context.Context, tx pgx.Tx, userID, planID string) (*model.UserSubscription, error) {
	return r.findActiveByUserAndPlanWithQuery(ctx, tx, userID, planID)
}

func (r *PostgresSubscriptionRepo) findActiveByUserAndPlanWithQuery(ctx context.Context, qx interface{}, userID, planID string) (*model.UserSubscription, error) {
	const sqlStr = `
SELECT id, user_id, plan_id, created_at, scheduled_start_at, start_at, expires_at, remaining_credits, status
FROM user_subscriptions
WHERE user_id = $1 AND plan_id = $2 AND status = 'active'
LIMIT 1;
`
	var row pgx.Row
	switch q := qx.(type) {
	case pgx.Tx:
		row = q.QueryRow(ctx, sqlStr, userID, planID)
	case *pgxpool.Conn:
		row = q.QueryRow(ctx, sqlStr, userID, planID)
	default:
		row = r.pool.QueryRow(ctx, sqlStr, userID, planID)
	}
	var s model.UserSubscription
	var status string
	err := row.Scan(&s.ID, &s.UserID, &s.PlanID, &s.CreatedAt, &s.ScheduledStartAt, &s.StartAt, &s.ExpiresAt, &s.RemainingCredits, &status)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("not found: %w", err)
		}
		return nil, fmt.Errorf("postgres FindActiveByUserAndPlan: %w", err)
	}
	s.Status = model.SubscriptionStatus(status)
	return &s, nil
}

// ---------- FindActiveByUser / Tx ----------
func (r *PostgresSubscriptionRepo) FindActiveByUser(ctx context.Context, userID string) (*model.UserSubscription, error) {
	conn, err := r.pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Release()
	return r.findActiveByUserWithQuery(ctx, conn.Conn(), userID)
}

func (r *PostgresSubscriptionRepo) FindActiveByUserTx(ctx context.Context, tx pgx.Tx, userID string) (*model.UserSubscription, error) {
	return r.findActiveByUserWithQuery(ctx, tx, userID)
}

func (r *PostgresSubscriptionRepo) findActiveByUserWithQuery(ctx context.Context, qx interface{}, userID string) (*model.UserSubscription, error) {
	const sqlStr = `
SELECT id, user_id, plan_id, created_at, scheduled_start_at, start_at, expires_at, remaining_credits, status
FROM user_subscriptions
WHERE user_id = $1 AND status IN ('active', 'reserved')
ORDER BY expires_at DESC;
`
	var rows pgx.Rows
	var err error
	switch q := qx.(type) {
	case pgx.Tx:
		rows, err = q.Query(ctx, sqlStr, userID)
	case *pgxpool.Conn:
		rows, err = q.Query(ctx, sqlStr, userID)
	default:
		rows, err = r.pool.Query(ctx, sqlStr, userID)
	}
	if err != nil {
		return nil, fmt.Errorf("postgres FindActiveByUser query: %w", err)
	}
	defer rows.Close()

	var s model.UserSubscription
	var status string
	if rows.Next() {
		if err := rows.Scan(&s.ID, &s.UserID, &s.PlanID, &s.CreatedAt, &s.ScheduledStartAt, &s.StartAt, &s.ExpiresAt, &s.RemainingCredits, &status); err != nil {
			return nil, fmt.Errorf("postgres FindActiveByUser scan: %w", err)
		}
	}
	s.Status = model.SubscriptionStatus(status)
	return &s, nil
}

// ---------- FindReservedByUser / Tx ----------
func (r *PostgresSubscriptionRepo) FindReservedByUser(ctx context.Context, userID string) ([]*model.UserSubscription, error) {
	conn, err := r.pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Release()
	return r.findReservedByUserWithQuery(ctx, conn.Conn(), userID)
}

func (r *PostgresSubscriptionRepo) FindReservedByUserTx(ctx context.Context, tx pgx.Tx, userID string) ([]*model.UserSubscription, error) {
	return r.findReservedByUserWithQuery(ctx, tx, userID)
}

func (r *PostgresSubscriptionRepo) findReservedByUserWithQuery(ctx context.Context, qx interface{}, userID string) ([]*model.UserSubscription, error) {
	const sqlStr = `
SELECT id, user_id, plan_id, created_at, scheduled_start_at, start_at, expires_at, remaining_credits, status
FROM user_subscriptions
WHERE user_id = $1 AND status = 'reserved'
ORDER BY scheduled_start_at ASC NULLS LAST, created_at ASC;
`
	var rows pgx.Rows
	var err error
	switch q := qx.(type) {
	case pgx.Tx:
		rows, err = q.Query(ctx, sqlStr, userID)
	case *pgxpool.Conn:
		rows, err = q.Query(ctx, sqlStr, userID)
	default:
		rows, err = r.pool.Query(ctx, sqlStr, userID)
	}
	if err != nil {
		return nil, fmt.Errorf("postgres FindReservedByUser query: %w", err)
	}
	defer rows.Close()

	out := []*model.UserSubscription{}
	for rows.Next() {
		var s model.UserSubscription
		var status string
		if err := rows.Scan(&s.ID, &s.UserID, &s.PlanID, &s.CreatedAt, &s.ScheduledStartAt, &s.StartAt, &s.ExpiresAt, &s.RemainingCredits, &status); err != nil {
			return nil, fmt.Errorf("postgres FindReservedByUser scan: %w", err)
		}
		s.Status = model.SubscriptionStatus(status)
		out = append(out, &s)
	}
	return out, nil
}

// ---------- FindByID / Tx ----------
func (r *PostgresSubscriptionRepo) FindByID(ctx context.Context, id string) (*model.UserSubscription, error) {
	conn, err := r.pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Release()
	return r.findByIDWithQuery(ctx, conn.Conn(), id)
}

func (r *PostgresSubscriptionRepo) FindByIDTx(ctx context.Context, tx pgx.Tx, id string) (*model.UserSubscription, error) {
	return r.findByIDWithQuery(ctx, tx, id)
}

func (r *PostgresSubscriptionRepo) findByIDWithQuery(ctx context.Context, qx interface{}, id string) (*model.UserSubscription, error) {
	const sqlStr = `
SELECT id, user_id, plan_id, created_at, scheduled_start_at, start_at, expires_at, remaining_credits, status
FROM user_subscriptions
WHERE id = $1;
`
	var row pgx.Row
	switch q := qx.(type) {
	case pgx.Tx:
		row = q.QueryRow(ctx, sqlStr, id)
	case *pgxpool.Conn:
		row = q.QueryRow(ctx, sqlStr, id)
	default:
		row = r.pool.QueryRow(ctx, sqlStr, id)
	}
	var s model.UserSubscription
	var status string
	err := row.Scan(&s.ID, &s.UserID, &s.PlanID, &s.CreatedAt, &s.ScheduledStartAt, &s.StartAt, &s.ExpiresAt, &s.RemainingCredits, &status)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("not found: %w", err)
		}
		return nil, fmt.Errorf("postgres FindByID: %w", err)
	}
	s.Status = model.SubscriptionStatus(status)
	return &s, nil
}

// ---------- FindExpiring ----------
func (r *PostgresSubscriptionRepo) FindExpiring(ctx context.Context, withinDays int) ([]*model.UserSubscription, error) {
	conn, err := r.pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Release()

	const sqlStr = `
SELECT id, user_id, plan_id, created_at, scheduled_start_at, start_at, expires_at, remaining_credits, status
FROM user_subscriptions
WHERE status = 'active' AND expires_at <= now() + ($1 || ' days')::interval
ORDER BY expires_at ASC;
`
	rows, err := conn.Query(ctx, sqlStr, withinDays)
	if err != nil {
		return nil, fmt.Errorf("postgres FindExpiring: %w", err)
	}
	defer rows.Close()

	out := []*model.UserSubscription{}
	for rows.Next() {
		var s model.UserSubscription
		var status string
		if err := rows.Scan(&s.ID, &s.UserID, &s.PlanID, &s.CreatedAt, &s.ScheduledStartAt, &s.StartAt, &s.ExpiresAt, &s.RemainingCredits, &status); err != nil {
			return nil, fmt.Errorf("postgres FindExpiring scan: %w", err)
		}
		s.Status = model.SubscriptionStatus(status)
		out = append(out, &s)
	}
	return out, nil
}

// ---------- CountActiveByPlan ----------
func (r *PostgresSubscriptionRepo) CountActiveByPlan(ctx context.Context) (map[string]int, error) {
	conn, err := r.pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Release()

	const sqlStr = `
SELECT plan_id, COUNT(1) AS cnt
FROM user_subscriptions
WHERE status IN ('active','reserved')
GROUP BY plan_id;
`
	rows, err := conn.Query(ctx, sqlStr)
	if err != nil {
		return nil, fmt.Errorf("postgres CountActiveByPlan query: %w", err)
	}
	defer rows.Close()

	out := map[string]int{}
	for rows.Next() {
		var pid string
		var cnt int
		if err := rows.Scan(&pid, &cnt); err != nil {
			return nil, fmt.Errorf("postgres CountActiveByPlan scan: %w", err)
		}
		out[pid] = cnt
	}
	return out, nil
}

func (r *PostgresSubscriptionRepo) TotalRemainingCredits(ctx context.Context) (int, error) {
	conn, err := r.pool.Acquire(ctx)
	if err != nil {
		return 0, err
	}
	defer conn.Release()

	const sqlStr = `
SELECT COALESCE(SUM(remaining_credits), 0) FROM user_subscriptions
WHERE status IN ('active','reserved');`
	row := conn.QueryRow(ctx, sqlStr)
	var total int
	if err := row.Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}
