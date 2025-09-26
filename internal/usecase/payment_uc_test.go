//go:build !integration

package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/repository"
	"telegram-ai-subscription/internal/usecase"

	"github.com/jackc/pgx/v4"
)

// paymentUCTestDeps holds all the mock dependencies for the payment use case tests.
type paymentUCTestDeps struct {
	payments  *MockPaymentRepo
	plans     *MockPlanRepo
	subs      *MockSubscriptionRepo
	purchases *MockPurchaseRepo
	gateway   *MockPaymentGateway
	tm        *MockTxManager
	subUC     usecase.SubscriptionUseCase
}

// newPaymentUCDeps creates a fresh set of mocks for each test run.
func newPaymentUCDeps() *paymentUCTestDeps {
	deps := &paymentUCTestDeps{
		payments:  NewMockPaymentRepo(),
		plans:     NewMockPlanRepo(),
		subs:      NewMockSubscriptionRepo(),
		purchases: NewMockPurchaseRepo(),
		gateway:   &MockPaymentGateway{},
		tm:        NewMockTxManager(),
	}
	// The real SubscriptionUseCase needs its own mocks. We create it here.
	mockCodeRepo := NewMockActivationCodeRepo()
	deps.subUC = usecase.NewSubscriptionUseCase(deps.subs, deps.plans, mockCodeRepo, deps.tm, newTestLogger())
	return deps
}

func TestPaymentUseCase_Initiate(t *testing.T) {
	ctx := context.Background()
	testLogger := newTestLogger()

	plan := &model.SubscriptionPlan{ID: "plan-1", PriceIRR: 10000}

	t.Run("should initiate a payment successfully", func(t *testing.T) {
		// --- Arrange ---
		deps := newPaymentUCDeps()
		deps.plans.Save(ctx, nil, plan)

		var savedPayment *model.Payment
		deps.payments.SaveFunc = func(ctx context.Context, tx repository.Tx, p *model.Payment) error {
			savedPayment = p
			return nil
		}

		uc := usecase.NewPaymentUseCase(deps.payments, deps.plans, deps.subUC, deps.purchases, deps.gateway, deps.tm, testLogger)

		// --- Act ---
		_, payURL, err := uc.Initiate(ctx, "user-1", "plan-1", "http://callback.url", "desc", nil)

		// --- Assert ---
		if err != nil {
			t.Fatalf("expected no error, but got: %v", err)
		}
		if payURL == "" {
			t.Error("expected a payment URL, but got empty string")
		}
		if savedPayment == nil {
			t.Fatal("expected a payment record to be saved")
		}
		if savedPayment.Status != model.PaymentStatusPending {
			t.Errorf("expected payment status to be 'pending', but got '%s'", savedPayment.Status)
		}
		if savedPayment.Amount != plan.PriceIRR {
			t.Errorf("expected payment amount to be %d, but got %d", plan.PriceIRR, savedPayment.Amount)
		}
	})

	t.Run("should fail if user already has a reserved subscription", func(t *testing.T) {
		// --- Arrange ---
		deps := newPaymentUCDeps()
		deps.plans.Save(ctx, nil, plan)
		// Simulate a user having a reserved subscription
		deps.subs.Save(ctx, nil, &model.UserSubscription{UserID: "user-1", Status: model.SubscriptionStatusReserved})

		uc := usecase.NewPaymentUseCase(deps.payments, deps.plans, deps.subUC, deps.purchases, deps.gateway, deps.tm, testLogger)

		// --- Act ---
		_, _, err := uc.Initiate(ctx, "user-1", "plan-1", "http://callback.url", "desc", nil)

		// --- Assert ---
		if err == nil {
			t.Fatal("expected an error, but got nil")
		}
		if !errors.Is(err, domain.ErrAlreadyHasReserved) {
			t.Errorf("expected error to be ErrAlreadyHasReserved, but got %v", err)
		}
	})
}

func TestPaymentUseCase_ConfirmAuto(t *testing.T) {
	ctx := context.Background()
	testLogger := newTestLogger()

	plan := &model.SubscriptionPlan{ID: "plan-1", PriceIRR: 10000}
	payment := &model.Payment{ID: "pay-1", UserID: "user-1", PlanID: "plan-1", Authority: "auth-123", Status: model.PaymentStatusPending, Amount: 10000}

	t.Run("should confirm payment and grant subscription on success", func(t *testing.T) {
		// --- Arrange ---
		deps := newPaymentUCDeps()
		deps.plans.Save(ctx, nil, plan)
		deps.payments.Save(ctx, nil, payment)

		// Simulate a successful transaction
		deps.tm.WithTxFunc = func(ctx context.Context, txOpt pgx.TxOptions, fn func(ctx context.Context, tx repository.Tx) error) error {
			return fn(ctx, nil) // Execute the function immediately
		}

		// Simulate gateway verification success
		deps.gateway.VerifyPaymentFunc = func(ctx context.Context, authority string, expectedAmount int64) (string, error) {
			return "ref-123", nil
		}

		// Simulate the payment status update succeeding
		deps.payments.UpdateStatusIfPendingFunc = func(ctx context.Context, tx repository.Tx, id string, newStatus model.PaymentStatus, refID *string, paidAt *time.Time) (bool, error) {
			// In a real test, this would update the mock's state
			return true, nil
		}

		var savedPurchase *model.Purchase
		deps.purchases.SaveFunc = func(ctx context.Context, tx repository.Tx, pur *model.Purchase) error {
			savedPurchase = pur
			return nil
		}

		uc := usecase.NewPaymentUseCase(deps.payments, deps.plans, deps.subUC, deps.purchases, deps.gateway, deps.tm, testLogger)

		// --- Act ---
		finalPayment, err := uc.ConfirmAuto(ctx, "auth-123")

		// --- Assert ---
		if err != nil {
			t.Fatalf("expected no error, but got: %v", err)
		}
		if savedPurchase == nil {
			t.Fatal("expected a purchase record to be saved")
		}
		if savedPurchase.PaymentID != payment.ID {
			t.Errorf("purchase record has incorrect PaymentID")
		}
		if finalPayment.SubscriptionID == nil || *finalPayment.SubscriptionID == "" {
			t.Error("expected subscription ID to be linked to the final payment")
		}
	})

	t.Run("should fail if gateway verification fails", func(t *testing.T) {
		// --- Arrange ---
		deps := newPaymentUCDeps()
		deps.plans.Save(ctx, nil, plan)
		deps.payments.Save(ctx, nil, payment)

		// Simulate a failing transaction
		expectedErr := errors.New("gateway failure")
		deps.tm.WithTxFunc = func(ctx context.Context, txOpt pgx.TxOptions, fn func(ctx context.Context, tx repository.Tx) error) error {
			// Simulate rollback by propagating the error
			return fn(ctx, nil)
		}

		// Simulate gateway verification failure
		deps.gateway.VerifyPaymentFunc = func(ctx context.Context, authority string, expectedAmount int64) (string, error) {
			return "", expectedErr
		}

		uc := usecase.NewPaymentUseCase(deps.payments, deps.plans, deps.subUC, deps.purchases, deps.gateway, deps.tm, testLogger)

		// --- Act ---
		_, err := uc.ConfirmAuto(ctx, "auth-123")

		// --- Assert ---
		if err == nil {
			t.Fatal("expected an error, but got nil")
		}
		if !errors.Is(err, expectedErr) {
			t.Errorf("expected error to wrap gateway failure, but it didn't")
		}
	})

	t.Run("should sum revenue by period", func(t *testing.T) {
		deps := newPaymentUCDeps()
		deps.payments.SumByPeriodFunc = func(ctx context.Context, tx repository.Tx, period string) (int64, error) {
			if period == "month" {
				return 100000, nil
			}
			return 0, nil
		}
		uc := usecase.NewPaymentUseCase(deps.payments, deps.plans, deps.subUC, deps.purchases, deps.gateway, deps.tm, testLogger)

		revenue, err := uc.SumByPeriod(ctx, nil, "month")
		if err != nil {
			t.Fatalf("SumByPeriod failed: %v", err)
		}
		if revenue != 100000 {
			t.Errorf("expected revenue to be 100000, got %d", revenue)
		}
	})
}
