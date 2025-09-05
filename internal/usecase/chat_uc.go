package usecase

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v4"
	"github.com/rs/zerolog"

	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/adapter"
	"telegram-ai-subscription/internal/domain/ports/repository"
	"telegram-ai-subscription/internal/infra/logging"
	red "telegram-ai-subscription/internal/infra/redis"
)

// Compile-time check
var _ ChatUseCase = (*chatUC)(nil)

type HistoryItem struct {
	SessionID    string
	Model        string
	FirstMessage string
	CreatedAt    time.Time
}

type ChatUseCase interface {
	StartChat(ctx context.Context, userID, modelName string) (*model.ChatSession, error)
	SendChatMessage(ctx context.Context, sessionID, userMessage string) (err error)
	EndChat(ctx context.Context, sessionID string) error
	FindActiveSession(ctx context.Context, userID string) (*model.ChatSession, error)
	ListModels(ctx context.Context) ([]string, error)

	ListHistory(ctx context.Context, userID string, offset, limit int) ([]HistoryItem, error)
	SwitchActiveSession(ctx context.Context, userID, sessionID string) error
	DeleteSession(ctx context.Context, sessionID string) error
}

type chatUC struct {
	sessions repository.ChatSessionRepository
	jobs     repository.AIJobRepository
	ai       adapter.AIServiceAdapter
	subs     SubscriptionUseCase
	devMode  bool
	prices   repository.ModelPricingRepository

	lock red.Locker
	tm   repository.TransactionManager
	log  *zerolog.Logger
}

func NewChatUseCase(
	sessions repository.ChatSessionRepository,
	jobs repository.AIJobRepository,
	ai adapter.AIServiceAdapter,
	subs SubscriptionUseCase,
	locker red.Locker,
	tm repository.TransactionManager,
	logger *zerolog.Logger,
	devMode bool,
	prices repository.ModelPricingRepository,
) *chatUC {
	return &chatUC{sessions: sessions, jobs: jobs, ai: ai, subs: subs, lock: locker, tm: tm, log: logger, devMode: devMode, prices: prices}
}

func (c *chatUC) StartChat(ctx context.Context, userID, modelName string) (*model.ChatSession, error) {
	defer logging.TraceDuration(c.log, "ChatUC.StartChat")()
	// Acquire a short lock to serialize concurrent /chat presses per user.
	lockKey := "chat:start:" + userID

	// brief, bounded backoff loop (e.g., total ~250ms) to reduce false negatives under load
	token, err := c.lock.TryLock(ctx, lockKey, 3*time.Second)
	if err != nil {
		c.log.Error().Msg("ChatUC.StartChat: Failed to initiate a chat")
		return nil, domain.ErrInitiateChat
	}
	defer func() { _ = c.lock.Unlock(ctx, lockKey, token) }()

	// Double-check existing active session.
	if s, err := c.sessions.FindActiveByUser(ctx, repository.NoTX, userID); err == nil && s != nil {
		return nil, domain.ErrActiveChatExists
	}

	s := model.NewChatSession(uuid.NewString(), userID, modelName)
	if err := c.sessions.Save(ctx, repository.NoTX, s); err != nil {
		c.log.Error().Msg("ChatUC.StartChat: Failed to initiate a session")
		return nil, domain.ErrInitiateChat
	}
	return s, nil
}

func (c *chatUC) SendChatMessage(ctx context.Context, sessionID, userMessage string) (err error) {
	defer logging.TraceDuration(c.log, "ChatUC.SendChatMessage")()

	s, err := c.sessions.FindByID(ctx, repository.NoTX, sessionID)
	if err != nil {
		return domain.ErrNotFound
	}

	if s.Status != model.ChatSessionActive {
		return domain.ErrNoActiveChat
	}
	userMessage = strings.TrimSpace(userMessage)
	if userMessage == "" {
		return domain.ErrInvalidArgument
	}

	// This whole block is now a single, fast transaction
	return c.tm.WithTx(ctx, pgx.TxOptions{}, func(ctx context.Context, tx repository.Tx) error {
		// Pre-check for active subscription (no credit check yet, worker will do that)
		if !c.devMode {
			if _, err := c.subs.GetActive(ctx, s.UserID); err != nil {
				return domain.ErrNoActiveSubscription
			}
		}

		// 1. Save user message
		// Note: We create a unique ID for the message here
		userMsg := model.ChatMessage{
			ID:        uuid.NewString(), // Add ID to the model if it's not there
			SessionID: s.ID,
			Role:      "user",
			Content:   userMessage,
			Timestamp: time.Now(),
		}
		if err := c.sessions.SaveMessage(ctx, tx, &userMsg); err != nil {
			return err
		}

		// 2. Create the AI job
		job := &model.AIJob{
			Status:        model.AIJobStatusPending,
			SessionID:     s.ID,
			UserMessageID: userMsg.ID,
			CreatedAt:     time.Now(),
		}
		if err := c.jobs.Save(ctx, tx, job); err != nil {
			return err
		}

		c.log.Info().Str("job_id", job.ID).Str("session_id", s.ID).Msg("AI job queued")
		return nil // Success!
	})
}

func (c *chatUC) EndChat(ctx context.Context, sessionID string) error {
	defer logging.TraceDuration(c.log, "ChatUC.EndChat")()
	s, err := c.sessions.FindByID(ctx, repository.NoTX, sessionID)
	switch err {
	case nil:
		break
	case domain.ErrNotFound:
		return domain.ErrNotFound
	default:
		return domain.ErrOperationFailed
	}

	err = c.sessions.UpdateStatus(ctx, repository.NoTX, s.ID, model.ChatSessionFinished)
	switch err {
	case nil:
		return nil
	default:
		return domain.ErrOperationFailed
	}
}

func (c *chatUC) FindActiveSession(ctx context.Context, userID string) (*model.ChatSession, error) {
	defer logging.TraceDuration(c.log, "ChatUC.FindActiveSession")()
	return c.sessions.FindActiveByUser(ctx, repository.NoTX, userID)
}

func (c *chatUC) ListModels(ctx context.Context) ([]string, error) {
	defer logging.TraceDuration(c.log, "ChatUC.ListModels")()

	rows, err := c.prices.ListActive(ctx, repository.NoTX)
	if err != nil {
		c.log.Error().Err(err).Msg("Failed to get active model price.")
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, p := range rows {
		// protect against empty names
		if name := strings.TrimSpace(p.ModelName); name != "" {
			out = append(out, name)
		}
	}
	return out, nil
}

func (c *chatUC) ListHistory(ctx context.Context, userID string, offset, limit int) ([]HistoryItem, error) {
	defer logging.TraceDuration(c.log, "ChatUC.ListHistory")()

	sessions, err := c.sessions.ListByUser(ctx, repository.NoTX, userID, offset, limit)
	if err != nil {
		c.log.Error().Err(err).Str("user_id", userID).Msg("Failed to retrieve user sessions.")
		return nil, err
	}
	items := make([]HistoryItem, 0, len(sessions))
	for _, s := range sessions {
		first := ""
		if len(s.Messages) > 0 {
			first = s.Messages[0].Content
			if r := []rune(first); len(r) > 120 {
				first = string(r[:120]) + "…"
			}
		}
		items = append(items, HistoryItem{
			SessionID:    s.ID,
			Model:        s.Model,
			FirstMessage: first,
			CreatedAt:    s.CreatedAt,
		})
	}
	return items, nil
}

func (c *chatUC) SwitchActiveSession(ctx context.Context, userID, sessionID string) error {
	defer logging.TraceDuration(c.log, "ChatUC.SwitchActiveSession")()

	// Wrap the entire logic in a transaction
	return c.tm.WithTx(ctx, pgx.TxOptions{}, func(ctx context.Context, tx repository.Tx) error {
		// Finish current active session if it's different from the target session
		if cur, err := c.sessions.FindActiveByUser(ctx, tx, userID); err == nil && cur != nil && cur.ID != sessionID {
			if err := c.sessions.UpdateStatus(ctx, tx, cur.ID, model.ChatSessionFinished); err != nil {
				c.log.Error().Err(err).Str("user_id", userID).Msg("Failed to close chat session")
				return err // Rollback
			}
		}
		// Activate the requested one
		return c.sessions.UpdateStatus(ctx, tx, sessionID, model.ChatSessionActive)
	})
}

func (c *chatUC) DeleteSession(ctx context.Context, sessionID string) error {
	defer logging.TraceDuration(c.log, "ChatUC.DeleteSession")()
	return c.sessions.Delete(ctx, repository.NoTX, sessionID)
}
