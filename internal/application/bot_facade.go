// File: internal/application/bot_facade.go
package application

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/usecase"
)

// BotFacade orchestrates bot commands over use cases.
// NOTE: We bind to the concrete usecase structs (pointers) to match your current codebase.
type BotFacade struct {
	UserUC         usecase.UserUseCase
	PlanUC         usecase.PlanUseCase
	SubscriptionUC usecase.SubscriptionUseCase
	PaymentUC      usecase.PaymentUseCase
	ChatUC         usecase.ChatUseCase

	callbackURL string
}

func NewBotFacade(
	userUC usecase.UserUseCase,
	planUC usecase.PlanUseCase,
	subUC usecase.SubscriptionUseCase,
	paymentUC usecase.PaymentUseCase,
	chatUC usecase.ChatUseCase,
	callbackURL string,
) *BotFacade {
	return &BotFacade{
		UserUC:         userUC,
		PlanUC:         planUC,
		SubscriptionUC: subUC,
		PaymentUC:      paymentUC,
		ChatUC:         chatUC,
		callbackURL:    callbackURL,
	}
}

// HandleStart ensures user exists and returns quick help text.
func (b *BotFacade) HandleStart(ctx context.Context, tgID int64, username string) (string, error) {
	if _, err := b.UserUC.RegisterOrFetch(ctx, tgID, username); err != nil {
		return "", fmt.Errorf("register user: %w", err)
	}
	return "Welcome! Use /plans to view plans, /status to check your subscription, /chat to talk to AI.", nil
}

// HandlePlans lists available plans.
func (b *BotFacade) HandlePlans(ctx context.Context, tgID int64) (string, error) {
	plans, err := b.PlanUC.List(ctx)
	if err != nil {
		return "", fmt.Errorf("list plans: %w", err)
	}
	if len(plans) == 0 {
		return "No plans available yet. Please check back later.", nil
	}
	var sb strings.Builder
	sb.WriteString("Available plans:\n")
	for _, p := range plans {
		sb.WriteString(fmt.Sprintf("- %s: %d credits, %d days (id: %s)\n", p.Name, p.Credits, p.DurationDays, p.ID))
	}
	sb.WriteString("\nUse /buy <plan_id> to purchase.")
	return sb.String(), nil
}

// HandleCreatePlan creates a new plan (admin).
// NOTE: your PlanUC.Create has signature: Create(ctx, name string, durationDays int, credits int, priceIRR int64) (string, error)
func (b *BotFacade) HandleCreatePlan(ctx context.Context, name string, durationDays, credits int) (string, error) {
	const priceIRR int64 = 1000 // dev price; adjust or pass from admin command parsing later
	planUC, err := b.PlanUC.Create(ctx, name, durationDays, credits, priceIRR)
	if err != nil {
		return "", fmt.Errorf("create plan: %w", err)
	}
	return fmt.Sprintf("Plan %q created with id %s", name, planUC.ID), nil
}

// HandleUpdatePlan updates an existing plan (admin).
func (b *BotFacade) HandleUpdatePlan(ctx context.Context, id, name string, durationDays, credits int) (string, error) {
	plan, err := b.PlanUC.Get(ctx, id)
	if err != nil {
		return "", fmt.Errorf("get plan: %w", err)
	}
	plan.Name = name
	plan.DurationDays = durationDays
	plan.Credits = credits
	// No UpdatedAt field on model.SubscriptionPlan in your codebase; repo handles timestamps.
	if err := b.PlanUC.Update(ctx, plan); err != nil {
		return "", fmt.Errorf("update plan: %w", err)
	}
	return fmt.Sprintf("Plan %s updated.", id), nil
}

// HandleDeletePlan deletes a plan (admin).
func (b *BotFacade) HandleDeletePlan(ctx context.Context, id string) (string, error) {
	if err := b.PlanUC.Delete(ctx, id); err != nil {
		return "", fmt.Errorf("delete plan: %w", err)
	}
	return fmt.Sprintf("Plan %s deleted.", id), nil
}

// HandleSubscribe starts payment flow for a plan.
// PaymentUC.Initiate signature in your code expects amount as STRING.
func (b *BotFacade) HandleSubscribe(ctx context.Context, tgID int64, planID string) (string, error) {
	user, err := b.UserUC.GetByTelegramID(ctx, tgID)
	if err != nil {
		return "", fmt.Errorf("ensure user: %w", err)
	}
	plan, err := b.PlanUC.Get(ctx, planID)
	if err != nil {
		return "", fmt.Errorf("plan not found: %w", err)
	}

	amountIRR := plan.PriceIRR
	amountStr := strconv.FormatInt(amountIRR, 10)

	meta := map[string]interface{}{
		"plan_name": plan.Name,
		"user_tg":   tgID,
	}
	p, payURL, err := b.PaymentUC.Initiate(ctx, user.ID, plan.ID, amountStr, "Plan purchase", meta)
	if err != nil {
		return "", fmt.Errorf("initiate payment: %w", err)
	}
	_ = p
	return fmt.Sprintf(
		"Please complete your payment at: %s\nAfter success you'll be redirected and your plan will activate.",
		payURL,
	), nil
}

// HandleStatus shows active subscription.
func (b *BotFacade) HandleStatus(ctx context.Context, tgID int64) (string, error) {
	user, err := b.UserUC.GetByTelegramID(ctx, tgID)
	if err != nil {
		return "", fmt.Errorf("user not found: %w", err)
	}
	sub, err := b.SubscriptionUC.GetActive(ctx, user.ID)
	if err != nil || sub == nil {
		return "You have no active or reserved subscription.", nil
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Active plan: %s\n", sub.PlanID))
	sb.WriteString(fmt.Sprintf("Credits: %d\n", sub.RemainingCredits))
	if !sub.ExpiresAt.IsZero() {
		sb.WriteString(fmt.Sprintf("Expires: %s\n", sub.ExpiresAt.Format("2006-01-02")))
	}
	return sb.String(), nil
}

// HandleBalance shows remaining credits of active sub.
func (b *BotFacade) HandleBalance(ctx context.Context, tgID int64) (string, error) {
	user, err := b.UserUC.GetByTelegramID(ctx, tgID)
	if err != nil {
		return "", fmt.Errorf("user not found: %w", err)
	}
	sub, err := b.SubscriptionUC.GetActive(ctx, user.ID)
	if err != nil || sub == nil {
		return "No active subscription.", nil
	}
	return fmt.Sprintf("Remaining credits: %d", sub.RemainingCredits), nil
}

// HandleStartChat opens a chat session via ChatUC.
// Your ChatUC.ListModels now matches the AI port: ListModels(ctx) ([]string, error)
func (b *BotFacade) HandleStartChat(ctx context.Context, tgID int64, modelName string) (string, error) {
	user, err := b.UserUC.GetByTelegramID(ctx, tgID)
	if err != nil {
		return "", fmt.Errorf("user not found: %w", err)
	}
	if modelName == "" {
		if b.ChatUC != nil {
			models, err := b.ChatUC.ListModels(ctx)
			if err == nil && len(models) > 0 {
				modelName = models[0]
			} else {
				modelName = "gpt-4o-mini"
			}
		} else {
			modelName = "gpt-4o-mini"
		}
	}
	if _, err := b.ChatUC.StartChat(ctx, user.ID, modelName); err != nil {
		return "", fmt.Errorf("start chat: %w", err)
	}
	return fmt.Sprintf("Started chat with %s. Send messages, or /bye to end.", modelName), nil
}

// HandleEndChat ends a chat session by id (adapter passes session id).
func (b *BotFacade) HandleEndChat(ctx context.Context, tgID int64, sessionID string) (string, error) {
	if sessionID == "" {
		return "No active chat session id provided.", nil
	}
	if err := b.ChatUC.EndChat(ctx, sessionID); err != nil {
		return "", fmt.Errorf("end chat: %w", err)
	}
	return "Chat session ended. Use /chat to start a new conversation.", nil
}

// HandleChatMessage sends a message via the user's active session.
// If SendMessage returns ErrNoActiveSubscription or similar, show a friendly instruction.
func (b *BotFacade) HandleChatMessage(ctx context.Context, tgID int64, text string) (string, error) {
	user, err := b.UserUC.GetByTelegramID(ctx, tgID)
	if err != nil || user == nil {
		return "", fmt.Errorf("user not found: %w", err)
	}

	sess, err := b.ChatUC.FindActiveSession(ctx, user.ID)
	if err != nil || sess == nil {
		return "You're not in a chat. Send /chat to start one.", nil
	}

	reply, err := b.ChatUC.SendMessage(ctx, sess.ID, text)
	if err != nil {
		// Prefer typed error mapping if your UC exposes it:
		//   usecase.ErrNoActiveSubscription
		// Fall back to string sniffing to avoid leaking internals.
		if errors.Is(err, domain.ErrNoActiveSubscription) ||
			strings.Contains(strings.ToLower(err.Error()), "entity not found") ||
			strings.Contains(strings.ToLower(err.Error()), "no active subscription") ||
			strings.Contains(strings.ToLower(err.Error()), "not enough credits") {
			return "You need an active subscription (with credits) to chat.\nUse /plans to purchase, then /chat.", nil
		}
		return "", fmt.Errorf("send message: %w", err)
	}
	return reply, nil
}
