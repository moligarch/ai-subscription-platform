package worker

import (
	"context"
	"fmt"
	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/adapter"
	"telegram-ai-subscription/internal/domain/ports/repository"
	"telegram-ai-subscription/internal/domain/ports/usecase"
	"telegram-ai-subscription/internal/infra/metrics"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v4"
	"github.com/rs/zerolog"
)

type AIJobProcessor struct {
	jobsRepo    repository.AIJobRepository
	chatRepo    repository.ChatSessionRepository
	pricingRepo repository.ModelPricingRepository
	subManager  usecase.SubscriptionManager
	aiAdapter   adapter.AIServiceAdapter
	botAdapter  adapter.TelegramBotAdapter
	tm          repository.TransactionManager
	log         *zerolog.Logger
}

func NewAIJobProcessor(
	jobsRepo repository.AIJobRepository,
	chatRepo repository.ChatSessionRepository,
	pricingRepo repository.ModelPricingRepository,
	subManager usecase.SubscriptionManager,
	aiAdapter adapter.AIServiceAdapter,
	botAdapter adapter.TelegramBotAdapter,
	tm repository.TransactionManager,
	log *zerolog.Logger,
) *AIJobProcessor {
	return &AIJobProcessor{
		jobsRepo:    jobsRepo,
		chatRepo:    chatRepo,
		pricingRepo: pricingRepo,
		subManager:  subManager,
		aiAdapter:   aiAdapter,
		botAdapter:  botAdapter,
		tm:          tm,
		log:         log,
	}
}

// Start runs a loop to fetch and process jobs.
// This should be run in a goroutine.
func (p *AIJobProcessor) Start(ctx context.Context, pool *Pool) {
	p.log.Info().Msg("AI Job Processor started")
	ticker := time.NewTicker(500 * time.Millisecond) // Poll for new jobs
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			p.log.Info().Msg("AI Job Processor stopping")
			return
		case <-ticker.C:
			// Submit the processing task to the worker pool
			_ = pool.Submit(func(ctx context.Context) error {
				p.processOne(ctx)
				return nil
			})
		}
	}
}

func (p *AIJobProcessor) processOne(ctx context.Context) {
	job, err := p.jobsRepo.FetchAndMarkProcessing(ctx)
	if err != nil {
		if err != domain.ErrNotFound {
			p.log.Error().Err(err).Msg("Failed to fetch AI job")
		}
		return // No job found, or an error occurred
	}

	p.log.Info().Str("job_id", job.ID).Str("session_id", job.SessionID).Msg("Processing AI job")
	start := time.Now()

	// The actual processing logic
	err = p.handleJob(ctx, job)
	latency := time.Since(start)

	// Final transaction to update job status
	finalStatus := model.AIJobStatusCompleted
	if err != nil {
		finalStatus = model.AIJobStatusFailed
		job.LastError = err.Error()
		p.log.Error().Err(err).Str("job_id", job.ID).Msg("AI job failed")
	}

	metrics.IncAIJob(string(finalStatus))
	job.Status = finalStatus
	_ = p.jobsRepo.Save(context.Background(), nil, job) // Use background context for final update
	p.log.Info().Str("job_id", job.ID).Str("status", string(finalStatus)).Dur("duration_ms", latency).Msg("AI job finished")
}

// handleJob contains the core logic for a single job.
func (p *AIJobProcessor) handleJob(ctx context.Context, job *model.AIJob) error {
	// 1. Fetch all necessary data
	session, err := p.chatRepo.FindByID(ctx, nil, job.SessionID)
	if err != nil {
		return fmt.Errorf("session not found: %w", err)
	}
	pricing, err := p.pricingRepo.GetByModelName(ctx, nil, session.Model)
	if err != nil {
		return fmt.Errorf("pricing not found: %w", err)
	}
	activeSub, err := p.subManager.GetActive(ctx, session.UserID)
	if err != nil {
		return domain.ErrNoActiveSubscription
	}

	// Build the message history for the AI.
	msgs := session.GetRecentMessages(15)
	adapterMsgs := make([]adapter.Message, 0, len(msgs)+1)
	for _, m := range msgs {
		adapterMsgs = append(adapterMsgs, adapter.Message{Role: m.Role, Content: m.Content})
	}
	// If the job carried its own content (because it wasn't saved), append it now.
	// This ensures the AI always receives the user's latest message.
	if job.UserMessageContent != "" {
		adapterMsgs = append(adapterMsgs, adapter.Message{Role: "user", Content: job.UserMessageContent})
	}

	// If after all that, we still have no messages, something is wrong.
	if len(adapterMsgs) == 0 {
		return domain.ErrAIJobWithNoMessage
	}

	// Pre-check tokens and cost
	promptTokens, err := p.aiAdapter.CountTokens(ctx, session.Model, adapterMsgs)
	if err != nil {
		return fmt.Errorf("could not count tokens: %w", err)
	}

	requiredMicros := int64(promptTokens) * pricing.InputTokenPriceMicros
	if activeSub.RemainingCredits < requiredMicros {
		return domain.ErrInsufficientBalance
	}

	// 2. Call the external AI service
	callStart := time.Now()
	reply, usage, err := p.aiAdapter.ChatWithUsage(ctx, session.Model, adapterMsgs)
	latency := time.Since(callStart) // Calculate latency immediately

	// We now handle metrics for both success and failure cases here.
	if err != nil {
		metrics.ObserveChatUsage("provider_guess", session.Model, 0, 0, 0, 0, int(latency/time.Millisecond), false)
		return fmt.Errorf("ai adapter failed: %w", err)
	}

	// Calculate exact cost and fire off the success metric
	spent := int64(usage.PromptTokens)*pricing.InputTokenPriceMicros +
		int64(usage.CompletionTokens)*pricing.OutputTokenPriceMicros

	metrics.ObserveChatUsage(
		"provider_guess", session.Model,
		usage.PromptTokens,
		usage.CompletionTokens,
		usage.TotalTokens,
		spent,
		int(latency/time.Millisecond),
		true, // Success
	)

	// 3. Final atomic write: save reply, update credits
	return p.tm.WithTx(ctx, pgx.TxOptions{}, func(ctx context.Context, tx repository.Tx) error {
		// Save assistant message
		aiMsg := model.ChatMessage{
			ID:        uuid.NewString(),
			SessionID: session.ID,
			Role:      "assistant",
			Content:   reply,
			Tokens:    usage.CompletionTokens,
			Timestamp: time.Now(),
		}
		if _, err := p.chatRepo.SaveMessage(ctx, tx, &aiMsg); err != nil {
			return err
		}

		// Deduct exact cost
		spent := int64(usage.PromptTokens)*pricing.InputTokenPriceMicros +
			int64(usage.CompletionTokens)*pricing.OutputTokenPriceMicros
		if _, err := p.subManager.DeductCredits(ctx, session.UserID, spent); err != nil {
			return err
		}

		// Send message back to the user
		user, err := p.chatRepo.FindUserBySessionID(ctx, tx, session.ID)
		if err != nil {
			p.log.Error().Err(err).Str("session_id", session.ID).Msg("could not find user to send AI reply")
			return nil // Don't fail the transaction, just log the error
		}

		if err := p.botAdapter.SendMessage(ctx, adapter.SendMessageParams{
			ChatID: user.TelegramID,
			Text:   reply,
		}); err != nil {
			p.log.Error().Err(err).Int64("tg_id", user.TelegramID).Msg("Failed to send final AI reply via Telegram")
			// Don't fail the transaction for this, just log it.
		}

		return nil
	})
}
