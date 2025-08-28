package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/adapter"
	"telegram-ai-subscription/internal/domain/ports/repository"
	derror "telegram-ai-subscription/internal/error"
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
	SendMessage(ctx context.Context, sessionID, userMessage string) (reply string, err error)
	EndChat(ctx context.Context, sessionID string) error
	FindActiveSession(ctx context.Context, userID string) (*model.ChatSession, error)
	ListModels(ctx context.Context) ([]string, error)

	ListHistory(ctx context.Context, userID string, offset, limit int) ([]HistoryItem, error)
	SwitchActiveSession(ctx context.Context, userID, sessionID string) error
	DeleteSession(ctx context.Context, sessionID string) error
}

type chatUC struct {
	sessions repository.ChatSessionRepository
	ai       adapter.AIServiceAdapter
	subs     SubscriptionUseCase
	devMode  bool
	prices   repository.ModelPricingRepository

	locker red.Locker
	log    *zerolog.Logger
}

func NewChatUseCase(
	sessions repository.ChatSessionRepository,
	ai adapter.AIServiceAdapter,
	subs SubscriptionUseCase,
	locker red.Locker,
	logger *zerolog.Logger,
	devMode bool,
	prices repository.ModelPricingRepository,
) *chatUC {
	return &chatUC{sessions: sessions, ai: ai, subs: subs, locker: locker, log: logger, devMode: devMode, prices: prices}
}

func (c *chatUC) StartChat(ctx context.Context, userID, modelName string) (*model.ChatSession, error) {
	defer logging.TraceDuration(c.log, "ChatUC.StartChat")()
	// Acquire a short lock to serialize concurrent /chat presses per user.
	lockKey := "chat:start:" + userID

	// brief, bounded backoff loop (e.g., total ~250ms) to reduce false negatives under load
	token, err := c.locker.TryLock(ctx, lockKey, 3*time.Second)
	if err != nil {
		return nil, err
	}
	defer func() { _ = c.locker.Unlock(ctx, lockKey, token) }()

	// Double-check existing active session.
	if s, err := c.sessions.FindActiveByUser(ctx, nil, userID); err == nil && s != nil {
		return nil, derror.ErrActiveChatExists
	}

	s := model.NewChatSession(uuid.NewString(), userID, modelName)
	if err := c.sessions.Save(ctx, nil, s); err != nil {
		return nil, err
	}
	return s, nil
}

func (c *chatUC) SendMessage(ctx context.Context, sessionID, userMessage string) (string, error) {
	defer logging.TraceDuration(c.log, "ChatUC.SendMessage")()

	s, err := c.sessions.FindByID(ctx, nil, sessionID)
	if err != nil {
		return "", err
	}
	if s.Status != model.ChatSessionActive {
		return "", derror.ErrNoActiveChat
	}
	userMessage = strings.TrimSpace(userMessage)
	if userMessage == "" {
		return "", domain.ErrInvalidArgument
	}

	// --- Phase A: pre-send affordability check (pricing/balance only here) ---
	var pricing *model.ModelPricing
	var balanceMicros int64
	if !c.devMode {
		active, err := c.subs.GetActive(ctx, s.UserID)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return "", domain.ErrNoActiveSubscription
			}
			return "", err
		}
		if active == nil {
			return "❌ You don't have an active subscription. Use /plans to get started.", nil
		}
		balanceMicros = active.RemainingCredits // BIGINT micro-credits

		pr, err := c.prices.GetByModelName(ctx, s.Model)
		if err != nil {
			return "⚠️ Pricing for this model is not configured yet. Please try later or choose another model.", nil
		}
		pricing = pr
	}

	// --- Persist user message (unchanged) ---
	s.AddMessage("user", userMessage, 0)
	if err := c.sessions.SaveMessage(ctx, nil, &s.Messages[len(s.Messages)-1]); err != nil {
		return "", err
	}

	// --- Prepare AI context (unchanged) ---
	msgs := s.GetRecentMessages(15)
	adapterMsgs := make([]adapter.Message, 0, len(msgs))
	for _, m := range msgs {
		adapterMsgs = append(adapterMsgs, adapter.Message{Role: m.Role, Content: m.Content})
	}

	// --- Accurate pre-check using provider CountTokens on the final prompt ---
	if !c.devMode && pricing != nil {
		promptTokens, err := c.ai.CountTokens(ctx, s.Model, adapterMsgs)
		if err != nil {
			// Conservative local fallback: ~4 chars/token on just the last user message
			rl := len([]rune(userMessage))
			if rl < 0 {
				rl = 0
			}
			promptTokens = rl/4 + 1
		}
		requiredMicros := int64(promptTokens) * pricing.InputTokenPriceMicros
		if requiredMicros > balanceMicros {
			return fmt.Sprintf(
				"⚠️ Insufficient balance.\nCurrent: %d µcr\nRequired for this message: %d µcr\n\nTip: try a shorter message or /plans to top up.",
				balanceMicros, requiredMicros,
			), nil
		}
	}

	// --- Call AI with usage ---
	reply, usage, err := c.ai.ChatWithUsage(ctx, s.Model, adapterMsgs)
	if err != nil {
		return "", err
	}

	// --- Persist assistant message (unchanged) ---
	s.AddMessage("assistant", reply, 0)
	if err := c.sessions.SaveMessage(ctx, nil, &s.Messages[len(s.Messages)-1]); err != nil {
		return "", err
	}
	s.UpdatedAt = time.Now()
	_ = c.sessions.Save(ctx, nil, s)

	// --- Post-call: exact deduction based on provider usage ---
	if !c.devMode && pricing != nil {
		spent := int64(usage.PromptTokens)*pricing.InputTokenPriceMicros +
			int64(usage.CompletionTokens)*pricing.OutputTokenPriceMicros
		if spent > 0 {
			if _, derr := c.subs.DeductCredits(ctx, s.UserID, spent); derr != nil {
				return "", derr
			}
		}
	}

	return reply, nil
}

func (c *chatUC) EndChat(ctx context.Context, sessionID string) error {
	defer logging.TraceDuration(c.log, "ChatUC.EndChat")()
	s, err := c.sessions.FindByID(ctx, nil, sessionID)
	if err != nil {
		return err
	}
	return c.sessions.UpdateStatus(ctx, nil, s.ID, model.ChatSessionFinished)
}

func (c *chatUC) FindActiveSession(ctx context.Context, userID string) (*model.ChatSession, error) {
	return c.sessions.FindActiveByUser(ctx, nil, userID)
}

func (c *chatUC) ListModels(ctx context.Context) ([]string, error) {
	defer logging.TraceDuration(c.log, "ChatUC.ListModels")()

	rows, err := c.prices.ListActive(ctx)
	if err != nil {
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

	sessions, err := c.sessions.ListByUser(ctx, nil, userID, offset, limit)
	if err != nil {
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

	// Finish current active if different
	if cur, err := c.sessions.FindActiveByUser(ctx, nil, userID); err == nil && cur != nil && cur.ID != sessionID {
		if err := c.sessions.UpdateStatus(ctx, nil, cur.ID, model.ChatSessionFinished); err != nil {
			return err
		}
	}
	// Activate the requested one
	return c.sessions.UpdateStatus(ctx, nil, sessionID, model.ChatSessionActive)
}

func (c *chatUC) DeleteSession(ctx context.Context, sessionID string) error {
	defer logging.TraceDuration(c.log, "ChatUC.DeleteSession")()
	return c.sessions.Delete(ctx, nil, sessionID)
}
