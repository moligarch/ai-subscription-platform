package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"telegram-ai-subscription/internal/domain/ports/repository"
)

// Ensure the adapter implements the port interface.
var _ repository.RegistrationStateRepository = (*RegistrationStateRepo)(nil)

// Renamed to RegistrationStateRepo to better reflect its role.
type RegistrationStateRepo struct {
	client *redClient
	ttl    time.Duration
}

func NewRegistrationStateRepo(client *redClient) repository.RegistrationStateRepository {
	return &RegistrationStateRepo{
		client: client,
		ttl:    15 * time.Minute,
	}
}

func (s *RegistrationStateRepo) setStateKey(tgID int64) string {
	return fmt.Sprintf("reg_state:%d", tgID)
}

func (s *RegistrationStateRepo) SetState(ctx context.Context, tgID int64, state *repository.RegistrationState) error {
	key := s.setStateKey(tgID)
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return s.client.Set(ctx, key, data, s.ttl)
}

func (s *RegistrationStateRepo) GetState(ctx context.Context, tgID int64) (*repository.RegistrationState, error) {
	key := s.setStateKey(tgID)
	data, err := s.client.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	var state repository.RegistrationState
	if err := json.Unmarshal([]byte(data), &state); err != nil {
		return nil, err
	}
	return &state, nil
}

func (s *RegistrationStateRepo) ClearState(ctx context.Context, tgID int64) error {
	key := s.setStateKey(tgID)
	return s.client.Del(ctx, key)
}
