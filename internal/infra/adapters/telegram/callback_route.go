package telegram

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/domain/ports/adapter"
	"telegram-ai-subscription/internal/domain/ports/repository"
	"telegram-ai-subscription/internal/usecase"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type cbHandler func(ctx context.Context, chatID int64, data string) error
type prefixCB struct {
	Prefix string
	Fn     cbHandler
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
			Prefix: "code:",
			Fn:     r.codePrefixCBRoute,
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
		{
			Prefix: "privacy:",
			Fn:     r.privacyToggleCBRoute,
		},
		{
			Prefix: "reg:",
			Fn:     r.registrationCBRoute,
		},
		{
			Prefix: "view_plan:",
			Fn:     r.viewPlanCBRoute,
		},
	}
}

func (r *RealTelegramBotAdapter) menuCBRoute(ctx context.Context, id int64, _ string) error {
	return r.sendMainMenu(ctx, id, r.translator.T("menu_prompt")) // Localized
}

func (r *RealTelegramBotAdapter) planCBRoute(ctx context.Context, id int64, _ string) error {
	return r.sendPlansMenu(ctx, id)
}

func (r *RealTelegramBotAdapter) statusCBRoute(ctx context.Context, id int64, _ string) error {
	info, err := r.facade.HandleStatus(ctx, id)
	if err != nil {
		return r.sendMainMenu(ctx, id, r.translator.T("error_generic"))
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

	return r.sendMainMenu(ctx, id, b.String())
}

func (r *RealTelegramBotAdapter) chatCBRoute(ctx context.Context, id int64, _ string) error {
	return r.sendModelMenu(ctx, id)
}

func (r *RealTelegramBotAdapter) chatEndCBRoute(ctx context.Context, id int64, _ string) error {
	user, err := r.facade.UserUC.GetByTelegramID(ctx, id)
	if err != nil || user == nil {
		return r.SendMessage(ctx, adapter.SendMessageParams{
			ChatID: id,
			Text:   r.translator.T("error_user_not_found"),
		}) // Localized
	}
	sess, err := r.facade.ChatUC.FindActiveSession(ctx, user.ID)
	if err != nil || sess == nil {
		return r.SendMessage(ctx, adapter.SendMessageParams{
			ChatID: id,
			Text:   r.translator.T("error_no_active_chat"),
		}) // Localized
	}

	text, err := r.facade.HandleEndChat(ctx, id, sess.ID)
	if err != nil {
		text = r.translator.T("error_chat_end") // Localized
	}

	return r.SendMessage(ctx, adapter.SendMessageParams{
		ChatID: id,
		Text:   text,
	}) // Localized
}

func (r *RealTelegramBotAdapter) historyCBRoute(ctx context.Context, id int64, _ string) error {
	return r.sendHistoryMenu(ctx, id)
}

func (r *RealTelegramBotAdapter) buyPrefixCBRoute(ctx context.Context, id int64, data string) error {
	planID := strings.TrimPrefix(data, "buy:")
	_ = r.SendMessage(ctx, adapter.SendMessageParams{
		ChatID: id,
		Text:   r.translator.T("callback_processing"),
	}) // Localized
	var rows *[][]adapter.Button
	text, url, err := r.facade.HandleSubscribe(ctx, id, planID)
	if err != nil {
		switch err {
		case domain.ErrInvalidArgument, domain.ErrPlanNotFound:
			text = r.translator.T("error_payment_no_plan")
		case domain.ErrUserNotFound:
			text = r.translator.T("error_user_not_found")
		case domain.ErrAlreadyHasReserved:
			text = r.translator.T("error_already_has_reserved")
		default:
			text = r.translator.T("error_payment_init")
		}

		rows = &[][]adapter.Button{
			{{Text: r.translator.T("back_to_menu"), Data: "cmd:menu"}}, // Localized
		}
	} else {
		rows = &[][]adapter.Button{
			{{Text: r.translator.T("button_pay_now"), URL: url}},       // Localized
			{{Text: r.translator.T("back_to_menu"), Data: "cmd:menu"}}, // Localized
		}
	}
	markup := adapter.ReplyMarkup{Buttons: *rows, IsInline: true}
	return r.SendMessage(ctx, adapter.SendMessageParams{
		ChatID:      id,
		Text:        text,
		ReplyMarkup: &markup,
	}) // Localized
}

func (r *RealTelegramBotAdapter) chatPrefixCBRoute(ctx context.Context, id int64, data string) error {
	model := strings.TrimPrefix(data, "chat:")
	text, err := r.facade.HandleStartChat(ctx, id, model)
	if err != nil {
		if errors.Is(err, domain.ErrModelNotAvailable) {
			_ = r.SendMessage(ctx, adapter.SendMessageParams{
				ChatID: id,
				Text:   r.translator.T("error_model_unavailable"),
			}) // Localized
			// Re-display the menu so they can choose another model
			return r.sendModelMenu(ctx, id)
		}
		if errors.Is(err, domain.ErrActiveChatExists) {
			text = r.translator.T("error_chat_active") // Localized
		} else {
			text = r.translator.T("error_chat_start") // Localized
		}
	}
	if err := r.SendMessage(ctx, adapter.SendMessageParams{
		ChatID: id,
		Text:   text,
	}); err != nil {
		return err
	}
	return r.sendEndChatButton(ctx, id)
}

func (r *RealTelegramBotAdapter) continueChatPrefixCBRoute(ctx context.Context, id int64, data string) error {
	sessionID := strings.TrimPrefix(data, "hist:cont:")
	user, err := r.facade.UserUC.GetByTelegramID(ctx, id)
	if err != nil || user == nil {
		return r.SendMessage(ctx, adapter.SendMessageParams{
			ChatID: id,
			Text:   r.translator.T("error_user_not_found"),
		}) // Localized
	}
	if err := r.facade.ChatUC.SwitchActiveSession(ctx, user.ID, sessionID); err != nil {
		return r.SendMessage(ctx, adapter.SendMessageParams{
			ChatID: id,
			Text:   r.translator.T("error_chat_continue"),
		}) // Localized
	}
	return r.sendEndChatButton(ctx, id)
}

func (r *RealTelegramBotAdapter) deleteChatPrefixCBRoute(ctx context.Context, id int64, data string) error {
	sessionID := strings.TrimPrefix(data, "hist:del:")
	if err := r.facade.ChatUC.DeleteSession(ctx, sessionID); err != nil {
		return r.SendMessage(ctx, adapter.SendMessageParams{
			ChatID: id,
			Text:   r.translator.T("error_chat_delete"),
		}) // Localized
	}
	return r.sendHistoryMenu(ctx, id)
}

func (r *RealTelegramBotAdapter) privacyToggleCBRoute(ctx context.Context, id int64, data string) error {
	err := r.facade.UserUC.ToggleMessageStorage(ctx, id)
	if err != nil {
		r.log.Error().Err(err).Int64("tg_id", id).Msg("failed to toggle message storage")
		_ = r.SendMessage(ctx, adapter.SendMessageParams{
			ChatID: id,
			Text:   r.translator.T("error_toggle_privacy"),
		}) // Localized
	}

	fakeMessage := &tgbotapi.Message{
		From: &tgbotapi.User{ID: id},
		Chat: &tgbotapi.Chat{ID: id},
	}
	return r.handleSettingsCommand(ctx, fakeMessage)
}

func (r *RealTelegramBotAdapter) registrationCBRoute(ctx context.Context, id int64, data string) error {
	action := strings.TrimPrefix(data, "reg:")

	switch action {
	case "verify":
		if err := r.facade.UserUC.CompleteRegistration(ctx, id); err != nil {
			r.log.Error().Err(err).Int64("tg_id", id).Msg("failed to complete registration")
			return r.SendMessage(ctx, adapter.SendMessageParams{
				ChatID: id,
				Text:   r.translator.T("error_generic"),
			}) // Localized
		}
		return r.sendMainMenu(ctx, id, r.translator.T("reg_success"))

	case "policy":
		markup := adapter.ReplyMarkup{
			Buttons: [][]adapter.Button{
				{{Text: r.translator.T("button_accept_policy"), Data: "reg:verify"}},
				{{Text: r.translator.T("button_cancel_reg"), Data: "reg:cancel"}},
			},
			IsInline: true,
		}
		return r.SendMessage(ctx, adapter.SendMessageParams{
			ChatID:      id,
			Text:        r.translator.Policy(),
			ReplyMarkup: &markup,
		}) // Localized
	case "cancel":
		_ = r.facade.UserUC.ClearRegistrationState(ctx, id)
		_ = r.SendMessage(ctx, adapter.SendMessageParams{
			ChatID: id,
			Text:   r.translator.T("reg_cancelled"),
		}) // Localized
		return nil
	default:
		r.log.Warn().Int64("tg_id", id).Str("action", action).Msg("unknown registration callback action")
		return r.SendMessage(ctx, adapter.SendMessageParams{
			ChatID: id,
			Text:   r.translator.T("error_generic"),
		}) // Localized
	}
}

func (r *RealTelegramBotAdapter) viewPlanCBRoute(ctx context.Context, chatID int64, data string) error {
	planID := strings.TrimPrefix(data, "view_plan:")

	plan, err := r.facade.PlanUC.Get(ctx, planID)
	if err != nil {
		return r.SendMessage(ctx, adapter.SendMessageParams{
			ChatID: chatID,
			Text:   r.translator.T("error_generic"),
		}) // Localized
	}

	// Build the detailed message body
	header := r.translator.T("plan_details_header", plan.Name)

	modelsStr := r.translator.T("plan_details_all_models")
	if len(plan.SupportedModels) > 0 {
		modelsStr = "• `" + strings.Join(plan.SupportedModels, "`\n• `") + "`"
	}

	body := r.translator.T("plan_details_body",
		plan.DurationDays,
		formatIRR(plan.PriceIRR),
		plan.Credits,
		modelsStr,
	)

	fullMessage := r.EscapeMarkdownV2(fmt.Sprintf("%s\n\n%s", header, body))

	// Build the new purchase option buttons
	markup := adapter.ReplyMarkup{
		Buttons: [][]adapter.Button{
			{{Text: r.translator.T("button_buy_gateway"), Data: "buy:" + plan.ID}},
			{{Text: r.translator.T("button_buy_code"), Data: "code:" + plan.ID}},
			{{Text: r.translator.T("back_to_menu"), Data: "cmd:menu"}},
		},
		IsInline: true,
	}

	return r.SendMessage(ctx, adapter.SendMessageParams{
		ChatID:      chatID,
		Text:        fullMessage,
		ParseMode:   tgbotapi.ModeMarkdownV2,
		ReplyMarkup: &markup,
	})
}

// codePrefixCBRoute starts the conversational flow for redeeming an activation code.
func (r *RealTelegramBotAdapter) codePrefixCBRoute(ctx context.Context, id int64, data string) error {
	planID := strings.TrimPrefix(data, "code:")

	// Create the state object using our new constant.
	state := &repository.ConversationState{
		Step: usecase.StepAwaitingActivationCode,
		Data: map[string]string{"plan_id": planID},
	}

	if err := r.facade.UserUC.SetConversationState(ctx, id, state); err != nil {
		r.log.Error().Err(err).Int64("tg_id", id).Msg("failed to set activation code state")
		return r.SendMessage(ctx, adapter.SendMessageParams{
			ChatID: id,
			Text:   r.translator.T("error_generic"),
		}) // Localized
	}

	return r.SendMessage(ctx, adapter.SendMessageParams{
		ChatID: id,
		Text:   r.translator.T("prompt_enter_activation_code"),
	}) // Localized
}
