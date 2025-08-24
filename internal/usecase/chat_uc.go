// File: internal/usecase/chat_uc.go
package usecase

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/adapter"
	"telegram-ai-subscription/internal/domain/ports/repository"
)

// Compile-time check
var _ ChatUseCase = (*chatUC)(nil)

type ChatUseCase interface {
	StartChat(ctx context.Context, userID, modelName string) (*model.ChatSession, error)
	SendMessage(ctx context.Context, sessionID, userMessage string) (reply string, err error)
	EndChat(ctx context.Context, userID string) error
	FindActiveSession(ctx context.Context, userID string) (*model.ChatSession, error)
	ListModels(ctx context.Context) ([]string, error)
}

type chatUC struct {
	sessions repository.ChatSessionRepository
	ai       adapter.AIServiceAdapter
	subs     SubscriptionUseCase
	devMode  bool
}

func NewChatUseCase(sessions repository.ChatSessionRepository, ai adapter.AIServiceAdapter, subs SubscriptionUseCase, devMode bool) *chatUC {
	return &chatUC{sessions: sessions, ai: ai, subs: subs, devMode: devMode}
}

func (c *chatUC) StartChat(ctx context.Context, userID, modelName string) (*model.ChatSession, error) {
	// Only one active session per user
	if s, err := c.sessions.FindActiveByUser(ctx, nil, userID); err == nil && s != nil {
		return s, nil
	}

	s := model.NewChatSession(uuid.NewString(), userID, modelName)
	if err := c.sessions.Save(ctx, nil, s); err != nil {
		return nil, err
	}
	return s, nil
}

func (c *chatUC) SendMessage(ctx context.Context, sessionID, userMessage string) (string, error) {
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
