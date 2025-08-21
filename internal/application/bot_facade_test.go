package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"telegram-ai-subscription/internal/application"
	"telegram-ai-subscription/internal/domain/model"
)

// simple mock plan usecase implementing the methods used by BotFacade
type mockPlanUC struct {
	created *model.SubscriptionPlan
	updated *model.SubscriptionPlan
	deleted string

	// control return values
	createErr error
	updateErr error
	deleteErr error
	getErr    error
	getFunc   func(id string) (*model.SubscriptionPlan, error)
	list      []*model.SubscriptionPlan
}

func (m *mockPlanUC) Create(ctx context.Context, p *model.SubscriptionPlan) error {
	if m.createErr != nil {
		return m.createErr
	}
	// record created
	cp := *p
	m.created = &cp
	return nil
}

func (m *mockPlanUC) Get(ctx context.Context, id string) (*model.SubscriptionPlan, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	if m.getFunc != nil {
		return m.getFunc(id)
	}
	// default: return a dummy plan if ids match created/updated
	if m.created != nil && m.created.ID == id {
		cp := *m.created
		return &cp, nil
	}
	if m.updated != nil && m.updated.ID == id {
		cp := *m.updated
		return &cp, nil
	}
	return nil, errors.New("not found")
}

func (m *mockPlanUC) List(ctx context.Context) ([]*model.SubscriptionPlan, error) {
	return m.list, nil
}

func (m *mockPlanUC) Update(ctx context.Context, p *model.SubscriptionPlan) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	cp := *p
	m.updated = &cp
	return nil
}

func (m *mockPlanUC) Delete(ctx context.Context, id string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	m.deleted = id
	return nil
}

func TestHandleCreateUpdateDeletePlan(t *testing.T) {
	ctx := context.Background()
	mock := &mockPlanUC{}
	// create facade with only PlanUC set (others nil are fine for these tests)
	f := application.NewBotFacade(nil, mock, nil, nil, nil, nil)

	// --- Create ---
	msg, err := f.HandleCreatePlan(ctx, "TestPlan", 7, 20)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if mock.created == nil {
		t.Fatalf("expected plan created recorded")
	}
	if mock.created.Name != "TestPlan" || mock.created.DurationDays != 7 || mock.created.Credits != 20 {
		t.Fatalf("created plan mismatch: %+v", mock.created)
	}
	if msg == "" {
		t.Fatalf("expected non-empty create message")
	}

	// Prepare existing plan for update/delete by ensuring mock.Get will find it
	mock.getFunc = func(id string) (*model.SubscriptionPlan, error) {
		// pretend to find a plan with this id
		return &model.SubscriptionPlan{
			ID:           id,
			Name:         "Original",
			DurationDays: 10,
			Credits:      50,
			CreatedAt:    time.Now(),
		}, nil
	}

	// --- Update ---
	uMsg, err := f.HandleUpdatePlan(ctx, "plan-1", "Updated", 15, 99)
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if mock.updated == nil {
		t.Fatalf("expected plan updated recorded")
	}
	if mock.updated.Name != "Updated" || mock.updated.DurationDays != 15 || mock.updated.Credits != 99 {
		t.Fatalf("updated plan mismatch: %+v", mock.updated)
	}
	if uMsg == "" {
		t.Fatalf("expected non-empty update message")
	}

	// --- Delete ---
	dMsg, err := f.HandleDeletePlan(ctx, "plan-1")
	if err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	if mock.deleted != "plan-1" {
		t.Fatalf("expected deleted id recorded 'plan-1', got %q", mock.deleted)
	}
	if dMsg == "" {
		t.Fatalf("expected non-empty delete message")
	}
}
