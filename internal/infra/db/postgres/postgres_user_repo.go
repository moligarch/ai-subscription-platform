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

var _ repository.UserRepository = (*PostgresUserRepository)(nil)

// PostgresUserRepository is a Postgres adapter for domain.UserRepository.
type PostgresUserRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresUserRepository constructs a new PostgresUserRepository.
func NewPostgresUserRepository(pool *pgxpool.Pool) *PostgresUserRepository {
	return &PostgresUserRepository{pool: pool}
}

// Save inserts or updates a user.
// On conflict of telegram_id, updates username and registered_at.
func (r *PostgresUserRepository) Save(ctx context.Context, u *model.User) error {
	const sql = `
INSERT INTO users (id, telegram_id, username, registered_at, last_active_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (telegram_id) DO UPDATE
  SET username        = EXCLUDED.username,
      last_active_at  = EXCLUDED.last_active_at;
`
	_, err := r.pool.Exec(ctx, sql,
		u.ID,
		u.TelegramID,
		u.Username,
		u.RegisteredAt,
		u.LastActiveAt,
	)
	if err != nil {
		return fmt.Errorf("postgres: saving user: %w", err)
	}
	return nil
}

// FindByTelegramID looks up a user by their Telegram ID.
func (r *PostgresUserRepository) FindByTelegramID(ctx context.Context, tgID int64) (*model.User, error) {
	const sql = `
SELECT id, telegram_id, username, registered_at, last_active_at
  FROM users
 WHERE telegram_id = $1;
`
	row := r.pool.QueryRow(ctx, sql, tgID)

	var (
		id           string
		telegramID   int64
		username     string
		registeredAt time.Time
		lastActiveAt time.Time
	)
	if err := row.Scan(&id, &telegramID, &username, &registeredAt, &lastActiveAt); err != nil {
		if err == pgx.ErrNoRows {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("postgres: querying user: %w", err)
	}

	return &model.User{
		ID:           id,
		TelegramID:   telegramID,
		Username:     username,
		RegisteredAt: registeredAt,
		LastActiveAt: lastActiveAt,
	}, nil
}

// CountUsers returns total users count.
func (r *PostgresUserRepository) CountUsers(ctx context.Context) (int, error) {
	const sql = `SELECT COUNT(*) FROM users;`
	var n int
	row := r.pool.QueryRow(ctx, sql)
	if err := row.Scan(&n); err != nil {
		return 0, fmt.Errorf("postgres CountUsers: %w", err)
	}
	return n, nil
}

// CountInactiveUsers counts users whose last_active_at <= since OR (last_active_at IS NULL AND registered_at <= since)
func (r *PostgresUserRepository) CountInactiveUsers(ctx context.Context, since time.Time) (int, error) {
	const sql = `
SELECT COUNT(*) FROM users
WHERE (last_active_at IS NULL AND registered_at <= $1) OR (last_active_at <= $1);
`
	var n int
	row := r.pool.QueryRow(ctx, sql, since)
	if err := row.Scan(&n); err != nil {
		return 0, fmt.Errorf("postgres CountInactiveUsers: %w", err)
	}
	return n, nil
}
