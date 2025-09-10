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

var _ repository.SubscriptionPlanRepository = (*planRepoCacheDecorator)(nil)

type planRepoCacheDecorator struct {
	inner repository.SubscriptionPlanRepository
	cache red.RedisClient
	ttl   time.Duration
}

func NewPlanRepoCacheDecorator(inner repository.SubscriptionPlanRepository, cache red.RedisClient) repository.SubscriptionPlanRepository {
	return &planRepoCacheDecorator{
		inner: inner,
		cache: cache,
		ttl:   1 * time.Hour,
	}
}

func (d *planRepoCacheDecorator) FindByID(ctx context.Context, tx repository.Tx, id string) (*model.SubscriptionPlan, error) {
	key := fmt.Sprintf("plan:%s", id)
	val, err := d.cache.Get(ctx, key)
	if err == nil {
		metrics.IncCacheRequest("plan", "hit")
		var plan model.SubscriptionPlan
		if json.Unmarshal([]byte(val), &plan) == nil {
			return &plan, nil
		}
	}
	if err != redis.Nil {
		// Log a real Redis error
	}

	metrics.IncCacheRequest("plan", "miss")
	plan, err := d.inner.FindByID(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	if plan != nil {
		bytes, _ := json.Marshal(plan)
		d.cache.Set(ctx, key, bytes, d.ttl)
	}
	return plan, nil
}

// For write operations, we must invalidate the cache.
func (d *planRepoCacheDecorator) Save(ctx context.Context, tx repository.Tx, plan *model.SubscriptionPlan) error {
	// Invalidate the cache for this specific plan
	key := fmt.Sprintf("plan:%s", plan.ID)
	d.cache.Del(ctx, key)
	// Also invalidate the cache for the list of all plans
	d.cache.Del(ctx, "plans:all")
	return d.inner.Save(ctx, tx, plan)
}

func (d *planRepoCacheDecorator) Delete(ctx context.Context, tx repository.Tx, id string) error {
	key := fmt.Sprintf("plan:%s", id)
	d.cache.Del(ctx, key)
	d.cache.Del(ctx, "plans:all")
	return d.inner.Delete(ctx, tx, id)
}

// Also cache the full list of plans
func (d *planRepoCacheDecorator) ListAll(ctx context.Context, tx repository.Tx) ([]*model.SubscriptionPlan, error) {
	key := "plans:all"
	val, err := d.cache.Get(ctx, key)
	if err == nil {
		metrics.IncCacheRequest("plan_list", "hit")
		var plans []*model.SubscriptionPlan
		if json.Unmarshal([]byte(val), &plans) == nil {
			return plans, nil
		}
	}

	metrics.IncCacheRequest("plan_list", "miss")
	plans, err := d.inner.ListAll(ctx, tx)
	if err != nil {
		return nil, err
	}
	if len(plans) > 0 {
		bytes, _ := json.Marshal(plans)
		d.cache.Set(ctx, key, bytes, d.ttl)
	}
	return plans, nil
}
