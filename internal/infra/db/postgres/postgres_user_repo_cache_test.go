//go:build !integration

package postgres

import (
	"context"
	"sync"
	"testing"
	"time"

	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/repository"

	"github.com/go-redis/redis/v8"
)

func TestUserRepoCacheDecorator(t *testing.T) {
	ctx := context.Background()
	user := &model.User{ID: "user-123", TelegramID: 98765}
	// userJSON, _ := json.Marshal(user)

	t.Run("FindByTelegramID should fetch from DB and set cache on miss", func(t *testing.T) {
		// Arrange
		innerRepoCalled := false
		var cacheSets sync.Map

		mockRedis := &mockRedisClient{
			GetFunc: func(ctx context.Context, key string) (string, error) {
				return "", redis.Nil // Simulate cache miss
			},
			SetFunc: func(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
				cacheSets.Store(key, value) // We expect this to be called twice
				return nil
			},
		}
		mockInnerRepo := &mockInnerUserRepo{
			FindByTelegramIDFunc: func(ctx context.Context, tx repository.Tx, tgID int64) (*model.User, error) {
				innerRepoCalled = true // We expect this to be called
				return user, nil
			},
		}

		decorator := NewUserRepoCacheDecorator(mockInnerRepo, mockRedis)

		// Act
		result, err := decorator.FindByTelegramID(ctx, nil, 98765)

		// Assert
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if !innerRepoCalled {
			t.Error("inner repository should be called on a cache miss")
		}

		// Check that both cache keys were set to warm the cache
		count := 0
		cacheSets.Range(func(key, value interface{}) bool {
			count++
			return true
		})
		if count != 2 {
			t.Errorf("expected 2 cache keys to be set, but got %d", count)
		}

		if result == nil || result.ID != "user-123" {
			t.Error("did not return the correct user from the inner repository")
		}
	})

	t.Run("Save should invalidate both cache keys", func(t *testing.T) {
		// Arrange
		var deletedKeys sync.Map
		mockRedis := &mockRedisClient{
			DelFunc: func(ctx context.Context, keys ...string) error {
				for _, k := range keys {
					deletedKeys.Store(k, true)
				}
				return nil
			},
		}
		mockInnerRepo := &mockInnerUserRepo{
			SaveFunc: func(ctx context.Context, tx repository.Tx, u *model.User) error {
				return nil
			},
		}

		decorator := NewUserRepoCacheDecorator(mockInnerRepo, mockRedis)

		// Act
		err := decorator.Save(ctx, nil, user)

		// Assert
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		count := 0
		deletedKeys.Range(func(key, value interface{}) bool {
			count++
			return true
		})
		if count != 2 {
			t.Fatalf("expected 2 keys to be deleted, but got %d", count)
		}

		if _, ok := deletedKeys.Load("user:id:user-123"); !ok {
			t.Error("did not invalidate cache by user ID")
		}
		if _, ok := deletedKeys.Load("user:tgid:98765"); !ok {
			t.Error("did not invalidate cache by telegram ID")
		}
	})
}
