package domain

import (
    "context"
    "regexp"
    "time"

    "github.com/google/uuid"
)

// User is an immutable domain entity.
type User struct {
    ID         string    // UUID
    TelegramID int64     // unique per Telegram
    FullName   string
    Phone      string
    CreatedAt  time.Time
}

// NewUser constructs and validates a User.
func NewUser(id string, tgID int64, fullName, phone string) (*User, error) {
    if id == "" {
        return nil, ErrInvalidArgument
    }
    if tgID <= 0 {
        return nil, ErrInvalidArgument
    }
    if fullName == "" {
        return nil, ErrInvalidArgument
    }
    // simple phone validation
    if phone != "" {
        re := regexp.MustCompile(`^\+?[0-9]{10,15}$`)
        if !re.MatchString(phone) {
            return nil, ErrInvalidArgument
        }
    }
    return &User{ID: id, TelegramID: tgID, FullName: fullName, Phone: phone, CreatedAt: time.Now()}, nil
}


func NewUUID() string {
	return uuid.NewString()
}


// UserRepository defines thread-safe methods (must support concurrent calls)
type UserRepository interface {
    // Save persists a new or existing user
    Save(ctx context.Context, u *User) error
    // FindByTelegramID returns ErrNotFound if missing
    FindByTelegramID(ctx context.Context, tgID int64) (*User, error)
}
