package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v4/pgxpool"

	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/repository"
)

var _ repository.UserRepository = (*userRepo)(nil)

type userRepo struct {
	pool *pgxpool.Pool
}

func NewUserRepo(pool *pgxpool.Pool) *userRepo {
	return &userRepo{pool: pool}
}

func (r *userRepo) Save(ctx context.Context, tx repository.Tx, u *model.User) error {
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
	_, err := execSQL(ctx, r.pool, tx, q, u.ID, u.TelegramID, u.Username, u.RegisteredAt, u.LastActiveAt, u.Privacy.AllowMessageStorage, u.Privacy.AutoDeleteMessages, u.Privacy.MessageRetentionDays, u.Privacy.DataEncrypted, u.IsAdmin)
	if err != nil {
		if err == domain.ErrInvalidArgument || err == domain.ErrInvalidExecContext {
			return err
		}
		return domain.ErrOperationFailed
	}
	return nil
}

func (r *userRepo) FindByTelegramID(ctx context.Context, tx repository.Tx, tgID int64) (*model.User, error) {
	const q = `
SELECT id, telegram_id, username, registered_at, last_active_at,
       allow_message_storage, auto_delete_messages, message_retention_days, data_encrypted, is_admin
  FROM users WHERE telegram_id=$1;`

	row, err := pickRow(ctx, r.pool, tx, q, tgID)
	if err != nil {
		return nil, err
	}

	var u model.User
	if err := row.Scan(&u.ID, &u.TelegramID, &u.Username, &u.RegisteredAt, &u.LastActiveAt, &u.Privacy.AllowMessageStorage, &u.Privacy.AutoDeleteMessages, &u.Privacy.MessageRetentionDays, &u.Privacy.DataEncrypted, &u.IsAdmin); err != nil {
		return nil, domain.ErrReadDatabaseRow
	}
	return &u, nil
}

func (r *userRepo) FindByID(ctx context.Context, tx repository.Tx, id string) (*model.User, error) {
	const q = `
SELECT id, telegram_id, username, registered_at, last_active_at,
       allow_message_storage, auto_delete_messages, message_retention_days, data_encrypted, is_admin
  FROM users WHERE id=$1;`

	row, err := pickRow(ctx, r.pool, tx, q, id)
	if err != nil {
		return nil, err
	}

	var u model.User
	if err := row.Scan(&u.ID, &u.TelegramID, &u.Username, &u.RegisteredAt, &u.LastActiveAt, &u.Privacy.AllowMessageStorage, &u.Privacy.AutoDeleteMessages, &u.Privacy.MessageRetentionDays, &u.Privacy.DataEncrypted, &u.IsAdmin); err != nil {
		return nil, domain.ErrReadDatabaseRow
	}
	return &u, nil
}

func (r *userRepo) CountUsers(ctx context.Context, tx repository.Tx) (int, error) {
	row, err := pickRow(ctx, r.pool, tx, `SELECT COUNT(*) FROM users;`)
	if err != nil {
		return 0, err
	}

	var n int
	if err := row.Scan(&n); err != nil {
		return 0, domain.ErrReadDatabaseRow
	}

	return n, nil
}

func (r *userRepo) CountInactiveUsers(ctx context.Context, tx repository.Tx, since time.Time) (int, error) {
	row, err := pickRow(ctx, r.pool, tx, `SELECT COUNT(*) FROM users WHERE last_active_at IS NULL OR last_active_at < $1;`, since)
	if err != nil {
		return 0, err
	}
	var n int
	if err := row.Scan(&n); err != nil {
		return 0, domain.ErrReadDatabaseRow
	}
	return n, nil
}
