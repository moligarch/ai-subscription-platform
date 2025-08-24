// File: internal/infra/adapters/telegram/real_bot.go
package telegram

import (
	"context"
	"errors"
	"log"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"telegram-ai-subscription/internal/application"
	"telegram-ai-subscription/internal/config"
	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/domain/ports/repository"
	red "telegram-ai-subscription/internal/infra/redis"
)

// RealTelegramBotAdapter uses tgbotapi to poll updates and delegates to BotFacade.
type RealTelegramBotAdapter struct {
	bot         *tgbotapi.BotAPI
	cfg         *config.BotConfig
	userRepo    repository.UserRepository
	facade      *application.BotFacade
	rateLimiter *red.RateLimiter

	adminIDsMap   map[int64]struct{}
	updateWorkers int
	cancelPolling context.CancelFunc
}

func NewRealTelegramBotAdapter(cfg *config.BotConfig, userRepo repository.UserRepository, facade *application.BotFacade, rateLimiter *red.RateLimiter, updateWorkers int) (*RealTelegramBotAdapter, error) {
	if cfg == nil {
		return nil, errors.New("bot config is nil")
	}
	if facade == nil {
		return nil, errors.New("bot facade is nil")
	}
	if userRepo == nil {
		return nil, errors.New("userRepo is nil")
	}
	if updateWorkers <= 0 {
		updateWorkers = 5
	}

	bot, err := tgbotapi.NewBotAPI(cfg.Token)
	if err != nil {
		return nil, err
	}

	adminMap := map[int64]struct{}{}
	for _, id := range cfg.AdminIDs {
		adminMap[id] = struct{}{}
	}

	return &RealTelegramBotAdapter{
		bot:           bot,
		cfg:           cfg,
		userRepo:      userRepo,
		facade:        facade,
		rateLimiter:   rateLimiter,
		adminIDsMap:   adminMap,
		updateWorkers: updateWorkers,
	}, nil
}

func (r *RealTelegramBotAdapter) StartPolling(ctx context.Context) error {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := r.bot.GetUpdatesChan(u)

	ctx, cancel := context.WithCancel(ctx)
	r.cancelPolling = cancel

	var wg sync.WaitGroup
	updateChan := make(chan tgbotapi.Update, 100)

	for i := 0; i < r.updateWorkers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case up := <-updateChan:
					if err := r.handleUpdate(ctx, up); err != nil {
						log.Printf("tg worker %d error: %v", id, err)
					}
				}
			}
		}(i)
	}

	for {
		select {
		case <-ctx.Done():
			close(updateChan)
			wg.Wait()
			return ctx.Err()
		case up := <-updates:
			updateChan <- up
		}
	}
}

func (r *RealTelegramBotAdapter) StopPolling() {
	if r.cancelPolling != nil {
		r.cancelPolling()
	}
}

// SendMessage implements the adapter port using internal user ID -> Telegram ID mapping via repo.
func (r *RealTelegramBotAdapter) SendMessage(ctx context.Context, tgID int64, text string) error {
	msg := tgbotapi.NewMessage(tgID, text)
	_, err := r.bot.Send(msg)
	return err
}

func (r *RealTelegramBotAdapter) handleUpdate(ctx context.Context, update tgbotapi.Update) error {
	if update.Message == nil {
		return nil
	}
	tgUser := update.Message.From
	if tgUser == nil {
		return nil
	}

	// Basic rate limiting per user per command
	cmd := strings.Fields(update.Message.Text)
	command := "message"
	if len(cmd) > 0 && strings.HasPrefix(cmd[0], "/") {
		command = cmd[0]
	}
	if r.rateLimiter != nil {
		allowed, err := r.rateLimiter.Allow(ctx, red.UserCommandKey(int64(tgUser.ID), command), 20, time.Minute)
		if err != nil {
			log.Printf("rate limit error: %v", err)
		} else if !allowed {
			return r.SendMessage(ctx, int64(tgUser.ID), "Rate limit exceeded. Please try again later.")
		}
	}

	// Resolve or register user via facade
	welcome := false
	if command == "/start" {
		welcome = true
	}
	if welcome {
		text, err := r.facade.HandleStart(ctx, int64(tgUser.ID), tgUser.UserName)
		if err != nil {
			return r.SendMessage(ctx, int64(tgUser.ID), "Failed to initialize user.")
		}
		return r.SendMessage(ctx, int64(tgUser.ID), text)
	}

	switch command {
	case "/plans":
		text, err := r.facade.HandlePlans(ctx, int64(tgUser.ID))
		if err != nil {
			text = "Failed to load plans."
		}
		return r.SendMessage(ctx, int64(tgUser.ID), text)
	case "/buy":
		if len(cmd) < 2 {
			return r.SendMessage(ctx, int64(tgUser.ID), "Usage: /buy <plan_id>")
		}
		planID := cmd[1]
		text, err := r.facade.HandleSubscribe(ctx, int64(tgUser.ID), planID)
		if err != nil {
			text = "Failed to initiate payment."
		}
		return r.SendMessage(ctx, int64(tgUser.ID), text)
	case "/status", "/myplan":
		text, err := r.facade.HandleStatus(ctx, int64(tgUser.ID))
		if err != nil {
			text = "Failed to get status."
		}
		return r.SendMessage(ctx, int64(tgUser.ID), text)
	case "/balance":
		text, err := r.facade.HandleBalance(ctx, int64(tgUser.ID))
		if err != nil {
			text = "Failed to get balance."
		}
		return r.SendMessage(ctx, int64(tgUser.ID), text)
	case "/chat":
		// Optional: /chat <model>
		model := ""
		if len(cmd) >= 2 {
			model = cmd[1]
		}
		if model == "" {
			models, _ := r.facade.ChatUC.ListModels(ctx)
			if len(models) == 0 {
				model = "gpt-4o-mini"
			} else {
				model = models[0]
			}
		}
		text, err := r.facade.HandleStartChat(ctx, int64(tgUser.ID), model)
		if err != nil {
			if errors.Is(err, domain.ErrActiveChatExists) {
				text = "You already have an active chat session."
			} else {
				text = "Failed to start chat."
			}
		}
		return r.SendMessage(ctx, int64(tgUser.ID), text)
	case "/bye":
		// Resolve internal user by Telegram ID
		user, err := r.facade.UserUC.GetByTelegramID(ctx, int64(tgUser.ID))
		if err != nil || user == nil {
			return r.SendMessage(ctx, int64(tgUser.ID), "No user found. Try /start first.")
		}

		// Find the active session for this internal user
		sess, err := r.facade.ChatUC.FindActiveSession(ctx, user.ID)
		if err != nil || sess == nil {
			return r.SendMessage(ctx, int64(tgUser.ID), "No active chat session found.")
		}

		// End the found session via the facade
		text, err := r.facade.HandleEndChat(ctx, int64(tgUser.ID), sess.ID)
		if err != nil {
			text = "Failed to end chat."
		}
		return r.SendMessage(ctx, int64(tgUser.ID), text)
	case "/help":
		reply := "Commands:\n/start - init\n/plans - list plans\n/status - subscription\n/chat - start chat\n/bye - end chat"
		return r.SendMessage(ctx, int64(tgUser.ID), reply)
	default:
		// Chat flow: forward any text to HandleChatMessage if a session exists
		if update.Message.Text != "" {
			reply, err := r.facade.HandleChatMessage(ctx, int64(tgUser.ID), update.Message.Text)
			if err != nil {
				return r.SendMessage(ctx, int64(tgUser.ID), "Error: "+err.Error())
			}
			if strings.TrimSpace(reply) != "" {
				return r.SendMessage(ctx, int64(tgUser.ID), reply)
			}
		}

		return nil
	}
}
