package redis

import (
	"context"
	"encoding/json"
	"time"

	"telegram-ai-subscription/internal/domain/model"
)

type ChatCache struct {
	client *redClient
	ttl    time.Duration
}

func NewChatCache(client *redClient, ttl time.Duration) *ChatCache {
	return &ChatCache{
		client: client,
		ttl:    ttl,
	}
}

func (c *ChatCache) StoreSession(ctx context.Context, session *model.ChatSession) error {
	key := "chat_session:" + session.ID
	data, err := json.Marshal(session)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, key, data, c.ttl)
}

func (c *ChatCache) GetSession(ctx context.Context, sessionID string) (*model.ChatSession, error) {
	key := "chat_session:" + sessionID
	data, err := c.client.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	var session model.ChatSession
	if err := json.Unmarshal([]byte(data), &session); err != nil {
		return nil, err
	}
	return &session, nil
}

func (c *ChatCache) DeleteSession(ctx context.Context, sessionID string) error {
	key := "chat_session:" + sessionID
	return c.client.cli.Del(ctx, key).Err()
}

func (c *ChatCache) ExtendSession(ctx context.Context, sessionID string) error {
	key := "chat_session:" + sessionID
	return c.client.Expire(ctx, key, c.ttl)
}
