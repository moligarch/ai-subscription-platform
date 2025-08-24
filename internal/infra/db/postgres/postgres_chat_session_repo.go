// File: internal/infra/db/postgres/postgres_chat_session_repo.go
package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"

	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/repository"
	"telegram-ai-subscription/internal/infra/redis"
	"telegram-ai-subscription/internal/infra/security"
)

// ChatSessionRepo is the default (and only) chat session repository.
// It persists messages with optional encryption-at-rest, based on user privacy settings.
var _ repository.ChatSessionRepository = (*ChatSessionRepo)(nil)

type ChatSessionRepo struct {
	pool          *pgxpool.Pool
	cache         *redis.ChatCache
	encryptionSvc *security.EncryptionService
}

func NewPostgresChatSessionRepo(pool *pgxpool.Pool, cache *redis.ChatCache, encryptionSvc *security.EncryptionService) *ChatSessionRepo {
	return &ChatSessionRepo{pool: pool, cache: cache, encryptionSvc: encryptionSvc}
}

func (r *ChatSessionRepo) Save(ctx context.Context, qx any, session *model.ChatSession) error {
	const q = `
INSERT INTO chat_sessions (id, user_id, model, status, created_at, updated_at)
VALUES ($1,$2,$3,$4,COALESCE($5,NOW()),COALESCE($6,NOW()))
ON CONFLICT (id) DO UPDATE SET
  user_id = EXCLUDED.user_id,
  model = EXCLUDED.model,
  status = EXCLUDED.status,
  updated_at = EXCLUDED.updated_at;`
	var err error
	switch v := qx.(type) {
	case pgx.Tx:
		_, err = v.Exec(ctx, q, session.ID, session.UserID, session.Model, string(session.Status), session.CreatedAt, session.UpdatedAt)
	case *pgxpool.Conn:
		_, err = v.Exec(ctx, q, session.ID, session.UserID, session.Model, string(session.Status), session.CreatedAt, session.UpdatedAt)
	default:
		_, err = r.pool.Exec(ctx, q, session.ID, session.UserID, session.Model, string(session.Status), session.CreatedAt, session.UpdatedAt)
	}
	if err != nil {
		return fmt.Errorf("save session: %w", err)
	}

	// Messages are appended separately via SaveMessage. Cache latest session state.
	if r.cache != nil {
		_ = r.cache.StoreSession(ctx, session)
	}
	return nil
}

func (r *ChatSessionRepo) SaveMessage(ctx context.Context, qx any, m *model.ChatMessage) error {
	// Resolve user_id from session (so model.ChatMessage doesn't need UserID field)
	const qUserFromSess = `SELECT user_id FROM chat_sessions WHERE id=$1;`
	var userID string
	if err := r.pool.QueryRow(ctx, qUserFromSess, m.SessionID).Scan(&userID); err != nil {
		if err == pgx.ErrNoRows {
			return domain.ErrNotFound
		}
		return fmt.Errorf("session->user lookup: %w", err)
	}

	// read user privacy from users table
	const qPrivacy = `SELECT data_encrypted, allow_message_storage FROM users WHERE id = $1;`
	var dataEncrypted, allowStore bool
	if err := r.pool.QueryRow(ctx, qPrivacy, userID).Scan(&dataEncrypted, &allowStore); err != nil {
		return fmt.Errorf("privacy read: %w", err)
	}
	if !allowStore {
		return nil // do not store messages at all
	}

	payload := m.Content
	encFlag := false
	var err error
	if dataEncrypted {
		payload, err = r.encryptionSvc.Encrypt(m.Content)
		if err != nil {
			return fmt.Errorf("encrypt msg: %w", err)
		}
		encFlag = true
	}

	const q = `
INSERT INTO chat_messages (session_id, role, content, tokens, encrypted, created_at)
VALUES ($1,$2,$3,$4,$5,COALESCE($6,NOW()));`
	switch v := qx.(type) {
	case pgx.Tx:
		_, err = v.Exec(ctx, q, m.SessionID, m.Role, payload, m.Tokens, encFlag, m.Timestamp)
	case *pgxpool.Conn:
		_, err = v.Exec(ctx, q, m.SessionID, m.Role, payload, m.Tokens, encFlag, m.Timestamp)
	default:
		_, err = r.pool.Exec(ctx, q, m.SessionID, m.Role, payload, m.Tokens, encFlag, m.Timestamp)
	}
	return err
}

func (r *ChatSessionRepo) Delete(ctx context.Context, qx any, id string) error {
	const q = `DELETE FROM chat_sessions WHERE id = $1;`
	var err error
	switch v := qx.(type) {
	case pgx.Tx:
		_, err = v.Exec(ctx, q, id)
	case *pgxpool.Conn:
		_, err = v.Exec(ctx, q, id)
	default:
		_, err = r.pool.Exec(ctx, q, id)
	}
	return err
}

func (r *ChatSessionRepo) FindActiveByUser(ctx context.Context, qx any, userID string) (*model.ChatSession, error) {
	const q = `SELECT id FROM chat_sessions WHERE user_id=$1 AND status='active' ORDER BY created_at DESC LIMIT 1;`
	row := pickRow(r.pool, qx, q, userID)
	var id string
	if err := row.Scan(&id); err != nil {
		if err == pgx.ErrNoRows {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return r.FindByID(ctx, qx, id)
}

func (r *ChatSessionRepo) FindAllByUser(ctx context.Context, qx any, userID string) ([]*model.ChatSession, error) {
	const q = `SELECT id FROM chat_sessions WHERE user_id=$1 ORDER BY created_at DESC;`
	rows, err := r.queryRows(ctx, qx, q, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.ChatSession
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		s, err := r.FindByID(ctx, qx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *ChatSessionRepo) FindByID(ctx context.Context, qx any, id string) (*model.ChatSession, error) {
	const qs = `SELECT id, user_id, model, status, created_at, updated_at FROM chat_sessions WHERE id=$1;`
	row := pickRow(r.pool, qx, qs, id)
	var s model.ChatSession
	var status string
	if err := row.Scan(&s.ID, &s.UserID, &s.Model, &status, &s.CreatedAt, &s.UpdatedAt); err != nil {
		if err == pgx.ErrNoRows {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("scan session: %w", err)
	}
	s.Status = model.ChatSessionStatus(status)

	// load messages
	const qm = `SELECT role, content, tokens, encrypted, created_at FROM chat_messages WHERE session_id=$1 ORDER BY created_at ASC;`
	rows, err := r.queryRows(ctx, qx, qm, id)
	if err != nil {
		return nil, fmt.Errorf("query messages: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var role string
		var content string
		var tokens int
		var enc sql.NullBool
		var ts time.Time
		if err := rows.Scan(&role, &content, &tokens, &enc, &ts); err != nil {
			return nil, fmt.Errorf("scan msg: %w", err)
		}
		if enc.Valid && enc.Bool {
			plain, err := r.encryptionSvc.Decrypt(content)
			if err != nil {
				return nil, fmt.Errorf("decrypt msg: %w", err)
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
		return nil, fmt.Errorf("rows err: %w", err)
	}
	// cache best-effort
	if r.cache != nil {
		_ = r.cache.StoreSession(ctx, &s)
	}
	return &s, nil
}

func (r *ChatSessionRepo) UpdateStatus(ctx context.Context, qx any, sessionID string, status model.ChatSessionStatus) error {
	const q = `UPDATE chat_sessions SET status=$2, updated_at=NOW() WHERE id=$1;`
	switch v := qx.(type) {
	case pgx.Tx:
		_, err := v.Exec(ctx, q, sessionID, string(status))
		return err
	case *pgxpool.Conn:
		_, err := v.Exec(ctx, q, sessionID, string(status))
		return err
	default:
		_, err := r.pool.Exec(ctx, q, sessionID, string(status))
		return err
	}
}

func (r *ChatSessionRepo) CleanupOldMessages(ctx context.Context, userID string, retentionDays int) (int64, error) {
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

func (r *ChatSessionRepo) queryRows(ctx context.Context, qx any, sql string, args ...any) (pgx.Rows, error) {
	switch v := qx.(type) {
	case pgx.Tx:
		return v.Query(ctx, sql, args...)
	case *pgxpool.Conn:
		return v.Query(ctx, sql, args...)
	default:
		return r.pool.Query(ctx, sql, args...)
	}
}
