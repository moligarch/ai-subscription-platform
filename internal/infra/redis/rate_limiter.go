package redis

import (
	"context"
	"fmt"
	"time"
)

type RateLimiter struct {
	client *Client
}

func NewRateLimiter(client *Client) *RateLimiter {
	return &RateLimiter{client: client}
}

func (r *RateLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	count, err := r.client.Incr(ctx, key)
	if err != nil {
		return false, err
	}

	if count == 1 {
		err = r.client.Expire(ctx, key, window)
		if err != nil {
			return false, err
		}
	}

	if count > int64(limit) {
		return false, nil
	}

	return true, nil
}

func UserCommandKey(userID int64, command string) string {
	return fmt.Sprintf("rate_limit:%d:%s", userID, command)
}
