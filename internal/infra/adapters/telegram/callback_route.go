package telegram

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/domain/ports/adapter"
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
			ChatID:    id,
			Text:      r.translator.T("error_user_not_found"),
			ParseMode: tgbotapi.ModeMarkdownV2,
		}) // Localized
	}
	sess, err := r.facade.ChatUC.FindActiveSession(ctx, user.ID)
	if err != nil || sess == nil {
		return r.SendMessage(ctx, adapter.SendMessageParams{
			ChatID:    id,
			Text:      r.translator.T("error_no_active_chat"),
			ParseMode: tgbotapi.ModeMarkdownV2,
		}) // Localized
	}

	text, err := r.facade.HandleEndChat(ctx, id, sess.ID)
	if err != nil {
		text = r.translator.T("error_chat_end") // Localized
	}

	return r.SendMessage(ctx, adapter.SendMessageParams{
		ChatID:    id,
		Text:      text,
		ParseMode: tgbotapi.ModeMarkdownV2,
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

	text, err := r.facade.HandleSubscribe(ctx, id, planID)
	if err != nil {
		text = r.translator.T("error_payment_init") // Localized
	}
	if url := extractFirstURL(text); url != "" {
		rows := [][]adapter.Button{
			{{Text: r.translator.T("button_pay_now"), URL: url}},                    // Localized
			{{Text: r.translator.T("callback_main_menu_button"), Data: "cmd:menu"}}, // Localized
		}
		markup := adapter.ReplyMarkup{Buttons: rows, IsInline: true}
		return r.SendMessage(ctx, adapter.SendMessageParams{
			ChatID:      id,
			Text:        text,
			ParseMode:   tgbotapi.ModeMarkdownV2,
			ReplyMarkup: &markup,
		}) // Localized
	}
	return r.SendMessage(ctx, adapter.SendMessageParams{
		ChatID:    id,
		Text:      text,
		ParseMode: tgbotapi.ModeMarkdownV2,
	}) // Localized
}

func (r *RealTelegramBotAdapter) chatPrefixCBRoute(ctx context.Context, id int64, data string) error {
	model := strings.TrimPrefix(data, "chat:")
	text, err := r.facade.HandleStartChat(ctx, id, model)
	if err != nil {
		if errors.Is(err, domain.ErrModelNotAvailable) {
			_ = r.SendMessage(ctx, adapter.SendMessageParams{
				ChatID:    id,
				Text:      r.translator.T("error_model_unavailable"),
				ParseMode: tgbotapi.ModeMarkdownV2,
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
		ChatID:    id,
		Text:      text,
		ParseMode: tgbotapi.ModeMarkdownV2,
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
			ChatID:    id,
			Text:      r.translator.T("error_user_not_found"),
			ParseMode: tgbotapi.ModeMarkdownV2,
		}) // Localized
	}
	if err := r.facade.ChatUC.SwitchActiveSession(ctx, user.ID, sessionID); err != nil {
		return r.SendMessage(ctx, adapter.SendMessageParams{
			ChatID:    id,
			Text:      r.translator.T("error_chat_continue"),
			ParseMode: tgbotapi.ModeMarkdownV2,
		}) // Localized
	}
	return r.sendEndChatButton(ctx, id)
}

func (r *RealTelegramBotAdapter) deleteChatPrefixCBRoute(ctx context.Context, id int64, data string) error {
	sessionID := strings.TrimPrefix(data, "hist:del:")
	if err := r.facade.ChatUC.DeleteSession(ctx, sessionID); err != nil {
		return r.SendMessage(ctx, adapter.SendMessageParams{
			ChatID:    id,
			Text:      r.translator.T("error_chat_delete"),
			ParseMode: tgbotapi.ModeMarkdownV2,
		}) // Localized
	}
	return r.sendHistoryMenu(ctx, id)
}

func (r *RealTelegramBotAdapter) privacyToggleCBRoute(ctx context.Context, id int64, data string) error {
	err := r.facade.UserUC.ToggleMessageStorage(ctx, id)
	if err != nil {
		r.log.Error().Err(err).Int64("tg_id", id).Msg("failed to toggle message storage")
		_ = r.SendMessage(ctx, adapter.SendMessageParams{
			ChatID:    id,
			Text:      r.translator.T("error_toggle_privacy"),
			ParseMode: tgbotapi.ModeMarkdownV2,
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
				ChatID:    id,
				Text:      r.translator.T("error_generic"),
				ParseMode: tgbotapi.ModeMarkdownV2,
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
			Text:        r.translator.T("policy_text"),
			ParseMode:   tgbotapi.ModeMarkdownV2,
			ReplyMarkup: &markup,
		}) // Localized
	case "cancel":
		_ = r.facade.UserUC.ClearRegistrationState(ctx, id)
		_ = r.SendMessage(ctx, adapter.SendMessageParams{
			ChatID:    id,
			Text:      r.translator.T("reg_cancelled"),
			ParseMode: tgbotapi.ModeMarkdownV2,
		}) // Localized
		return nil
	default:
		r.log.Warn().Int64("tg_id", id).Str("action", action).Msg("unknown registration callback action")
		return r.SendMessage(ctx, adapter.SendMessageParams{
			ChatID:    id,
			Text:      r.translator.T("error_generic"),
			ParseMode: tgbotapi.ModeMarkdownV2,
		}) // Localized
	}
}
