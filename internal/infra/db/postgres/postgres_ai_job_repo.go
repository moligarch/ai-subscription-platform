package postgres

import (
	"context"
	"errors"
	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/repository"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

var _ repository.AIJobRepository = (*aiJobRepo)(nil)

type aiJobRepo struct {
	pool *pgxpool.Pool
}

func NewAIJobRepo(pool *pgxpool.Pool) *aiJobRepo {
	return &aiJobRepo{pool: pool}
}

func (r *aiJobRepo) Save(ctx context.Context, tx repository.Tx, job *model.AIJob) error {
	if job.ID == "" {
		job.ID = uuid.NewString()
	}
	job.UpdatedAt = time.Now()

	const q = `
INSERT INTO ai_jobs (id, status, session_id, user_message_id, retries, last_error, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (id) DO UPDATE SET
  status = EXCLUDED.status,
  retries = EXCLUDED.retries,
  last_error = EXCLUDED.last_error,
  updated_at = EXCLUDED.updated_at;`

	_, err := execSQL(ctx, r.pool, tx, q,
		job.ID, job.Status, job.SessionID, job.UserMessageID, job.Retries, job.LastError, job.CreatedAt, job.UpdatedAt)
	return err
}

func (r *aiJobRepo) FetchAndMarkProcessing(ctx context.Context) (*model.AIJob, error) {
	var job model.AIJob
	// This transaction ensures the SELECT and UPDATE are atomic.
	// `FOR UPDATE SKIP LOCKED` is a powerful Postgres feature that tells the query
	// to lock the row it finds, and if it's already locked by another worker,
	// just skip it and find the next unlocked one. This is perfect for a work queue.
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	const fetchQuery = `
SELECT id, status, session_id, user_message_id, retries, last_error, created_at, updated_at
FROM ai_jobs
WHERE status = 'pending'
ORDER BY created_at
LIMIT 1
FOR UPDATE SKIP LOCKED;`

	row := tx.QueryRow(ctx, fetchQuery)
	var statusStr string
	err = row.Scan(
		&job.ID, &statusStr, &job.SessionID, &job.UserMessageID, &job.Retries, &job.LastError, &job.CreatedAt, &job.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound // No pending jobs, which is not an error.
		}
		return nil, err
	}
	job.Status = model.AIJobStatus(statusStr)

	// Mark the job as processing so no one else picks it up
	job.Status = model.AIJobStatusProcessing
	job.UpdatedAt = time.Now()

	if err := r.Save(ctx, tx, &job); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return &job, nil
}
