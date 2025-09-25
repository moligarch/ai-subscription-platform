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
	ID                 string             `json:"id"`
	TelegramID         int64              `json:"telegram_id"`
	Username           string             `json:"username"`
	FullName           string             `json:"full_name"`
	PhoneNumber        string             `json:"phone_number"`
	RegistrationStatus RegistrationStatus `json:"registration_status"`
	RegisteredAt       time.Time          `json:"registered_at"`
	LastActiveAt       time.Time          `json:"last_active_at"`
	IsAdmin            bool               `json:"is_admin"`
	LanguageCode       string             `json:"language_code"`
	Privacy            PrivacySettings    `json:"privacy"`
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
