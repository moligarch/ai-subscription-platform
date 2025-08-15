package domain

import (
	"time"

	"github.com/google/uuid"
)

// User is an immutable domain entity.
type User struct {
	ID           string
	TelegramID   int64
	Username     string
	RegisteredAt time.Time
	LastActiveAt time.Time
}

// NewUser constructs and validates a User.
func NewUser(id string, tgID int64, username string) (*User, error) {
	if id == "" {
		return nil, ErrInvalidArgument
	}
	if tgID <= 0 {
		return nil, ErrInvalidArgument
	}
	if username == "" {
		return nil, ErrInvalidArgument
	}

	return &User{ID: id, TelegramID: tgID, Username: username, RegisteredAt: time.Now(), LastActiveAt: time.Now()}, nil
}

func NewUUID() string {
	return uuid.NewString()
}
