package repository

import (
	"context"
)

// ConversationState holds the user's progress in any multi-step conversation.
type ConversationState struct {
	Step string            `json:"step"` // e.g., "awaiting_fullname", "awaiting_activation_code"
	Data map[string]string `json:"data"` // To store collected info like plan_id or full_name
}

// StateRepository is the port for managing any user's conversational state.
type StateRepository interface {
	SetState(ctx context.Context, tgID int64, state *ConversationState) error
	GetState(ctx context.Context, tgID int64) (*ConversationState, error)
	ClearState(ctx context.Context, tgID int64) error
}
