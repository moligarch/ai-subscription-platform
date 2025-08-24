package repository

import (
	"context"
	"telegram-ai-subscription/internal/domain/model"
)

// -----------------------------
// Chat Sessions
// -----------------------------

type ChatSessionRepository interface {
	Save(ctx context.Context, qx any, session *model.ChatSession) error
	SaveMessage(ctx context.Context, qx any, message *model.ChatMessage) error
	Delete(ctx context.Context, qx any, id string) error
	FindActiveByUser(ctx context.Context, qx any, userID string) (*model.ChatSession, error)
	FindAllByUser(ctx context.Context, qx any, userID string) ([]*model.ChatSession, error)
	FindByID(ctx context.Context, qx any, id string) (*model.ChatSession, error)
	UpdateStatus(ctx context.Context, qx any, sessionID string, status model.ChatSessionStatus) error
	// CleanupOldMessages deletes messages older than the provided retention for a given user.
	CleanupOldMessages(ctx context.Context, userID string, retentionDays int) (int64, error)
}
