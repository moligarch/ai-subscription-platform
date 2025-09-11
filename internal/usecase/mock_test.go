//go:build !integration

package usecase_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"testing/fstest"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v4"
	"github.com/rs/zerolog"

	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/adapter"
	"telegram-ai-subscription/internal/domain/ports/repository"
	"telegram-ai-subscription/internal/infra/i18n"
)

// -----------------------------
// Utilities: tiny helpers
// -----------------------------

func now() time.Time { return time.Now().Truncate(time.Millisecond) }

func cloneMessages(ms []*model.ChatMessage) []model.ChatMessage {
	out := make([]model.ChatMessage, len(ms))
	for i, m := range ms {
		out[i] = *m
	}
	return out
}

// =============================
// Adapters
// =============================

// ---- Mock TelegramBotAdapter ----

// MockTelegramBot now implements the new SendMessage(SendMessageParams) interface.
type MockTelegramBot struct {
	mu   sync.Mutex
	Sent []adapter.SendMessageParams // Capture all sent message parameters

	SendMessageFunc     func(ctx context.Context, params adapter.SendMessageParams) error
	SetMenuCommandsFunc func(ctx context.Context, chatID int64, isAdmin bool) error
}

var _ adapter.TelegramBotAdapter = (*MockTelegramBot)(nil)

func (m *MockTelegramBot) SendMessage(ctx context.Context, params adapter.SendMessageParams) error {
	if m.SendMessageFunc != nil {
		return m.SendMessageFunc(ctx, params)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Sent = append(m.Sent, params)
	return nil
}

func (m *MockTelegramBot) SetMenuCommands(ctx context.Context, chatID int64, isAdmin bool) error {
	if m.SetMenuCommandsFunc != nil {
		return m.SetMenuCommandsFunc(ctx, chatID, isAdmin)
	}
	return nil
}

// ---- Mock AIServiceAdapter ----

type MockAI struct {
	mu sync.Mutex

	// configurable behavior
	ListModelsFunc    func(ctx context.Context) ([]string, error)
	GetModelInfoFunc  func(modelName string) (adapter.ModelInfo, error)
	CountTokensFunc   func(ctx context.Context, model string, msgs []adapter.Message) (int, error)
	ChatFunc          func(ctx context.Context, model string, msgs []adapter.Message) (string, error)
	ChatWithUsageFunc func(ctx context.Context, model string, msgs []adapter.Message) (string, adapter.Usage, error)

	// tracing of invocations
	Calls struct {
		ListModels int
		ModelInfo  []string
		Count      []struct {
			Model string
			N     int
		}
		Chat []string
	}
}

var _ adapter.AIServiceAdapter = (*MockAI)(nil)

func (m *MockAI) ListModels(ctx context.Context) ([]string, error) {
	m.mu.Lock()
	m.Calls.ListModels++
	m.mu.Unlock()
	if m.ListModelsFunc != nil {
		return m.ListModelsFunc(ctx)
	}
	return []string{"gpt-4o-mini"}, nil
}

func (m *MockAI) GetModelInfo(model string) (adapter.ModelInfo, error) {
	m.mu.Lock()
	m.Calls.ModelInfo = append(m.Calls.ModelInfo, model)
	m.mu.Unlock()
	if m.GetModelInfoFunc != nil {
		return m.GetModelInfoFunc(model)
	}
	return adapter.ModelInfo{Name: model, MaxTokens: 0}, nil
}

func (m *MockAI) CountTokens(ctx context.Context, model string, msgs []adapter.Message) (int, error) {
	if m.CountTokensFunc != nil {
		return m.CountTokensFunc(ctx, model, msgs)
	}
	n := 0
	for _, x := range msgs {
		n += len(x.Content)
	} // dumb baseline
	m.mu.Lock()
	m.Calls.Count = append(m.Calls.Count, struct {
		Model string
		N     int
	}{model, n})
	m.mu.Unlock()
	return n, nil
}

func (m *MockAI) Chat(ctx context.Context, model string, msgs []adapter.Message) (string, error) {
	if m.ChatFunc != nil {
		return m.ChatFunc(ctx, model, msgs)
	}
	return "ok", nil
}

func (m *MockAI) ChatWithUsage(ctx context.Context, model string, msgs []adapter.Message) (string, adapter.Usage, error) {
	if m.ChatWithUsageFunc != nil {
		return m.ChatWithUsageFunc(ctx, model, msgs)
	}
	return "ok", adapter.Usage{TotalTokens: 1, PromptTokens: 1, CompletionTokens: 0}, nil
}

// ---- Mock PaymentGateway (adapter) ----

type MockPaymentGateway struct {
	NameVal string

	RequestPaymentFunc func(ctx context.Context, amount int64, description, callbackURL string, meta map[string]interface{}) (authority, payURL string, err error)
	VerifyPaymentFunc  func(ctx context.Context, authority string, expectedAmount int64) (refID string, err error)
	RefundPaymentFunc  func(ctx context.Context, sessionID string, amount int64, description string, method adapter.RefundMethod, reason adapter.RefundReason) (adapter.RefundResult, error)
}

var _ adapter.PaymentGateway = (*MockPaymentGateway)(nil)

func (m *MockPaymentGateway) Name() string {
	if m.NameVal == "" {
		return "mockpay"
	}
	return m.NameVal
}

func (m *MockPaymentGateway) RequestPayment(ctx context.Context, amount int64, description, callbackURL string, meta map[string]interface{}) (string, string, error) {
	if m.RequestPaymentFunc != nil {
		return m.RequestPaymentFunc(ctx, amount, description, callbackURL, meta)
	}
	auth := "AUTH-" + uuid.NewString()
	return auth, "https://pay.example/" + auth, nil
}

func (m *MockPaymentGateway) VerifyPayment(ctx context.Context, authority string, expectedAmount int64) (string, error) {
	if m.VerifyPaymentFunc != nil {
		return m.VerifyPaymentFunc(ctx, authority, expectedAmount)
	}
	return "REF-" + authority, nil
}

func (m *MockPaymentGateway) RefundPayment(ctx context.Context, sessionID string, amount int64, description string, method adapter.RefundMethod, reason adapter.RefundReason) (adapter.RefundResult, error) {
	if m.RefundPaymentFunc != nil {
		return m.RefundPaymentFunc(ctx, sessionID, amount, description, method, reason)
	}
	return adapter.RefundResult{ID: "R-" + sessionID, Status: "DONE", RefundAmount: amount, RefundTime: now()}, nil
}

// =============================
// Repositories
// =============================

// ---- Mock UserRepository ----

type MockUserRepo struct {
	mu   sync.Mutex
	byID map[string]*model.User
	byTG map[int64]*model.User

	SaveFunc               func(ctx context.Context, tx repository.Tx, u *model.User) error
	FindByTelegramIDFunc   func(ctx context.Context, tx repository.Tx, tgID int64) (*model.User, error)
	FindByIDFunc           func(ctx context.Context, tx repository.Tx, id string) (*model.User, error)
	CountUsersFunc         func(ctx context.Context, tx repository.Tx) (int, error)
	CountInactiveUsersFunc func(ctx context.Context, tx repository.Tx, olderThan time.Time) (int, error)
}

var _ repository.UserRepository = (*MockUserRepo)(nil)

func NewMockUserRepo() *MockUserRepo {
	return &MockUserRepo{byID: map[string]*model.User{}, byTG: map[int64]*model.User{}}
}

func (r *MockUserRepo) Save(ctx context.Context, tx repository.Tx, u *model.User) error {
	if r.SaveFunc != nil {
		return r.SaveFunc(ctx, tx, u)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *u
	if cp.ID == "" {
		cp.ID = uuid.NewString()
	}

	r.byID[cp.ID] = &cp
	r.byTG[cp.TelegramID] = &cp
	return nil
}

func (r *MockUserRepo) FindByTelegramID(ctx context.Context, tx repository.Tx, tgID int64) (*model.User, error) {
	if r.FindByTelegramIDFunc != nil {
		return r.FindByTelegramIDFunc(ctx, tx, tgID)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if u, ok := r.byTG[tgID]; ok {
		cp := *u
		return &cp, nil
	}
	return nil, nil
}

func (r *MockUserRepo) FindByID(ctx context.Context, tx repository.Tx, id string) (*model.User, error) {
	if r.FindByIDFunc != nil {
		return r.FindByIDFunc(ctx, tx, id)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if u, ok := r.byID[id]; ok {
		cp := *u
		return &cp, nil
	}
	return nil, nil
}

func (r *MockUserRepo) CountUsers(ctx context.Context, tx repository.Tx) (int, error) {
	if r.CountUsersFunc != nil {
		return r.CountUsersFunc(ctx, tx)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.byID), nil
}

func (r *MockUserRepo) CountInactiveUsers(ctx context.Context, tx repository.Tx, olderThan time.Time) (int, error) {
	if r.CountInactiveUsersFunc != nil {
		return r.CountInactiveUsersFunc(ctx, tx, olderThan)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, u := range r.byID {
		if u.LastActiveAt.Before(olderThan) {
			n++
		}
	}
	return n, nil
}

// ---- Mock SubscriptionPlanRepository ----

type MockPlanRepo struct {
	mu   sync.Mutex
	data map[string]*model.SubscriptionPlan

	SaveFunc     func(ctx context.Context, p *model.SubscriptionPlan) error
	FindByIDFunc func(ctx context.Context, id string) (*model.SubscriptionPlan, error)
	ListAllFunc  func(ctx context.Context) ([]*model.SubscriptionPlan, error)
	DeleteFunc   func(ctx context.Context, id string) error
}

var _ repository.SubscriptionPlanRepository = (*MockPlanRepo)(nil)

func NewMockPlanRepo() *MockPlanRepo {
	return &MockPlanRepo{data: map[string]*model.SubscriptionPlan{}}
}

func (r *MockPlanRepo) Save(ctx context.Context, tx repository.Tx, p *model.SubscriptionPlan) error {
	if r.SaveFunc != nil {
		return r.SaveFunc(ctx, p)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	cp := *p
	r.data[p.ID] = &cp
	return nil
}

func (r *MockPlanRepo) FindByID(ctx context.Context, tx repository.Tx, id string) (*model.SubscriptionPlan, error) {
	if r.FindByIDFunc != nil {
		return r.FindByIDFunc(ctx, id)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if p, ok := r.data[id]; ok {
		cp := *p
		return &cp, nil
	}
	return nil, nil
}

func (r *MockPlanRepo) ListAll(ctx context.Context, tx repository.Tx) ([]*model.SubscriptionPlan, error) {
	if r.ListAllFunc != nil {
		return r.ListAllFunc(ctx)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*model.SubscriptionPlan, 0, len(r.data))
	for _, p := range r.data {
		cp := *p
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PriceIRR < out[j].PriceIRR })
	return out, nil
}

func (r *MockPlanRepo) Delete(ctx context.Context, tx repository.Tx, id string) error {
	if r.DeleteFunc != nil {
		return r.DeleteFunc(ctx, id)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.data, id)
	return nil
}

// ---- Mock SubscriptionRepository ----

type MockSubscriptionRepo struct {
	mu   sync.Mutex
	data map[string]*model.UserSubscription // by id

	SaveFunc                    func(ctx context.Context, tx repository.Tx, s *model.UserSubscription) error
	FindActiveByUserAndPlanFunc func(ctx context.Context, tx repository.Tx, userID, planID string) (*model.UserSubscription, error)
	FindActiveByUserFunc        func(ctx context.Context, tx repository.Tx, userID string) (*model.UserSubscription, error)
	FindReservedByUserFunc      func(ctx context.Context, tx repository.Tx, userID string) ([]*model.UserSubscription, error)
	FindByIDFunc                func(ctx context.Context, tx repository.Tx, id string) (*model.UserSubscription, error)
	FindExpiringFunc            func(ctx context.Context, tx repository.Tx, within int) ([]*model.UserSubscription, error)
	CountActiveByPlanFunc       func(ctx context.Context, tx repository.Tx) (map[string]int, error)
	TotalRemainingCreditsFunc   func(ctx context.Context, tx repository.Tx) (int64, error)
	UpdateRemainingCreditsFunc  func(ctx context.Context, tx repository.Tx, id string, delta int64) error
	UpdateStatusFunc            func(ctx context.Context, tx repository.Tx, id string, status model.SubscriptionStatus) error
	CountByStatusFunc           func(ctx context.Context, tx repository.Tx) (map[model.SubscriptionStatus]int, error)
}

var _ repository.SubscriptionRepository = (*MockSubscriptionRepo)(nil)

func NewMockSubscriptionRepo() *MockSubscriptionRepo {
	return &MockSubscriptionRepo{data: map[string]*model.UserSubscription{}}
}

func (r *MockSubscriptionRepo) Save(ctx context.Context, tx repository.Tx, s *model.UserSubscription) error {
	if r.SaveFunc != nil {
		return r.SaveFunc(ctx, tx, s)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if s.ID == "" {
		s.ID = uuid.NewString()
	}
	cp := *s
	r.data[s.ID] = &cp
	return nil
}

func (r *MockSubscriptionRepo) FindActiveByUserAndPlan(ctx context.Context, tx repository.Tx, userID, planID string) (*model.UserSubscription, error) {
	if r.FindActiveByUserAndPlanFunc != nil {
		return r.FindActiveByUserAndPlanFunc(ctx, tx, userID, planID)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, s := range r.data {
		if s.UserID == userID && s.PlanID == planID && s.Status == model.SubscriptionStatusActive {
			cp := *s
			return &cp, nil
		}
	}
	return nil, nil
}

func (r *MockSubscriptionRepo) FindActiveByUser(ctx context.Context, tx repository.Tx, userID string) (*model.UserSubscription, error) {
	if r.FindActiveByUserFunc != nil {
		return r.FindActiveByUserFunc(ctx, tx, userID)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, s := range r.data {
		if s.UserID == userID && s.Status == model.SubscriptionStatusActive {
			cp := *s
			return &cp, nil
		}
	}
	return nil, nil
}

func (r *MockSubscriptionRepo) FindReservedByUser(ctx context.Context, tx repository.Tx, userID string) ([]*model.UserSubscription, error) {
	if r.FindReservedByUserFunc != nil {
		return r.FindReservedByUserFunc(ctx, tx, userID)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*model.UserSubscription
	for _, s := range r.data {
		if s.UserID == userID && s.Status == model.SubscriptionStatusReserved {
			cp := *s
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (r *MockSubscriptionRepo) FindByID(ctx context.Context, tx repository.Tx, id string) (*model.UserSubscription, error) {
	if r.FindByIDFunc != nil {
		return r.FindByIDFunc(ctx, tx, id)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if s, ok := r.data[id]; ok {
		cp := *s
		return &cp, nil
	}
	return nil, nil
}

// Replace the existing FindExpiring with this
func (r *MockSubscriptionRepo) FindExpiring(ctx context.Context, tx repository.Tx, withinDays int) ([]*model.UserSubscription, error) {
	if r.FindExpiringFunc != nil {
		return r.FindExpiringFunc(ctx, tx, withinDays)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	deadline := now().AddDate(0, 0, withinDays)
	var out []*model.UserSubscription
	for _, s := range r.data {
		if s.Status == model.SubscriptionStatusActive && s.ExpiresAt != nil && s.ExpiresAt.Before(deadline) {
			cp := *s
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (r *MockSubscriptionRepo) CountActiveByPlan(ctx context.Context, tx repository.Tx) (map[string]int, error) {
	if r.CountActiveByPlanFunc != nil {
		return r.CountActiveByPlanFunc(ctx, tx)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	res := map[string]int{}
	for _, s := range r.data {
		if s.Status == model.SubscriptionStatusActive {
			res[s.PlanID]++
		}
	}
	return res, nil
}

func (r *MockSubscriptionRepo) TotalRemainingCredits(ctx context.Context, tx repository.Tx) (int64, error) {
	if r.TotalRemainingCreditsFunc != nil {
		return r.TotalRemainingCreditsFunc(ctx, tx)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var t int64
	for _, s := range r.data {
		if s.Status == model.SubscriptionStatusActive {
			t += s.RemainingCredits
		}
	}
	return t, nil
}

func (r *MockSubscriptionRepo) UpdateRemainingCredits(ctx context.Context, tx repository.Tx, id string, delta int64) error {
	if r.UpdateRemainingCreditsFunc != nil {
		return r.UpdateRemainingCreditsFunc(ctx, tx, id, delta)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if s, ok := r.data[id]; ok {
		s.RemainingCredits += delta
		if s.RemainingCredits < 0 {
			s.RemainingCredits = 0
		}
		return nil
	}
	return errors.New("not found")
}

func (r *MockSubscriptionRepo) UpdateStatus(ctx context.Context, tx repository.Tx, id string, status model.SubscriptionStatus) error {
	if r.UpdateStatusFunc != nil {
		return r.UpdateStatusFunc(ctx, tx, id, status)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if s, ok := r.data[id]; ok {
		s.Status = status
		return nil
	}
	return errors.New("not found")
}

func (r *MockSubscriptionRepo) CountByStatus(ctx context.Context, tx repository.Tx) (map[model.SubscriptionStatus]int, error) {
	if r.CountByStatusFunc != nil {
		return r.CountByStatusFunc(ctx, tx)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	counts := make(map[model.SubscriptionStatus]int)
	for _, s := range r.data {
		counts[s.Status]++
	}
	return counts, nil
}

// ---- Mock PaymentRepository ----

type MockPaymentRepo struct {
	mu     sync.Mutex
	data   map[string]*model.Payment // by id
	byAuth map[string]string         // authority -> id

	SaveFunc                  func(ctx context.Context, tx repository.Tx, p *model.Payment) error
	FindByIDFunc              func(ctx context.Context, tx repository.Tx, id string) (*model.Payment, error)
	FindByAuthorityFunc       func(ctx context.Context, tx repository.Tx, authority string) (*model.Payment, error)
	UpdateStatusIfPendingFunc func(ctx context.Context, tx repository.Tx, id string, newStatus model.PaymentStatus) (bool, error)
	UpdateStatusFunc          func(ctx context.Context, tx repository.Tx, id string, newStatus model.PaymentStatus) error
	SumByPeriodFunc           func(ctx context.Context, tx repository.Tx, period string) (int64, error)
	SetActivationCodeFunc     func(ctx context.Context, tx repository.Tx, id, code string) error
	FindByActivationCodeFunc  func(ctx context.Context, tx repository.Tx, code string) (*model.Payment, error)
	ListPendingOlderThanFunc  func(ctx context.Context, tx repository.Tx, olderThan time.Time) ([]*model.Payment, error)
}

var _ repository.PaymentRepository = (*MockPaymentRepo)(nil)

func NewMockPaymentRepo() *MockPaymentRepo {
	return &MockPaymentRepo{data: map[string]*model.Payment{}, byAuth: map[string]string{}}
}

func (r *MockPaymentRepo) Save(ctx context.Context, tx repository.Tx, p *model.Payment) error {
	if r.SaveFunc != nil {
		return r.SaveFunc(ctx, tx, p)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	cp := *p
	r.data[p.ID] = &cp
	if p.Authority != "" {
		r.byAuth[p.Authority] = p.ID
	}
	return nil
}

func (r *MockPaymentRepo) FindByID(ctx context.Context, tx repository.Tx, id string) (*model.Payment, error) {
	if r.FindByIDFunc != nil {
		return r.FindByIDFunc(ctx, tx, id)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if p, ok := r.data[id]; ok {
		cp := *p
		return &cp, nil
	}
	return nil, nil
}

func (r *MockPaymentRepo) FindByAuthority(ctx context.Context, tx repository.Tx, authority string) (*model.Payment, error) {
	if r.FindByAuthorityFunc != nil {
		return r.FindByAuthorityFunc(ctx, tx, authority)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if id, ok := r.byAuth[authority]; ok {
		cp := *r.data[id]
		return &cp, nil
	}
	return nil, nil
}

func (r *MockPaymentRepo) UpdateStatus(ctx context.Context, tx repository.Tx, id string, status model.PaymentStatus, refID *string, paidAt *time.Time) error {
	if r.UpdateStatusFunc != nil {
		return r.UpdateStatusFunc(ctx, tx, id, status)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.data[id]
	if !ok {
		return errors.New("not found")
	}
	p.Status = status
	// optional: if your Payment model has RefID/PaidAt fields, set them here when non-nil
	return nil
}

func (r *MockPaymentRepo) UpdateStatusIfPending(ctx context.Context, tx repository.Tx, id string, status model.PaymentStatus, refID *string, paidAt *time.Time) (bool, error) {
	if r.UpdateStatusIfPendingFunc != nil {
		return r.UpdateStatusIfPendingFunc(ctx, tx, id, status)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.data[id]
	if !ok {
		return false, errors.New("not found")
	}
	cur := strings.ToLower(string(p.Status))
	if cur != "pending" && cur != "initiated" {
		return false, nil
	}
	p.Status = status
	// optional set refID/paidAt if your model has them
	return true, nil
}

func (r *MockPaymentRepo) ListPendingOlderThan(ctx context.Context, tx repository.Tx, olderThan time.Time, limit int) ([]*model.Payment, error) {
	if r.ListPendingOlderThanFunc != nil {
		return r.ListPendingOlderThanFunc(ctx, tx, olderThan)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*model.Payment
	for _, p := range r.data {
		if strings.ToLower(string(p.Status)) == "pending" && p.CreatedAt.Before(olderThan) {
			cp := *p
			out = append(out, &cp)
			if limit > 0 && len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

func (r *MockPaymentRepo) SetActivationCode(ctx context.Context, tx repository.Tx, id, code string, expiresAt time.Time) error {
	if r.SetActivationCodeFunc != nil {
		return r.SetActivationCodeFunc(ctx, tx, id, code)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.data[id]
	if !ok {
		return errors.New("not found")
	}
	p.ActivationCode = &code
	// you can store expiresAt in your model if a field exists; mock ignores it safely
	return nil
}

func (r *MockPaymentRepo) SumByPeriod(ctx context.Context, tx repository.Tx, period string) (int64, error) {
	if r.SumByPeriodFunc != nil {
		return r.SumByPeriodFunc(ctx, tx, period)
	}
	// naive total sum as default
	r.mu.Lock()
	defer r.mu.Unlock()
	var sum int64
	for _, p := range r.data {
		sum += p.Amount
	}
	return sum, nil
}

func (r *MockPaymentRepo) FindByActivationCode(ctx context.Context, tx repository.Tx, code string) (*model.Payment, error) {
	if r.FindByActivationCodeFunc != nil {
		return r.FindByActivationCodeFunc(ctx, tx, code)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, p := range r.data {
		if *p.ActivationCode == code {
			cp := *p
			return &cp, nil
		}
	}
	return nil, nil
}

// ---- Mock PurchaseRepository ----

type MockPurchaseRepo struct {
	mu   sync.Mutex
	data map[string]*model.Purchase

	SaveFunc       func(ctx context.Context, tx repository.Tx, pur *model.Purchase) error
	ListByUserFunc func(ctx context.Context, tx repository.Tx, userID string) ([]*model.Purchase, error)
}

var _ repository.PurchaseRepository = (*MockPurchaseRepo)(nil)

func NewMockPurchaseRepo() *MockPurchaseRepo {
	return &MockPurchaseRepo{data: map[string]*model.Purchase{}}
}

func (r *MockPurchaseRepo) Save(ctx context.Context, tx repository.Tx, pur *model.Purchase) error {
	if r.SaveFunc != nil {
		return r.SaveFunc(ctx, tx, pur)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if pur.ID == "" {
		pur.ID = uuid.NewString()
	}
	cp := *pur
	r.data[pur.ID] = &cp
	return nil
}

func (r *MockPurchaseRepo) ListByUser(ctx context.Context, tx repository.Tx, userID string) ([]*model.Purchase, error) {
	if r.ListByUserFunc != nil {
		return r.ListByUserFunc(ctx, tx, userID)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*model.Purchase
	for _, p := range r.data {
		if p.UserID == userID {
			cp := *p
			out = append(out, &cp)
		}
	}
	return out, nil
}

// ---- Mock ModelPricingRepository ----

type MockModelPricingRepo struct {
	mu      sync.Mutex
	byModel map[string]*model.ModelPricing

	GetByModelNameFunc func(ctx context.Context, model string) (*model.ModelPricing, error)
	ListActiveFunc     func(ctx context.Context) ([]*model.ModelPricing, error)
	CreateFunc         func(ctx context.Context, p *model.ModelPricing) error
	UpdateFunc         func(ctx context.Context, p *model.ModelPricing) error
}

var _ repository.ModelPricingRepository = (*MockModelPricingRepo)(nil)

func NewMockModelPricingRepo() *MockModelPricingRepo {
	return &MockModelPricingRepo{byModel: map[string]*model.ModelPricing{}}
}

func (r *MockModelPricingRepo) Seed(mp *model.ModelPricing) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *mp
	r.byModel[strings.ToLower(mp.ModelName)] = &cp
}

func (r *MockModelPricingRepo) GetByModelName(ctx context.Context, tx repository.Tx, model string) (*model.ModelPricing, error) {
	if r.GetByModelNameFunc != nil {
		return r.GetByModelNameFunc(ctx, model)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if p, ok := r.byModel[strings.ToLower(model)]; ok {
		cp := *p
		return &cp, nil
	}
	return nil, errors.New("not found")
}

func (r *MockModelPricingRepo) ListActive(ctx context.Context, tx repository.Tx) ([]*model.ModelPricing, error) {
	if r.ListActiveFunc != nil {
		return r.ListActiveFunc(ctx)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*model.ModelPricing
	for _, p := range r.byModel {
		if !p.Active {
			continue
		}
		cp := *p
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ModelName < out[j].ModelName })
	return out, nil
}

func (r *MockModelPricingRepo) Create(ctx context.Context, tx repository.Tx, p *model.ModelPricing) error {
	if r.CreateFunc != nil {
		return r.CreateFunc(ctx, p)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	cp := *p
	r.byModel[strings.ToLower(p.ModelName)] = &cp
	return nil
}

func (r *MockModelPricingRepo) Update(ctx context.Context, tx repository.Tx, p *model.ModelPricing) error {
	if r.UpdateFunc != nil {
		return r.UpdateFunc(ctx, p)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byModel[strings.ToLower(p.ModelName)]; !ok {
		return errors.New("not found")
	}
	cp := *p
	r.byModel[strings.ToLower(p.ModelName)] = &cp
	return nil
}

// ---- Mock ChatSessionRepository ----

type MockChatSessionRepo struct {
	mu            sync.Mutex
	byID          map[string]*model.ChatSession
	msgByID       map[string][]*model.ChatMessage // sessionID -> messages
	usersBySessID map[string]*model.User          // sessionID -> user

	SaveFunc                func(ctx context.Context, tx repository.Tx, s *model.ChatSession) error
	SaveMessageFunc         func(ctx context.Context, tx repository.Tx, m *model.ChatMessage) (bool, error)
	DeleteFunc              func(ctx context.Context, tx repository.Tx, id string) error
	FindActiveByUserFunc    func(ctx context.Context, tx repository.Tx, userID string) (*model.ChatSession, error)
	FindByIDFunc            func(ctx context.Context, tx repository.Tx, id string) (*model.ChatSession, error)
	UpdateStatusFunc        func(ctx context.Context, tx repository.Tx, sessionID string, status model.ChatSessionStatus) error
	ListByUserFunc          func(ctx context.Context, tx repository.Tx, userID string, offset, limit int) ([]*model.ChatSession, error)
	CleanupOldMessagesFunc  func(ctx context.Context, userID string, retentionDays int) (int64, error)
	FindUserBySessionIDFunc func(ctx context.Context, tx repository.Tx, sessionID string) (*model.User, error)
	DeleteAllByUserIDFunc   func(ctx context.Context, tx repository.Tx, userID string) error
}

var _ repository.ChatSessionRepository = (*MockChatSessionRepo)(nil)

func NewMockChatSessionRepo() *MockChatSessionRepo {
	return &MockChatSessionRepo{
		byID:          map[string]*model.ChatSession{},
		msgByID:       map[string][]*model.ChatMessage{},
		usersBySessID: map[string]*model.User{},
	}
}

func (r *MockChatSessionRepo) Save(ctx context.Context, tx repository.Tx, s *model.ChatSession) error {
	if r.SaveFunc != nil {
		return r.SaveFunc(ctx, tx, s)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if s.ID == "" {
		s.ID = uuid.NewString()
	}
	cp := *s
	r.byID[s.ID] = &cp
	// Also store the user for FindUserBySessionID lookups
	if _, ok := r.usersBySessID[s.ID]; !ok {
		r.usersBySessID[s.ID] = &model.User{ID: s.UserID, TelegramID: 12345} // Default TG ID for tests
	}
	return nil
}

func (r *MockChatSessionRepo) SaveMessage(ctx context.Context, tx repository.Tx, m *model.ChatMessage) (bool, error) {
	if r.SaveMessageFunc != nil {
		return r.SaveMessageFunc(ctx, tx, m)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *m
	r.msgByID[m.SessionID] = append(r.msgByID[m.SessionID], &cp)
	return true, nil
}

func (r *MockChatSessionRepo) Delete(ctx context.Context, tx repository.Tx, id string) error {
	if r.DeleteFunc != nil {
		return r.DeleteFunc(ctx, tx, id)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.byID, id)
	delete(r.msgByID, id)
	return nil
}

func (r *MockChatSessionRepo) FindActiveByUser(ctx context.Context, tx repository.Tx, userID string) (*model.ChatSession, error) {
	if r.FindActiveByUserFunc != nil {
		return r.FindActiveByUserFunc(ctx, tx, userID)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, s := range r.byID {
		if s.UserID == userID && s.Status == model.ChatSessionActive {
			cp := *s
			cp.Messages = cloneMessages(r.msgByID[s.ID])
			return &cp, nil
		}
	}
	return nil, nil
}

func (r *MockChatSessionRepo) FindByID(ctx context.Context, tx repository.Tx, id string) (*model.ChatSession, error) {
	if r.FindByIDFunc != nil {
		return r.FindByIDFunc(ctx, tx, id)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if s, ok := r.byID[id]; ok {
		cp := *s
		cp.Messages = cloneMessages(r.msgByID[id])
		return &cp, nil
	}
	return nil, nil
}

func (r *MockChatSessionRepo) FindUserBySessionID(ctx context.Context, tx repository.Tx, sessionID string) (*model.User, error) {
	if r.FindUserBySessionIDFunc != nil {
		return r.FindUserBySessionIDFunc(ctx, tx, sessionID)
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	if user, ok := r.usersBySessID[sessionID]; ok {
		return user, nil
	}

	// Fallback for sessions created without the user side-channel
	if sess, ok := r.byID[sessionID]; ok {
		return &model.User{ID: sess.UserID, TelegramID: 12345}, nil // Default TG ID
	}

	return nil, errors.New("user for session not found")
}

func (r *MockChatSessionRepo) UpdateStatus(ctx context.Context, tx repository.Tx, sessionID string, status model.ChatSessionStatus) error {
	if r.UpdateStatusFunc != nil {
		return r.UpdateStatusFunc(ctx, tx, sessionID, status)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if s, ok := r.byID[sessionID]; ok {
		s.Status = status
		s.UpdatedAt = now()
		return nil
	}
	return errors.New("not found")
}

func (r *MockChatSessionRepo) ListByUser(ctx context.Context, tx repository.Tx, userID string, offset, limit int) ([]*model.ChatSession, error) {
	if r.ListByUserFunc != nil {
		return r.ListByUserFunc(ctx, tx, userID, offset, limit)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var all []*model.ChatSession
	for _, s := range r.byID {
		if s.UserID == userID {
			cp := *s
			cp.Messages = cloneMessages(r.msgByID[s.ID])
			all = append(all, &cp)
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].CreatedAt.After(all[j].CreatedAt) })
	if offset > len(all) {
		return []*model.ChatSession{}, nil
	}
	all = all[offset:]
	if limit > 0 && limit < len(all) {
		all = all[:limit]
	}
	return all, nil
}

func (r *MockChatSessionRepo) CleanupOldMessages(ctx context.Context, userID string, retentionDays int) (int64, error) {
	if r.CleanupOldMessagesFunc != nil {
		return r.CleanupOldMessagesFunc(ctx, userID, retentionDays)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	// no-op for in-memory
	return 0, nil
}

func (r *MockChatSessionRepo) DeleteAllByUserID(ctx context.Context, tx repository.Tx, userID string) error {
	if r.DeleteAllByUserIDFunc != nil {
		return r.DeleteAllByUserIDFunc(ctx, tx, userID)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, s := range r.byID {
		if s.UserID == userID {
			delete(r.byID, id)
			delete(r.msgByID, id)
			delete(r.usersBySessID, id)
		}
	}
	return nil
}

// ---- Mock AIJobRepository ----

type MockAIJobRepo struct {
	mu   sync.Mutex
	data map[string]*model.AIJob

	SaveFunc                   func(ctx context.Context, tx repository.Tx, job *model.AIJob) error
	FetchAndMarkProcessingFunc func(ctx context.Context) (*model.AIJob, error)
}

var _ repository.AIJobRepository = (*MockAIJobRepo)(nil)

func NewMockAIJobRepo() *MockAIJobRepo {
	return &MockAIJobRepo{data: map[string]*model.AIJob{}}
}

func (r *MockAIJobRepo) Save(ctx context.Context, tx repository.Tx, job *model.AIJob) error {
	if r.SaveFunc != nil {
		return r.SaveFunc(ctx, tx, job)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if job.ID == "" {
		job.ID = uuid.NewString()
	}
	cp := *job
	r.data[job.ID] = &cp
	return nil
}

func (r *MockAIJobRepo) FetchAndMarkProcessing(ctx context.Context) (*model.AIJob, error) {
	if r.FetchAndMarkProcessingFunc != nil {
		return r.FetchAndMarkProcessingFunc(ctx)
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	// Find the oldest pending job
	var oldestJob *model.AIJob
	for _, job := range r.data {
		if job.Status == model.AIJobStatusPending {
			if oldestJob == nil || job.CreatedAt.Before(oldestJob.CreatedAt) {
				j := *job
				oldestJob = &j
			}
		}
	}

	if oldestJob == nil {
		return nil, nil // No pending jobs found
	}

	// Mark as processing and return
	jobToProcess := r.data[oldestJob.ID]
	jobToProcess.Status = model.AIJobStatusProcessing
	cp := *jobToProcess
	return &cp, nil
}

// ---- Mock NotificationLogRepository ----

// MockNotificationLogRepo mocks the repository for tracking sent notifications.
type MockNotificationLogRepo struct {
	mu sync.Mutex
	// The key is a composite: "subscriptionID:kind:thresholdDays"
	entries map[string]struct{}

	SaveFunc   func(ctx context.Context, tx repository.Tx, subscriptionID, userID, kind string, thresholdDays int) error
	ExistsFunc func(ctx context.Context, tx repository.Tx, subscriptionID, kind string, thresholdDays int) (bool, error)
}

var _ repository.NotificationLogRepository = (*MockNotificationLogRepo)(nil)

func NewMockNotificationLogRepo() *MockNotificationLogRepo {
	return &MockNotificationLogRepo{
		entries: make(map[string]struct{}),
	}
}

// makeKey is a helper to create a consistent key for the in-memory map.
func (r *MockNotificationLogRepo) makeKey(subscriptionID, kind string, thresholdDays int) string {
	return fmt.Sprintf("%s:%s:%d", subscriptionID, kind, thresholdDays)
}

func (r *MockNotificationLogRepo) Save(ctx context.Context, tx repository.Tx, subscriptionID, userID, kind string, thresholdDays int) error {
	if r.SaveFunc != nil {
		return r.SaveFunc(ctx, tx, subscriptionID, userID, kind, thresholdDays)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	key := r.makeKey(subscriptionID, kind, thresholdDays)
	r.entries[key] = struct{}{}
	return nil
}

func (r *MockNotificationLogRepo) Exists(ctx context.Context, tx repository.Tx, subscriptionID, kind string, thresholdDays int) (bool, error) {
	if r.ExistsFunc != nil {
		return r.ExistsFunc(ctx, tx, subscriptionID, kind, thresholdDays)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	key := r.makeKey(subscriptionID, kind, thresholdDays)
	_, exists := r.entries[key]
	return exists, nil
}

// ---- Mock RegistrationStateRepository ----

// MockRegistrationStateRepo mocks the repository for registration state.
type MockRegistrationStateRepo struct {
	mu   sync.Mutex
	data map[int64]*repository.RegistrationState

	SetStateFunc   func(ctx context.Context, tgID int64, state *repository.RegistrationState) error
	GetStateFunc   func(ctx context.Context, tgID int64) (*repository.RegistrationState, error)
	ClearStateFunc func(ctx context.Context, tgID int64) error
}

var _ repository.RegistrationStateRepository = (*MockRegistrationStateRepo)(nil)

func NewMockRegistrationStateRepo() *MockRegistrationStateRepo {
	return &MockRegistrationStateRepo{data: make(map[int64]*repository.RegistrationState)}
}

func (m *MockRegistrationStateRepo) SetState(ctx context.Context, tgID int64, state *repository.RegistrationState) error {
	if m.SetStateFunc != nil {
		return m.SetStateFunc(ctx, tgID, state)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[tgID] = state
	return nil
}

func (m *MockRegistrationStateRepo) GetState(ctx context.Context, tgID int64) (*repository.RegistrationState, error) {
	if m.GetStateFunc != nil {
		return m.GetStateFunc(ctx, tgID)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if state, ok := m.data[tgID]; ok {
		return state, nil
	}
	return nil, redis.Nil // Simulate key not found
}

func (m *MockRegistrationStateRepo) ClearState(ctx context.Context, tgID int64) error {
	if m.ClearStateFunc != nil {
		return m.ClearStateFunc(ctx, tgID)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, tgID)
	return nil
}

// =============================
// Infra helpers for tests
// =============================

// ---- Mock TransactionManager ----

type MockTxManager struct {
	WithTxFunc func(ctx context.Context, txOpt pgx.TxOptions, fn func(ctx context.Context, tx repository.Tx) error) error
}

func NewMockTxManager() *MockTxManager {
	return &MockTxManager{}
}

var _ repository.TransactionManager = (*MockTxManager)(nil)

// WithTx provides a way to control transaction behavior during tests.
// By default, it runs the function immediately without a real transaction.
// For specific transactional tests, you can assign a custom function to WithTxFunc.
func (m *MockTxManager) WithTx(ctx context.Context, txOpt pgx.TxOptions, fn func(ctx context.Context, tx repository.Tx) error) error {
	if m.WithTxFunc != nil {
		return m.WithTxFunc(ctx, txOpt, fn)
	}
	// By default, execute the function immediately with NoTX.
	// This is suitable for tests that don't need to verify transactional logic.
	return fn(ctx, repository.NoTX)
}

// ---- In-memory Locker (implements redis.Locker port) ----

type MockLocker struct {
	mu    sync.Mutex
	held  map[string]string
	ErrOn map[string]error
}

func NewMockLocker() *MockLocker {
	return &MockLocker{held: map[string]string{}, ErrOn: map[string]error{}}
}

func (l *MockLocker) TryLock(ctx context.Context, key string, ttl time.Duration) (string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if err, bad := l.ErrOn[key]; bad {
		return "", err
	}
	if tok, ok := l.held[key]; ok && tok != "" {
		return "", errors.New("locked")
	}
	tok := uuid.NewString()
	l.held[key] = tok
	return tok, nil
}

func (l *MockLocker) Unlock(ctx context.Context, key, token string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.held[key] == token {
		delete(l.held, key)
		return nil
	}
	return errors.New("unlock token mismatch")
}

// newTestLogger creates a silent zerolog.Logger for use in tests.
// It writes to io.Discard to prevent logs from cluttering test output.
func newTestLogger() *zerolog.Logger {
	logger := zerolog.New(io.Discard)
	return &logger
}

// --- Mock Translator

func newTestTranslator() *i18n.Translator {
	// Create a minimal, in-memory virtual filesystem for the test translator.
	// This ensures the test is self-contained and doesn't rely on real files.
	testFS := fstest.MapFS{
		"locales/fa.yaml": {
			Data: []byte("reg_start: 'Welcome %s'"), // Minimal content to prevent parsing errors
		},
		"locales/policy-fa.txt": {
			Data: []byte("Test Policy"),
		},
	}

	// Now, call the real NewTranslator with our in-memory filesystem.
	// We ignore the error because we control the test data and know it's valid.
	translator, _ := i18n.NewTranslator(testFS, "fa")
	return translator
}
