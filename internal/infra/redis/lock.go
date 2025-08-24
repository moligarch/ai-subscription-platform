// File: internal/infra/redis/lock.go
package redis

import (
	"context"
	"telegram-ai-subscription/internal/domain"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
)

type Locker interface {
	TryLock(ctx context.Context, key string, ttl time.Duration) (token string, err error)
	Unlock(ctx context.Context, key, token string) error
}

type RedisLocker struct {
	cli *redis.Client
}

func NewLocker(c *Client) *RedisLocker {
	return &RedisLocker{cli: c.cli}
}

func (l *RedisLocker) TryLock(ctx context.Context, key string, ttl time.Duration) (string, error) {
	token := uuid.NewString()
	for i := 0; i < 5; i++ { // 5 tries
		ok, err := l.cli.SetNX(ctx, key, token, ttl).Result()
		if err != nil {
			continue
		}
		if ok {
			return token, nil
		}
		time.Sleep(50 * time.Millisecond) // wait before retrying
	}
	return "", domain.ErrActiveChatExists
}

var luaUnlock = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("DEL", KEYS[1])
else
	return 0
end`)

func (l *RedisLocker) Unlock(ctx context.Context, key, token string) error {
	_, err := luaUnlock.Run(ctx, l.cli, []string{key}, token).Result()
	return err
}
