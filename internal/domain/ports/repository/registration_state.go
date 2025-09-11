package repository

import (
	"context"
)

// RegistrationStep defines the possible steps in the registration flow.
type RegistrationStep string

const (
	StateAwaitingFullName     RegistrationStep = "awaiting_fullname"
	StateAwaitingPhone        RegistrationStep = "awaiting_phone"
	StateAwaitingVerification RegistrationStep = "awaiting_verification"
)

// RegistrationState holds the user's current progress.
type RegistrationState struct {
	Step RegistrationStep  `json:"step"`
	Data map[string]string `json:"data"` // To store collected info like full_name
}

// RegistrationStateRepository is the port for managing user registration state.
type RegistrationStateRepository interface {
	SetState(ctx context.Context, tgID int64, state *RegistrationState) error
	GetState(ctx context.Context, tgID int64) (*RegistrationState, error)
	ClearState(ctx context.Context, tgID int64) error
}
