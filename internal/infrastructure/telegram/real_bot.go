package telegram

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/google/uuid"

	"telegram-ai-subscription/internal/config"
	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/domain/repository"
	"telegram-ai-subscription/internal/usecase"
)

// RealTelegramBotAdapter implements adapter.TelegramBotAdapter using tgbotapi with concurrent polling.
type RealTelegramBotAdapter struct {
	bot         *tgbotapi.BotAPI
	cfg         *config.BotConfig
	userRepo    repository.UserRepository
	statsUC     *usecase.StatsUseCase
	adminIDsMap map[int64]struct{}

	// updateWorkers is how many goroutines will concurrently process updates.
	updateWorkers int
	// cancelPolling cancels polling when called
	cancelPolling context.CancelFunc
}

// NewRealTelegramBotAdapter creates a new bot adapter.
// statsUC is required for admin /stats command. updateWorkers controls concurrency.
func NewRealTelegramBotAdapter(cfg *config.BotConfig, userRepo repository.UserRepository, statsUC *usecase.StatsUseCase, updateWorkers int) (*RealTelegramBotAdapter, error) {
	if cfg == nil {
		return nil, errors.New("bot config is nil")
	}
	if userRepo == nil {
		return nil, errors.New("userRepo is nil")
	}
	if statsUC == nil {
		return nil, errors.New("statsUC is nil")
	}
	if updateWorkers <= 0 {
		updateWorkers = 5
	}

	bot, err := tgbotapi.NewBotAPI(cfg.Token)
	if err != nil {
		return nil, err
	}

	adminMap := make(map[int64]struct{}, len(cfg.AdminIDs))
	for _, id := range cfg.AdminIDs {
		adminMap[id] = struct{}{}
	}

	return &RealTelegramBotAdapter{
		bot:           bot,
		cfg:           cfg,
		userRepo:      userRepo,
		statsUC:       statsUC,
		adminIDsMap:   adminMap,
		updateWorkers: updateWorkers,
	}, nil
}

// StartPolling begins polling Telegram for updates concurrently.
// It runs until ctx is canceled.
func (r *RealTelegramBotAdapter) StartPolling(ctx context.Context) error {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := r.bot.GetUpdatesChan(u)

	ctx, cancel := context.WithCancel(ctx)
	r.cancelPolling = cancel

	var wg sync.WaitGroup
	updateChan := make(chan tgbotapi.Update, 100)

	// Start worker goroutines to process updates concurrently
	for i := 0; i < r.updateWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for {
				select {
				case update, ok := <-updateChan:
					if !ok {
						return
					}
					if err := r.handleUpdate(ctx, update); err != nil {
						log.Printf("[telegram][worker-%d] error handling update: %v", workerID, err)
					}
				case <-ctx.Done():
					return
				}
			}
		}(i + 1)
	}

	// Dispatcher goroutine: feed updates into updateChan
	go func() {
		defer close(updateChan)
		for {
			select {
			case update := <-updates:
				select {
				case updateChan <- update:
				case <-ctx.Done():
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	<-ctx.Done()
	r.bot.StopReceivingUpdates()
	wg.Wait()
	return nil
}

// StopPolling stops the polling loop gracefully.
func (r *RealTelegramBotAdapter) StopPolling() {
	if r.cancelPolling != nil {
		r.cancelPolling()
	}
}

// SendMessage sends a text message to the user identified by domain userID
// It looks up the TelegramID via userRepo.FindByTelegramID (we have domain User -> TelegramID).
func (r *RealTelegramBotAdapter) SendMessage(ctx context.Context, userID string, text string) error {
	// The domain UserRepository has no FindByID in your repo; we already store TelegramID on user,
	// so try to find by TelegramID or use your repo Save mapping approach. Here we attempt to find by
	// searching users (if your repo has FindByID, replace with that).
	// For simplicity, we assume userID is the internal ID and userRepo has Save/FindByTelegramID only.
	// If you have a FindByID method, replace the below logic with that call.
	// We'll attempt to scan all (not ideal) — but best is to have FindByID implemented.
	//
	// NOTE: earlier you fixed usage to use user.TelegramID. So we assume you will call SendMessage
	// using the domain User object's TelegramID elsewhere. If you prefer, change signature to accept tgID.

	// We'll attempt to call a FindByTelegramID-like reverse mapping is not available.
	// So we try to find user by scanning — but this is inefficient. Best approach: add FindByID in repo.
	return errors.New("SendMessage: please call SendMessageWithTelegramID or add FindByID in UserRepository")
}

// helper: SendMessageWithTelegramID — convenience that sends directly using Telegram chat id.
func (r *RealTelegramBotAdapter) SendMessageWithTelegramID(ctx context.Context, tgID int64, text string) error {
	msg := tgbotapi.NewMessage(tgID, text)
	_, err := r.bot.Send(msg)
	return err
}

// handleUpdate processes a single Telegram update.
func (r *RealTelegramBotAdapter) handleUpdate(ctx context.Context, update tgbotapi.Update) error {
	if update.Message == nil {
		return nil
	}
	tgUser := update.Message.From
	if tgUser == nil {
		return nil
	}

	// Try find domain user by TelegramID
	user, err := r.userRepo.FindByTelegramID(ctx, tgUser.ID)
	if err != nil {
		// if not found, attempt to register a new domain user
		if errors.Is(err, domain.ErrNotFound) {
			newUser := &domain.User{
				ID:           uuid.NewString(),
				TelegramID:   tgUser.ID,
				Username:     tgUser.UserName,
				RegisteredAt: time.Now(),
				LastActiveAt: time.Now(),
			}
			if err := r.userRepo.Save(ctx, newUser); err != nil {
				log.Printf("[telegram] failed to register new user %d: %v", tgUser.ID, err)
				// still allow the user to use the bot but send failure
				_ = r.SendMessageWithTelegramID(ctx, tgUser.ID, "Failed to register you in the system. Contact admin.")
				return err
			}
			user = newUser
		} else {
			return err
		}
	} else {
		// Update last active time (best-effort)
		user.LastActiveAt = time.Now()
		_ = r.userRepo.Save(ctx, user)
	}

	text := update.Message.Text
	if len(text) > 0 && text[0] == '/' {
		return r.handleCommand(ctx, user, text)
	}

	// Default reply
	return r.SendMessageWithTelegramID(ctx, user.TelegramID, "Sorry, I didn't understand that. Send /help for commands.")
}

func (r *RealTelegramBotAdapter) handleCommand(ctx context.Context, user *domain.User, text string) error {
	cmd := strings.TrimSpace(text)
	switch {
	case cmd == "/start":
		return r.SendMessageWithTelegramID(ctx, user.TelegramID, "Welcome to the subscription bot! Use /help to see commands.")
	case cmd == "/help":
		return r.SendMessageWithTelegramID(ctx, user.TelegramID, "Available commands:\n/start\n/help\n/stats (admin only)\n/plans\n/subscribe <plan_id>\n/myplan")
	case cmd == "/stats":
		if !r.isAdmin(user.TelegramID) {
			return r.SendMessageWithTelegramID(ctx, user.TelegramID, "You are not authorized to use this command.")
		}
		statsText, err := r.getStatsText(ctx)
		if err != nil {
			log.Printf("[telegram] failed to get stats: %v", err)
			return r.SendMessageWithTelegramID(ctx, user.TelegramID, "Failed to get stats. Please try again later.")
		}
		return r.SendMessageWithTelegramID(ctx, user.TelegramID, statsText)
	default:
		// For unimplemented commands, show help
		return r.SendMessageWithTelegramID(ctx, user.TelegramID, "Unknown command. Send /help for the list of commands.")
	}
}

func (r *RealTelegramBotAdapter) isAdmin(tgID int64) bool {
	_, ok := r.adminIDsMap[tgID]
	return ok
}

func (r *RealTelegramBotAdapter) getStatsText(ctx context.Context) (string, error) {
	totalUsers, inactiveUsers, byPlan, totalCredits, err := r.statsUC.GetCounts(ctx, 30*24*time.Hour)
	if err != nil {
		return "", err
	}
	weekCents, monthCents, yearCents, err := r.statsUC.GetPaymentsForPeriods(ctx)
	if err != nil {
		return "", err
	}

	formatMoney := func(cents float64) string { return fmt.Sprintf("%.2f", float64(cents)/100.0) }

	var sb strings.Builder
	sb.WriteString("📊 System Statistics:\n\n")
	sb.WriteString(fmt.Sprintf("👥 Users: %d\n", totalUsers))
	sb.WriteString(fmt.Sprintf("🚫 Deactivated (30d): %d\n\n", inactiveUsers))
	sb.WriteString("📦 Active subscriptions by plan:\n")
	for name, cnt := range byPlan {
		sb.WriteString(fmt.Sprintf("  - %s: %d\n", name, cnt))
	}
	sb.WriteString("\n💰 Payments:\n")
	sb.WriteString(fmt.Sprintf("  - This Week: %s\n", formatMoney(weekCents)))
	sb.WriteString(fmt.Sprintf("  - This Month: %s\n", formatMoney(monthCents)))
	sb.WriteString(fmt.Sprintf("  - This Year: %s\n\n", formatMoney(yearCents)))
	sb.WriteString(fmt.Sprintf("🎫 Total Active Credits: %d\n", totalCredits))
	return sb.String(), nil
}
