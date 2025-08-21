package redis

import (
	"context"
	"time"

	"telegram-ai-subscription/internal/config"

	"github.com/go-redis/redis/v8"
)

type Client struct {
	cli *redis.Client
}

func NewRedisClient(cfg *config.RedisConfig) *Client {
	opt := &redis.Options{
		Addr:     cfg.URL,
		Password: cfg.Password,
		DB:       cfg.DB,
	}
	cli := redis.NewClient(opt)
	return &Client{cli: cli}
}

func (c *Client) Ping(ctx context.Context) error {
	return c.cli.Ping(ctx).Err()
}

func (c *Client) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	return c.cli.Set(ctx, key, value, expiration).Err()
}

func (c *Client) Get(ctx context.Context, key string) (string, error) {
	return c.cli.Get(ctx, key).Result()
}

func (c *Client) Incr(ctx context.Context, key string) (int64, error) {
	return c.cli.Incr(ctx, key).Result()
}

func (c *Client) Expire(ctx context.Context, key string, expiration time.Duration) error {
	return c.cli.Expire(ctx, key, expiration).Err()
}

func (c *Client) Close() error {
	return c.cli.Close()
}
