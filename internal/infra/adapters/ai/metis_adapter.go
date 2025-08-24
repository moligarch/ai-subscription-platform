package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"telegram-ai-subscription/internal/domain/ports/adapter"
)

// Compile-time assurance this adapter satisfies the port
var _ adapter.AIServiceAdapter = (*MetisOpenAIAdapter)(nil)

// MetisOpenAIAdapter implements adapter.AIServiceAdapter against Metis's OpenAI-compatible gateway.
// Base URL defaults to https://api.metisai.ir/openai/v1 (configurable).
// Docs: https://docs.metisai.ir/api/openai  (OpenAI-compatible wrapper)
// Chat completions path is the same as OpenAI: /chat/completions
// Authorization: Bearer <METIS_API_KEY>
type MetisOpenAIAdapter struct {
	apiKey string
	base   string // e.g., https://api.metisai.ir/openai/v1
	model  string
	client *http.Client
}

func NewMetisOpenAIAdapter(apiKey, model, base string) (*MetisOpenAIAdapter, error) {
	if apiKey == "" {
		return nil, errors.New("metis api key empty")
	}
	if model == "" {
		model = "gpt-4o-mini"
	}
	if base == "" {
		base = "https://api.metisai.ir/openai/v1"
	}
	return &MetisOpenAIAdapter{
		apiKey: apiKey,
		base:   strings.TrimRight(base, "/"),
		model:  model,
		client: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

func (m *MetisOpenAIAdapter) ListModels(ctx context.Context) ([]string, error) {
	return []string{m.model}, nil
}

func (m *MetisOpenAIAdapter) GetModelInfo(model string) (adapter.ModelInfo, error) {
	if model == "" {
		model = m.model
	}
	return adapter.ModelInfo{
		Name:        model,
		Description: "Metis OpenAI-compatible model",
		MaxTokens:   0,
		Supports:    []string{"text"},
	}, nil
}

func (m *MetisOpenAIAdapter) Chat(ctx context.Context, model string, messages []adapter.Message) (string, error) {
	if model == "" {
		model = m.model
	}
	// Build the request using the shared adapter.Message with JSON tags
	reqBody := struct {
		Model    string            `json:"model"`
		Messages []adapter.Message `json:"messages"`
	}{Model: model, Messages: messages}

	b, _ := json.Marshal(reqBody)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, m.base+"/chat/completions", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+m.apiKey)

	resp, err := m.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("metis(openai) http %d", resp.StatusCode)
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
