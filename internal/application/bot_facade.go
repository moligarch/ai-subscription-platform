package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"telegram-ai-subscription/internal/domain"
	"telegram-ai-subscription/internal/domain/model"
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
func (b *BotFacade) HandleCreatePlan(ctx context.Context, name string, durationDays int, credits, priceIRR int64, supportedModels []string) (*model.SubscriptionPlan, error) {
	plan, err := b.PlanUC.Create(ctx, name, durationDays, credits, priceIRR, supportedModels)
	if err != nil {
		return nil, fmt.Errorf("create plan: %w", err)
	}
	return plan, nil
}

// HandleUpdatePlan updates an existing plan (admin).
func (b *BotFacade) HandleUpdatePlan(ctx context.Context, id, name string, durationDays int, credits, priceIRR int64) (string, error) {
	plan, err := b.PlanUC.Get(ctx, id)
	if err != nil {
		// Translate a not found error to a user-friendly message
		if errors.Is(err, domain.ErrNotFound) {
			return "Plan not found with that ID.", nil
		}
		return "", fmt.Errorf("get plan: %w", err)
	}
	plan.Name = name
	plan.DurationDays = durationDays
	plan.Credits = credits
	plan.PriceIRR = priceIRR // Set the new price

	if err := b.PlanUC.Update(ctx, plan); err != nil {
		return "", fmt.Errorf("update plan: %w", err)
	}
	return fmt.Sprintf("Plan %s updated.", id), nil
}

// HandleUpdatePricing updates model pricing (admin).
func (b *BotFacade) HandleUpdatePricing(ctx context.Context, modelName string, inputPrice, outputPrice int64) (string, error) {
	if err := b.PlanUC.UpdatePricing(ctx, modelName, inputPrice, outputPrice); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return "Model pricing not found for that name.", nil
		}
		return "", fmt.Errorf("update pricing: %w", err)
	}
	return fmt.Sprintf("Pricing for model %s updated.", modelName), nil
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
func (f *BotFacade) HandleSubscribe(ctx context.Context, telegramID int64, planID string) (string, error) {
	if strings.TrimSpace(planID) == "" {
		return "Usage: /buy <plan_id>", nil
	}
	user, err := f.UserUC.GetByTelegramID(ctx, telegramID)
	if err != nil {
		return "No user found. Try /start first.", nil
	}

	// Build payment description (optional)
	desc := "subscription purchase"
	if f.PlanUC != nil {
		if plan, _ := f.PlanUC.Get(ctx, planID); plan != nil {
			desc = "Purchase: " + plan.Name
		}
	}

	meta := map[string]interface{}{
		"user_tg": telegramID,
	}
	_, payURL, err := f.PaymentUC.Initiate(ctx, user.ID, planID, f.callbackURL, desc, meta)
	if err != nil {
		// Handle all known business errors with specific user-facing messages.
		if errors.Is(err, domain.ErrAlreadyHasReserved) {
			return "You already have a reserved subscription. You can purchase a new plan after it activates.", nil
		}
		if errors.Is(err, domain.ErrPlanNotFound) {
			return "The plan ID you provided is invalid. Please use /plans to see available plans.", nil
		}
		// For unexpected errors, return a generic message and log the details.
		return "Failed to initiate payment. Please try again later.", nil
	}

	msg := "Please complete your payment at: " + payURL + "\n" +
		"After success you'll be redirected and your plan will activate."
	return msg, nil
}

// ReservedPlanInfo holds details for a single reserved plan.
type ReservedPlanInfo struct {
	PlanName         string
	ScheduledStartAt *time.Time
}

// StatusInfo is a struct to hold status data for easy localization.
type StatusInfo struct {
	ActivePlanName  string
	ActiveCredits   int64
	ActiveExpiresAt *time.Time
	HasActiveSub    bool
	ReservedPlan    *ReservedPlanInfo
	HasReservedSub  bool
}

// HandleStatus now returns the StatusInfo struct.
func (f *BotFacade) HandleStatus(ctx context.Context, telegramID int64) (*StatusInfo, error) {
	user, err := f.UserUC.GetByTelegramID(ctx, telegramID)
	if err != nil || user == nil {
		return nil, errors.New("user not found")
	}

	info := &StatusInfo{}

	// Active subscription
	active, _ := f.SubscriptionUC.GetActive(ctx, user.ID)
	if active != nil {
		info.HasActiveSub = true
		info.ActiveCredits = active.RemainingCredits
		info.ActiveExpiresAt = active.ExpiresAt
		if plan, err := f.PlanUC.Get(ctx, active.PlanID); err == nil {
			info.ActivePlanName = plan.Name
		} else {
			info.ActivePlanName = active.PlanID // Fallback to ID
		}
	}

	// Reserved subscriptions
	reserved, _ := f.SubscriptionUC.GetReserved(ctx, user.ID)
	if len(reserved) > 0 {
		info.HasReservedSub = true
		for _, rs := range reserved {
			planName := rs.PlanID // Fallback to ID
			if plan, err := f.PlanUC.Get(ctx, rs.PlanID); err == nil {
				planName = plan.Name
			}
			info.ReservedPlan = &ReservedPlanInfo{
				PlanName:         planName,
				ScheduledStartAt: rs.ScheduledStartAt,
			}
		}
	}

	return info, nil
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

	models, err := b.ChatUC.ListModels(ctx, user.ID)
	if err != nil {
		return "", fmt.Errorf("list models: %w", err)
	}

	if modelName == "" {
		if len(models) > 0 {
			modelName = models[0]
		} else {
			// This user's plan supports no models.
			return "Your current plan does not support any active AI models.", nil
		}
	}

	if _, err := b.ChatUC.StartChat(ctx, user.ID, modelName); err != nil {
		if errors.Is(err, domain.ErrActiveChatExists) {
			return "You already have an active chat. Please end it with /bye before starting a new one.", nil
		}
		return "Failed to start a new chat.", fmt.Errorf("start chat: %w", err)
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
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return "You're not in a chat. Send /chat to start one.", nil
		}
		return "Could not find an active chat session.", err
	}

	err = b.ChatUC.SendChatMessage(ctx, sess.ID, text)
	if err != nil {
		if errors.Is(err, domain.ErrNoActiveSubscription) {
			return "❌ You don't have an active subscription. Use /plans to get started.", nil
		}
		return "", fmt.Errorf("send message: %w", err)
	}

	// On success, we return an immediate confirmation message.
	// The actual AI reply will be sent later by the AIJobProcessor worker.
	return "⏳ thinking...", nil
}
