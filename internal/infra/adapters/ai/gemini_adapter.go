// File: internal/infra/adapters/ai/gemini_adapter.go
package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"telegram-ai-subscription/internal/domain/ports/adapter"
)

var _ adapter.AIServiceAdapter = (*GeminiAdapter)(nil)

// GeminiAdapter implements adapter.AIServiceAdapter against Gemini's generateContent API.
type GeminiAdapter struct {
	apiKey  string
	baseURL string // e.g., https://generativelanguage.googleapis.com
	client  *http.Client
	models  []string
}

func NewGeminiAdapter(apiKey, baseURL string, models []string) (*GeminiAdapter, error) {
	if apiKey == "" {
		return nil, errors.New("gemini api key empty")
	}
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com"
	}
	return &GeminiAdapter{
		apiKey:  apiKey,
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: 30 * time.Second},
		models:  models,
	}, nil
}

func (g *GeminiAdapter) ListModels(ctx context.Context) ([]string, error) {
	if len(g.models) > 0 {
		return g.models, nil
	}
	return []string{"gemini-1.5-pro"}, nil
}

func (g *GeminiAdapter) Chat(ctx context.Context, model string, messages []adapter.Message) (string, error) {
	if model == "" {
		model = "gemini-1.5-pro"
	}
	contents := make([]map[string]any, 0, len(messages))
	for _, m := range messages {
		role := strings.ToUpper(m.Role[:1]) + m.Role[1:]
		contents = append(contents, map[string]any{
			"role":  role,
			"parts": []map[string]string{{"text": m.Content}},
		})
	}
	body := map[string]any{
		"contents":         contents,
		"generationConfig": map[string]any{"temperature": 0.7},
	}
	b, _ := json.Marshal(body)
	url := fmt.Sprintf("%s/v1beta/models/%s:generateContent?key=%s", g.baseURL, model, g.apiKey)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(b)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := g.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("gemini http %d", resp.StatusCode)
	}
	var payload struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	for _, c := range payload.Candidates {
		for _, p := range c.Content.Parts {
			if p.Text != "" {
				return p.Text, nil
			}
		}
	}
	return "", errors.New("no candidate text")
}

func (g *GeminiAdapter) GetModelInfo(model string) (adapter.ModelInfo, error) {
	if model == "" {
		model = "gemini-1.5-pro"
	}
	url := fmt.Sprintf("%s/v1beta/models/%s?key=%s", g.baseURL, model, g.apiKey)
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	resp, err := g.client.Do(req)
	if err != nil {
		return adapter.ModelInfo{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return adapter.ModelInfo{}, fmt.Errorf("gemini http %d", resp.StatusCode)
	}
	var payload struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Parameters  struct {
			MaxTokens int `json:"max_tokens"`
		} `json:"parameters"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return adapter.ModelInfo{}, err
	}
	return adapter.ModelInfo{
		Name:        payload.Name,
		Description: payload.Description,
		MaxTokens:   payload.Parameters.MaxTokens,
	}, nil
}
