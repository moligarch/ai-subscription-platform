package telegram

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/domain/ports/adapter"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// handleStartCommand handles the /start command.
func (r *RealTelegramBotAdapter) handleStartCommand(ctx context.Context, message *tgbotapi.Message) error {
	text, err := r.facade.HandleStart(ctx, message.From.ID, message.From.UserName)
	if err != nil {
		return r.SendMessage(ctx, message.Chat.ID, "Failed to initialize user.")
	}
	return r.sendMainMenu(ctx, message.Chat.ID, text)
}

// handlePlansCommand handles the /plans command.
func (r *RealTelegramBotAdapter) handlePlansCommand(ctx context.Context, message *tgbotapi.Message) error {
	return r.sendPlansMenu(ctx, message.Chat.ID)
}

// handleStatusCommand handles the /status command.
func (r *RealTelegramBotAdapter) handleStatusCommand(ctx context.Context, message *tgbotapi.Message) error {
	text, err := r.facade.HandleStatus(ctx, message.From.ID)
	if err != nil {
		text = "Failed to get status."
	}
	return r.sendMainMenu(ctx, message.Chat.ID, text)
}

// handleBuyCommand handles the /buy command.
func (r *RealTelegramBotAdapter) handleBuyCommand(ctx context.Context, message *tgbotapi.Message) error {
	planID := message.CommandArguments()
	if strings.TrimSpace(planID) == "" {
		return r.SendMessage(ctx, message.Chat.ID, "Usage: /buy <plan_id>")
	}
	text, err := r.facade.HandleSubscribe(ctx, message.From.ID, planID)
	if err != nil {
		text = "Failed to initiate payment."
	}
	if url := extractFirstURL(text); url != "" {
		rows := [][]adapter.InlineButton{{{Text: "Pay now", URL: url}}}
		return r.SendButtons(ctx, message.Chat.ID, text, rows)
	}
	return r.SendMessage(ctx, message.Chat.ID, text)
}

// handleChatCommand handles the /chat command.
func (r *RealTelegramBotAdapter) handleChatCommand(ctx context.Context, message *tgbotapi.Message) error {
	model := message.CommandArguments()
	// If no model is specified, show the selection menu.
	if strings.TrimSpace(model) == "" {
		return r.sendModelMenu(ctx, message.Chat.ID)
	}
	text, err := r.facade.HandleStartChat(ctx, message.From.ID, model)
	if err != nil {
		if errors.Is(err, domain.ErrActiveChatExists) {
			text = "You already have an active chat session."
		} else {
			text = "Failed to start chat."
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
		return r.SendMessage(ctx, message.Chat.ID, "No user found. Try /start first.")
	}
	sess, err := r.facade.ChatUC.FindActiveSession(ctx, user.ID)
	if err != nil || sess == nil {
		return r.SendMessage(ctx, message.Chat.ID, "No active chat session found.")
	}
	text, err := r.facade.HandleEndChat(ctx, message.From.ID, sess.ID)
	if err != nil {
		text = "Failed to end chat."
	}
	return r.SendMessage(ctx, message.Chat.ID, text)
}

// handleHelpCommand provides a list of commands.
func (r *RealTelegramBotAdapter) handleHelpCommand(ctx context.Context, message *tgbotapi.Message) error {
	reply := "Commands:\n/start - Welcome message\n/plans - View subscription plans\n/status - Check your current status\n/chat - Start a new conversation\n/bye - End the current conversation"
	return r.SendMessage(ctx, message.Chat.ID, reply)
}

// handleCreatePlanCommand is our new admin command handler.
func (r *RealTelegramBotAdapter) handleCreatePlanCommand(ctx context.Context, message *tgbotapi.Message) error {
	args := strings.Fields(message.CommandArguments())
	if len(args) != 4 {
		return r.SendMessage(ctx, message.Chat.ID, "Usage: /create_plan <Name> <Days> <Credits> <PriceIRR>")
	}

	name := args[0]
	days, err1 := strconv.Atoi(args[1])
	credits, err2 := strconv.ParseInt(args[2], 10, 64)
	price, err3 := strconv.ParseInt(args[3], 10, 64)

	if err1 != nil || err2 != nil || err3 != nil {
		return r.SendMessage(ctx, message.Chat.ID, "Invalid arguments. Days, Credits, and Price must be numbers.")
	}

	plan, err := r.facade.PlanUC.Create(ctx, name, days, credits, price)
	if err != nil {
		r.log.Error().Err(err).Msg("failed to create plan")
		return r.SendMessage(ctx, message.Chat.ID, "Failed to create the plan.")
	}

	reply := fmt.Sprintf("✅ Plan '%s' created successfully with ID: %s", plan.Name, plan.ID)
	return r.SendMessage(ctx, message.Chat.ID, reply)
}

func (r *RealTelegramBotAdapter) handleDeletePlanCommand(ctx context.Context, message *tgbotapi.Message) error {
	planID := message.CommandArguments()
	if strings.TrimSpace(planID) == "" {
		return r.SendMessage(ctx, message.Chat.ID, "Usage: /delete_plan <plan_id>")
	}

	text, err := r.facade.HandleDeletePlan(ctx, planID)
	if err != nil {
		// Handle specific business rule error
		if errors.Is(err, domain.ErrSubsciptionWithActiveUser) {
			return r.SendMessage(ctx, message.Chat.ID, "Cannot delete plan: it is currently in use by active or reserved subscriptions.")
		}
		r.log.Error().Err(err).Str("plan_id", planID).Msg("failed to delete plan")
		return r.SendMessage(ctx, message.Chat.ID, "Failed to delete the plan.")
	}

	return r.SendMessage(ctx, message.Chat.ID, text)
}

func (r *RealTelegramBotAdapter) handleUpdatePlanCommand(ctx context.Context, message *tgbotapi.Message) error {
	args := strings.Fields(message.CommandArguments())
	if len(args) != 5 {
		return r.SendMessage(ctx, message.Chat.ID, "Usage: /update_plan <ID> <Name> <Days> <Credits> <PriceIRR>")
	}

	id := args[0]
	name := args[1]
	days, err1 := strconv.Atoi(args[2])
	credits, err2 := strconv.ParseInt(args[3], 10, 64)
	price, err3 := strconv.ParseInt(args[4], 10, 64)

	if err1 != nil || err2 != nil || err3 != nil {
		return r.SendMessage(ctx, message.Chat.ID, "Invalid arguments. Days, Credits, and Price must be numbers.")
	}

	text, err := r.facade.HandleUpdatePlan(ctx, id, name, days, credits, price)
	if err != nil {
		r.log.Error().Err(err).Str("plan_id", id).Msg("failed to update plan")
		return r.SendMessage(ctx, message.Chat.ID, "Failed to update the plan.")
	}

	return r.SendMessage(ctx, message.Chat.ID, text)
}

func (r *RealTelegramBotAdapter) handleUpdatePricingCommand(ctx context.Context, message *tgbotapi.Message) error {
	args := strings.Fields(message.CommandArguments())
	if len(args) != 3 {
		return r.SendMessage(ctx, message.Chat.ID, "Usage: /update_pricing <model_name> <input_price_micros> <output_price_micros>")
	}

	modelName := args[0]
	inputPrice, err1 := strconv.ParseInt(args[1], 10, 64)
	outputPrice, err2 := strconv.ParseInt(args[2], 10, 64)

	if err1 != nil || err2 != nil {
		return r.SendMessage(ctx, message.Chat.ID, "Invalid arguments. Prices must be numbers.")
	}

	text, err := r.facade.HandleUpdatePricing(ctx, modelName, inputPrice, outputPrice)
	if err != nil {
		r.log.Error().Err(err).Str("model_name", modelName).Msg("failed to update pricing")
		return r.SendMessage(ctx, message.Chat.ID, "Failed to update pricing.")
	}

	return r.SendMessage(ctx, message.Chat.ID, text)
}
