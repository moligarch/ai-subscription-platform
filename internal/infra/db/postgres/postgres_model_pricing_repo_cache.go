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

var _ repository.ModelPricingRepository = (*modelPricingRepoCacheDecorator)(nil)

type modelPricingRepoCacheDecorator struct {
	inner repository.ModelPricingRepository
	cache red.RedisClient
	ttl   time.Duration
}

func NewModelPricingRepoCacheDecorator(inner repository.ModelPricingRepository, cache red.RedisClient) repository.ModelPricingRepository {
	return &modelPricingRepoCacheDecorator{
		inner: inner,
		cache: cache,
		ttl:   1 * time.Hour,
	}
}

func (d *modelPricingRepoCacheDecorator) GetByModelName(ctx context.Context, tx repository.Tx, modelName string) (*model.ModelPricing, error) {
	key := fmt.Sprintf("model_pricing:%s", modelName)
	val, err := d.cache.Get(ctx, key)
	if err == nil {
		metrics.IncCacheRequest("model_pricing", "hit")
		var p model.ModelPricing
		if json.Unmarshal([]byte(val), &p) == nil {
			return &p, nil
		}
	}
	if err != redis.Nil {
		// Log a real Redis error
	}

	metrics.IncCacheRequest("model_pricing", "miss")
	p, err := d.inner.GetByModelName(ctx, tx, modelName)
	if err != nil {
		return nil, err
	}
	if p != nil {
		bytes, _ := json.Marshal(p)
		_ = d.cache.Set(ctx, key, bytes, d.ttl)
	}
	return p, nil
}

// Write operations must invalidate the cache
func (d *modelPricingRepoCacheDecorator) Create(ctx context.Context, tx repository.Tx, p *model.ModelPricing) error {
	_ = d.cache.Del(ctx, "model_pricing:all_active") // Invalidate the list cache
	return d.inner.Create(ctx, tx, p)
}

func (d *modelPricingRepoCacheDecorator) Update(ctx context.Context, tx repository.Tx, p *model.ModelPricing) error {
	_ = d.cache.Del(ctx, fmt.Sprintf("model_pricing:%s", p.ModelName)) // Invalidate the item cache
	_ = d.cache.Del(ctx, "model_pricing:all_active")                   // Invalidate the list cache
	return d.inner.Update(ctx, tx, p)
}

func (d *modelPricingRepoCacheDecorator) ListActive(ctx context.Context, tx repository.Tx) ([]*model.ModelPricing, error) {
	key := "model_pricing:all_active"
	val, err := d.cache.Get(ctx, key)
	if err == nil {
		metrics.IncCacheRequest("model_pricing_list", "hit")
		var prices []*model.ModelPricing
		if json.Unmarshal([]byte(val), &prices) == nil {
			return prices, nil
		}
	}

	metrics.IncCacheRequest("model_pricing_list", "miss")
	prices, err := d.inner.ListActive(ctx, tx)
	if err != nil {
		return nil, err
	}
	if len(prices) > 0 {
		bytes, _ := json.Marshal(prices)
		_ = d.cache.Set(ctx, key, bytes, d.ttl)
	}
	return prices, nil
}
