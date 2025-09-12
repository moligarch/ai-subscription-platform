package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"telegram-ai-subscription/internal/domain/ports/repository"
)

var _ repository.StateRepository = (*StateRepo)(nil)

// StateRepo manages user conversational state in Redis.
type StateRepo struct {
	client *redClient
	ttl    time.Duration
}

func NewStateRepo(client *redClient) repository.StateRepository {
	return &StateRepo{
		client: client,
		ttl:    15 * time.Minute, // Give users 15 minutes to complete any conversational flow.
	}
}

func (s *StateRepo) stateKey(tgID int64) string {
	return fmt.Sprintf("conv_state:%d", tgID)
}

func (s *StateRepo) SetState(ctx context.Context, tgID int64, state *repository.ConversationState) error {
	key := s.stateKey(tgID)
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return s.client.Set(ctx, key, data, s.ttl)
}

func (s *StateRepo) GetState(ctx context.Context, tgID int64) (*repository.ConversationState, error) {
	key := s.stateKey(tgID)
	data, err := s.client.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	var state repository.ConversationState
	if err := json.Unmarshal([]byte(data), &state); err != nil {
		return nil, err
	}
	return &state, nil
}

func (s *StateRepo) ClearState(ctx context.Context, tgID int64) error {
	key := s.stateKey(tgID)
	return s.client.Del(ctx, key)
}
