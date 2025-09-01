// File: internal/infra/adapters/ai/multi_adapter.go
package ai

import (
	"context"
	"strings"

	"telegram-ai-subscription/internal/domain/ports/adapter"
)

var _ adapter.AIServiceAdapter = (*MultiAIAdapter)(nil)

type MultiAIAdapter struct {
	defaultProvider string // e.g., "openai" or "gemini"
	byProvider      map[string]adapter.AIServiceAdapter
	modelToProvider map[string]string // model -> provider ("openai" | "gemini")
}

// NewMultiAIAdapter does not inject any default model; it only knows a default provider.
// Each provider adapter is responsible for its own default model.
func NewMultiAIAdapter(
	defaultProvider string,
	byProvider map[string]adapter.AIServiceAdapter,
	modelToProvider map[string]string,
) *MultiAIAdapter {
	return &MultiAIAdapter{
		defaultProvider: strings.ToLower(defaultProvider),
		byProvider:      byProvider,
		modelToProvider: modelToProvider,
	}
}

func (m *MultiAIAdapter) resolveProvider(model string) string {
	if p := m.modelToProvider[model]; p != "" {
		return strings.ToLower(p)
	}
	l := strings.ToLower(model)
	switch {
	case strings.HasPrefix(l, "gemini"):
		return "gemini"
	case strings.HasPrefix(l, "gpt"): // OpenAI models
		return "openai"
	default:
		return m.defaultProvider
	}
}

func (m *MultiAIAdapter) pick(model string) adapter.AIServiceAdapter {
	prov := m.resolveProvider(model)
	if a := m.byProvider[prov]; a != nil {
		return a
	}
	// last resort: first available
	for _, a := range m.byProvider {
		if a != nil {
			return a
		}
	}
	return nil
}

func (m *MultiAIAdapter) ListModels(ctx context.Context) ([]string, error) {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(m.modelToProvider)+4)

	// 1) models explicitly mapped in config
	for model := range m.modelToProvider {
		if _, ok := seen[model]; !ok {
			seen[model] = struct{}{}
			out = append(out, model)
		}
	}

	// 2) union of each provider's ListModels (often returns their default)
	for _, a := range m.byProvider {
		list, _ := a.ListModels(ctx)
		for _, name := range list {
			if name == "" {
				continue
			}
			if _, ok := seen[name]; !ok {
				seen[name] = struct{}{}
				out = append(out, name)
			}
		}
	}
	return out, nil
}

func (m *MultiAIAdapter) GetModelInfo(model string) (adapter.ModelInfo, error) {
	a := m.pick(model)
	if a == nil {
		return adapter.ModelInfo{Name: model}, nil
	}
	return a.GetModelInfo(model)
}

func (m *MultiAIAdapter) CountTokens(ctx context.Context, model string, messages []adapter.Message) (int, error) {
	a := m.pick(model)
	if a == nil {
		return 0, nil
	}
	return a.CountTokens(ctx, model, messages)
}

func (m *MultiAIAdapter) Chat(ctx context.Context, model string, messages []adapter.Message) (string, error) {
	a := m.pick(model)
	if a == nil {
		return "", nil
	}
	return a.Chat(ctx, model, messages)
}

func (m *MultiAIAdapter) ChatWithUsage(ctx context.Context, model string, messages []adapter.Message) (string, adapter.Usage, error) {
	a := m.pick(model)
	if a == nil {
		return "", adapter.Usage{}, nil
	}
	return a.ChatWithUsage(ctx, model, messages)
}
