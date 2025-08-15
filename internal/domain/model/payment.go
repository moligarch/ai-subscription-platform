package model

import (
	"telegram-ai-subscription/internal/domain"
	"time"
)

// PaymentStatus tracks result of a payment attempt.
type PaymentStatus string

const (
	PaymentPending PaymentStatus = "pending"
	PaymentSuccess PaymentStatus = "success"
	PaymentFailed  PaymentStatus = "failed"
)

// Payment entity.
type Payment struct {
	ID        string // UUID
	UserID    string
	Amount    float64 // in Toman
	Method    string  // e.g., "mellat", "zarinpal"
	Status    PaymentStatus
	CreatedAt time.Time
}

// NewPayment constructs a Payment.
func NewPayment(id, userID, method string, amount float64) (*Payment, error) {
	if id == "" || userID == "" || method == "" || amount <= 0 {
		return nil, domain.ErrInvalidArgument
	}
	return &Payment{
		ID:        id,
		UserID:    userID,
		Amount:    amount,
		Method:    method,
		Status:    PaymentPending,
		CreatedAt: time.Now(),
	}, nil
}

// MarkSuccess and MarkFailed return updated copies.
func (p *Payment) MarkSuccess() *Payment {
	copy := *p
	copy.Status = PaymentSuccess
	return &copy
}

func (p *Payment) MarkFailed() *Payment {
	copy := *p
	copy.Status = PaymentFailed
	return &copy
}
