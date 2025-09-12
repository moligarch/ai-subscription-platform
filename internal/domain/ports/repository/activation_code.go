package repository

import (
	"context"
	"telegram-ai-subscription/internal/domain/model"
)

// ActivationCodeRepository is the port for managing activation codes.
type ActivationCodeRepository interface {
	// Save creates or updates an activation code.
	Save(ctx context.Context, tx Tx, code *model.ActivationCode) error
	// FindByCode finds an unredeemed activation code.
	FindByCode(ctx context.Context, tx Tx, code string) (*model.ActivationCode, error)
}
