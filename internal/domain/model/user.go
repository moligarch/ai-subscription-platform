package model

import (
	"time"

	"telegram-ai-subscription/internal/domain"

	"github.com/google/uuid"
)

// Define the RegistrationStatus type and its possible values.
type RegistrationStatus string

const (
	RegistrationStatusPending   RegistrationStatus = "pending"
	RegistrationStatusCompleted RegistrationStatus = "completed"
)

// User is a domain entity representing a Telegram user in our system.
// Privacy settings are embedded to ensure a single source of truth in-memory.
type User struct {
	ID                 string
	TelegramID         int64
	Username           string
	FullName           string
	PhoneNumber        string
	RegistrationStatus RegistrationStatus
	RegisteredAt       time.Time
	LastActiveAt       time.Time
	IsAdmin            bool
	LanguageCode       string
	Privacy            PrivacySettings
}

func NewUser(id string, tgID int64, username string) (*User, error) {
	if id == "" {
		id = uuid.NewString()
	}
	if tgID <= 0 {
		return nil, domain.ErrInvalidArgument
	}

	u := &User{
		ID:                 id,
		TelegramID:         tgID,
		Username:           username,
		RegistrationStatus: RegistrationStatusPending,
		RegisteredAt:       time.Now(),
		LastActiveAt:       time.Now(),
		IsAdmin:            false,
		LanguageCode:       "fa",
		Privacy:            *NewPrivacySettings(id),
	}
	return u, nil
}

func (u *User) IsZero() bool { return u == nil || u.ID == "" }
func (u *User) Touch()       { u.LastActiveAt = time.Now() }
