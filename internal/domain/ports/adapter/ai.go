package adapter

import "context"

// Message represents a chat message.
type Message struct {
	Role    string `json:"role"` // "user", "assistant", "system"
	Content string `json:"content"`
}

// ModelInfo describes a model.
type ModelInfo struct {
	Name        string
	Description string
	MaxTokens   int
	Supports    []string
}

// Usage for a single chat call.
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// AIServiceAdapter is the port for LLM chat.
type AIServiceAdapter interface {
	ListModels(ctx context.Context) ([]string, error)
	GetModelInfo(model string) (ModelInfo, error)

	// CountTokens must return prompt tokens for the provided messages
	// (provider-specific counting; best-effort when exact isn’t available).
	CountTokens(ctx context.Context, model string, messages []Message) (int, error)

	// Chat returns only the assistant text
	Chat(ctx context.Context, model string, messages []Message) (string, error)

	// ChatWithUsage returns assistant text + usage as reported by the provider.
	ChatWithUsage(ctx context.Context, model string, messages []Message) (string, Usage, error)
}
