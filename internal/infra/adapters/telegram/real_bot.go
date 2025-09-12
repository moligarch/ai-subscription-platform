package telegram

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/rs/zerolog"

	"telegram-ai-subscription/internal/application"
	"telegram-ai-subscription/internal/config"
	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/adapter"
	"telegram-ai-subscription/internal/domain/ports/repository"
	"telegram-ai-subscription/internal/infra/i18n"
	"telegram-ai-subscription/internal/infra/metrics"
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

	translator *i18n.Translator
	log        *zerolog.Logger
}

var _ adapter.TelegramBotAdapter = (*RealTelegramBotAdapter)(nil)

func NewRealTelegramBotAdapter(
	cfg *config.BotConfig,
	userRepo repository.UserRepository,
	facade *application.BotFacade,
	translator *i18n.Translator,
	rateLimiter *red.RateLimiter,
	updateWorkers int,
	logger *zerolog.Logger,
) (*RealTelegramBotAdapter, error) {
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
		translator:    translator,
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

// SendMessage is the single method for sending any kind of message.
func (r *RealTelegramBotAdapter) SendMessage(ctx context.Context, params adapter.SendMessageParams) error {
	text := params.Text
	if params.ParseMode == tgbotapi.ModeMarkdownV2 {
		text = r.escapeMarkdownV2(params.Text)
	}
	msg := tgbotapi.NewMessage(params.ChatID, text)

	// Apply ParseMode if provided.
	if params.ParseMode != "" {
		msg.ParseMode = params.ParseMode
	}

	// Apply ReplyMarkup if provided.
	if params.ReplyMarkup != nil {
		markup := params.ReplyMarkup
		if markup.IsInline {
			// Build an InlineKeyboardMarkup
			kbRows := make([][]tgbotapi.InlineKeyboardButton, 0, len(markup.Buttons))
			for _, row := range markup.Buttons {
				r := make([]tgbotapi.InlineKeyboardButton, 0, len(row))
				for _, btn := range row {
					var kb tgbotapi.InlineKeyboardButton
					if btn.URL != "" {
						kb = tgbotapi.NewInlineKeyboardButtonURL(btn.Text, btn.URL)
					} else {
						kb = tgbotapi.NewInlineKeyboardButtonData(btn.Text, btn.Data)
					}
					r = append(r, kb)
				}
				kbRows = append(kbRows, r)
			}
			msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(kbRows...)
		} else {
			// Build a ReplyKeyboardMarkup
			kbRows := make([][]tgbotapi.KeyboardButton, 0, len(markup.Buttons))
			for _, row := range markup.Buttons {
				r := make([]tgbotapi.KeyboardButton, 0, len(row))
				for _, btn := range row {
					kb := tgbotapi.NewKeyboardButton(btn.Text)
					if btn.RequestContact {
						kb.RequestContact = true
					}
					r = append(r, kb)
				}
				kbRows = append(kbRows, r)
			}
			replyKeyboard := tgbotapi.NewReplyKeyboard(kbRows...)
			replyKeyboard.OneTimeKeyboard = markup.IsOneTime
			replyKeyboard.Selective = markup.IsPersonal
			msg.ReplyMarkup = replyKeyboard
		}
	}

	_, err := r.bot.Send(msg)
	return err
}

// SetMenuCommands configures the bot's persistent menu for a specific user.
func (r *RealTelegramBotAdapter) SetMenuCommands(ctx context.Context, chatID int64, isAdmin bool) error {
	// Define commands for regular users
	userCommands := []tgbotapi.BotCommand{
		{Command: "start", Description: r.translator.T("menu_restart")},
		{Command: "plans", Description: r.translator.T("menu_plans")},
		{Command: "status", Description: r.translator.T("menu_status")},
		{Command: "history", Description: r.translator.T("menu_history")},
		{Command: "settings", Description: r.translator.T("menu_settings")},
		{Command: "help", Description: r.translator.T("menu_help")},
	}

	commands := userCommands
	// If the user is an admin, add admin-specific commands
	if isAdmin {
		adminCommands := []tgbotapi.BotCommand{
			{Command: "create_plan", Description: "➕ Create Plan"},
			{Command: "update_plan", Description: "✏️ Update Plan"},
			{Command: "delete_plan", Description: "🗑️ Delete Plan"},
			{Command: "update_pricing", Description: "💲 Update Pricing"},
		}
		// Prepend admin commands to the user commands
		commands = append(adminCommands, userCommands...)
	}

	// Set the commands for the specific chat with the user
	scope := tgbotapi.NewBotCommandScopeChat(chatID)
	config := tgbotapi.NewSetMyCommandsWithScope(scope, commands...)

	_, err := r.bot.Request(config)
	return err
}

func (r *RealTelegramBotAdapter) handleUpdate(ctx context.Context, update tgbotapi.Update) error {
	var tgUser *tgbotapi.User
	var chatID int64
	var message *tgbotapi.Message

	// 1. Uniformly extract user, chat, and message info from the update.
	if update.CallbackQuery != nil {
		tgUser = update.CallbackQuery.From
		if update.CallbackQuery.Message != nil {
			chatID = update.CallbackQuery.Message.Chat.ID
		}
	} else if update.Message != nil {
		tgUser = update.Message.From
		chatID = update.Message.Chat.ID
		message = update.Message
	} else {
		return nil // Not an update we can handle.
	}

	if tgUser == nil {
		return nil
	}

	// 2. Get or create the user record.
	user, err := r.facade.UserUC.RegisterOrFetch(ctx, tgUser.ID, tgUser.UserName)
	if err != nil {
		r.log.Error().Err(err).Int64("tg_id", tgUser.ID).Msg("failed to register or fetch user")
		return r.SendMessage(ctx, adapter.SendMessageParams{
			ChatID: chatID,
			Text:   r.translator.T("error_generic"),
		})
	}

	// --- ROUTING LOGIC ---

	// 3. HIGHEST PRIORITY: Handle the mandatory registration flow.
	if user.RegistrationStatus == model.RegistrationStatusPending {
		// Callbacks from pending users (e.g., "Verify", "Cancel") MUST be handled by the callback router.
		if update.CallbackQuery != nil {
			return r.handleQuery(ctx, update.CallbackQuery)
		}
		// The /start command should always be allowed to (re)start the flow.
		if message != nil && message.IsCommand() && message.Command() == "start" {
			return r.handleStartCommand(ctx, message)
		}
		// Any other message is an answer to a registration question.
		if message != nil {
			return r.handleRegistrationMessage(ctx, message)
		}
		return nil // Ignore other update types (e.g., photos) during registration.
	}

	// 4. SECOND PRIORITY: Check for any other active conversational state (like activation codes).
	state, err := r.facade.UserUC.GetConversationState(ctx, tgUser.ID)
	if err != nil && !errors.Is(err, redis.Nil) {
		r.log.Error().Err(err).Int64("tg_id", tgUser.ID).Msg("failed to get conversation state")
		return r.SendMessage(ctx, adapter.SendMessageParams{ChatID: chatID, Text: r.translator.T("error_generic")})
	}

	if state != nil {
		if message != nil {
			return r.handleConversationalReply(ctx, message, state)
		}
		// Callbacks are still handled by the normal callback router.
		if update.CallbackQuery != nil {
			return r.handleQuery(ctx, update.CallbackQuery)
		}
	}

	// 5. DEFAULT: Normal operation for fully registered users with no active conversation.
	var commandType string
	if update.CallbackQuery != nil {
		parts := strings.Split(update.CallbackQuery.Data, ":")
		commandType = "callback:" + parts[0]
	} else if message != nil && message.IsCommand() {
		commandType = "/" + message.Command()
	} else {
		commandType = "message"
	}
	metrics.IncTelegramCommand(commandType)

	if r.rateLimiter != nil {
		allowed, err := r.rateLimiter.Allow(ctx, red.UserCommandKey(tgUser.ID, commandType), 20, time.Minute)
		if err != nil {
			r.log.Error().Err(err).Msg("rate limit error")
		} else if !allowed {
			metrics.IncRateLimitTriggered()
			return r.SendMessage(ctx, adapter.SendMessageParams{ChatID: chatID, Text: r.translator.T("rate_limit_exceeded")})
		}
	}

	// Route to appropriate handlers
	if update.CallbackQuery != nil {
		return r.handleQuery(ctx, update.CallbackQuery)
	}
	if message.IsCommand() {
		if handler, ok := r.commandRoutes()[message.Command()]; ok {
			return handler(ctx, message)
		}
		return r.SendMessage(ctx, adapter.SendMessageParams{ChatID: chatID, Text: r.translator.T("unknown_command")})
	}
	if message.Text != "" {
		reply, err := r.facade.HandleChatMessage(ctx, tgUser.ID, message.Text)
		if err != nil {
			r.log.Error().Err(err).Int64("tg_id", tgUser.ID).Msg("HandleChatMessage failed")
			_ = r.SendMessage(ctx, adapter.SendMessageParams{ChatID: chatID, Text: r.translator.T("error_generic")})
			return nil
		}
		if strings.TrimSpace(reply) != "" {
			return r.SendMessage(ctx, adapter.SendMessageParams{ChatID: chatID, Text: reply})
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
			return r.SendMessage(ctx, adapter.SendMessageParams{
				ChatID: chatID,
				Text:   r.translator.T("rate_limit_exceeded"),
			})
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

	rows := [][]adapter.Button{
		{{Text: r.translator.T("button_plans"), Data: "cmd:plans"}},
		{{Text: r.translator.T("button_status"), Data: "cmd:status"}},
		{{Text: r.translator.T("button_history"), Data: "cmd:history"}},
		{{Text: r.translator.T("button_start_chat"), Data: "cmd:chat"}},
	}
	if hasActive {
		rows = append(rows, []adapter.Button{{Text: r.translator.T("button_end_chat"), Data: "cmd:bye"}})
	}

	markup := adapter.ReplyMarkup{Buttons: rows, IsInline: true}
	return r.SendMessage(ctx, adapter.SendMessageParams{
		ChatID:      telegramID,
		Text:        intro,
		ReplyMarkup: &markup,
	})
}

// sendPlansMenu lists all plans as buttons; pressing a plan starts the buy flow.
func (r *RealTelegramBotAdapter) sendPlansMenu(ctx context.Context, telegramID int64) error {
	plans, err := r.facade.PlanUC.List(ctx)
	if err != nil {
		return r.SendMessage(ctx, adapter.SendMessageParams{
			ChatID: telegramID,
			Text:   r.translator.T("error_generic"),
		}) // Localized
	}
	if len(plans) == 0 {
		return r.SendMessage(ctx, adapter.SendMessageParams{
			ChatID: telegramID,
			Text:   r.translator.T("no_plan_header"),
		}) // Localized
	}

	rows := make([][]adapter.Button, 0, len(plans)+1)
	for _, p := range plans {
		label := fmt.Sprintf("%s — %s / %d روز", p.Name, formatIRR(p.PriceIRR), p.DurationDays)
		rows = append(rows, []adapter.Button{{Text: label, Data: "view_plan:" + p.ID}})
	}
	rows = append(rows, []adapter.Button{{Text: r.translator.T("back_to_menu"), Data: "cmd:menu"}})

	markup := adapter.ReplyMarkup{Buttons: rows, IsInline: true}
	return r.SendMessage(ctx, adapter.SendMessageParams{
		ChatID:      telegramID,
		Text:        r.translator.T("plans_header"),
		ReplyMarkup: &markup,
	})
	// Localized
}

// sendModelMenu shows available models as buttons.
func (r *RealTelegramBotAdapter) sendModelMenu(ctx context.Context, telegramID int64) error {
	user, err := r.userRepo.FindByTelegramID(ctx, repository.NoTX, telegramID)
	if err != nil {
		return fmt.Errorf("user not found: %w", err)
	}

	models, _ := r.facade.ChatUC.ListModels(ctx, user.ID)
	if len(models) == 0 {
		models = nil
	}

	rows := make([][]adapter.Button, 0, len(models)+1)
	for _, m := range models {
		rows = append(rows, []adapter.Button{{Text: m, Data: "chat:" + m}})
	}
	rows = append(rows, []adapter.Button{{Text: r.translator.T("back_to_menu"), Data: "cmd:menu"}})

	markup := adapter.ReplyMarkup{Buttons: rows, IsInline: true}
	return r.SendMessage(ctx, adapter.SendMessageParams{
		ChatID:      telegramID,
		Text:        r.translator.T("model_menu_header"),
		ParseMode:   tgbotapi.ModeMarkdownV2,
		ReplyMarkup: &markup,
	}) // Localized
}

// sendEndChatButton renders a single End Chat button after chat starts.
func (r *RealTelegramBotAdapter) sendEndChatButton(ctx context.Context, telegramID int64) error {
	rows := [][]adapter.Button{
		{{Text: r.translator.T("button_end_chat"), Data: "cmd:bye"}},
		{{Text: r.translator.T("back_to_menu"), Data: "cmd:menu"}},
	}
	markup := adapter.ReplyMarkup{Buttons: rows, IsInline: true}
	return r.SendMessage(ctx, adapter.SendMessageParams{
		ChatID:      telegramID,
		Text:        r.translator.T("success_chat_continue"),
		ParseMode:   tgbotapi.ModeMarkdownV2,
		ReplyMarkup: &markup,
	}) // Localized
}

func (r *RealTelegramBotAdapter) sendHistoryMenu(ctx context.Context, telegramID int64) error {
	user, err := r.facade.UserUC.GetByTelegramID(ctx, telegramID)
	if err != nil || user == nil {
		return r.SendMessage(ctx, adapter.SendMessageParams{
			ChatID: telegramID,
			Text:   r.translator.T("error_user_not_found"),
		}) // Localized
	}

	items, err := r.facade.ChatUC.ListHistory(ctx, user.ID, 0, 10)
	if err != nil {
		return r.SendMessage(ctx, adapter.SendMessageParams{
			ChatID: telegramID,
			Text:   r.translator.T("error_generic"),
		}) // Localized
	}
	if len(items) == 0 {
		markup := adapter.ReplyMarkup{
			Buttons:  [][]adapter.Button{{{Text: r.translator.T("back_to_menu"), Data: "cmd:menu"}}},
			IsInline: true,
		}
		return r.SendMessage(ctx, adapter.SendMessageParams{
			ChatID:      telegramID,
			Text:        r.translator.T("history_empty"),
			ParseMode:   tgbotapi.ModeMarkdownV2,
			ReplyMarkup: &markup,
		}) // Localized
	}

	rows := make([][]adapter.Button, 0, len(items)+1)
	for idx, it := range items {
		label := it.FirstMessage
		if strings.TrimSpace(label) == "" {
			label = "(خالی)"
		}
		if r := []rune(label); len(r) > 25 {
			label = string(r[:25]) + "…"
		}

		display := fmt.Sprintf("%d) [%s] %s", idx+1, it.Model, label)
		rows = append(rows, []adapter.Button{
			{Text: display, Data: "hist:cont:" + it.SessionID},
			{Text: r.translator.T("button_delete"), Data: "hist:del:" + it.SessionID},
		})
	}
	rows = append(rows, []adapter.Button{{Text: r.translator.T("back_to_menu"), Data: "cmd:menu"}})

	markup := adapter.ReplyMarkup{Buttons: rows, IsInline: true}
	return r.SendMessage(ctx, adapter.SendMessageParams{
		ChatID:      telegramID,
		Text:        r.translator.T("history_menu_header"),
		ParseMode:   tgbotapi.ModeMarkdownV2,
		ReplyMarkup: &markup,
	}) // Localized
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

// It will safely escape any string for use in MarkdownV2.
func (r *RealTelegramBotAdapter) EscapeMarkdownV2(s string) string {
	// List of all characters that must be escaped in MarkdownV2
	replacer := strings.NewReplacer(
		"_", "\\_" /*"*", "\\*", */, "[", "\\[", "]", "\\]", "(", "\\(", ")", "\\)",
		"~", "\\~" /*, "`", "\\`"*/, ">", "\\>", "#", "\\#", "+", "\\+", "-", "\\-",
		"=", "\\=", "|", "\\|", "{", "\\{", "}", "\\}", ".", "\\.", "!", "\\!",
	)
	return replacer.Replace(s)
}
