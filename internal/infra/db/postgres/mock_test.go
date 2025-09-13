//go:build !integration

package postgres

import (
	"context"
	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/repository"
	red "telegram-ai-subscription/internal/infra/redis"
	"time"
)

// --- Mocks for Cache Decorator Tests ---

// mockInnerPlanRepo mocks the database repository that the Plan decorator wraps.
type mockInnerPlanRepo struct {
	SaveFunc     func(ctx context.Context, tx repository.Tx, plan *model.SubscriptionPlan) error
	DeleteFunc   func(ctx context.Context, tx repository.Tx, id string) error
	FindByIDFunc func(ctx context.Context, tx repository.Tx, id string) (*model.SubscriptionPlan, error)
	ListAllFunc  func(ctx context.Context, tx repository.Tx) ([]*model.SubscriptionPlan, error)
}

func (m *mockInnerPlanRepo) Save(ctx context.Context, tx repository.Tx, plan *model.SubscriptionPlan) error {
	return m.SaveFunc(ctx, tx, plan)
}
func (m *mockInnerPlanRepo) Delete(ctx context.Context, tx repository.Tx, id string) error {
	return m.DeleteFunc(ctx, tx, id)
}
func (m *mockInnerPlanRepo) FindByID(ctx context.Context, tx repository.Tx, id string) (*model.SubscriptionPlan, error) {
	return m.FindByIDFunc(ctx, tx, id)
}
func (m *mockInnerPlanRepo) ListAll(ctx context.Context, tx repository.Tx) ([]*model.SubscriptionPlan, error) {
	return m.ListAllFunc(ctx, tx)
}

// mockInnerUserRepo mocks the database repository that the User decorator wraps.
type mockInnerUserRepo struct {
	SaveFunc               func(ctx context.Context, tx repository.Tx, u *model.User) error
	FindByTelegramIDFunc   func(ctx context.Context, tx repository.Tx, tgID int64) (*model.User, error)
	FindByIDFunc           func(ctx context.Context, tx repository.Tx, id string) (*model.User, error)
	CountUsersFunc         func(ctx context.Context, tx repository.Tx) (int, error)
	CountInactiveUsersFunc func(ctx context.Context, tx repository.Tx, since time.Time) (int, error)
	ListFunc               func(ctx context.Context, tx repository.Tx, offset, limit int) ([]*model.User, error)
}

func (m *mockInnerUserRepo) Save(ctx context.Context, tx repository.Tx, u *model.User) error {
	return m.SaveFunc(ctx, tx, u)
}
func (m *mockInnerUserRepo) FindByTelegramID(ctx context.Context, tx repository.Tx, tgID int64) (*model.User, error) {
	return m.FindByTelegramIDFunc(ctx, tx, tgID)
}
func (m *mockInnerUserRepo) FindByID(ctx context.Context, tx repository.Tx, id string) (*model.User, error) {
	return m.FindByIDFunc(ctx, tx, id)
}
func (m *mockInnerUserRepo) CountUsers(ctx context.Context, tx repository.Tx) (int, error) {
	return m.CountUsersFunc(ctx, tx)
}
func (m *mockInnerUserRepo) CountInactiveUsers(ctx context.Context, tx repository.Tx, since time.Time) (int, error) {
	return m.CountInactiveUsersFunc(ctx, tx, since)
}
func (m *mockInnerUserRepo) List(ctx context.Context, tx repository.Tx, offset, limit int) ([]*model.User, error) {
	return m.ListFunc(ctx, tx, offset, limit)
}

// mockRedisClient mocks our Redis client wrapper.
type mockRedisClient struct {
	GetFunc    func(ctx context.Context, key string) (string, error)
	SetFunc    func(ctx context.Context, key string, value interface{}, expiration time.Duration) error
	DelFunc    func(ctx context.Context, keys ...string) error
	PingFunc   func(ctx context.Context) error
	IncrFunc   func(ctx context.Context, key string) (int64, error)
	ExpireFunc func(ctx context.Context, key string, expiration time.Duration) error
	CloseFunc  func() error
}

var _ red.RedisClient = &mockRedisClient{}

func (m *mockRedisClient) Get(ctx context.Context, key string) (string, error) {
	return m.GetFunc(ctx, key)
}
func (m *mockRedisClient) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	return m.SetFunc(ctx, key, value, expiration)
}
func (m *mockRedisClient) Del(ctx context.Context, keys ...string) error {
	return m.DelFunc(ctx, keys...)
}
func (m *mockRedisClient) Ping(ctx context.Context) error { return m.PingFunc(ctx) }
func (m *mockRedisClient) Incr(ctx context.Context, key string) (int64, error) {
	return m.IncrFunc(ctx, key)
}
func (m *mockRedisClient) Expire(ctx context.Context, key string, expiration time.Duration) error {
	return m.ExpireFunc(ctx, key, expiration)
}
func (m *mockRedisClient) FlushDB(ctx context.Context) error { return nil }
func (m *mockRedisClient) Close() error                      { return m.CloseFunc() }
