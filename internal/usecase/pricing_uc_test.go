//go:build !integration

package usecase_test

import (
	"context"
	"testing"

	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/usecase"

	"github.com/rs/zerolog"
)

// helper: get a no-op logger (provided in your central mocks too, but keep local fallback)
func testLogger() *zerolog.Logger {
	// prefer newTestLogger() if exported in your mocks file:
	// return newTestLogger()
	l := zerolog.Nop()
	return &l
}

func TestPricingUseCase_CRUD_WithCentralMocks(t *testing.T) {
	ctx := context.Background()
	logger := testLogger()

	// Central mocks from internal/usecase/mock_test.go
	priceRepo := NewMockModelPricingRepo()
	tx := NewMockTxManager()

	uc := usecase.NewPricingUseCase(priceRepo, tx, logger)

	// --- Create ---
	got, err := uc.Create(ctx, "gpt-4o", 1, 2, "IRR")
	if err != nil {
		t.Fatalf("Create: unexpected error: %v", err)
	}
	if got == nil || got.ModelName != "gpt-4o" || got.InputTokenPriceMicros != 1 || got.OutputTokenPriceMicros != 2 || !got.Active {
		t.Fatalf("Create: got %+v", got)
	}

	// Duplicate create should return ErrAlreadyExists
	if _, err := uc.Create(ctx, "gpt-4o", 1, 2, "IRR"); err == nil || err != domain.ErrAlreadyExists {
		t.Fatalf("Create duplicate: expected ErrAlreadyExists, got %v", err)
	}

	// --- Get ---
	got2, err := uc.Get(ctx, "gpt-4o")
	if err != nil {
		t.Fatalf("Get: unexpected error: %v", err)
	}
	if got2.ModelName != "gpt-4o" {
		t.Fatalf("Get: wrong model: %+v", got2)
	}

	// --- Update (partial) ---
	newIn := int64(3)
	got3, err := uc.Update(ctx, "gpt-4o", &newIn, nil, nil)
	if err != nil {
		t.Fatalf("Update: unexpected: %v", err)
	}
	if got3.InputTokenPriceMicros != 3 || got3.OutputTokenPriceMicros != 2 {
		t.Fatalf("Update: wrong values: %+v", got3)
	}

	// --- List (active only) ---
	list, err := uc.List(ctx)
	if err != nil {
		t.Fatalf("List: unexpected: %v", err)
	}
	if len(list) != 1 || list[0].ModelName != "gpt-4o" {
		t.Fatalf("List: wrong items: %+v", list)
	}

	// --- Delete (soft) ---
	if err := uc.Delete(ctx, "gpt-4o"); err != nil {
		t.Fatalf("Delete: unexpected: %v", err)
	}

	// After delete, List should be empty (mock filters Active in ListActive)
	list, err = uc.List(ctx)
	if err != nil {
		t.Fatalf("List after delete: unexpected: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("List after delete: expected empty, got %d", len(list))
	}

	// IMPORTANT NOTE about current central mock:
	// MockModelPricingRepo.GetByModelName (default) does NOT filter Active.
	// Our UC's Get expects "active only". To test that behavior now, override
	// the function to return ErrNotFound when the record is inactive.
	priceRepo.GetByModelNameFunc = func(_ context.Context, modelName string) (*model.ModelPricing, error) {
		priceRepo.mu.Lock()
		defer priceRepo.mu.Unlock()
		p, ok := priceRepo.byModel[modelName]
		if !ok || !p.Active {
			return nil, domain.ErrNotFound
		}
		cp := *p
		return &cp, nil
	}

	// Now Get should return ErrNotFound for deleted/inactive record
	if _, err := uc.Get(ctx, "gpt-4o"); err == nil || err != domain.ErrNotFound {
		t.Fatalf("Get after delete: expected ErrNotFound, got %v", err)
	}

	// Delete is idempotent: deleting again should return entity not found error
	if err := uc.Delete(ctx, "gpt-4o"); err != nil {
		if err != domain.ErrNotFound{
			t.Fatalf("Delete again (idempotent) failed: %v", err)
		}
	}
}

func TestPricingUseCase_Create_InvalidName_WithCentralMocks(t *testing.T) {
	ctx := context.Background()
	logger := testLogger()

	priceRepo := NewMockModelPricingRepo()
	tx := NewMockTxManager()
	uc := usecase.NewPricingUseCase(priceRepo, tx, logger)

	if _, err := uc.Create(ctx, "   ", 1, 2, "IRR"); err == nil || err != domain.ErrInvalidArgument {
		t.Fatalf("Create with invalid name: expected ErrInvalidArgument, got %v", err)
	}
}

func TestPricingUseCase_Update_NotFound_WithCentralMocks(t *testing.T) {
	ctx := context.Background()
	logger := testLogger()

	priceRepo := NewMockModelPricingRepo()
	tx := NewMockTxManager()
	uc := usecase.NewPricingUseCase(priceRepo, tx, logger)

	// Your default mock returns errors.New("not found") when absent.
	// To assert precise ErrNotFound, override to return domain.ErrNotFound.
	priceRepo.GetByModelNameFunc = func(_ context.Context, modelName string) (*model.ModelPricing, error) {
		return nil, domain.ErrNotFound
	}

	if _, err := uc.Update(ctx, "nope", nil, nil, nil); err == nil || err != domain.ErrNotFound {
		t.Fatalf("Update missing: expected ErrNotFound, got %v", err)
	}
}
