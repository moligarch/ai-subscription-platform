package model

import (
	"time"

	"telegram-ai-subscription/internal/domain"

	"github.com/google/uuid"
)

// User is a domain entity representing a Telegram user in our system.
// Privacy settings are embedded to ensure a single source of truth in-memory.
type User struct {
	ID           string
	TelegramID   int64
	Username     string
	RegisteredAt time.Time
	LastActiveAt time.Time
	IsAdmin      bool
	Privacy      PrivacySettings
}

func NewUser(id string, tgID int64, username string) (*User, error) {
	if id == "" {
		id = uuid.NewString()
	}
	if tgID <= 0 {
		return nil, domain.ErrInvalidArgument
	}
	if username == "" {
		return nil, domain.ErrInvalidArgument
	}
	u := &User{
		ID:           id,
		TelegramID:   tgID,
		Username:     username,
		RegisteredAt: time.Now(),
		LastActiveAt: time.Now(),
		IsAdmin:      false,
		Privacy:      *NewPrivacySettings(id),
	}
	return u, nil
}

func (u *User) IsZero() bool { return u == nil || u.ID == "" }
func (u *User) Touch()       { u.LastActiveAt = time.Now() }
