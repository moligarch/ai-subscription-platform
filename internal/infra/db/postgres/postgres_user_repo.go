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

var _ repository.UserRepository = (*PostgresUserRepo)(nil)

type PostgresUserRepo struct {
	pool *pgxpool.Pool
}

func NewPostgresUserRepo(pool *pgxpool.Pool) *PostgresUserRepo {
	return &PostgresUserRepo{pool: pool}
}

func (r *PostgresUserRepo) Save(ctx context.Context, qx any, u *model.User) error {
	const q = `
INSERT INTO users (
  id, telegram_id, username, registered_at, last_active_at,
  allow_message_storage, auto_delete_messages, message_retention_days, data_encrypted, is_admin
) VALUES (
  $1,$2,$3,$4,$5,$6,$7,$8,$9,$10
) ON CONFLICT (id) DO UPDATE SET
  telegram_id=$2, username=$3, last_active_at=$5,
  allow_message_storage=$6, auto_delete_messages=$7, message_retention_days=$8, data_encrypted=$9, is_admin=$10;
`
	switch v := qx.(type) {
	case pgx.Tx:
		_, err := v.Exec(ctx, q, u.ID, u.TelegramID, u.Username, u.RegisteredAt, u.LastActiveAt, u.Privacy.AllowMessageStorage, u.Privacy.AutoDeleteMessages, u.Privacy.MessageRetentionDays, u.Privacy.DataEncrypted, u.IsAdmin)
		return err
	case *pgxpool.Conn:
		_, err := v.Exec(ctx, q, u.ID, u.TelegramID, u.Username, u.RegisteredAt, u.LastActiveAt, u.Privacy.AllowMessageStorage, u.Privacy.AutoDeleteMessages, u.Privacy.MessageRetentionDays, u.Privacy.DataEncrypted, u.IsAdmin)
		return err
	default:
		_, err := r.pool.Exec(ctx, q, u.ID, u.TelegramID, u.Username, u.RegisteredAt, u.LastActiveAt, u.Privacy.AllowMessageStorage, u.Privacy.AutoDeleteMessages, u.Privacy.MessageRetentionDays, u.Privacy.DataEncrypted, u.IsAdmin)
		return err
	}
}

func (r *PostgresUserRepo) FindByTelegramID(ctx context.Context, qx any, tgID int64) (*model.User, error) {
	const q = `
SELECT id, telegram_id, username, registered_at, last_active_at,
       allow_message_storage, auto_delete_messages, message_retention_days, data_encrypted, is_admin
  FROM users WHERE telegram_id=$1;
`
	row := pickRow(r.pool, qx, q, tgID)
	var u model.User
	if err := row.Scan(&u.ID, &u.TelegramID, &u.Username, &u.RegisteredAt, &u.LastActiveAt, &u.Privacy.AllowMessageStorage, &u.Privacy.AutoDeleteMessages, &u.Privacy.MessageRetentionDays, &u.Privacy.DataEncrypted, &u.IsAdmin); err != nil {
		if err == pgx.ErrNoRows {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &u, nil
}

func (r *PostgresUserRepo) FindByID(ctx context.Context, qx any, id string) (*model.User, error) {
	const q = `
SELECT id, telegram_id, username, registered_at, last_active_at,
       allow_message_storage, auto_delete_messages, message_retention_days, data_encrypted, is_admin
  FROM users WHERE id=$1;`
	row := pickRow(r.pool, qx, q, id)
	var u model.User
	if err := row.Scan(&u.ID, &u.TelegramID, &u.Username, &u.RegisteredAt, &u.LastActiveAt, &u.Privacy.AllowMessageStorage, &u.Privacy.AutoDeleteMessages, &u.Privacy.MessageRetentionDays, &u.Privacy.DataEncrypted, &u.IsAdmin); err != nil {
		if err == pgx.ErrNoRows {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &u, nil
}

func (r *PostgresUserRepo) CountUsers(ctx context.Context, qx any) (int, error) {
	row := pickRow(r.pool, qx, `SELECT COUNT(*) FROM users;`)
	var n int
	if err := row.Scan(&n); err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}
	return n, nil
}

func (r *PostgresUserRepo) CountInactiveUsers(ctx context.Context, qx any, since time.Time) (int, error) {
	row := pickRow(r.pool, qx, `SELECT COUNT(*) FROM users WHERE last_active_at IS NULL OR last_active_at < $1;`, since)
	var n int
	if err := row.Scan(&n); err != nil {
		return 0, fmt.Errorf("count inactive: %w", err)
	}
	return n, nil
}
