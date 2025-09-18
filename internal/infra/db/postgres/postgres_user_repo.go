package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v4"
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
  id, telegram_id, username, full_name, phone_number, registration_status, registered_at, last_active_at,
  allow_message_storage, auto_delete_messages, message_retention_days, data_encrypted, is_admin
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
) ON CONFLICT (id) DO UPDATE SET
  username = EXCLUDED.username,
  full_name = EXCLUDED.full_name,
  phone_number = EXCLUDED.phone_number,
  registration_status = EXCLUDED.registration_status,
  last_active_at = EXCLUDED.last_active_at,
  allow_message_storage = EXCLUDED.allow_message_storage,
  is_admin = EXCLUDED.is_admin;
`
	_, err := execSQL(ctx, r.pool, tx, q, u.ID, u.TelegramID, u.Username, u.FullName, u.PhoneNumber, u.RegistrationStatus, u.RegisteredAt, u.LastActiveAt, u.Privacy.AllowMessageStorage, u.Privacy.AutoDeleteMessages, u.Privacy.MessageRetentionDays, u.Privacy.DataEncrypted, u.IsAdmin)
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
SELECT id, telegram_id, username, full_name, phone_number, registration_status, registered_at, last_active_at,
       allow_message_storage, auto_delete_messages, message_retention_days, data_encrypted, is_admin
  FROM users WHERE telegram_id=$1;`

	row, err := pickRow(ctx, r.pool, tx, q, tgID)
	if err != nil {
		return nil, err
	}

	var u model.User
	if err := row.Scan(&u.ID, &u.TelegramID, &u.Username, &u.FullName, &u.PhoneNumber, &u.RegistrationStatus, &u.RegisteredAt, &u.LastActiveAt, &u.Privacy.AllowMessageStorage, &u.Privacy.AutoDeleteMessages, &u.Privacy.MessageRetentionDays, &u.Privacy.DataEncrypted, &u.IsAdmin); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrUserNotFound
		}
		return nil, domain.ErrReadDatabaseRow
	}
	return &u, nil
}

func (r *userRepo) FindByID(ctx context.Context, tx repository.Tx, id string) (*model.User, error) {
	const q = `
SELECT id, telegram_id, username, full_name, phone_number, registration_status, registered_at, last_active_at,
       allow_message_storage, auto_delete_messages, message_retention_days, data_encrypted, is_admin
  FROM users WHERE id=$1;`

	row, err := pickRow(ctx, r.pool, tx, q, id)
	if err != nil {
		return nil, err
	}

	var u model.User
	if err := row.Scan(&u.ID, &u.TelegramID, &u.Username, &u.FullName, &u.PhoneNumber, &u.RegistrationStatus, &u.RegisteredAt, &u.LastActiveAt, &u.Privacy.AllowMessageStorage, &u.Privacy.AutoDeleteMessages, &u.Privacy.MessageRetentionDays, &u.Privacy.DataEncrypted, &u.IsAdmin); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrUserNotFound
		}
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
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, domain.ErrNotFound
		}
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
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, domain.ErrNotFound
		}
		return 0, domain.ErrReadDatabaseRow
	}
	return n, nil
}

func (r *userRepo) List(ctx context.Context, tx repository.Tx, offset, limit int) ([]*model.User, error) {
	q := `
SELECT id, telegram_id, username, full_name, phone_number, registration_status, registered_at, last_active_at,
       allow_message_storage, auto_delete_messages, message_retention_days, data_encrypted, is_admin
  FROM users ORDER BY registered_at DESC`

	var args []interface{}

	if limit == 0 {
		// Case 1: limit is exactly 0. Fetch all users, no LIMIT or OFFSET.
	} else {
		// Case 2: limit is not 0. This is a paginated query.
		if limit < 0 {
			// Sub-case: limit is negative. Use the default page size.
			limit = 50
		}
		q += " OFFSET $1 LIMIT $2"
		args = append(args, offset, limit)
	}

	rows, err := queryRows(ctx, r.pool, tx, q, args...)
	if err != nil {
		return nil, domain.ErrOperationFailed
	}
	defer rows.Close()

	var users []*model.User
	for rows.Next() {
		var u model.User
		if err := rows.Scan(&u.ID, &u.TelegramID, &u.Username, &u.FullName, &u.PhoneNumber, &u.RegistrationStatus, &u.RegisteredAt, &u.LastActiveAt, &u.Privacy.AllowMessageStorage, &u.Privacy.AutoDeleteMessages, &u.Privacy.MessageRetentionDays, &u.Privacy.DataEncrypted, &u.IsAdmin); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, domain.ErrNotFound
			}
			return nil, domain.ErrReadDatabaseRow
		}
		users = append(users, &u)
	}
	if err := rows.Err(); err != nil {
		return nil, domain.ErrReadDatabaseRow
	}
	return users, nil
}
