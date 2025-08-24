package ai

import (
	"context"
	"log"
	"time"

	"telegram-ai-subscription/internal/domain/ports/adapter"
)

var _ adapter.AIServiceAdapter = (*NoopAIAdapter)(nil)

// NoopAIAdapter implements adapter.AIServiceAdapter for local/dev testing.
// It logs messages instead of sending real AI requests.
type NoopAIAdapter struct {
	// you can add fields like logger or rate-limiting configs later
}

// NewNoopAIAdapter constructs the noop adapter.
func NewNoopAIAdapter() *NoopAIAdapter {
	return &NoopAIAdapter{}
}

// SendMessage logs the message and simulates small delay.
func (a *NoopAIAdapter) SendMessage(ctx context.Context, userID string, text string) error {
	// Simulate slight processing time and respect ctx
	select {
	case <-time.After(100 * time.Millisecond):
		// proceed
	case <-ctx.Done():
		return ctx.Err()
	}
	log.Printf("[noop-ai] To user %s: %s\n", userID, text)
	return nil
}

// Chat implements the missing method for AIServiceAdapter interface.
func (a *NoopAIAdapter) Chat(ctx context.Context, userID string, messages []adapter.Message) (string, error) {
	// Simulate processing and log the messages
	select {
	case <-time.After(100 * time.Millisecond):
		// proceed
	case <-ctx.Done():
		return "", ctx.Err()
	}
	log.Printf("[noop-ai] Chat with user %s: %v\n", userID, messages)
	// Return a dummy response
	return "This is a noop AI response.", nil
}

// GetModelInfo implements the missing method for AIServiceAdapter interface.
func (a *NoopAIAdapter) GetModelInfo(model string) (adapter.ModelInfo, error) {
	// Simulate dummy model info
	info := adapter.ModelInfo{
		Name:        "noop-ai-model",
		Description: "Noop AI model for testing",
		MaxTokens:   1024,
		Supports:    []string{"chat", "completion"},
	}
	return info, nil
}

// ListModels implements the missing method for AIServiceAdapter interface.
func (a *NoopAIAdapter) ListModels(ctx context.Context) ([]string, error) {
	// Simulate slight processing time and respect ctx
	select {
	case <-time.After(100 * time.Millisecond):
		// proceed
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	models := []string{"noop-ai-model"}
	return models, nil
}
