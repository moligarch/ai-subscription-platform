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

func NewClient(ctx context.Context, cfg *config.RedisConfig) (*Client, error) {
	opts := &redis.Options{
		Addr:     cfg.URL,
		Password: cfg.Password,
		DB:       cfg.DB,
	}
	c := redis.NewClient(opts)
	if err := c.Ping(ctx).Err(); err != nil {
		return nil, err
	}
	return &Client{cli: c}, nil
}

func (c *Client) Ping(ctx context.Context) error { return c.cli.Ping(ctx).Err() }

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

func (c *Client) Del(ctx context.Context, keys ...string) error { return c.cli.Del(ctx, keys...).Err() }

func (c *Client) Close() error { return c.cli.Close() }
