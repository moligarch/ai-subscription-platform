package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"telegram-ai-subscription/internal/domain/ports/adapter"
)

// Compile-time assurance this adapter satisfies the port
var _ adapter.AIServiceAdapter = (*OpenAIAdapter)(nil)

// OpenAIAdapter implements adapter.AIServiceAdapter using Chat Completions API.
type OpenAIAdapter struct {
	apiKey string
	base   string // e.g., https://api.openai.com/v1
	model  string
	client *http.Client
}

func NewOpenAIAdapter(apiKey, model string) (*OpenAIAdapter, error) {
	if apiKey == "" {
		return nil, errors.New("openai api key empty")
	}
	if model == "" {
		model = "gpt-4o-mini"
	}
	return &OpenAIAdapter{
		apiKey: apiKey,
		base:   "https://api.openai.com/v1",
		model:  model,
		client: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

func (o *OpenAIAdapter) ListModels(ctx context.Context) ([]string, error) {
	return []string{o.model}, nil
}

func (o *OpenAIAdapter) GetModelInfo(model string) (adapter.ModelInfo, error) {
	if model == "" {
		model = o.model
	}
	return adapter.ModelInfo{
		Name:        model,
		Description: "OpenAI Chat Completions model",
		MaxTokens:   0,
		Supports:    []string{"text"},
	}, nil
}

func (o *OpenAIAdapter) Chat(ctx context.Context, model string, messages []adapter.Message) (string, error) {
	if model == "" {
		model = o.model
	}

	// Build the request using the shared adapter.Message with JSON tags
	reqBody := struct {
		Model    string            `json:"model"`
		Messages []adapter.Message `json:"messages"`
	}{Model: model, Messages: messages}

	b, _ := json.Marshal(reqBody)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, o.base+"/chat/completions", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.apiKey)

	resp, err := o.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("openai http %d", resp.StatusCode)
	}

	var payload struct {
		Choices []struct {
			Message adapter.Message `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	for _, c := range payload.Choices {
		if c.Message.Content != "" {
			return c.Message.Content, nil
		}
	}
	return "", errors.New("no choice content")
}
