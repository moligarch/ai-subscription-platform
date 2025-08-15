package adapter

import "context"

// TelegramBotAdapter is a domain-level port for sending Telegram messages.
// Keep it minimal so other layers can implement it.
type TelegramBotAdapter interface {
	// SendMessage sends a simple text message to a user identified by userID (domain user id).
	// Implementation is responsible for mapping domain user id -> Telegram chat id.
	SendMessage(ctx context.Context, userID string, text string) error
}
