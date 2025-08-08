// File: internal/infrastructure/db/postgres_user_repo.go
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
// Call this once during app bootstrap, passing in a configured pgxpool.Pool.
func NewPostgresUserRepository(pool *pgxpool.Pool) *PostgresUserRepository {
    return &PostgresUserRepository{pool: pool}
}

// Save inserts or updates a user.
// On conflict (same telegram_id), it updates full_name, phone and created_at.
func (r *PostgresUserRepository) Save(ctx context.Context, u *domain.User) error {
    const sql = `
INSERT INTO users (id, telegram_id, full_name, phone, created_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (telegram_id) DO UPDATE
  SET full_name    = EXCLUDED.full_name,
      phone        = EXCLUDED.phone,
      created_at   = EXCLUDED.created_at;
`
    // Use Exec, passing the domain.User fields.
    _, err := r.pool.Exec(ctx, sql,
        u.ID,
        u.TelegramID,
        u.FullName,
        u.Phone,
        u.CreatedAt,
    )
    if err != nil {
        return fmt.Errorf("postgres: saving user: %w", err)
    }
    return nil
}

// FindByTelegramID looks up a user by their Telegram ID.
func (r *PostgresUserRepository) FindByTelegramID(ctx context.Context, tgID int64) (*domain.User, error) {
    const sql = `
SELECT id, telegram_id, full_name, phone, created_at
  FROM users
 WHERE telegram_id = $1;
`
    row := r.pool.QueryRow(ctx, sql, tgID)

    var (
        id          string
        telegramID  int64
        fullName    string
        phone       string
        createdAt   time.Time
    )
    if err := row.Scan(&id, &telegramID, &fullName, &phone, &createdAt); err != nil {
        // pgx returns pgx.ErrNoRows if not found
        if err == pgx.ErrNoRows {
            return nil, domain.ErrNotFound
        }
        return nil, fmt.Errorf("postgres: querying user: %w", err)
    }

    // Construct the domain entity with its original CreatedAt
    user := &domain.User{
        ID:         id,
        TelegramID: telegramID,
        FullName:   fullName,
        Phone:      phone,
        CreatedAt:  createdAt,
    }
    return user, nil
}
