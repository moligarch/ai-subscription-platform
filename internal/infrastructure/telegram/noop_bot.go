package telegram

import (
	"context"
	"log"
	"time"

	"telegram-ai-subscription/internal/domain/adapter"
)

// NoopBotAdapter implements adapter.TelegramBotAdapter for local/dev testing.
// It logs messages instead of sending real Telegram messages.
type NoopBotAdapter struct {
	// you can add fields like logger or rate-limiting configs later
}

// NewNoopBotAdapter constructs the noop adapter.
func NewNoopBotAdapter() *NoopBotAdapter {
	return &NoopBotAdapter{}
}

// SendMessage logs the message and simulates small delay.
func (b *NoopBotAdapter) SendMessage(ctx context.Context, userID string, text string) error {
	// Simulate slight processing time and respect ctx
	select {
	case <-time.After(100 * time.Millisecond):
		// proceed
	case <-ctx.Done():
		return ctx.Err()
	}
	log.Printf("[noop-telegram] To user %s: %s\n", userID, text)
	return nil
}

// Ensure interface compliance
var _ adapter.TelegramBotAdapter = (*NoopBotAdapter)(nil)
