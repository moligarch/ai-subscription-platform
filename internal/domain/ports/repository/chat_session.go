package repository

import (
	"context"
	"telegram-ai-subscription/internal/domain/model"
)

// -----------------------------
// Chat Sessions
// -----------------------------

type ChatSessionRepository interface {
	Save(ctx context.Context, tx Tx, session *model.ChatSession) error
	SaveMessage(ctx context.Context, tx Tx, message *model.ChatMessage) error
	Delete(ctx context.Context, tx Tx, id string) error
	FindActiveByUser(ctx context.Context, tx Tx, userID string) (*model.ChatSession, error)
	ListByUser(ctx context.Context, tx Tx, userID string, offset, limit int) ([]*model.ChatSession, error)
	FindByID(ctx context.Context, tx Tx, id string) (*model.ChatSession, error)
	UpdateStatus(ctx context.Context, tx Tx, sessionID string, status model.ChatSessionStatus) error
	CleanupOldMessages(ctx context.Context, userID string, retentionDays int) (int64, error)
}
