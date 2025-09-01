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
	"telegram-ai-subscription/internal/infra/metrics"
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

	// Provider "guess" (for logging only; no behavior change)
	providerGuess := func(m string) string {
		l := strings.ToLower(m)
		if strings.HasPrefix(l, "gpt-") {
			return "openai"
		}
		if strings.HasPrefix(l, "gemini") {
			return "gemini"
		}
		return "default"
	}(s.Model)

	// ---------- Phase A: pre-send affordability check + pre-token count ----------
	var (
		pricing       *model.ModelPricing
		balanceMicros int64
		promptTokens  int // saved on the user message
	)

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
		balanceMicros = active.RemainingCredits

		pr, err := c.prices.GetByModelName(ctx, s.Model)
		if err != nil {
			return "⚠️ Pricing for this model is not configured yet. Please try later or choose another model.", nil
		}
		pricing = pr

		// Build history INCLUDING this new user message (but don't persist yet).
		msgsHist := s.GetRecentMessages(15)
		adapterMsgs := make([]adapter.Message, 0, len(msgsHist)+1)
		for _, m := range msgsHist {
			adapterMsgs = append(adapterMsgs, adapter.Message{Role: m.Role, Content: m.Content})
		}
		adapterMsgs = append(adapterMsgs, adapter.Message{Role: "user", Content: userMessage})

		// Count tokens for the full prompt we would send.
		preStart := time.Now()
		if n, err := c.ai.CountTokens(ctx, s.Model, adapterMsgs); err == nil {
			promptTokens = n
		} else {
			// Fallback: rough estimate on the new text (~4 chars/token).
			rl := len([]rune(userMessage))
			if rl < 0 {
				rl = 0
			}
			promptTokens = rl/4 + 1
			c.log.Warn().
				Str("event", "chat.precheck").
				Str("user_id", s.UserID).
				Str("session_id", s.ID).
				Str("model", s.Model).
				Str("provider_guess", providerGuess).
				Str("action", "proceed_no_count").
				Err(err).
				Int("latency_ms", int(time.Since(preStart)/time.Millisecond)).
				Msg("CountTokens failed; proceeding with heuristic")
		}

		requiredMicros := int64(promptTokens) * pricing.InputTokenPriceMicros
		action := "proceed"
		if requiredMicros > balanceMicros {
			action = "block"
		}

		// Structured pre-check log
		c.log.Info().
			Str("event", "chat.precheck").
			Str("user_id", s.UserID).
			Str("session_id", s.ID).
			Str("model", s.Model).
			Str("provider_guess", providerGuess).
			Int("prompt_tokens_est", promptTokens).
			Int64("required_micro", requiredMicros).
			Int64("balance_micro", balanceMicros).
			Str("action", action).
			Msg("")

		if action == "block" {
			metrics.PrecheckBlocked(providerGuess, s.Model)

			return fmt.Sprintf(
				"⚠️ Insufficient balance.\nCurrent: %d µcr\nRequired for this message: %d µcr\n\nTip: try a shorter message or /plans to top up.",
				balanceMicros, requiredMicros,
			), nil
		}
	}

	// ---------- Persist user message (store pre-count tokens) ----------
	s.AddMessage("user", userMessage, promptTokens)
	if err := c.sessions.SaveMessage(ctx, nil, &s.Messages[len(s.Messages)-1]); err != nil {
		return "", err
	}

	// ---------- Prepare AI context (now includes the just-saved user message) ----------
	msgs := s.GetRecentMessages(15)
	adapterMsgs := make([]adapter.Message, 0, len(msgs))
	for _, m := range msgs {
		adapterMsgs = append(adapterMsgs, adapter.Message{Role: m.Role, Content: m.Content})
	}

	// ---------- Call AI and get precise usage ----------
	callStart := time.Now()
	reply, usage, err := c.ai.ChatWithUsage(ctx, s.Model, adapterMsgs)
	if err != nil {
		metrics.ObserveChatUsage(providerGuess, s.Model, 0, 0, 0, 0, int(time.Since(callStart)/time.Millisecond), false)
		return "", err
	}

	// ---------- Persist assistant message with completion tokens ----------
	s.AddMessage("assistant", reply, usage.CompletionTokens)
	if err := c.sessions.SaveMessage(ctx, nil, &s.Messages[len(s.Messages)-1]); err != nil {
		return "", err
	}
	s.UpdatedAt = time.Now()
	_ = c.sessions.Save(ctx, nil, s)

	// ---------- Post-call: exact deduction based on usage + structured log ----------
	var spent int64
	if !c.devMode && pricing != nil {
		spent = int64(usage.PromptTokens)*pricing.InputTokenPriceMicros +
			int64(usage.CompletionTokens)*pricing.OutputTokenPriceMicros
		if spent > 0 {
			if _, derr := c.subs.DeductCredits(ctx, s.UserID, spent); derr != nil {
				return "", derr
			}
		}
	}
	metrics.ObserveChatUsage(
		providerGuess, s.Model,
		usage.PromptTokens,
		usage.CompletionTokens,
		usage.TotalTokens,
		spent, // micro-credits
		int(time.Since(callStart)/time.Millisecond),
		true, // success=true
	)
	c.log.Info().
		Str("event", "chat.usage").
		Str("user_id", s.UserID).
		Str("session_id", s.ID).
		Str("model", s.Model).
		Str("provider_guess", providerGuess).
		Int("tokens_in", usage.PromptTokens).
		Int("tokens_out", usage.CompletionTokens).
		Int("tokens_total", usage.TotalTokens).
		Int64("cost_micro", spent).
		Int("latency_ms", int(time.Since(callStart)/time.Millisecond)).
		Msg("")

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
