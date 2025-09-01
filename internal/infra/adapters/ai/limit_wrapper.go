package ai

import (
	"context"

	"telegram-ai-subscription/internal/domain/ports/adapter"
)

// Compile-time check
var _ adapter.AIServiceAdapter = (*limitedAI)(nil)

type limitedAI struct {
	inner adapter.AIServiceAdapter
	sem   chan struct{}
}

func NewLimitedAI(inner adapter.AIServiceAdapter, maxConcurrent int) adapter.AIServiceAdapter {
	if maxConcurrent <= 0 {
		return inner
	}
	return &limitedAI{
		inner: inner,
		sem:   make(chan struct{}, maxConcurrent),
	}
}

func (l *limitedAI) ListModels(ctx context.Context) ([]string, error) {
	return l.inner.ListModels(ctx)
}

func (l *limitedAI) GetModelInfo(model string) (adapter.ModelInfo, error) {
	return l.inner.GetModelInfo(model)
}

func (l *limitedAI) Chat(ctx context.Context, model string, messages []adapter.Message) (string, error) {
	l.sem <- struct{}{}
	defer func() { <-l.sem }()
	return l.inner.Chat(ctx, model, messages)
}

func (l *limitedAI) ChatWithUsage(ctx context.Context, model string, messages []adapter.Message) (string, adapter.Usage, error) {
	l.sem <- struct{}{}
	defer func() { <-l.sem }()
	return l.inner.ChatWithUsage(ctx, model, messages)
}

func (l *limitedAI) CountTokens(ctx context.Context, model string, messages []adapter.Message) (int, error) {
	l.sem <- struct{}{}
	defer func() { <-l.sem }()
	return l.inner.CountTokens(ctx, model, messages)
}
