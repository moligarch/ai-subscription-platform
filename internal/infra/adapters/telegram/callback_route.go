package telegram

import (
	"context"
	"errors"
	"strings"
	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/domain/ports/adapter"
)

type cbHandler func(ctx context.Context, chatID int64, data string) error

func (r *RealTelegramBotAdapter) menuCBRoute(ctx context.Context, id int64, _ string) error {
	return r.sendMainMenu(ctx, id, "برای ادامه یک گزینه را انتخاب کنید:")
}
func (r *RealTelegramBotAdapter) planCBRoute(ctx context.Context, id int64, _ string) error {
	return r.sendPlansMenu(ctx, id)
}
func (r *RealTelegramBotAdapter) statusCBRoute(ctx context.Context, id int64, _ string) error {
	text, err := r.facade.HandleStatus(ctx, id)
	if err != nil {
		text = "دریافت اطلاعات با مشکل مواجه شد."
	}
	return r.sendMainMenu(ctx, id, text)
}
func (r *RealTelegramBotAdapter) chatCBRoute(ctx context.Context, id int64, _ string) error {
	return r.sendModelMenu(ctx, id)
}
func (r *RealTelegramBotAdapter) chatEndCBRoute(ctx context.Context, id int64, _ string) error {
	user, err := r.facade.UserUC.GetByTelegramID(ctx, id)
	if err != nil || user == nil {
		return r.SendMessage(ctx, id, "شما هنوز ثبتنام نکرده اید. برای ثبت نام از /start استفاده کنید.")
	}
	sess, err := r.facade.ChatUC.FindActiveSession(ctx, user.ID)
	if err != nil || sess == nil {
		return r.SendMessage(ctx, id, "چت فعالی یافت نشد. برای شروع مکالمه جدید از /chat استفاده کنید.")
	}
	text, err := r.facade.HandleEndChat(ctx, id, sess.ID)
	if err != nil {
		text = "Failed to end chat."
	}
	return r.SendMessage(ctx, id, text)
}
func (r *RealTelegramBotAdapter) historyCBRoute(ctx context.Context, id int64, _ string) error {
	return r.sendHistoryMenu(ctx, id)
}

type prefixCB struct {
	Prefix string
	Fn     cbHandler
}

func (r *RealTelegramBotAdapter) buyPrefixCBRoute(ctx context.Context, id int64, data string) error {
	planID := strings.TrimPrefix(data, "buy:")
	_ = r.SendMessage(ctx, id, "در حال پردازش درخواست شما هستیم...")
	text, err := r.facade.HandleSubscribe(ctx, id, planID)
	if err != nil {
		text = "فرآیند پرداخت با مشکل مواجه شد."
	}
	if url := extractFirstURL(text); url != "" {
		rows := [][]adapter.InlineButton{
			{{Text: "پرداخت", URL: url}},
			{{Text: "◀️ منو اصلی", Data: "cmd:menu"}},
		}
		return r.SendButtons(ctx, id, text, rows)
	}
	return r.SendMessage(ctx, id, text)
}
func (r *RealTelegramBotAdapter) chatPrefixCBRoute(ctx context.Context, id int64, data string) error {
	model := strings.TrimPrefix(data, "chat:")
	text, err := r.facade.HandleStartChat(ctx, id, model)
	if err != nil {
		if errors.Is(err, domain.ErrActiveChatExists) {
			text = "شما پیش از این یک چت فعال دارید. برای شروع چت جدید ابتدا چت قبلی را متوقف کنید. (/bye)"
		} else {
			text = "مشکلی در شروع چت جدید پیش آمد."
		}
	}
	if err := r.SendMessage(ctx, id, text); err != nil {
		return err
	}
	return r.sendEndChatButton(ctx, id)
}
func (r *RealTelegramBotAdapter) continueChatPrefixCBRoute(ctx context.Context, id int64, data string) error {
	sessionID := strings.TrimPrefix(data, "hist:cont:")
	user, err := r.facade.UserUC.GetByTelegramID(ctx, id)
	if err != nil || user == nil {
		return r.SendMessage(ctx, id, "شما هنوز ثبتنام نکرده اید. برای ثبت نام از /start استفاده کنید.")
	}
	if err := r.facade.ChatUC.SwitchActiveSession(ctx, user.ID, sessionID); err != nil {
		return r.SendMessage(ctx, id, "مشکلی در ادامه این چت پیش آمد.")
	}
	if err := r.SendMessage(ctx, id, "✅ این چت هم اکنون فعال است. می‌توانید به مکالمه خود ادامه دهید."); err != nil {
		return err
	}
	// show End Chat button like after /chat
	return r.sendEndChatButton(ctx, id)
}
func (r *RealTelegramBotAdapter) deleteChatPrefixCBRoute(ctx context.Context, id int64, data string) error {
	sessionID := strings.TrimPrefix(data, "hist:del:")
	if err := r.facade.ChatUC.DeleteSession(ctx, sessionID); err != nil {
		return r.SendMessage(ctx, id, "مشکلی در حذف چت به وجود آمد.")
	}
	// Refresh history list after deletion
	return r.sendHistoryMenu(ctx, id)
}
