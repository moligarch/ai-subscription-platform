package postgres_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/repository"
	"telegram-ai-subscription/internal/infra/db/postgres"

	"github.com/go-redis/redis/v8"
)

// --- Mocks for the Cache Decorator Test ---

// mockInnerPlanRepo mocks the database repository that the decorator wraps.
type mockInnerPlanRepo struct {
	SaveFunc     func(ctx context.Context, tx repository.Tx, plan *model.SubscriptionPlan) error
	DeleteFunc   func(ctx context.Context, tx repository.Tx, id string) error
	FindByIDFunc func(ctx context.Context, tx repository.Tx, id string) (*model.SubscriptionPlan, error)
	ListAllFunc  func(ctx context.Context, tx repository.Tx) ([]*model.SubscriptionPlan, error)
}

func (m *mockInnerPlanRepo) Save(ctx context.Context, tx repository.Tx, plan *model.SubscriptionPlan) error {
	return m.SaveFunc(ctx, tx, plan)
}
func (m *mockInnerPlanRepo) Delete(ctx context.Context, tx repository.Tx, id string) error {
	return m.DeleteFunc(ctx, tx, id)
}
func (m *mockInnerPlanRepo) FindByID(ctx context.Context, tx repository.Tx, id string) (*model.SubscriptionPlan, error) {
	return m.FindByIDFunc(ctx, tx, id)
}
func (m *mockInnerPlanRepo) ListAll(ctx context.Context, tx repository.Tx) ([]*model.SubscriptionPlan, error) {
	return m.ListAllFunc(ctx, tx)
}

// mockRedisClient mocks our Redis client wrapper.
type mockRedisClient struct {
	GetFunc func(ctx context.Context, key string) (string, error)
	SetFunc func(ctx context.Context, key string, value interface{}, expiration time.Duration) error
	DelFunc func(ctx context.Context, keys ...string) error
}

func NewMockRedisClient() *mockRedisClient {
	return &mockRedisClient{}
}

func (m *mockRedisClient) Get(ctx context.Context, key string) (string, error) {
	return m.GetFunc(ctx, key)
}
func (m *mockRedisClient) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	return m.SetFunc(ctx, key, value, expiration)
}
func (m *mockRedisClient) Del(ctx context.Context, keys ...string) error {
	return m.DelFunc(ctx, keys...)
}
func (m *mockRedisClient) Ping(ctx context.Context) error                      { return nil }
func (m *mockRedisClient) Incr(ctx context.Context, key string) (int64, error) { return 0, nil }
func (m *mockRedisClient) Expire(ctx context.Context, key string, expiration time.Duration) error {
	return nil
}
func (m *mockRedisClient) Close() error { return nil }

// --- End Mocks ---

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

		decorator := postgres.NewPlanRepoCacheDecorator(mockInnerRepo, mockRedis)

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

	t.Run("FindByID should fetch from DB and set cache on miss", func(t *testing.T) {
		// Arrange
		innerRepoCalled := false
		cacheSetCalled := false

		mockRedis := &mockRedisClient{
			GetFunc: func(ctx context.Context, key string) (string, error) {
				return "", redis.Nil // Simulate cache miss
			},
			SetFunc: func(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
				cacheSetCalled = true // We expect this to be called
				return nil
			},
		}
		mockInnerRepo := &mockInnerPlanRepo{
			FindByIDFunc: func(ctx context.Context, tx repository.Tx, id string) (*model.SubscriptionPlan, error) {
				innerRepoCalled = true // We expect this to be called
				return plan, nil
			},
		}

		decorator := postgres.NewPlanRepoCacheDecorator(mockInnerRepo, mockRedis)

		// Act
		result, err := decorator.FindByID(ctx, nil, "plan-123")

		// Assert
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if !innerRepoCalled {
			t.Error("inner repository should be called on a cache miss")
		}
		if !cacheSetCalled {
			t.Error("cache should be set on a cache miss")
		}
		if result == nil || result.ID != "plan-123" {
			t.Error("did not return the correct plan from the inner repository")
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

		decorator := postgres.NewPlanRepoCacheDecorator(mockInnerRepo, mockRedis)

		// Act
		err := decorator.Save(ctx, nil, plan)

		// Assert
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if len(deletedKeys) != 2 {
			t.Fatalf("expected 2 keys to be deleted, but got %d", len(deletedKeys))
		}

		foundItemKey := false
		foundListKey := false
		for _, key := range deletedKeys {
			if key == "plan:plan-123" {
				foundItemKey = true
			}
			if key == "plans:all" {
				foundListKey = true
			}
		}
		if !foundItemKey || !foundListKey {
			t.Error("did not invalidate the correct keys")
		}
	})
}
