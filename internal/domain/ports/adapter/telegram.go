// File: internal/domain/ports/adapter/telegram.go
package adapter

import "context"

type InlineButton struct {
	Text string
	Data string
	URL  string
}

type TelegramBotAdapter interface {
	SendMessage(ctx context.Context, telegramID int64, text string) error
	SendButtons(ctx context.Context, telegramID int64, text string, rows [][]InlineButton) error
}
