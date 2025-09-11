package telegram

import (
	"context"
	"log"
	"time"

	"telegram-ai-subscription/internal/domain/ports/adapter"
)

var _ adapter.TelegramBotAdapter = (*NoopBotAdapter)(nil)

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
func (b *NoopBotAdapter) SendMessage(ctx context.Context, params adapter.SendMessageParams) error {
	select {
	case <-time.After(100 * time.Millisecond):
	case <-ctx.Done():
		return ctx.Err()
	}
	log.Printf("[noop-telegram] To user %d: %s [markup: %+v]\n", params.ChatID, params.Text, params.ReplyMarkup)
	return nil
}

// SetMenuCommands is a no-op that logs the call details.
func (b *NoopBotAdapter) SetMenuCommands(ctx context.Context, chatID int64, isAdmin bool) error {
	log.Printf("[noop-telegram] SetMenuCommands called for chatID %d, isAdmin: %t", chatID, isAdmin)
	return nil
}
