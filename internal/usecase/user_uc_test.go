package usecase

import (
	"context"
	"errors"
	"sync"
	"testing"

	"telegram-ai-subscription/internal/domain"
)

func TestRegisterOrFetch_CreatesNewUser(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := newMemUserRepo()
	uc := NewUserUseCase(repo)

	tgID := int64(123456789)
	username := "Demo User"

	u, err := uc.RegisterOrFetch(ctx, tgID, username)
	if err != nil {
		t.Fatalf("RegisterOrFetch returned error: %v", err)
	}

	if u == nil {
		t.Fatalf("expected user, got nil")
	}
	if u.TelegramID != tgID {
		t.Fatalf("expected TelegramID %d, got %d", tgID, u.TelegramID)
	}
	if u.Username != username {
		t.Fatalf("expected username %q, got %q", username, u.Username)
	}
	if u.ID == "" {
		t.Fatalf("expected generated ID to be non-empty")
	}

	// ensure it was persisted in repo
	stored, err := repo.FindByTelegramID(ctx, tgID)
	if err != nil {
		t.Fatalf("repo.FindByTelegramID after save returned error: %v", err)
	}
	if stored.ID != u.ID {
		t.Fatalf("stored user ID mismatch: expected %s got %s", u.ID, stored.ID)
	}
}

func TestRegisterOrFetch_ReturnsExistingUser(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := newMemUserRepo()
	// seed existing user
	existing := &domain.User{
		ID:         "existing-id-1",
		TelegramID: int64(7777),
		Username:   "Existing",
	}
	repo.store[existing.TelegramID] = existing

	uc := NewUserUseCase(repo)

	// Simulate Telegram reporting an updated username
	newUsername := "Ignored Name"
	u, err := uc.RegisterOrFetch(ctx, existing.TelegramID, newUsername)
	if err != nil {
		t.Fatalf("RegisterOrFetch returned error for existing user: %v", err)
	}

	// ID and TelegramID must remain same
	if u.ID != existing.ID {
		t.Fatalf("expected existing ID %s got %s", existing.ID, u.ID)
	}
	if u.TelegramID != existing.TelegramID {
		t.Fatalf("expected telegram id %d got %d", existing.TelegramID, u.TelegramID)
	}

	// Because we intentionally update username on RegisterOrFetch, expect it to be overwritten
	if u.Username != newUsername {
		t.Fatalf("expected username to be updated to %q got %q", newUsername, u.Username)
	}

	// ensure repository persisted the updated username as well
	stored, err := repo.FindByTelegramID(ctx, existing.TelegramID)
	if err != nil {
		t.Fatalf("repo.FindByTelegramID returned error: %v", err)
	}
	if stored.Username != newUsername {
		t.Fatalf("expected repo to persist username %q got %q", newUsername, stored.Username)
	}
}

func TestRegisterOrFetch_EmptyUsername(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := newMemUserRepo()
	uc := NewUserUseCase(repo)

	tgID := int64(222333444)
	empty := ""

	u, err := uc.RegisterOrFetch(ctx, tgID, empty)
	if err != nil {
		t.Fatalf("RegisterOrFetch returned error with empty username: %v", err)
	}
	if u == nil {
		t.Fatalf("expected user, got nil")
	}
	if u.Username != empty {
		t.Fatalf("expected empty username persisted, got %q", u.Username)
	}

	// verify persisted
	stored, err := repo.FindByTelegramID(ctx, tgID)
	if err != nil {
		t.Fatalf("FindByTelegramID after save error: %v", err)
	}
	if stored.Username != empty {
		t.Fatalf("expected repo stored empty username, got %q", stored.Username)
	}
}

func TestRegisterOrFetch_ConcurrentCalls(t *testing.T) {
	// do not use t.Parallel here; we control concurrency inside
	ctx := context.Background()
	repo := newMemUserRepo()
	uc := NewUserUseCase(repo)

	tgID := int64(555666777)
	username := "ConcurrentUser"

	// run many concurrent RegisterOrFetch calls
	const goroutines = 50
	var wg sync.WaitGroup
	results := make(chan string, goroutines)
	errs := make(chan error, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			u, err := uc.RegisterOrFetch(ctx, tgID, username)
			if err != nil {
				errs <- err
				return
			}
			results <- u.ID
		}()
	}

	wg.Wait()
	close(results)
	close(errs)

	// ensure no errors
	if len(errs) != 0 {
		for e := range errs {
			t.Errorf("concurrent RegisterOrFetch error: %v", e)
		}
		t.Fatalf("concurrent RegisterOrFetch had errors")
	}

	// collect unique returned IDs
	unique := map[string]struct{}{}
	count := 0
	for id := range results {
		unique[id] = struct{}{}
		count++
	}
	if count != goroutines {
		t.Fatalf("expected %d successful results, got %d", goroutines, count)
	}
	if len(unique) == 0 {
		t.Fatalf("expected at least one unique returned ID")
	}

	// verify repository has exactly one entry for that telegram ID
	stored, err := repo.FindByTelegramID(ctx, tgID)
	if err != nil {
		t.Fatalf("FindByTelegramID returned error: %v", err)
	}

	// Ensure the stored ID is one of the returned IDs
	if _, ok := unique[stored.ID]; !ok {
		t.Fatalf("stored ID %s was not among returned IDs %v", stored.ID, unique)
	}

	// ensure username persisted correctly
	if stored.Username != username {
		t.Fatalf("expected stored username %q, got %q", username, stored.Username)
	}
}

func TestRegisterOrFetch_SaveErrorPropagated(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := newMemUserRepo()
	// simulate save error
	repo.saveErr = errors.New("db failure")
	uc := NewUserUseCase(repo)

	tgID := int64(999999)
	_, err := uc.RegisterOrFetch(ctx, tgID, "Should Fail")
	if err == nil {
		t.Fatalf("expected error due to save failure, got nil")
	}
	if err.Error() != "db failure" && !errors.Is(err, repo.saveErr) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetByTelegramID_FoundAndNotFound(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := newMemUserRepo()
	uc := NewUserUseCase(repo)

	// not found case
	_, err := uc.GetByTelegramID(ctx, 42)
	if err == nil {
		t.Fatalf("expected ErrNotFound for unknown telegram id")
	}
	if err != domain.ErrNotFound {
		t.Fatalf("expected domain.ErrNotFound, got %v", err)
	}

	// seed and fetch
	user := &domain.User{
		ID:         "uid-100",
		TelegramID: int64(100),
		Username:   "U100",
	}
	_ = repo.Save(ctx, user)

	got, err := uc.GetByTelegramID(ctx, 100)
	if err != nil {
		t.Fatalf("GetByTelegramID returned error: %v", err)
	}
	if got.ID != user.ID {
		t.Fatalf("expected id %s got %s", user.ID, got.ID)
	}
	if got.TelegramID != user.TelegramID {
		t.Fatalf("expected telegram id %d got %d", user.TelegramID, got.TelegramID)
	}
}
