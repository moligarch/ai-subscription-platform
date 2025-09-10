package telegram

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/rs/zerolog"

	"telegram-ai-subscription/internal/application"
	"telegram-ai-subscription/internal/config"
	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/domain/ports/adapter"
	"telegram-ai-subscription/internal/domain/ports/repository"
	"telegram-ai-subscription/internal/infra/metrics"
	red "telegram-ai-subscription/internal/infra/redis"
)

type commandHandler func(ctx context.Context, message *tgbotapi.Message) error

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

	log *zerolog.Logger
}

func NewRealTelegramBotAdapter(cfg *config.BotConfig, userRepo repository.UserRepository, facade *application.BotFacade, rateLimiter *red.RateLimiter, updateWorkers int, logger *zerolog.Logger) (*RealTelegramBotAdapter, error) {
	if cfg == nil || facade == nil || userRepo == nil {
		return nil, domain.ErrInvalidArgument
	}

	if updateWorkers <= 0 {
		updateWorkers = 5
	}

	bot, err := tgbotapi.NewBotAPI(cfg.Token)
	if err != nil {
		return nil, domain.ErrRequestFailed
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
		log:           logger,
	}, nil
}

func (r *RealTelegramBotAdapter) StartPolling(ctx context.Context) error {
	r.log.Info().Msg("telegram start pooling")
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
						r.log.Error().Err(err).Msgf("tg worker %d", id)
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

// adminOnly is a middleware that wraps a commandHandler to restrict access to admins.
func (r *RealTelegramBotAdapter) adminOnly(next commandHandler) commandHandler {
	return func(ctx context.Context, message *tgbotapi.Message) error {
		if _, isAdmin := r.adminIDsMap[message.From.ID]; !isAdmin {
			metrics.IncAdminCommand("/"+message.Command(), "unauthorized")
			return r.SendMessage(ctx, message.Chat.ID, "You are not authorized to use this command.")
		}
		metrics.IncAdminCommand("/"+message.Command(), "authorized")
		return next(ctx, message)
	}
}

// commandRoutes defines all available bot commands and their handlers.
func (r *RealTelegramBotAdapter) commandRoutes() map[string]commandHandler {
	return map[string]commandHandler{
		"start":  r.handleStartCommand,
		"plans":  r.handlePlansCommand,
		"status": r.handleStatusCommand,
		"buy":    r.handleBuyCommand,
		"chat":   r.handleChatCommand,
		"bye":    r.handleByeCommand,
		"help":   r.handleHelpCommand,

		// These handlers are wrapped in our adminOnly middleware.
		"create_plan":    r.adminOnly(r.handleCreatePlanCommand),
		"delete_plan":    r.adminOnly(r.handleDeletePlanCommand),
		"update_plan":    r.adminOnly(r.handleUpdatePlanCommand),
		"update_pricing": r.adminOnly(r.handleUpdatePricingCommand),
	}
}

func (r *RealTelegramBotAdapter) cbRoutes() map[string]cbHandler {
	return map[string]cbHandler{
		"cmd:menu":    r.menuCBRoute,
		"cmd:plans":   r.planCBRoute,
		"cmd:status":  r.statusCBRoute,
		"cmd:chat":    r.chatCBRoute,
		"cmd:bye":     r.chatEndCBRoute,
		"cmd:history": r.historyCBRoute,
	}
}

// Prefix-match callbacks
func (r *RealTelegramBotAdapter) cbPrefixRoutes() []prefixCB {
	return []prefixCB{
		{
			Prefix: "buy:",
			Fn:     r.buyPrefixCBRoute,
		},
		{
			Prefix: "chat:",
			Fn:     r.chatPrefixCBRoute,
		},
		{
			Prefix: "hist:cont:",
			Fn:     r.continueChatPrefixCBRoute,
		},
		{
			Prefix: "hist:del:",
			Fn:     r.deleteChatPrefixCBRoute,
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
	// 1. Determine command type for metrics and rate limiting
	var commandType string
	var tgUser *tgbotapi.User
	var chatID int64

	if update.CallbackQuery != nil {
		tgUser = update.CallbackQuery.From
		chatID = update.CallbackQuery.Message.Chat.ID
		parts := strings.Split(update.CallbackQuery.Data, ":")
		if len(parts) > 0 {
			commandType = "callback:" + parts[0]
		} else {
			commandType = "callback:unknown"
		}
	} else if update.Message != nil {
		tgUser = update.Message.From
		chatID = update.Message.Chat.ID
		if update.Message.IsCommand() {
			commandType = "/" + update.Message.Command()
		} else if update.Message.Text != "" {
			commandType = "message"
		} else {
			commandType = "other"
		}
	} else {
		return nil // Not an update we can handle
	}

	if tgUser == nil {
		return nil
	}

	metrics.IncTelegramCommand(commandType)

	// 2. Rate Limiting
	if r.rateLimiter != nil {
		allowed, err := r.rateLimiter.Allow(ctx, red.UserCommandKey(tgUser.ID, commandType), 20, time.Minute)
		if err != nil {
			r.log.Error().Err(err).Msg("rate limit error")
		} else if !allowed {
			metrics.IncRateLimitTriggered()
			return r.SendMessage(ctx, chatID, "Rate limit exceeded. Please try again later.")
		}
	}

	// 3. Route the update
	if update.CallbackQuery != nil {
		return r.handleQuery(ctx, update.CallbackQuery)
	}

	if update.Message.IsCommand() {
		if handler, ok := r.commandRoutes()[update.Message.Command()]; ok {
			return handler(ctx, update.Message)
		}
		return r.SendMessage(ctx, chatID, "Unknown command.")
	}

	if update.Message.Text != "" {
		reply, err := r.facade.HandleChatMessage(ctx, tgUser.ID, update.Message.Text)
		if err != nil {
			r.log.Error().Err(err).Int64("tg_id", tgUser.ID).Msg("HandleChatMessage failed")
			_ = r.SendMessage(ctx, chatID, "Sorry, I encountered an error.")
			return nil
		}
		if strings.TrimSpace(reply) != "" {
			return r.SendMessage(ctx, chatID, reply)
		}
	}

	return nil
}

func (r *RealTelegramBotAdapter) handleQuery(ctx context.Context, query *tgbotapi.CallbackQuery) error {
	if query == nil || query.From == nil {
		return domain.ErrInvalidArgument
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
			metrics.IncRateLimitTriggered()
			return r.SendMessage(ctx, chatID, "تعداد درخواست غیرمجاز. دسترسی شما به مدت 30 دقیقه محدود شد.")
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
		return r.SendMessage(ctx, telegramID, "پلنی یافت نشد.")
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
	rows = append(rows, []adapter.InlineButton{{Text: "◀️ منو اصلی", Data: "cmd:menu"}})
	return r.SendButtons(ctx, telegramID, "پلن های موجود برای خریداری:", rows)
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

	rows = append(rows, []adapter.InlineButton{{Text: "◀️ منو اصلی", Data: "cmd:menu"}})
	return r.SendButtons(ctx, telegramID, "مدل مدنظر خود را برای شروع مکالمه انتخاب کنید:", rows)
}

// sendEndChatButton renders a single End Chat button after chat starts.
func (r *RealTelegramBotAdapter) sendEndChatButton(ctx context.Context, telegramID int64) error {
	rows := [][]adapter.InlineButton{
		{{Text: "⏹ پایان", Data: "cmd:bye"}},
		{{Text: "◀️ منو اصلی", Data: "cmd:menu"}},
	}
	return r.SendButtons(ctx, telegramID, "Chat started. Type your message, or tap to end:", rows)
}

func (r *RealTelegramBotAdapter) sendHistoryMenu(ctx context.Context, telegramID int64) error {
	user, err := r.facade.UserUC.GetByTelegramID(ctx, telegramID)
	if err != nil || user == nil {
		return r.SendMessage(ctx, telegramID, "شما هنوز ثبتنام نکرده اید. برای ثبت نام از /start استفاده کنید.")
	}

	items, err := r.facade.ChatUC.ListHistory(ctx, user.ID, 0, 10)
	if err != nil {
		return r.SendMessage(ctx, telegramID, "دریافت تاریخچه مکالمات با خطا مواجه شد.")
	}
	if len(items) == 0 {
		rows := [][]adapter.InlineButton{{{Text: "◀️ منو اصلی", Data: "cmd:menu"}}}
		return r.SendButtons(ctx, telegramID, "مکالمه ای یافت نشد.", rows)
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
