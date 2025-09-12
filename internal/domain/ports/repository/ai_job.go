package repository

import (
	"context"
	"telegram-ai-subscription/internal/domain/model"
)

type AIJobRepository interface {
	Save(ctx context.Context, tx Tx, job *model.AIJob) error
	// FetchAndMarkProcessing atomically fetches a pending job and marks it as 'processing'.
	// This prevents other workers from picking up the same job.
	FetchAndMarkProcessing(ctx context.Context) (*model.AIJob, error)
}
