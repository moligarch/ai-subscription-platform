// File: internal/domain/ports/adapter/telegram.go
package adapter

import "context"

// TelegramBotAdapter is a domain-level port for sending Telegram messages.
// Implementations may choose to support either domain user IDs or raw Telegram IDs.
type TelegramBotAdapter interface {
	// SendMessage sends a simple text message to a user identified by internal user ID (UUID).
	SendMessage(ctx context.Context, userID string, text string) error
	// SendMessageWithTelegramID sends directly using the Telegram user/chat id.
	SendMessageWithTelegramID(ctx context.Context, telegramID int64, text string) error
}
