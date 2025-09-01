package telegram

import (
	"context"
	"errors"
	"log"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"telegram-ai-subscription/internal/application"
	"telegram-ai-subscription/internal/config"
	"telegram-ai-subscription/internal/domain/ports/adapter"
	"telegram-ai-subscription/internal/domain/ports/repository"
	derror "telegram-ai-subscription/internal/error"
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

type cbHandler func(ctx context.Context, chatID int64, data string) error

// Exact-match callbacks
func (r *RealTelegramBotAdapter) cbRoutes() map[string]cbHandler {
	return map[string]cbHandler{
		"cmd:menu": func(ctx context.Context, id int64, _ string) error {
			return r.sendMainMenu(ctx, id, "Choose an action:")
		},
		"cmd:plans": func(ctx context.Context, id int64, _ string) error { return r.sendPlansMenu(ctx, id) },
		"cmd:status": func(ctx context.Context, id int64, _ string) error {
			text, err := r.facade.HandleStatus(ctx, id)
			if err != nil {
				text = "Failed to get status."
			}
			return r.sendMainMenu(ctx, id, text)
		},
		"cmd:chat": func(ctx context.Context, id int64, _ string) error { return r.sendModelMenu(ctx, id) },
		"cmd:bye": func(ctx context.Context, id int64, _ string) error {
			user, err := r.facade.UserUC.GetByTelegramID(ctx, id)
			if err != nil || user == nil {
				return r.SendMessage(ctx, id, "No user found. Try /start first.")
			}
			sess, err := r.facade.ChatUC.FindActiveSession(ctx, user.ID)
			if err != nil || sess == nil {
				return r.SendMessage(ctx, id, "No active chat session found.")
			}
			text, err := r.facade.HandleEndChat(ctx, id, sess.ID)
			if err != nil {
				text = "Failed to end chat."
			}
			return r.SendMessage(ctx, id, text)
		},
		"cmd:history": func(ctx context.Context, id int64, _ string) error { return r.sendHistoryMenu(ctx, id) },
	}
}

// Prefix-match callbacks
func (r *RealTelegramBotAdapter) cbPrefixRoutes() []struct {
	Prefix string
	Fn     cbHandler
} {
	return []struct {
		Prefix string
		Fn     cbHandler
	}{
		{
			Prefix: "buy:",
			Fn: func(ctx context.Context, id int64, data string) error {
				planID := strings.TrimPrefix(data, "buy:")
				_ = r.SendMessage(ctx, id, "در حال پردازش درخواست شما هستیم...")
				text, err := r.facade.HandleSubscribe(ctx, id, planID)
				if err != nil {
					text = "Failed to initiate payment."
				}
				if url := extractFirstURL(text); url != "" {
					rows := [][]adapter.InlineButton{
						{{Text: "Pay now", URL: url}},
						{{Text: "◀️ Menu", Data: "cmd:menu"}},
					}
					return r.SendButtons(ctx, id, text, rows)
				}
				return r.SendMessage(ctx, id, text)
			},
		},
		{
			Prefix: "chat:",
			Fn: func(ctx context.Context, id int64, data string) error {
				model := strings.TrimPrefix(data, "chat:")
				text, err := r.facade.HandleStartChat(ctx, id, model)
				if err != nil {
					if errors.Is(err, derror.ErrActiveChatExists) {
						text = "You already have an active chat session."
					} else {
						text = "Failed to start chat."
					}
				}
				if err := r.SendMessage(ctx, id, text); err != nil {
					return err
				}
				return r.sendEndChatButton(ctx, id)
			},
		},
		{
			Prefix: "hist:cont:",
			Fn: func(ctx context.Context, id int64, data string) error {
				sessionID := strings.TrimPrefix(data, "hist:cont:")
				user, err := r.facade.UserUC.GetByTelegramID(ctx, id)
				if err != nil || user == nil {
					return r.SendMessage(ctx, id, "No user found. Try /start first.")
				}
				if err := r.facade.ChatUC.SwitchActiveSession(ctx, user.ID, sessionID); err != nil {
					return r.SendMessage(ctx, id, "Failed to switch to this chat.")
				}
				if err := r.SendMessage(ctx, id, "This chat is now active. You can continue the conversation."); err != nil {
					return err
				}
				// show End Chat button like after /chat
				return r.sendEndChatButton(ctx, id)
			},
		},
		{
			Prefix: "hist:del:",
			Fn: func(ctx context.Context, id int64, data string) error {
				sessionID := strings.TrimPrefix(data, "hist:del:")
				if err := r.facade.ChatUC.DeleteSession(ctx, sessionID); err != nil {
					return r.SendMessage(ctx, id, "Failed to delete this chat.")
				}
				// Refresh history list after deletion
				return r.sendHistoryMenu(ctx, id)
			},
		},
	}
}

// SendMessage implements the adapter port using internal user ID -> Telegram ID mapping via repo.
func (r *RealTelegramBotAdapter) SendMessage(ctx context.Context, tgID int64, text string) error {
	msg := tgbotapi.NewMessage(tgID, text)
	_, err := r.bot.Send(msg)
	return err
}

// SendButtons sends a message with inline buttons using tgbotapi.
// - If btn.URL is set, the button opens a link
// - Else if btn.Data is set, the button sends callback data
// - Else a safe fallback uses btn.Text as callback data
func (b *RealTelegramBotAdapter) SendButtons(
	ctx context.Context,
	telegramID int64,
	text string,
	rows [][]adapter.InlineButton,
) error {
	// Support early cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Build inline keyboard rows
	kbRows := make([][]tgbotapi.InlineKeyboardButton, 0, len(rows))
	for _, row := range rows {
		if len(row) == 0 {
			continue
		}
		r := make([]tgbotapi.InlineKeyboardButton, 0, len(row))
		for _, btn := range row {
			label := strings.TrimSpace(btn.Text)
			if label == "" {
				label = "•"
			}
			var kb tgbotapi.InlineKeyboardButton
			switch {
			case btn.URL != "":
				kb = tgbotapi.NewInlineKeyboardButtonURL(label, btn.URL)
			case btn.Data != "":
				kb = tgbotapi.NewInlineKeyboardButtonData(label, btn.Data)
			default:
				// safe fallback: use text as callback data
				kb = tgbotapi.NewInlineKeyboardButtonData(label, label)
			}
			r = append(r, kb)
		}
		kbRows = append(kbRows, r)
	}

	markup := tgbotapi.NewInlineKeyboardMarkup(kbRows...)
	msg := tgbotapi.NewMessage(telegramID, text)
	msg.ReplyMarkup = markup

	// NOTE: field name is commonly 'bot' on the adapter; adjust if your struct uses a different name.
	// This matches the same underlying client you already use in SendMessage.
	_, err := b.bot.Send(msg)
	return err
}

func (r *RealTelegramBotAdapter) handleUpdate(ctx context.Context, update tgbotapi.Update) error {
	// ----- Inline button callbacks -----
	if update.CallbackQuery != nil {
		return r.handleQuery(ctx, update.CallbackQuery)
	}

	// ----- Regular messages -----
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

	// /start → welcome + main menu buttons
	if command == "/start" {
		text, err := r.facade.HandleStart(ctx, int64(tgUser.ID), tgUser.UserName)
		if err != nil {
			return r.SendMessage(ctx, int64(tgUser.ID), "Failed to initialize user.")
		}
		// Show main menu as buttons (includes End Chat if already active)
		if err := r.sendMainMenu(ctx, int64(tgUser.ID), text); err != nil {
			// Fallback plain message on error
			return r.SendMessage(ctx, int64(tgUser.ID), text)
		}
		return nil
	}

	switch command {
	case "/plans", "/plan":
		// Show plan list with buy buttons
		return r.sendPlansMenu(ctx, int64(tgUser.ID))

	case "/buy":
		if len(cmd) < 2 {
			return r.SendMessage(ctx, int64(tgUser.ID), "Usage: /buy <plan_id>")
		}
		planID := cmd[1]
		text, err := r.facade.HandleSubscribe(ctx, int64(tgUser.ID), planID)
		if err != nil {
			text = "Failed to initiate payment."
		}
		if url := extractFirstURL(text); url != "" {
			rows := [][]adapter.InlineButton{
				{{Text: "Pay now", URL: url}},
			}
			return r.SendButtons(ctx, int64(tgUser.ID), text, rows)
		}
		return r.SendMessage(ctx, int64(tgUser.ID), text)

	case "/status", "/myplan":
		text, err := r.facade.HandleStatus(ctx, int64(tgUser.ID))
		if err != nil {
			text = "Failed to get status."
		}
		return r.sendMainMenu(ctx, int64(tgUser.ID), text)

	case "/balance":
		text, err := r.facade.HandleBalance(ctx, int64(tgUser.ID))
		if err != nil {
			text = "Failed to get balance."
		}
		return r.SendMessage(ctx, int64(tgUser.ID), text)

	case "/chat":
		// If model not provided, show model picker buttons
		if len(cmd) < 2 || strings.TrimSpace(cmd[1]) == "" {
			return r.sendModelMenu(ctx, int64(tgUser.ID))
		}
		model := cmd[1]
		text, err := r.facade.HandleStartChat(ctx, int64(tgUser.ID), model)
		if err != nil {
			if errors.Is(err, derror.ErrActiveChatExists) {
				text = "You already have an active chat session."
			} else {
				text = "Failed to start chat."
			}
		}
		if err := r.SendMessage(ctx, int64(tgUser.ID), text); err != nil {
			return err
		}
		// After starting chat, show End Chat button
		return r.sendEndChatButton(ctx, int64(tgUser.ID))

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

func (r *RealTelegramBotAdapter) handleQuery(ctx context.Context, query *tgbotapi.CallbackQuery) error {
	if query == nil || query.From == nil {
		return errors.New("invalid callback query")
	}

	// Stop telegram spinner when we return
	defer func() { _, _ = r.bot.Request(tgbotapi.NewCallback(query.ID, "")) }()

	var chatID int64
	if query.Message != nil && query.Message.Chat != nil {
		chatID = query.Message.Chat.ID
	} else {
		chatID = int64(query.From.ID)
	}
	if chatID == 0 {
		return nil
	}

	data := strings.TrimSpace(query.Data)

	// Rate limit for callbacks
	if r.rateLimiter != nil {
		if allowed, err := r.rateLimiter.Allow(ctx, red.UserCommandKey(chatID, "cb:"+data), 30, time.Minute); err == nil && !allowed {
			return r.SendMessage(ctx, chatID, "Rate limit exceeded. Please try again later.")
		}
	}

	// Exact matches
	if fn, ok := r.cbRoutes()[data]; ok {
		return fn(ctx, chatID, data)
	}
	// Prefix matches
	for _, pr := range r.cbPrefixRoutes() {
		if strings.HasPrefix(data, pr.Prefix) {
			return pr.Fn(ctx, chatID, data)
		}
	}
	return errors.New("unknown callback data")
}

// sendMainMenu shows the main actions as inline buttons.
// If the user already has an active chat, it also shows an "End Chat" button.
func (r *RealTelegramBotAdapter) sendMainMenu(ctx context.Context, telegramID int64, intro string) error {
	hasActive := false
	if user, err := r.facade.UserUC.GetByTelegramID(ctx, telegramID); err == nil && user != nil {
		if sess, _ := r.facade.ChatUC.FindActiveSession(ctx, user.ID); sess != nil {
			hasActive = true
		}
	}

	rows := [][]adapter.InlineButton{
		{{Text: "🛒 Plans", Data: "cmd:plans"}},
		{{Text: "📊 Status", Data: "cmd:status"}},
		{{Text: "💾 History", Data: "cmd:history"}},
		{{Text: "💬 Start Chat", Data: "cmd:chat"}},
	}
	if hasActive {
		rows = append(rows, []adapter.InlineButton{{Text: "⏹ End Chat", Data: "cmd:bye"}})
	}
	if strings.TrimSpace(intro) == "" {
		intro = "Welcome! Choose an action:"
	}
	return r.SendButtons(ctx, telegramID, intro, rows)
}

// sendPlansMenu lists all plans as buttons; pressing a plan starts the buy flow.
func (r *RealTelegramBotAdapter) sendPlansMenu(ctx context.Context, telegramID int64) error {
	plans, err := r.facade.PlanUC.List(ctx)
	if err != nil || len(plans) == 0 {
		return r.SendMessage(ctx, telegramID, "No plans available.")
	}
	rows := make([][]adapter.InlineButton, 0, len(plans))
	for _, p := range plans {
		label := p.Name
		// Minimal extra info
		if p.PriceIRR > 0 && p.DurationDays > 0 {
			label = p.Name + " — " + formatIRR(p.PriceIRR) + " / " + strconv.Itoa(p.DurationDays) + "d"
		}
		rows = append(rows, []adapter.InlineButton{{Text: label, Data: "buy:" + p.ID}})
	}
	rows = append(rows, []adapter.InlineButton{{Text: "◀️ Menu", Data: "cmd:menu"}})
	return r.SendButtons(ctx, telegramID, "Available plans (tap to buy):", rows)
}

// sendModelMenu shows available models as buttons.
func (r *RealTelegramBotAdapter) sendModelMenu(ctx context.Context, telegramID int64) error {
	models, _ := r.facade.ChatUC.ListModels(ctx)
	if len(models) == 0 {
		// default if none reported
		models = []string{"gpt-4o-mini"}
	}
	rows := make([][]adapter.InlineButton, 0, len(models))
	for _, m := range models {
		rows = append(rows, []adapter.InlineButton{{Text: m, Data: "chat:" + m}})
	}

	rows = append(rows, []adapter.InlineButton{{Text: "◀️ Menu", Data: "cmd:menu"}})
	return r.SendButtons(ctx, telegramID, "Choose a model:", rows)
}

// sendEndChatButton renders a single End Chat button after chat starts.
func (r *RealTelegramBotAdapter) sendEndChatButton(ctx context.Context, telegramID int64) error {
	rows := [][]adapter.InlineButton{
		{{Text: "⏹ End Chat", Data: "cmd:bye"}},
		{{Text: "◀️ Menu", Data: "cmd:menu"}},
	}
	return r.SendButtons(ctx, telegramID, "Chat started. Type your message, or tap to end:", rows)
}

func (r *RealTelegramBotAdapter) sendHistoryMenu(ctx context.Context, telegramID int64) error {
	user, err := r.facade.UserUC.GetByTelegramID(ctx, telegramID)
	if err != nil || user == nil {
		return r.SendMessage(ctx, telegramID, "No user found. Try /start first.")
	}

	items, err := r.facade.ChatUC.ListHistory(ctx, user.ID, 0, 10)
	if err != nil {
		return r.SendMessage(ctx, telegramID, "Failed to load history.")
	}
	if len(items) == 0 {
		rows := [][]adapter.InlineButton{{{Text: "◀️ Menu", Data: "cmd:menu"}}}
		return r.SendButtons(ctx, telegramID, "No past chats found.", rows)
	}

	// Build rows: one row per session with [Continue] [Delete]
	rows := make([][]adapter.InlineButton, 0, len(items)+1)
	for idx, it := range items {
		label := it.FirstMessage
		if strings.TrimSpace(label) == "" {
			label = "(empty)"
		}
		// prefix with index and model for clarity
		display := strconv.Itoa(idx+1) + ") [" + it.Model + "] " + label
		rows = append(rows, []adapter.InlineButton{
			{Text: display, Data: "hist:cont:" + it.SessionID},
			{Text: "🗑 Delete", Data: "hist:del:" + it.SessionID},
		})
	}
	// Footer row
	rows = append(rows, []adapter.InlineButton{{Text: "◀️ Menu", Data: "cmd:menu"}})

	return r.SendButtons(ctx, telegramID, "🗂️ Your chats:", rows)
}


var httpURLRe = regexp.MustCompile(`https?:\/\/(?:[-\w]+\.)+[a-zA-Z]{2,}(?:\/[^\s\\\n]*)`)

func extractFirstURL(s string) string {
	if s == "" {
		return ""
	}
	loc := httpURLRe.FindStringIndex(s)
	if loc == nil {
		return ""
	}
	return s[loc[0]:loc[1]]
}

// simple IRR pretty printer; optional
func formatIRR(v int64) string {
	s := strconv.FormatInt(v, 10)
	// add thousands separators
	n := len(s)
	if n <= 3 {
		return s + " IRR"
	}
	var b strings.Builder
	pre := n % 3
	if pre == 0 {
		pre = 3
	}
	b.WriteString(s[:pre])
	for i := pre; i < n; i += 3 {
		b.WriteString(",")
		b.WriteString(s[i : i+3])
	}
	return b.String() + " IRR"
}
