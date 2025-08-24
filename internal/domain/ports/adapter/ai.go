// File: internal/domain/ports/adapter/ai.go
package adapter

import "context"

// Message represents a chat message. We add JSON tags so it can be serialized
// directly in OpenAI/Metis-compatible requests (messages[] with role/content).
// This keeps provider-agnostic semantics while staying convenient for adapters.
type Message struct {
	Role    string `json:"role"` // "user", "assistant", or "system"
	Content string `json:"content"`
}

// ModelInfo gives basic metadata about a model; adapters can fill what they know.
type ModelInfo struct {
	Name        string
	Description string
	MaxTokens   int
	Supports    []string // e.g. ["text"], or later ["text","image"]
}

// AIServiceAdapter is the hex-port for LLM chat.
type AIServiceAdapter interface {
	ListModels(ctx context.Context) ([]string, error)
	GetModelInfo(model string) (ModelInfo, error)
	Chat(ctx context.Context, model string, messages []Message) (string, error)
}
