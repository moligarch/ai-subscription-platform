package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/repository"
	"telegram-ai-subscription/internal/infra/metrics"
	red "telegram-ai-subscription/internal/infra/redis"
	"time"

	"github.com/go-redis/redis/v8"
)

var _ repository.UserRepository = (*userRepoCacheDecorator)(nil)

type userRepoCacheDecorator struct {
	inner repository.UserRepository
	cache red.RedisClient
	ttl   time.Duration
}

func NewUserRepoCacheDecorator(inner repository.UserRepository, cache red.RedisClient) repository.UserRepository {
	return &userRepoCacheDecorator{
		inner: inner,
		cache: cache,
		ttl:   1 * time.Hour,
	}
}

// For write operations, we must invalidate all possible keys for that user.
func (d *userRepoCacheDecorator) Save(ctx context.Context, tx repository.Tx, u *model.User) error {
	// Invalidate cache entries by both ID and Telegram ID
	_ = d.cache.Del(ctx, fmt.Sprintf("user:id:%s", u.ID))
	_ = d.cache.Del(ctx, fmt.Sprintf("user:tgid:%d", u.TelegramID))
	return d.inner.Save(ctx, tx, u)
}

func (d *userRepoCacheDecorator) FindByTelegramID(ctx context.Context, tx repository.Tx, tgID int64) (*model.User, error) {
	key := fmt.Sprintf("user:tgid:%d", tgID)
	val, err := d.cache.Get(ctx, key)
	if err == nil {
		metrics.IncCacheRequest("user", "hit")
		var user model.User
		if json.Unmarshal([]byte(val), &user) == nil {
			return &user, nil
		}
	}
	if err != redis.Nil {
		// Log a real Redis error
	}

	metrics.IncCacheRequest("user", "miss")
	user, err := d.inner.FindByTelegramID(ctx, tx, tgID)
	if err != nil {
		return nil, err
	}
	if user != nil {
		bytes, _ := json.Marshal(user)
		// Set the cache for both keys to warm the cache for FindByID calls
		_ = d.cache.Set(ctx, key, bytes, d.ttl)
		_ = d.cache.Set(ctx, fmt.Sprintf("user:id:%s", user.ID), bytes, d.ttl)
	}
	return user, nil
}

func (d *userRepoCacheDecorator) FindByID(ctx context.Context, tx repository.Tx, id string) (*model.User, error) {
	key := fmt.Sprintf("user:id:%s", id)
	val, err := d.cache.Get(ctx, key)
	if err == nil {
		metrics.IncCacheRequest("user", "hit")
		var user model.User
		if json.Unmarshal([]byte(val), &user) == nil {
			return &user, nil
		}
	}

	metrics.IncCacheRequest("user", "miss")
	user, err := d.inner.FindByID(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	if user != nil {
		bytes, _ := json.Marshal(user)
		_ = d.cache.Set(ctx, key, bytes, d.ttl)
		_ = d.cache.Set(ctx, fmt.Sprintf("user:tgid:%d", user.TelegramID), bytes, d.ttl)
	}
	return user, nil
}

// Pass-through methods that don't need caching
func (d *userRepoCacheDecorator) CountUsers(ctx context.Context, tx repository.Tx) (int, error) {
	return d.inner.CountUsers(ctx, tx)
}

func (d *userRepoCacheDecorator) CountInactiveUsers(ctx context.Context, tx repository.Tx, since time.Time) (int, error) {
	return d.inner.CountInactiveUsers(ctx, tx, since)
}

func (d *userRepoCacheDecorator) List(ctx context.Context, tx repository.Tx, offset, limit int) ([]*model.User, error) {
	// Bypass the cache if we are fetching all users.
	if limit == 0 {
		metrics.IncCacheRequest("user_list", "bypass")
		return d.inner.List(ctx, tx, offset, limit)
	}

	// Caching for paginated lists can be complex; for now, I'll keep it as a simple pass-through.
	// This logic can be expanded later if list caching is needed.
	return d.inner.List(ctx, tx, offset, limit)
}
