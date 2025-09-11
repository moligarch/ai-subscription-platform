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
	tm   repository.TransactionManager
}

func NewAIJobRepo(pool *pgxpool.Pool, tm repository.TransactionManager) *aiJobRepo {
	return &aiJobRepo{
		pool: pool,
		tm:   tm,
	}
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
	var job *model.AIJob

	// Use the TransactionManager to handle Begin/Commit/Rollback automatically.
	err := r.tm.WithTx(ctx, pgx.TxOptions{}, func(ctx context.Context, tx repository.Tx) error {
		const fetchQuery = `
SELECT id, status, session_id, user_message_id, retries, last_error, created_at, updated_at
FROM ai_jobs
WHERE status = 'pending'
ORDER BY created_at
LIMIT 1
FOR UPDATE SKIP LOCKED;`

		// We can now use our pickRow helper inside the transaction
		row, err := pickRow(ctx, r.pool, tx, fetchQuery)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return domain.ErrNotFound // Return the specific error to stop the transaction wrapper
			}
			return err
		}

		var fetchedJob model.AIJob
		var statusStr string
		err = row.Scan(
			&fetchedJob.ID, &statusStr, &fetchedJob.SessionID, &fetchedJob.UserMessageID,
			&fetchedJob.Retries, &fetchedJob.LastError, &fetchedJob.CreatedAt, &fetchedJob.UpdatedAt,
		)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound // Translate driver error to our domain error
			}
			return domain.ErrReadDatabaseRow // For all other scan errors
		}
		fetchedJob.Status = model.AIJobStatus(statusStr)

		// Mark the job as processing so no one else picks it up
		fetchedJob.Status = model.AIJobStatusProcessing
		fetchedJob.UpdatedAt = time.Now()

		// The existing Save method will correctly use the transaction context (tx)
		if err := r.Save(ctx, tx, &fetchedJob); err != nil {
			return err
		}

		job = &fetchedJob // Assign the result to the outer scope variable
		return nil
	})

	// Handle the specific case where no rows were found
	if errors.Is(err, domain.ErrNotFound) {
		return nil, domain.ErrNotFound
	}

	return job, err
}
