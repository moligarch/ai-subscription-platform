package db

import (
    "context"
    "fmt"
    "time"

    "github.com/jackc/pgx/v4"
    "github.com/jackc/pgx/v4/pgxpool"

    "telegram-ai-subscription/internal/domain"
)

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
func (r *PostgresUserRepository) Save(ctx context.Context, u *domain.User) error {
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
func (r *PostgresUserRepository) FindByTelegramID(ctx context.Context, tgID int64) (*domain.User, error) {
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

    return &domain.User{
        ID:           id,
        TelegramID:   telegramID,
        Username:     username,
        RegisteredAt: registeredAt,
        LastActiveAt: lastActiveAt,
    }, nil
}
