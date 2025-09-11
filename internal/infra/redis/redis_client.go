package redis

import (
	"context"
	"time"

	"telegram-ai-subscription/internal/config"

	"github.com/go-redis/redis/v8"
)

type RedisClient interface {
	Ping(ctx context.Context) error
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error
	Get(ctx context.Context, key string) (string, error)
	Incr(ctx context.Context, key string) (int64, error)
	Expire(ctx context.Context, key string, expiration time.Duration) error
	Del(ctx context.Context, keys ...string) error
	Close() error
}

var _ RedisClient = (*redClient)(nil)

type redClient struct {
	cli *redis.Client
}

func NewClient(ctx context.Context, cfg *config.RedisConfig) (*redClient, error) {
	opts := &redis.Options{
		Addr:     cfg.URL,
		Password: cfg.Password,
		DB:       cfg.DB,
	}
	c := redis.NewClient(opts)
	if err := c.Ping(ctx).Err(); err != nil {
		return nil, err
	}
	return &redClient{cli: c}, nil
}

func (c *redClient) Ping(ctx context.Context) error { return c.cli.Ping(ctx).Err() }

func (c *redClient) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	return c.cli.Set(ctx, key, value, expiration).Err()
}

func (c *redClient) Get(ctx context.Context, key string) (string, error) {
	return c.cli.Get(ctx, key).Result()
}

func (c *redClient) Incr(ctx context.Context, key string) (int64, error) {
	return c.cli.Incr(ctx, key).Result()
}

func (c *redClient) Expire(ctx context.Context, key string, expiration time.Duration) error {
	return c.cli.Expire(ctx, key, expiration).Err()
}

func (c *redClient) Del(ctx context.Context, keys ...string) error {
	return c.cli.Del(ctx, keys...).Err()
}

func (c *redClient) Close() error { return c.cli.Close() }
