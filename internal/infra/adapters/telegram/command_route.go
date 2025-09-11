package telegram

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/domain/model"
	"telegram-ai-subscription/internal/domain/ports/adapter"
	"telegram-ai-subscription/internal/infra/metrics"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type commandHandler func(ctx context.Context, message *tgbotapi.Message) error

// commandRoutes defines all available bot commands and their handlers.
func (r *RealTelegramBotAdapter) commandRoutes() map[string]commandHandler {
	return map[string]commandHandler{
		"start":    r.handleStartCommand,
		"plans":    r.handlePlansCommand,
		"status":   r.handleStatusCommand,
		"settings": r.handleSettingsCommand,
		"buy":      r.handleBuyCommand,
		"chat":     r.handleChatCommand,
		"bye":      r.handleByeCommand,
		"help":     r.handleHelpCommand,

		// These handlers are wrapped in our adminOnly middleware.
		"create_plan":    r.adminOnly(r.handleCreatePlanCommand),
		"delete_plan":    r.adminOnly(r.handleDeletePlanCommand),
		"update_plan":    r.adminOnly(r.handleUpdatePlanCommand),
		"update_pricing": r.adminOnly(r.handleUpdatePricingCommand),
	}
}

// adminOnly middleware remains the same...
func (r *RealTelegramBotAdapter) adminOnly(next commandHandler) commandHandler {
	return func(ctx context.Context, message *tgbotapi.Message) error {
		if _, isAdmin := r.adminIDsMap[message.From.ID]; !isAdmin {
			metrics.IncAdminCommand("/"+message.Command(), "unauthorized")
			return r.SendMessage(ctx, adapter.SendMessageParams{
				ChatID:    message.Chat.ID,
				Text:      r.translator.T("error_unauthorized"),
				ParseMode: tgbotapi.ModeMarkdownV2,
			}) // Localized
		}
		metrics.IncAdminCommand("/"+message.Command(), "authorized")
		return next(ctx, message)
	}
}

// The /start command now has slightly different logic for new vs. existing users.
func (r *RealTelegramBotAdapter) handleStartCommand(ctx context.Context, message *tgbotapi.Message) error {
	user, err := r.facade.UserUC.RegisterOrFetch(ctx, message.From.ID, message.From.UserName)
	if err != nil {
		return r.SendMessage(ctx, adapter.SendMessageParams{
			ChatID:    message.Chat.ID,
			Text:      r.translator.T("error_generic"),
			ParseMode: tgbotapi.ModeMarkdownV2,
		}) // Localized
	}

	_, isAdmin := r.adminIDsMap[message.From.ID]
	if err := r.SetMenuCommands(ctx, message.Chat.ID, isAdmin); err != nil {
		r.log.Warn().Err(err).Int64("tg_id", message.From.ID).Msg("failed to set dynamic menu commands")
	}

	if user.RegistrationStatus == model.RegistrationStatusPending {
		// Explicitly start or restart the registration flow by setting the state.
		if err := r.facade.UserUC.StartRegistration(ctx, user.TelegramID); err != nil {
			return r.SendMessage(ctx, adapter.SendMessageParams{
				ChatID:    message.Chat.ID,
				Text:      r.translator.T("error_generic"),
				ParseMode: tgbotapi.ModeMarkdownV2,
			}) // Localized
		}

		// Greet the user and ask for their name.
		accountName := message.From.FirstName
		if message.From.LastName != "" {
			accountName += " " + message.From.LastName
		}
		return r.SendMessage(ctx, adapter.SendMessageParams{
			ChatID:    message.Chat.ID,
			Text:      r.translator.T("reg_start", accountName),
			ParseMode: tgbotapi.ModeMarkdownV2,
		}) // Localized
	}

	// For existing, completed users, just show the main menu.
	return r.sendMainMenu(ctx, message.Chat.ID, r.translator.T("welcome_message"))
}

// handleRegistrationMessage processes non-command messages from users in the registration flow.
func (r *RealTelegramBotAdapter) handleRegistrationMessage(ctx context.Context, message *tgbotapi.Message) error {
	messageText := message.Text
	phoneNumber := ""
	if message.Contact != nil {
		phoneNumber = message.Contact.PhoneNumber
	}

	// Do not treat the /start command text as a full name
	if message.Text == "/start" {
		return r.handleStartCommand(ctx, message)
	}

	reply, markup, err := r.facade.UserUC.ProcessRegistrationStep(ctx, message.From.ID, messageText, phoneNumber)
	if err != nil {
		r.log.Error().Err(err).Int64("tg_id", message.From.ID).Msg("failed to process registration step")
		return r.SendMessage(ctx, adapter.SendMessageParams{
			ChatID:    message.Chat.ID,
			Text:      r.translator.T("error_generic"),
			ParseMode: tgbotapi.ModeMarkdownV2,
		}) // Localized
	}

	if markup != nil {
		return r.SendMessage(ctx, adapter.SendMessageParams{
			ChatID:      message.Chat.ID,
			Text:        reply,
			ParseMode:   tgbotapi.ModeMarkdownV2,
			ReplyMarkup: markup,
		}) // Localized
	}

	return r.SendMessage(ctx, adapter.SendMessageParams{
		ChatID:    message.Chat.ID,
		Text:      reply,
		ParseMode: tgbotapi.ModeMarkdownV2,
	}) // Localized
}

// handlePlansCommand handles the /plans command.
func (r *RealTelegramBotAdapter) handlePlansCommand(ctx context.Context, message *tgbotapi.Message) error {
	return r.sendPlansMenu(ctx, message.Chat.ID)
}

// handleStatusCommand handles the /status command.
func (r *RealTelegramBotAdapter) handleStatusCommand(ctx context.Context, message *tgbotapi.Message) error {
	info, err := r.facade.HandleStatus(ctx, message.From.ID)
	if err != nil {
		return r.SendMessage(ctx, adapter.SendMessageParams{
			ChatID:    message.Chat.ID,
			Text:      r.translator.T("error_generic"),
			ParseMode: tgbotapi.ModeMarkdownV2,
		}) // Localized
	}

	var b strings.Builder
	b.WriteString(r.translator.T("status_header") + "\n\n")

	if info.HasActiveSub {
		b.WriteString(fmt.Sprintf(r.translator.T("status_active_plan"), info.ActivePlanName) + "\n")
		b.WriteString(fmt.Sprintf(r.translator.T("status_credits"), info.ActiveCredits) + "\n")
		if info.ActiveExpiresAt != nil {
			days := int(time.Until(*info.ActiveExpiresAt).Hours() / 24)
			if days < 0 {
				days = 0
			}
			b.WriteString(fmt.Sprintf(r.translator.T("status_expires_at"), info.ActiveExpiresAt.Format("2006-01-02"), days) + "\n")
		}
	} else {
		b.WriteString(r.translator.T("status_no_active_plan") + "\n")
	}

	b.WriteString("\n") // Add a newline for spacing
	if info.HasReservedSub && info.ReservedPlan != nil {
		startDate := "N/A"
		if info.ReservedPlan.ScheduledStartAt != nil {
			startDate = info.ReservedPlan.ScheduledStartAt.Format("2006-01-02")
		}
		b.WriteString(fmt.Sprintf(r.translator.T("status_reserved_plan"), info.ReservedPlan.PlanName, startDate) + "\n")
	} else {
		b.WriteString(r.translator.T("status_no_reserved_plan") + "\n")
	}

	return r.sendMainMenu(ctx, message.Chat.ID, b.String())
}

// handleBuyCommand handles the /buy command.
func (r *RealTelegramBotAdapter) handleBuyCommand(ctx context.Context, message *tgbotapi.Message) error {
	planID := message.CommandArguments()
	if strings.TrimSpace(planID) == "" {
		return r.SendMessage(ctx, adapter.SendMessageParams{
			ChatID:    message.Chat.ID,
			Text:      r.translator.T("usage_buy"),
			ParseMode: tgbotapi.ModeMarkdownV2,
		}) // Localized
	}
	text, err := r.facade.HandleSubscribe(ctx, message.From.ID, planID)
	if err != nil {
		text = r.translator.T("error_payment_init") // Localized
	}
	if url := extractFirstURL(text); url != "" {
		markup := adapter.ReplyMarkup{
			Buttons:  [][]adapter.Button{{{Text: r.translator.T("button_pay_now"), URL: url}}},
			IsInline: true,
		}
		return r.SendMessage(ctx, adapter.SendMessageParams{
			ChatID:      message.Chat.ID,
			Text:        text,
			ParseMode:   tgbotapi.ModeMarkdownV2,
			ReplyMarkup: &markup,
		}) // Localized
	}
	return r.SendMessage(ctx, adapter.SendMessageParams{
		ChatID:    message.Chat.ID,
		Text:      text,
		ParseMode: tgbotapi.ModeMarkdownV2,
	}) // Localized
}

// handleChatCommand handles the /chat command.
func (r *RealTelegramBotAdapter) handleChatCommand(ctx context.Context, message *tgbotapi.Message) error {
	model := message.CommandArguments()
	if strings.TrimSpace(model) == "" {
		return r.sendModelMenu(ctx, message.Chat.ID)
	}
	text, err := r.facade.HandleStartChat(ctx, message.From.ID, model)
	if err != nil {
		if errors.Is(err, domain.ErrModelNotAvailable) {
			return r.SendMessage(ctx, adapter.SendMessageParams{
				ChatID:    message.Chat.ID,
				Text:      r.translator.T("error_model_unavailable"),
				ParseMode: tgbotapi.ModeMarkdownV2,
			}) // Localized
		}
		if errors.Is(err, domain.ErrActiveChatExists) {
			text = r.translator.T("error_chat_active") // Localized
		} else {
			text = r.translator.T("error_chat_start") // Localized
		}
	}
	if err := r.SendMessage(ctx, adapter.SendMessageParams{
		ChatID:    message.Chat.ID,
		Text:      text,
		ParseMode: tgbotapi.ModeMarkdownV2,
	}); err != nil {
		return err
	}
	return r.sendEndChatButton(ctx, message.Chat.ID)
}

// handleByeCommand handles the /bye command to end a chat.
func (r *RealTelegramBotAdapter) handleByeCommand(ctx context.Context, message *tgbotapi.Message) error {
	user, err := r.facade.UserUC.GetByTelegramID(ctx, message.From.ID)
	if err != nil || user == nil {
		return r.SendMessage(ctx, adapter.SendMessageParams{
			ChatID:    message.Chat.ID,
			Text:      r.translator.T("error_user_not_found"),
			ParseMode: tgbotapi.ModeMarkdownV2,
		}) // Localized
	}
	sess, err := r.facade.ChatUC.FindActiveSession(ctx, user.ID)
	if err != nil || sess == nil {
		return r.SendMessage(ctx, adapter.SendMessageParams{
			ChatID:    message.Chat.ID,
			Text:      r.translator.T("error_no_active_chat"),
			ParseMode: tgbotapi.ModeMarkdownV2,
		}) // Localized
	}
	text, err := r.facade.HandleEndChat(ctx, message.From.ID, sess.ID)
	if err != nil {
		text = r.translator.T("error_chat_end") // Localized
	}
	return r.SendMessage(ctx, adapter.SendMessageParams{
		ChatID:    message.Chat.ID,
		Text:      text,
		ParseMode: tgbotapi.ModeMarkdownV2,
	}) // Localized
}

// handleHelpCommand provides a list of commands.
func (r *RealTelegramBotAdapter) handleHelpCommand(ctx context.Context, message *tgbotapi.Message) error {
	return r.SendMessage(ctx, adapter.SendMessageParams{
		ChatID:    message.Chat.ID,
		Text:      r.translator.T("help_message"),
		ParseMode: tgbotapi.ModeMarkdownV2,
	}) // Localized
}

// handleSettingsCommand remains the same, as it was already written with the translator.
func (r *RealTelegramBotAdapter) handleSettingsCommand(ctx context.Context, message *tgbotapi.Message) error {
	user, err := r.facade.UserUC.GetByTelegramID(ctx, message.From.ID)
	if err != nil {
		return r.SendMessage(ctx, adapter.SendMessageParams{
			ChatID:    message.Chat.ID,
			Text:      r.translator.T("error_generic"),
			ParseMode: tgbotapi.ModeMarkdownV2,
		}) // Localized
	}

	var b strings.Builder
	b.WriteString(r.translator.T("settings_header") + "\n\n")

	var storageButton adapter.Button
	if user.Privacy.AllowMessageStorage {
		b.WriteString(r.translator.T("storage_enabled_title") + "\n")
		b.WriteString(r.translator.T("storage_enabled_desc"))
		storageButton = adapter.Button{Text: r.translator.T("button_disable_storage"), Data: "privacy:toggle_storage"}
	} else {
		b.WriteString(r.translator.T("storage_disabled_title") + "\n")
		b.WriteString(r.translator.T("storage_disabled_desc"))
		storageButton = adapter.Button{Text: r.translator.T("button_enable_storage"), Data: "privacy:toggle_storage"}
	}

	markup := adapter.ReplyMarkup{
		Buttons: [][]adapter.Button{
			{storageButton},
			{{Text: r.translator.T("back_to_menu"), Data: "cmd:menu"}},
		},
		IsInline: true,
	}
	return r.SendMessage(ctx, adapter.SendMessageParams{
		ChatID:      message.Chat.ID,
		Text:        b.String(),
		ParseMode:   tgbotapi.ModeMarkdownV2,
		ReplyMarkup: &markup,
	}) // Localized
}

// handleCreatePlanCommand is our new admin command handler.
func (r *RealTelegramBotAdapter) handleCreatePlanCommand(ctx context.Context, message *tgbotapi.Message) error {
	args := strings.Fields(message.CommandArguments())
	if len(args) != 4 {
		return r.SendMessage(ctx, adapter.SendMessageParams{
			ChatID:    message.Chat.ID,
			Text:      r.translator.T("usage_create_plan"),
			ParseMode: tgbotapi.ModeMarkdownV2,
		}) // Localized
	}

	name := args[0]
	days, err1 := strconv.Atoi(args[1])
	credits, err2 := strconv.ParseInt(args[2], 10, 64)
	price, err3 := strconv.ParseInt(args[3], 10, 64)

	if err1 != nil || err2 != nil || err3 != nil {
		return r.SendMessage(ctx, adapter.SendMessageParams{
			ChatID:    message.Chat.ID,
			Text:      r.translator.T("error_invalid_numbers"), // Localized
			ParseMode: tgbotapi.ModeMarkdownV2,
		})
	}

	plan, err := r.facade.PlanUC.Create(ctx, name, days, credits, price)

	var reply string
	if err != nil {
		r.log.Error().Err(err).Msg("failed to create plan")
		reply = r.translator.T("error_create_plan")
	} else {
		// 1. Escape the plan ID before putting it in the message.
		escapedID := r.escapeMarkdownV2(plan.ID)

		// 2. Get the localized string from your fa.yaml file.
		reply = r.translator.T("success_plan_created", plan.Name, escapedID)
	}

	// 3. Send the message with the MarkdownV2 ParseMode enabled.
	return r.SendMessage(ctx, adapter.SendMessageParams{
		ChatID:    message.Chat.ID,
		Text:      reply,
		ParseMode: tgbotapi.ModeMarkdownV2,
	})
}

func (r *RealTelegramBotAdapter) handleDeletePlanCommand(ctx context.Context, message *tgbotapi.Message) error {
	planID := message.CommandArguments()
	if strings.TrimSpace(planID) == "" {
		return r.SendMessage(ctx, adapter.SendMessageParams{
			ChatID:    message.Chat.ID,
			Text:      r.translator.T("usage_delete_plan"),
			ParseMode: tgbotapi.ModeMarkdownV2,
		}) // Localized
	}

	var resultMessage string
	_, err := r.facade.HandleDeletePlan(ctx, planID)
	if err != nil {
		// Catch our new specific error and provide a helpful hint.
		if errors.Is(err, domain.ErrInvalidArgument) {
			resultMessage = r.translator.T("error_invalid_plan_id")
		} else if errors.Is(err, domain.ErrSubsciptionWithActiveUser) {
			resultMessage = r.translator.T("error_delete_plan_in_use")
		} else {
			r.log.Error().Err(err).Str("plan_id", planID).Msg("failed to delete plan")
			resultMessage = r.translator.T("error_delete_plan")
		}
	} else {
		resultMessage = r.translator.T("success_plan_deleted", planID)
	}

	fullMessage := fmt.Sprintf("%s\n\n%s", resultMessage, r.translator.T("welcome_message"))
	return r.sendMainMenu(ctx, message.Chat.ID, fullMessage)

}

func (r *RealTelegramBotAdapter) handleUpdatePlanCommand(ctx context.Context, message *tgbotapi.Message) error {
	args := strings.Fields(message.CommandArguments())
	if len(args) != 5 {
		return r.SendMessage(ctx, adapter.SendMessageParams{
			ChatID:    message.Chat.ID,
			Text:      r.translator.T("usage_update_plan"),
			ParseMode: tgbotapi.ModeMarkdownV2,
		}) // Localized
	}

	id := args[0]
	name := args[1]
	days, err1 := strconv.Atoi(args[2])
	credits, err2 := strconv.ParseInt(args[3], 10, 64)
	price, err3 := strconv.ParseInt(args[4], 10, 64)

	if err1 != nil || err2 != nil || err3 != nil {
		return r.SendMessage(ctx, adapter.SendMessageParams{
			ChatID:    message.Chat.ID,
			Text:      r.translator.T("error_invalid_numbers"),
			ParseMode: tgbotapi.ModeMarkdownV2,
		}) // Localized
	}

	text, err := r.facade.HandleUpdatePlan(ctx, id, name, days, credits, price)
	if err != nil {
		r.log.Error().Err(err).Str("plan_id", id).Msg("failed to update plan")
		return r.SendMessage(ctx, adapter.SendMessageParams{
			ChatID:    message.Chat.ID,
			Text:      r.translator.T("error_update_plan"),
			ParseMode: tgbotapi.ModeMarkdownV2,
		}) // Localized
	}

	return r.SendMessage(ctx, adapter.SendMessageParams{
		ChatID:    message.Chat.ID,
		Text:      text,
		ParseMode: tgbotapi.ModeMarkdownV2,
	}) // Localized
}

func (r *RealTelegramBotAdapter) handleUpdatePricingCommand(ctx context.Context, message *tgbotapi.Message) error {
	args := strings.Fields(message.CommandArguments())
	if len(args) != 3 {
		return r.SendMessage(ctx, adapter.SendMessageParams{
			ChatID:    message.Chat.ID,
			Text:      r.translator.T("usage_update_pricing"),
			ParseMode: tgbotapi.ModeMarkdownV2,
		}) // Localized
	}

	modelName := args[0]
	inputPrice, err1 := strconv.ParseInt(args[1], 10, 64)
	outputPrice, err2 := strconv.ParseInt(args[2], 10, 64)

	if err1 != nil || err2 != nil {
		return r.SendMessage(ctx, adapter.SendMessageParams{
			ChatID:    message.Chat.ID,
			Text:      r.translator.T("error_invalid_numbers"),
			ParseMode: tgbotapi.ModeMarkdownV2,
		}) // Localized
	}

	text, err := r.facade.HandleUpdatePricing(ctx, modelName, inputPrice, outputPrice)
	if err != nil {
		r.log.Error().Err(err).Str("model_name", modelName).Msg("failed to update pricing")
		return r.SendMessage(ctx, adapter.SendMessageParams{
			ChatID:    message.Chat.ID,
			Text:      r.translator.T("error_update_pricing"),
			ParseMode: tgbotapi.ModeMarkdownV2,
		}) // Localized
	}

	return r.SendMessage(ctx, adapter.SendMessageParams{
		ChatID:    message.Chat.ID,
		Text:      text,
		ParseMode: tgbotapi.ModeMarkdownV2,
	}) // Localized
}
