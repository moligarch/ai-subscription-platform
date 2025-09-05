package ai_test

import (
	"context"
	"testing"

	"telegram-ai-subscription/internal/domain/ports/adapter"
	ai "telegram-ai-subscription/internal/infra/adapters/ai"
)

type stubAI struct {
	name         string
	ctN          int
	cwuN         int
	lastModelCT  string
	lastModelCWU string
}

func (s *stubAI) ListModels(ctx context.Context) ([]string, error) {
	return []string{s.name + "-model"}, nil
}
func (s *stubAI) GetModelInfo(model string) (adapter.ModelInfo, error) {
	return adapter.ModelInfo{Name: model}, nil
}
func (s *stubAI) CountTokens(ctx context.Context, model string, messages []adapter.Message) (int, error) {
	s.ctN++
	s.lastModelCT = model
	return 1, nil
}
func (s *stubAI) Chat(ctx context.Context, model string, messages []adapter.Message) (string, error) {
	return "ok", nil
}
func (s *stubAI) ChatWithUsage(ctx context.Context, model string, messages []adapter.Message) (string, adapter.Usage, error) {
	s.cwuN++
	s.lastModelCWU = model
	return "ok", adapter.Usage{PromptTokens: 1, CompletionTokens: 1}, nil
}

func TestRouting_ExplicitMap_Heuristics_And_Fallback(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	open := &stubAI{name: "openai"}
	gem := &stubAI{name: "gemini"}

	m := ai.NewMultiAIAdapter(
		"openai",
		map[string]adapter.AIServiceAdapter{"openai": open, "gemini": gem},
		map[string]string{"custom-x": "gemini"},
	)

	// explicit map wins
	_, _ = m.CountTokens(ctx, "custom-x", nil)
	if gem.ctN != 1 || open.ctN != 0 {
		t.Fatalf("explicit map should route to gemini, got open:%d gem:%d", open.ctN, gem.ctN)
	}
	open.ctN, gem.ctN = 0, 0

	// gpt-* -> openai
	_, _, _ = m.ChatWithUsage(ctx, "gpt-4o-mini", nil)
	if open.cwuN != 1 || gem.cwuN != 0 {
		t.Fatalf("heuristic gpt-* should go openai")
	}
	open.cwuN, gem.cwuN = 0, 0

	// gemini-* -> gemini
	_, _, _ = m.ChatWithUsage(ctx, "gemini-1.5-flash", nil)
	if gem.cwuN != 1 || open.cwuN != 0 {
		t.Fatalf("heuristic gemini-* should go gemini")
	}

	// unknown -> default provider (openai)
	open.ctN, gem.ctN = 0, 0
	_, _ = m.CountTokens(ctx, "unknown", nil)
	if open.ctN != 1 || gem.ctN != 0 {
		t.Fatalf("unknown model should go to default provider (openai)")
	}
}
