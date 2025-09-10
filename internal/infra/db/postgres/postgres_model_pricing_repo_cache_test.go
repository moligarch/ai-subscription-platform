package postgres

import (
	"context"
	"encoding/json"
	"testing"

	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/repository"
)

// --- Mocks for the ModelPricing Cache Test ---

type mockInnerPricingRepo struct {
	CreateFunc         func(ctx context.Context, tx repository.Tx, p *model.ModelPricing) error
	UpdateFunc         func(ctx context.Context, tx repository.Tx, p *model.ModelPricing) error
	GetByModelNameFunc func(ctx context.Context, tx repository.Tx, model string) (*model.ModelPricing, error)
	ListActiveFunc     func(ctx context.Context, tx repository.Tx) ([]*model.ModelPricing, error)
}

func (m *mockInnerPricingRepo) Create(ctx context.Context, tx repository.Tx, p *model.ModelPricing) error {
	return m.CreateFunc(ctx, tx, p)
}
func (m *mockInnerPricingRepo) Update(ctx context.Context, tx repository.Tx, p *model.ModelPricing) error {
	return m.UpdateFunc(ctx, tx, p)
}
func (m *mockInnerPricingRepo) GetByModelName(ctx context.Context, tx repository.Tx, model string) (*model.ModelPricing, error) {
	return m.GetByModelNameFunc(ctx, tx, model)
}
func (m *mockInnerPricingRepo) ListActive(ctx context.Context, tx repository.Tx) ([]*model.ModelPricing, error) {
	return m.ListActiveFunc(ctx, tx)
}

// --- End Mocks ---

func TestModelPricingRepoCacheDecorator(t *testing.T) {
	ctx := context.Background()
	pricing := &model.ModelPricing{ID: "price-123", ModelName: "gpt-4o"}
	pricingJSON, _ := json.Marshal(pricing)

	t.Run("GetByModelName should return from cache on hit", func(t *testing.T) {
		// Arrange
		mockRedis := &mockRedisClient{
			GetFunc: func(ctx context.Context, key string) (string, error) {
				return string(pricingJSON), nil // Simulate cache hit
			},
		}
		innerRepoCalled := false
		mockInnerRepo := &mockInnerPricingRepo{
			GetByModelNameFunc: func(ctx context.Context, tx repository.Tx, model string) (*model.ModelPricing, error) {
				innerRepoCalled = true // This should NOT be called
				return nil, nil
			},
		}

		decorator := NewModelPricingRepoCacheDecorator(mockInnerRepo, mockRedis)

		// Act
		result, err := decorator.GetByModelName(ctx, nil, "gpt-4o")

		// Assert
		if err != nil {
			t.Fatalf("expected no error, but got %v", err)
		}
		if innerRepoCalled {
			t.Error("inner repository should not be called on a cache hit")
		}
		if result == nil || result.ID != "price-123" {
			t.Error("did not return the correct pricing from cache")
		}
	})

	t.Run("Update should invalidate the cache", func(t *testing.T) {
		// Arrange
		var deletedKeys []string
		mockRedis := &mockRedisClient{
			DelFunc: func(ctx context.Context, keys ...string) error {
				deletedKeys = append(deletedKeys, keys...)
				return nil
			},
		}
		mockInnerRepo := &mockInnerPricingRepo{
			UpdateFunc: func(ctx context.Context, tx repository.Tx, p *model.ModelPricing) error {
				return nil
			},
		}

		decorator := NewModelPricingRepoCacheDecorator(mockInnerRepo, mockRedis)

		// Act
		err := decorator.Update(ctx, nil, pricing)

		// Assert
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if len(deletedKeys) != 2 {
			t.Fatalf("expected 2 keys to be deleted, but got %d", len(deletedKeys))
		}
	})
}
