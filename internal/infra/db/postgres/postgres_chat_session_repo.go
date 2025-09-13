package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"

	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/repository"
	"telegram-ai-subscription/internal/infra/redis"
	"telegram-ai-subscription/internal/infra/security"
)

// chatSessionRepo is the default (and only) chat session repository.
// It persists messages with optional encryption-at-rest, based on user privacy settings.
var _ repository.ChatSessionRepository = (*chatSessionRepo)(nil)

type chatSessionRepo struct {
	pool          *pgxpool.Pool
	cache         *redis.ChatCache
	encryptionSvc *security.EncryptionService
}

func NewChatSessionRepo(pool *pgxpool.Pool, cache *redis.ChatCache, encryptionSvc *security.EncryptionService) *chatSessionRepo {
	return &chatSessionRepo{pool: pool, cache: cache, encryptionSvc: encryptionSvc}
}

func (r *chatSessionRepo) Save(ctx context.Context, tx repository.Tx, session *model.ChatSession) error {
	const q = `
INSERT INTO chat_sessions (id, user_id, model, status, created_at, updated_at)
VALUES ($1,$2,$3,$4,COALESCE($5,NOW()),COALESCE($6,NOW()))
ON CONFLICT (id) DO UPDATE SET
  user_id = EXCLUDED.user_id,
  model = EXCLUDED.model,
  status = EXCLUDED.status,
  updated_at = EXCLUDED.updated_at;`
	_, err := execSQL(ctx, r.pool, tx, q, session.ID, session.UserID, session.Model, string(session.Status), session.CreatedAt, session.UpdatedAt)
	switch err {
	case nil:
		// Messages are appended separately via SaveMessage. Cache latest session state.
		if r.cache != nil {
			_ = r.cache.StoreSession(ctx, session)
		}
		return nil
	case domain.ErrInvalidArgument, domain.ErrInvalidExecContext:
		return err
	default:
		return domain.ErrOperationFailed
	}
}

func (r *chatSessionRepo) SaveMessage(ctx context.Context, tx repository.Tx, m *model.ChatMessage) (bool, error) {
	// Resolve user_id from session (so model.ChatMessage doesn't need UserID field)
	const qUserFromSess = `SELECT user_id FROM chat_sessions WHERE id=$1;`
	var userID string
	row, err := pickRow(ctx, r.pool, tx, qUserFromSess, m.SessionID)
	if err != nil {
		return false, err
	}
	if err := row.Scan(&userID); err != nil {
		return false, domain.ErrReadDatabaseRow
	}

	// read user privacy from users table
	const qPrivacy = `SELECT data_encrypted, allow_message_storage FROM users WHERE id = $1;`
	var dataEncrypted, allowStore bool
	rows, err := pickRow(ctx, r.pool, tx, qPrivacy, userID)
	if err != nil {
		switch err {
		case pgx.ErrNoRows:
			return false, domain.ErrNotFound
		case domain.ErrInvalidArgument, domain.ErrInvalidExecContext:
			return false, err
		default:
			return false, domain.ErrOperationFailed
		}
	}
	if err := rows.Scan(&dataEncrypted, &allowStore); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, domain.ErrNotFound
		}
		return false, domain.ErrReadDatabaseRow
	}

	if !allowStore {
		return false, nil // do not store messages at all
	}

	payload := m.Content
	encFlag := false
	if dataEncrypted {
		payload, err = r.encryptionSvc.Encrypt(m.Content)
		if err != nil {
			return false, domain.ErrEncryptionFailed
		}
		encFlag = true
	}

	const q = `
INSERT INTO chat_messages (id, session_id, role, content, tokens, encrypted, created_at)
VALUES ($1,$2,$3,$4,$5,$6,COALESCE($7,NOW()));`

	_, err = execSQL(ctx, r.pool, tx, q, m.ID, m.SessionID, m.Role, payload, m.Tokens, encFlag, m.Timestamp)
	if err != nil {
		switch err {
		case domain.ErrInvalidArgument, domain.ErrInvalidExecContext:
			return false, err
		default:
			return false, domain.ErrOperationFailed
		}
	}
	return true, nil

}

func (r *chatSessionRepo) Delete(ctx context.Context, tx repository.Tx, id string) error {
	const q = `DELETE FROM chat_sessions WHERE id = $1;`
	_, err := execSQL(ctx, r.pool, tx, q, id)
	switch err {
	case nil:
		return nil
	case domain.ErrInvalidArgument, domain.ErrInvalidExecContext:
		return err
	default:
		return domain.ErrOperationFailed
	}
}

func (r *chatSessionRepo) FindActiveByUser(ctx context.Context, tx repository.Tx, userID string) (*model.ChatSession, error) {
	const q = `SELECT id FROM chat_sessions WHERE user_id=$1 AND status='active' ORDER BY created_at DESC LIMIT 1;`
	row, err := pickRow(ctx, r.pool, nil, q, userID) // Read operation outside transaction
	if err != nil {
		return nil, err
	}

	var id string
	if err := row.Scan(&id); err != nil {
		return nil, domain.ErrReadDatabaseRow
	}
	return r.FindByID(ctx, tx, id)
}

func (r *chatSessionRepo) ListByUser(ctx context.Context, tx repository.Tx, userID string, offset, limit int) ([]*model.ChatSession, error) {
	if offset < 0 {
		offset = 0
	}

	var q = `
SELECT s.id, s.user_id, s.model, s.status, s.created_at, s.updated_at,
       fm.role, fm.content, fm.tokens, fm.created_at, fm.encrypted
FROM chat_sessions s
LEFT JOIN LATERAL (
    SELECT role, content, tokens, created_at, encrypted
    FROM chat_messages
    WHERE session_id = s.id
    ORDER BY created_at ASC
    LIMIT 1
) fm ON TRUE
WHERE s.user_id = $1
ORDER BY s.updated_at DESC
OFFSET $2
`
	var rows pgx.Rows
	var err error
	if limit > 0 {
		q += " LIMIT $3;"
		rows, err = queryRows(ctx, r.pool, nil, q, userID, offset, limit)
	} else {
		q += ";"
		rows, err = queryRows(ctx, r.pool, nil, q, userID, offset)
	}
	if err != nil {
		switch err {
		case pgx.ErrNoRows:
			return nil, domain.ErrNotFound
		case domain.ErrInvalidArgument, domain.ErrInvalidExecContext:
			return nil, err
		default:
			return nil, domain.ErrOperationFailed
		}
	}
	defer rows.Close()

	out := make([]*model.ChatSession, 0, 16)
	for rows.Next() {
		var s model.ChatSession
		var firstRole, firstContent sql.NullString
		var firstTokens sql.NullInt32
		var firstCreated sql.NullTime
		var isEncrypted sql.NullBool

		if err := rows.Scan(
			&s.ID, &s.UserID, &s.Model, &s.Status, &s.CreatedAt, &s.UpdatedAt,
			&firstRole, &firstContent, &firstTokens, &firstCreated, &isEncrypted,
		); err != nil {
			return nil, domain.ErrReadDatabaseRow
		}
		if firstRole.Valid && firstContent.Valid {
			content := firstContent.String
			// Only decrypt if the encrypted flag is true.
			if isEncrypted.Valid && isEncrypted.Bool {
				plain, err := r.encryptionSvc.Decrypt(firstContent.String)
				if err != nil {
					return nil, domain.ErrDecryptionFailed
				}
				content = plain
			}

			s.Messages = append(s.Messages, model.ChatMessage{
				SessionID: s.ID,
				Role:      firstRole.String,
				Content:   content,
				Tokens:    int(firstTokens.Int32),
				Timestamp: firstCreated.Time,
			})
		}
		out = append(out, &s)
	}
	if err := rows.Err(); err != nil {
		return nil, domain.ErrReadDatabaseRow
	}
	return out, nil
}

func (r *chatSessionRepo) FindByID(ctx context.Context, tx repository.Tx, id string) (*model.ChatSession, error) {
	const qs = `SELECT id, user_id, model, status, created_at, updated_at FROM chat_sessions WHERE id=$1;`
	row, err := pickRow(ctx, r.pool, nil, qs, id)
	if err != nil {
		return nil, err
	}

	var s model.ChatSession
	var status string
	if err := row.Scan(&s.ID, &s.UserID, &s.Model, &status, &s.CreatedAt, &s.UpdatedAt); err != nil {
		return nil, domain.ErrReadDatabaseRow
	}
	s.Status = model.ChatSessionStatus(status)

	// load messages
	const qm = `SELECT role, content, tokens, encrypted, created_at FROM chat_messages WHERE session_id=$1 ORDER BY created_at ASC;`
	rows, err := queryRows(ctx, r.pool, nil, qm, id)
	if err != nil {
		switch err {
		case pgx.ErrNoRows:
			return nil, domain.ErrNotFound
		case domain.ErrInvalidArgument, domain.ErrInvalidExecContext:
			return nil, err
		default:
			return nil, domain.ErrOperationFailed
		}
	}
	defer rows.Close()
	for rows.Next() {
		var role string
		var content string
		var tokens int
		var enc sql.NullBool
		var ts time.Time
		if err := rows.Scan(&role, &content, &tokens, &enc, &ts); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, domain.ErrNotFound
			}
			return nil, domain.ErrReadDatabaseRow
		}
		if enc.Valid && enc.Bool {
			plain, err := r.encryptionSvc.Decrypt(content)
			if err != nil {
				return nil, domain.ErrDecryptionFailed
			}
			content = plain
		}
		s.Messages = append(s.Messages, model.ChatMessage{
			SessionID: s.ID,
			Role:      role,
			Content:   content,
			Tokens:    tokens,
			Timestamp: ts,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, domain.ErrReadDatabaseRow
	}
	// cache best-effort
	if r.cache != nil {
		_ = r.cache.StoreSession(ctx, &s)
	}
	return &s, nil
}

func (r *chatSessionRepo) FindUserBySessionID(ctx context.Context, tx repository.Tx, sessionID string) (*model.User, error) {
	const q = `
SELECT u.id, u.telegram_id, u.username, u.registered_at, u.last_active_at, u.allow_message_storage, u.auto_delete_messages, u.message_retention_days, u.data_encrypted, u.is_admin
FROM users u
JOIN chat_sessions s ON s.user_id = u.id
WHERE s.id = $1;`
	row, err := pickRow(ctx, r.pool, tx, q, sessionID)
	if err != nil {
		return nil, err
	}

	var u model.User
	var p model.PrivacySettings
	if err := row.Scan(&u.ID, &u.TelegramID, &u.Username, &u.RegisteredAt, &u.LastActiveAt, &p.AllowMessageStorage, &p.AutoDeleteMessages, &p.MessageRetentionDays, &p.DataEncrypted, &u.IsAdmin); err != nil {
		return nil, domain.ErrReadDatabaseRow
	}
	u.Privacy = p
	return &u, nil
}

func (r *chatSessionRepo) UpdateStatus(ctx context.Context, tx repository.Tx, sessionID string, status model.ChatSessionStatus) error {
	const q = `UPDATE chat_sessions SET status=$2, updated_at=NOW() WHERE id=$1;`

	_, err := execSQL(ctx, r.pool, tx, q, sessionID, string(status))
	switch err {
	case nil:
		return nil
	case domain.ErrInvalidArgument, domain.ErrInvalidExecContext:
		return err
	default:
		return domain.ErrOperationFailed
	}
}

func (r *chatSessionRepo) CleanupOldMessages(ctx context.Context, userID string, retentionDays int) (int64, error) {
	const q = `
DELETE FROM chat_messages
 WHERE session_id IN (SELECT id FROM chat_sessions WHERE user_id = $1)
   AND created_at < NOW() - ($2::int * INTERVAL '1 day');`
	tag, err := r.pool.Exec(ctx, q, userID, retentionDays)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (r *chatSessionRepo) DeleteAllByUserID(ctx context.Context, tx repository.Tx, userID string) error {
	const q = `DELETE FROM chat_sessions WHERE user_id = $1;`
	_, err := execSQL(ctx, r.pool, tx, q, userID)
	// The ON DELETE CASCADE constraint on chat_messages will handle deleting the messages.
	return err
}
