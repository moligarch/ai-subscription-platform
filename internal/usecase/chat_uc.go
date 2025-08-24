// File: internal/usecase/chat_uc.go
package usecase

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
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

type ChatUseCase interface {
	StartChat(ctx context.Context, userID, modelName string) (*model.ChatSession, error)
	SendMessage(ctx context.Context, sessionID, userMessage string) (reply string, err error)
	EndChat(ctx context.Context, sessionID string) error
	FindActiveSession(ctx context.Context, userID string) (*model.ChatSession, error)
	ListModels(ctx context.Context) ([]string, error)
}

type chatUC struct {
	sessions repository.ChatSessionRepository
	ai       adapter.AIServiceAdapter
	subs     SubscriptionUseCase
	devMode  bool

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
) *chatUC {
	return &chatUC{sessions: sessions, ai: ai, subs: subs, locker: locker, log: logger, devMode: devMode}
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
		return nil, domain.ErrActiveChatExists
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
		return "", errors.New("chat is not active")
	}
	userMessage = strings.TrimSpace(userMessage)
	if userMessage == "" {
		return "", domain.ErrInvalidArgument
	}

	// Deduct 1 credit per user message (flat v1)
	if !c.devMode {
		if _, err := c.subs.DeductCredits(ctx, s.UserID, 1); err != nil {
			return "", err
		}
	}

	// Persist user message
	s.AddMessage("user", userMessage, 0)
	if err := c.sessions.SaveMessage(ctx, nil, &s.Messages[len(s.Messages)-1]); err != nil {
		return "", err
	}

	// Prepare AI context (recent messages)
	msgs := s.GetRecentMessages(15)
	adapterMsgs := make([]adapter.Message, 0, len(msgs))
	for _, m := range msgs {
		adapterMsgs = append(adapterMsgs, adapter.Message{Role: m.Role, Content: m.Content})
	}

	reply, err := c.ai.Chat(ctx, s.Model, adapterMsgs)
	if err != nil {
		return "", err
	}

	// Persist assistant message
	s.AddMessage("assistant", reply, 0)
	if err := c.sessions.SaveMessage(ctx, nil, &s.Messages[len(s.Messages)-1]); err != nil {
		return "", err
	}
	// Update updated_at
	s.UpdatedAt = time.Now()
	_ = c.sessions.Save(ctx, nil, s)
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
	return c.ai.ListModels(ctx)
}
