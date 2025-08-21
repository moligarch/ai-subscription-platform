package telegram

import (
	"testing"

	"telegram-ai-subscription/internal/config"
)

func TestIsAdmin(t *testing.T) {
	cfg := &config.BotConfig{
		Token:    "dummy",
		AdminIDs: []int64{1111, 2222},
		Mode:     "polling",
		Port:     9000,
		URL:      "http://localhost:9000",
	}

	// We need a minimal constructor call; the NewRealTelegramBotAdapter expects a facade and userRepo, but
	// isAdmin doesn't use them. To keep test simple we pass nil for those (constructor checks them).
	// So use the zero struct and assign admin map manually for testing.
	r := &RealTelegramBotAdapter{
		cfg:         cfg,
		adminIDsMap: map[int64]struct{}{1111: {}, 2222: {}},
	}

	if !r.isAdmin(1111) {
		t.Fatalf("expected 1111 to be admin")
	}
	if r.isAdmin(3333) {
		t.Fatalf("expected 3333 to NOT be admin")
	}
}
