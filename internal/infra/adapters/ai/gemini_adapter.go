// File: .\internal\infra\adapters\ai\gemini_adapter.go
package ai

import (
	"context"
	"errors"
	"strings"

	"google.golang.org/genai"

	"telegram-ai-subscription/internal/domain/ports/adapter"
)

var _ adapter.AIServiceAdapter = (*GeminiAdapter)(nil)

type GeminiAdapter struct {
	client       *genai.Client
	defaultModel string
	maxOut       int
}

// NewGeminiAdapter creates a Gemini adapter using the official SDK.
// If your wiring expects a different constructor signature, keep it and
// call this initializer logic inside it.
func NewGeminiAdapter(ctx context.Context, apiKey, baseUrl, defaultModel string, maxOut int) (*GeminiAdapter, error) {
	if apiKey == "" {
		return nil, errors.New("gemini: empty api key")
	}
	c, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
		HTTPOptions: genai.HTTPOptions{
			BaseURL: baseUrl,
		},
	})
	if err != nil {
		return nil, err
	}
	return &GeminiAdapter{client: c, defaultModel: defaultModel, maxOut: maxOut}, nil
}

func (g *GeminiAdapter) ListModels(ctx context.Context) ([]string, error) {
	// Using the v1 SDK’s Models listing utilities.
	// We keep it simple and collect names that support text generation.
	models := g.client.Models.All(ctx)
	var out []string
	for m := range models {
		// Include models that support text. (Name example: "gemini-2.0-flash")
		if m.Name != "" {
			out = append(out, m.Name)
		}
	}
	if len(out) == 0 && g.defaultModel != "" {
		// Best-effort fallback to default
		out = []string{g.defaultModel}
	}
	return out, nil
}

func (g *GeminiAdapter) GetModelInfo(model string) (adapter.ModelInfo, error) {
	ctx := context.Background()
	m, err := g.client.Models.Get(ctx, model, nil)
	if err != nil {
		// Return minimal info on error so callers aren’t blocked.
		return adapter.ModelInfo{Name: model}, nil
	}
	return adapter.ModelInfo{
		Name:        m.Name,
		Description: m.Description,
		MaxTokens:   int(m.InputTokenLimit),
		Supports:    m.SupportedActions,
	}, nil
}

func (g *GeminiAdapter) CountTokens(ctx context.Context, model string, messages []adapter.Message) (int, error) {
	contents := toGenAIHistory(messages)
	// Per docs, CountTokens takes []*genai.Content. (NOT []genai.Part)
	// https://ai.google.dev/gemini-api/docs/tokens?hl=en#go
	resp, err := g.client.Models.CountTokens(ctx, modelOrDefault(model, g.defaultModel), contents, nil)
	if err != nil {
		return 0, err
	}
	return int(resp.TotalTokens), nil
}

func (g *GeminiAdapter) Chat(ctx context.Context, model string, messages []adapter.Message) (string, error) {
	reply, _, err := g.chatCore(ctx, model, messages)
	return reply, err
}

func (g *GeminiAdapter) ChatWithUsage(ctx context.Context, model string, messages []adapter.Message) (string, adapter.Usage, error) {
	return g.chatCore(ctx, model, messages)
}

// --- internal ---

func (g *GeminiAdapter) chatCore(ctx context.Context, model string, messages []adapter.Message) (string, adapter.Usage, error) {
	if len(messages) == 0 {
		return "", adapter.Usage{}, errors.New("gemini: no messages")
	}
	history := toGenAIHistory(messages[:len(messages)-1])

	chat, err := g.client.Chats.Create(
		ctx,
		modelOrDefault(model, g.defaultModel),
		&genai.GenerateContentConfig{ // NEW
			MaxOutputTokens: int32(g.maxOut),
		},
		history,
	)
	if err != nil {
		return "", adapter.Usage{}, err
	}

	last := messages[len(messages)-1]
	if strings.ToLower(last.Role) != "user" {
		return "", adapter.Usage{}, errors.New("gemini: last message must be from user")
	}

	resp, err := chat.SendMessage(ctx, genai.Part{Text: last.Content})
	if err != nil {
		return "", adapter.Usage{}, err
	}

	// Extract text
	text := ""
	if resp != nil && len(resp.Candidates) > 0 && resp.Candidates[0].Content != nil && len(resp.Candidates[0].Content.Parts) > 0 {
		if t := resp.Candidates[0].Content.Parts[0].Text; t != "" {
			text = t
		}
	}
	// Usage (if present)
	u := adapter.Usage{}
	if resp != nil && resp.UsageMetadata != nil {
		u.PromptTokens = int(resp.UsageMetadata.PromptTokenCount)
		u.CompletionTokens = int(resp.UsageMetadata.CandidatesTokenCount)
		u.TotalTokens = int(resp.UsageMetadata.TotalTokenCount)
	}
	return text, u, nil
}

func toGenAIHistory(msgs []adapter.Message) []*genai.Content {
	out := make([]*genai.Content, 0, len(msgs))
	for _, m := range msgs {
		role := genai.RoleUser
		switch strings.ToLower(m.Role) {
		case "assistant", "model":
			role = genai.RoleModel
		case "system":
			// Gemini doesn’t have a separate "system" role in history;
			// treat as a user instruction here. Alternatively, supply
			// system instruction via config if you need it.
			role = genai.RoleUser
		}
		out = append(out, &genai.Content{
			Role:  role,
			Parts: []*genai.Part{{Text: m.Content}},
		})
	}
	return out
}

func modelOrDefault(model, def string) string {
	if strings.TrimSpace(model) != "" {
		return model
	}
	return def
}
