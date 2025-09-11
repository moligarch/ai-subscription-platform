package adapter

import "context"

// Button is a generic representation of a button.
type Button struct {
	Text           string
	Data           string // For callbacks
	URL            string
	RequestContact bool // Signal for a "Share Contact" button
}

// ReplyMarkup represents any kind of keyboard markup.
type ReplyMarkup struct {
	Buttons    [][]Button
	IsInline   bool // Differentiates between Inline and Reply keyboards
	IsOneTime  bool // For reply keyboards, should it disappear after use?
	IsPersonal bool // For reply keyboards, show only to a specific user?
}

// SendMessageParams holds all possible options for sending a message.
type SendMessageParams struct {
	ChatID      int64
	Text        string
	ParseMode   string
	ReplyMarkup *ReplyMarkup // Pointer, so it can be nil
}

type TelegramBotAdapter interface {
	SendMessage(ctx context.Context, params SendMessageParams) error
	SetMenuCommands(ctx context.Context, chatID int64, isAdmin bool) error
}
