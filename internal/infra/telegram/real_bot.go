package telegram

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/google/uuid"

	"telegram-ai-subscription/internal/application"
	"telegram-ai-subscription/internal/config"
	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/repository"
)

// RealTelegramBotAdapter implements adapter.TelegramBotAdapter using tgbotapi with concurrent polling.
type RealTelegramBotAdapter struct {
	bot         *tgbotapi.BotAPI
	cfg         *config.BotConfig
	userRepo    repository.UserRepository
	facade      *application.BotFacade
	adminIDsMap map[int64]struct{}

	// updateWorkers is how many goroutines will concurrently process updates.
	updateWorkers int
	// cancelPolling cancels polling when called
	cancelPolling context.CancelFunc
}

// NewRealTelegramBotAdapter creates a new bot adapter.
// facade is required for commands. updateWorkers controls concurrency.
func NewRealTelegramBotAdapter(cfg *config.BotConfig, userRepo repository.UserRepository, facade *application.BotFacade, updateWorkers int) (*RealTelegramBotAdapter, error) {
	if cfg == nil {
		return nil, errors.New("bot config is nil")
	}
	if userRepo == nil {
		return nil, errors.New("userRepo is nil")
	}
	if facade == nil {
		return nil, errors.New("bot facade is nil")
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
		facade:        facade,
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
// It looks up the TelegramID via userRepo.FindByID
func (r *RealTelegramBotAdapter) SendMessage(ctx context.Context, userID string, text string) error {
	// Attempt to find domain user by internal ID
	user, err := r.userRepo.FindByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("SendMessage: find user by id %s: %w", userID, err)
	}
	if user == nil {
		return fmt.Errorf("SendMessage: user not found: %s", userID)
	}
	return r.SendMessageWithTelegramID(ctx, user.TelegramID, text)
}

// SendMessageWithTelegramID — convenience that sends directly using Telegram chat id.
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
			newUser := &model.User{
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

func (r *RealTelegramBotAdapter) handleCommand(ctx context.Context, user *model.User, text string) error {
	cmd := strings.TrimSpace(text)
	switch {
	case cmd == "/start":
		if r.facade == nil {
			return r.SendMessageWithTelegramID(ctx, user.TelegramID, "Bot is not fully initialized.")
		}
		welcomeText, err := r.facade.HandleStart(ctx, user.TelegramID, user.Username)
		if err != nil {
			log.Printf("[telegram] failed to handle /start for user %d: %v", user.TelegramID, err)
			return r.SendMessageWithTelegramID(ctx, user.TelegramID, "Failed to start. Please try again later.")
		}
		return r.SendMessageWithTelegramID(ctx, user.TelegramID, welcomeText)
	case cmd == "/help":
		helpText := "Available commands:\n" +
			"/start - Start the bot and register your account.\n" +
			"/plans - List available subscription plans.\n" +
			"/subscribe <plan_id> - Subscribe to a plan (demo mode: immediate subscription).\n" +
			"/myplan - Show your current subscription plan.\n" +
			"/balance - Show your current credit balance.\n" +
			"/help - Show this help message."
		if r.isAdmin(user.TelegramID) {
			helpText += "\n\nAdmin commands:\n" +
				"/plans create <name>;<duration_days>;<credits> - Create a new subscription plan.\n" +
				"/stats - Show bot statistics."
		}
		return r.SendMessageWithTelegramID(ctx, user.TelegramID, helpText)
	case cmd == "/stats":
		// Admin command to get stats
		if !r.isAdmin(user.TelegramID) {
			return r.SendMessageWithTelegramID(ctx, user.TelegramID, "You are not authorized to use this command.")
		}
		if r.facade == nil {
			return r.SendMessageWithTelegramID(ctx, user.TelegramID, "Stats feature not available.")
		}
		statsText, err := r.facade.HandleStats(ctx)
		if err != nil {
			log.Printf("[telegram] failed to get stats: %v", err)
			return r.SendMessageWithTelegramID(ctx, user.TelegramID, "Failed to get stats. Please try again later.")
		}
		return r.SendMessageWithTelegramID(ctx, user.TelegramID, statsText)
	case strings.HasPrefix(cmd, "/plans"):
		parts := strings.SplitN(cmd, " ", 3)
		// just "/plans" -> list plans
		if len(parts) == 1 {
			if r.facade == nil {
				return r.SendMessageWithTelegramID(ctx, user.TelegramID, "Plans feature not available.")
			}
			resp, err := r.facade.HandlePlans(ctx)
			if err != nil {
				return r.SendMessageWithTelegramID(ctx, user.TelegramID, "Failed to list plans: "+err.Error())
			}
			return r.SendMessageWithTelegramID(ctx, user.TelegramID, resp)
		}

		// subcommands e.g. "/plans create ..." (admin required)
		sub := strings.ToLower(strings.TrimSpace(parts[1]))
		if sub == "create" {
			if !r.isAdmin(user.TelegramID) {
				return r.SendMessageWithTelegramID(ctx, user.TelegramID, "You are not authorized to create plans.")
			}
			if r.facade == nil {
				return r.SendMessageWithTelegramID(ctx, user.TelegramID, "Plan creation feature not available.")
			}
			// Expect args like: name;durationDays;credits
			if len(parts) < 3 || strings.TrimSpace(parts[2]) == "" {
				return r.SendMessageWithTelegramID(ctx, user.TelegramID, "Usage: /plans create <name>;<duration_days>;<credits>\nExample: /plans create Pro;30;100")
			}
			args := parts[2]
			fields := strings.SplitN(args, ";", 3)
			if len(fields) != 3 {
				return r.SendMessageWithTelegramID(ctx, user.TelegramID, "Usage: /plans create <name>;<duration_days>;<credits>\nExample: /plans create Pro;30;100")
			}
			name := strings.TrimSpace(fields[0])
			durS := strings.TrimSpace(fields[1])
			credS := strings.TrimSpace(fields[2])

			dur, err := strconv.Atoi(durS)
			if err != nil || dur <= 0 {
				return r.SendMessageWithTelegramID(ctx, user.TelegramID, "Invalid duration. It must be a positive integer (days).")
			}
			credits, err := strconv.Atoi(credS)
			if err != nil || credits < 0 {
				return r.SendMessageWithTelegramID(ctx, user.TelegramID, "Invalid credits. It must be a non-negative integer.")
			}

			resp, err := r.facade.HandleCreatePlan(ctx, name, dur, credits)
			if err != nil {
				log.Printf("[telegram] create plan failed: %v", err)
				return r.SendMessageWithTelegramID(ctx, user.TelegramID, "Failed to create plan: "+err.Error())
			}
			return r.SendMessageWithTelegramID(ctx, user.TelegramID, resp)
		}

		// unknown /plans subcommand
		return r.SendMessageWithTelegramID(ctx, user.TelegramID, "Unknown /plans subcommand. Use /plans to list or /plans create <name>;<days>;<credits> (admin).")
	case strings.HasPrefix(cmd, "/subscribe"):
		// Handle subscription command
		if r.facade == nil {
			return r.SendMessageWithTelegramID(ctx, user.TelegramID, "Subscription feature not available.")
		}
		parts := strings.SplitN(cmd, " ", 2)
		if len(parts) < 2 {
			return r.SendMessageWithTelegramID(ctx, user.TelegramID, "Usage: /subscribe <plan_id>")
		}
		planID := strings.TrimSpace(parts[1])
		subscribeText, err := r.facade.HandleSubscribe(ctx, user.TelegramID, user.Username, planID)
		if err != nil {
			log.Printf("[telegram] failed to handle /subscribe for user %d: %v", user.TelegramID, err)
			return r.SendMessageWithTelegramID(ctx, user.TelegramID, "Failed to subscribe. Please check the plan ID and try again.")
		}
		return r.SendMessageWithTelegramID(ctx, user.TelegramID, subscribeText)
	case cmd == "/myplan":
		// Show user's current plan
		if r.facade == nil {
			return r.SendMessageWithTelegramID(ctx, user.TelegramID, "My plan feature not available.")
		}
		planText, err := r.facade.HandleMyPlan(ctx, user.TelegramID)
		if err != nil {
			log.Printf("[telegram] failed to get my plan for user %d: %v", user.TelegramID, err)
			return r.SendMessageWithTelegramID(ctx, user.TelegramID, "Failed to get your plan. Please try again later.")
		}
		if planText == "" {
			planText = "You have no active subscription."
		}
		return r.SendMessageWithTelegramID(ctx, user.TelegramID, planText)
	case cmd == "/balance":
		// Show user's balance (if implemented)
		if r.facade == nil {
			return r.SendMessageWithTelegramID(ctx, user.TelegramID, "Balance feature not available.")
		}
		balanceText, err := r.facade.HandleBalance(ctx, user.TelegramID)
		if err != nil {
			log.Printf("[telegram] failed to get balance for user %d: %v", user.TelegramID, err)
			return r.SendMessageWithTelegramID(ctx, user.TelegramID, "Failed to get your balance. Please try again later.")
		}
		if balanceText == "" {
			balanceText = "You have no balance information available."
		}
		return r.SendMessageWithTelegramID(ctx, user.TelegramID, balanceText)
	default:
		// Handle other csommands (if any)
		if r.facade == nil {
			return r.SendMessageWithTelegramID(ctx, user.TelegramID, "Command not available.")
		}
		// You can add more command handlers here as needed
		log.Printf("[telegram] unhandled command: %s for user %d", cmd, user.TelegramID)
		return r.SendMessageWithTelegramID(ctx, user.TelegramID, "Unknown command. Send /help for the list of commands.")
	}
}

func (r *RealTelegramBotAdapter) isAdmin(tgID int64) bool {
	_, ok := r.adminIDsMap[tgID]
	return ok
}
