//go:build !integration

package postgres

import (
	"context"
	"encoding/json"
	"testing"

	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/repository"
)

func TestPlanRepoCacheDecorator(t *testing.T) {
	ctx := context.Background()
	plan := &model.SubscriptionPlan{ID: "plan-123", Name: "Pro"}
	planJSON, _ := json.Marshal(plan)

	t.Run("FindByID should return from cache on hit", func(t *testing.T) {
		// Arrange
		mockRedis := &mockRedisClient{
			GetFunc: func(ctx context.Context, key string) (string, error) {
				return string(planJSON), nil // Simulate cache hit
			},
		}
		innerRepoCalled := false
		mockInnerRepo := &mockInnerPlanRepo{
			FindByIDFunc: func(ctx context.Context, tx repository.Tx, id string) (*model.SubscriptionPlan, error) {
				innerRepoCalled = true // This should not be called
				return nil, nil
			},
		}

		decorator := NewPlanRepoCacheDecorator(mockInnerRepo, mockRedis)

		// Act
		result, err := decorator.FindByID(ctx, nil, "plan-123")

		// Assert
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if innerRepoCalled {
			t.Error("inner repository should not be called on a cache hit")
		}
		if result == nil || result.ID != "plan-123" {
			t.Error("did not return the correct plan from cache")
		}
	})

	t.Run("Save should invalidate the cache", func(t *testing.T) {
		// Arrange
		var deletedKeys []string
		mockRedis := &mockRedisClient{
			DelFunc: func(ctx context.Context, keys ...string) error {
				deletedKeys = append(deletedKeys, keys...)
				return nil
			},
		}
		mockInnerRepo := &mockInnerPlanRepo{
			SaveFunc: func(ctx context.Context, tx repository.Tx, plan *model.SubscriptionPlan) error {
				return nil
			},
		}

		decorator := NewPlanRepoCacheDecorator(mockInnerRepo, mockRedis)

		// Act
		err := decorator.Save(ctx, nil, plan)

		// Assert
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if len(deletedKeys) != 2 {
			t.Fatalf("expected 2 keys to be deleted, but got %d", len(deletedKeys))
		}
	})
}
