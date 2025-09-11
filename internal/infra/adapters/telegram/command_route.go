package telegram

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"telegram-ai-subscription/internal/domain"
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
			return r.SendMessage(ctx, message.Chat.ID, r.translator.T("error_unauthorized")) // Localized
		}
		metrics.IncAdminCommand("/"+message.Command(), "authorized")
		return next(ctx, message)
	}
}

// handleStartCommand handles the /start command.
func (r *RealTelegramBotAdapter) handleStartCommand(ctx context.Context, message *tgbotapi.Message) error {
	if _, err := r.facade.UserUC.RegisterOrFetch(ctx, message.From.ID, message.From.UserName); err != nil {
		return r.SendMessage(ctx, message.Chat.ID, r.translator.T("error_generic"))
	}

	// Check if the user is an admin
	_, isAdmin := r.adminIDsMap[message.From.ID]
	// Set the appropriate menu for the user
	if err := r.SetMenuCommands(ctx, message.Chat.ID, isAdmin); err != nil {
		// Log the error but don't block the user
		r.log.Warn().Err(err).Int64("tg_id", message.From.ID).Msg("failed to set dynamic menu commands")
	}

	return r.sendMainMenu(ctx, message.Chat.ID, r.translator.T("welcome_message"))
}

// handlePlansCommand handles the /plans command.
func (r *RealTelegramBotAdapter) handlePlansCommand(ctx context.Context, message *tgbotapi.Message) error {
	return r.sendPlansMenu(ctx, message.Chat.ID)
}

// handleStatusCommand handles the /status command.
func (r *RealTelegramBotAdapter) handleStatusCommand(ctx context.Context, message *tgbotapi.Message) error {
	info, err := r.facade.HandleStatus(ctx, message.From.ID)
	if err != nil {
		return r.SendMessage(ctx, message.Chat.ID, r.translator.T("error_generic"))
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
		return r.SendMessage(ctx, message.Chat.ID, r.translator.T("usage_buy"))
	}
	text, err := r.facade.HandleSubscribe(ctx, message.From.ID, planID)
	if err != nil {
		text = r.translator.T("error_payment_init") // Localized
	}
	if url := extractFirstURL(text); url != "" {
		rows := [][]adapter.InlineButton{{{Text: r.translator.T("button_pay_now"), URL: url}}} // Localized
		return r.SendButtons(ctx, message.Chat.ID, text, rows)
	}
	return r.SendMessage(ctx, message.Chat.ID, text)
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
			return r.SendMessage(ctx, message.Chat.ID, r.translator.T("error_model_unavailable")) // Localized
		}
		if errors.Is(err, domain.ErrActiveChatExists) {
			text = r.translator.T("error_chat_active") // Localized
		} else {
			text = r.translator.T("error_chat_start") // Localized
		}
	}
	if err := r.SendMessage(ctx, message.Chat.ID, text); err != nil {
		return err
	}
	return r.sendEndChatButton(ctx, message.Chat.ID)
}

// handleByeCommand handles the /bye command to end a chat.
func (r *RealTelegramBotAdapter) handleByeCommand(ctx context.Context, message *tgbotapi.Message) error {
	user, err := r.facade.UserUC.GetByTelegramID(ctx, message.From.ID)
	if err != nil || user == nil {
		return r.SendMessage(ctx, message.Chat.ID, r.translator.T("error_user_not_found")) // Localized
	}
	sess, err := r.facade.ChatUC.FindActiveSession(ctx, user.ID)
	if err != nil || sess == nil {
		return r.SendMessage(ctx, message.Chat.ID, r.translator.T("error_no_active_chat")) // Localized
	}
	text, err := r.facade.HandleEndChat(ctx, message.From.ID, sess.ID)
	if err != nil {
		text = r.translator.T("error_chat_end") // Localized
	}
	return r.SendMessage(ctx, message.Chat.ID, text)
}

// handleHelpCommand provides a list of commands.
func (r *RealTelegramBotAdapter) handleHelpCommand(ctx context.Context, message *tgbotapi.Message) error {
	return r.SendMessage(ctx, message.Chat.ID, r.translator.T("help_message"))
}

// handleCreatePlanCommand is our new admin command handler.
func (r *RealTelegramBotAdapter) handleCreatePlanCommand(ctx context.Context, message *tgbotapi.Message) error {
	args := strings.Fields(message.CommandArguments())
	if len(args) != 4 {
		return r.SendMessage(ctx, message.Chat.ID, r.translator.T("usage_create_plan")) // Localized
	}

	name := args[0]
	days, err1 := strconv.Atoi(args[1])
	credits, err2 := strconv.ParseInt(args[2], 10, 64)
	price, err3 := strconv.ParseInt(args[3], 10, 64)

	if err1 != nil || err2 != nil || err3 != nil {
		return r.SendMessage(ctx, message.Chat.ID, r.translator.T("error_invalid_numbers")) // Localized
	}

	plan, err := r.facade.PlanUC.Create(ctx, name, days, credits, price)

	var resultMessage string
	if err != nil {
		r.log.Error().Err(err).Msg("failed to create plan")
		resultMessage = r.translator.T("error_create_plan")
	} else {
		resultMessage = r.translator.T("success_plan_created", plan.Name, plan.ID)
	}

	// Always show the main menu after the action is complete.
	// The result of the action is prepended to the standard welcome message.
	fullMessage := fmt.Sprintf("%s\n\n%s", resultMessage, r.translator.T("welcome_message"))
	return r.sendMainMenu(ctx, message.Chat.ID, fullMessage)
}

func (r *RealTelegramBotAdapter) handleDeletePlanCommand(ctx context.Context, message *tgbotapi.Message) error {
	planID := message.CommandArguments()
	if strings.TrimSpace(planID) == "" {
		return r.SendMessage(ctx, message.Chat.ID, r.translator.T("usage_delete_plan")) // Localized
	}

	text, err := r.facade.HandleDeletePlan(ctx, planID)
	if err != nil {
		if errors.Is(err, domain.ErrSubsciptionWithActiveUser) {
			return r.SendMessage(ctx, message.Chat.ID, r.translator.T("error_delete_plan_in_use")) // Localized
		}
		r.log.Error().Err(err).Str("plan_id", planID).Msg("failed to delete plan")
		return r.SendMessage(ctx, message.Chat.ID, r.translator.T("error_delete_plan")) // Localized
	}

	return r.SendMessage(ctx, message.Chat.ID, text)
}

func (r *RealTelegramBotAdapter) handleUpdatePlanCommand(ctx context.Context, message *tgbotapi.Message) error {
	args := strings.Fields(message.CommandArguments())
	if len(args) != 5 {
		return r.SendMessage(ctx, message.Chat.ID, r.translator.T("usage_update_plan")) // Localized
	}

	id := args[0]
	name := args[1]
	days, err1 := strconv.Atoi(args[2])
	credits, err2 := strconv.ParseInt(args[3], 10, 64)
	price, err3 := strconv.ParseInt(args[4], 10, 64)

	if err1 != nil || err2 != nil || err3 != nil {
		return r.SendMessage(ctx, message.Chat.ID, r.translator.T("error_invalid_numbers")) // Localized
	}

	text, err := r.facade.HandleUpdatePlan(ctx, id, name, days, credits, price)
	if err != nil {
		r.log.Error().Err(err).Str("plan_id", id).Msg("failed to update plan")
		return r.SendMessage(ctx, message.Chat.ID, r.translator.T("error_update_plan")) // Localized
	}

	return r.SendMessage(ctx, message.Chat.ID, text)
}

func (r *RealTelegramBotAdapter) handleUpdatePricingCommand(ctx context.Context, message *tgbotapi.Message) error {
	args := strings.Fields(message.CommandArguments())
	if len(args) != 3 {
		return r.SendMessage(ctx, message.Chat.ID, r.translator.T("usage_update_pricing")) // Localized
	}

	modelName := args[0]
	inputPrice, err1 := strconv.ParseInt(args[1], 10, 64)
	outputPrice, err2 := strconv.ParseInt(args[2], 10, 64)

	if err1 != nil || err2 != nil {
		return r.SendMessage(ctx, message.Chat.ID, r.translator.T("error_invalid_numbers")) // Localized
	}

	text, err := r.facade.HandleUpdatePricing(ctx, modelName, inputPrice, outputPrice)
	if err != nil {
		r.log.Error().Err(err).Str("model_name", modelName).Msg("failed to update pricing")
		return r.SendMessage(ctx, message.Chat.ID, r.translator.T("error_update_pricing")) // Localized
	}

	return r.SendMessage(ctx, message.Chat.ID, text)
}

// handleSettingsCommand remains the same, as it was already written with the translator.
func (r *RealTelegramBotAdapter) handleSettingsCommand(ctx context.Context, message *tgbotapi.Message) error {
	user, err := r.facade.UserUC.GetByTelegramID(ctx, message.From.ID)
	if err != nil {
		return r.SendMessage(ctx, message.Chat.ID, r.translator.T("error_generic"))
	}

	var b strings.Builder
	b.WriteString(r.translator.T("settings_header") + "\n\n")

	var storageButton adapter.InlineButton
	if user.Privacy.AllowMessageStorage {
		b.WriteString(r.translator.T("storage_enabled_title") + "\n")
		b.WriteString(r.translator.T("storage_enabled_desc"))
		storageButton = adapter.InlineButton{Text: r.translator.T("button_disable_storage"), Data: "privacy:toggle_storage"}
	} else {
		b.WriteString(r.translator.T("storage_disabled_title") + "\n")
		b.WriteString(r.translator.T("storage_disabled_desc"))
		storageButton = adapter.InlineButton{Text: r.translator.T("button_enable_storage"), Data: "privacy:toggle_storage"}
	}

	rows := [][]adapter.InlineButton{
		{storageButton},
		{{Text: r.translator.T("back_to_menu"), Data: "cmd:menu"}},
	}

	return r.SendButtons(ctx, message.Chat.ID, b.String(), rows)
}
