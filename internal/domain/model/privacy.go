package model

import "time"

// PrivacySettings captures per-user storage and encryption preferences.
// Mirrors columns on the users table for simple persistence.
type PrivacySettings struct {
	UserID               string
	AllowMessageStorage  bool
	AutoDeleteMessages   bool
	MessageRetentionDays int
	DataEncrypted        bool
	EncryptionKeyID      string
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

func NewPrivacySettings(userID string) *PrivacySettings {
	now := time.Now()
	return &PrivacySettings{
		UserID:               userID,
		AllowMessageStorage:  true,
		AutoDeleteMessages:   false,
		MessageRetentionDays: 30,
		DataEncrypted:        false,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
}

func (p *PrivacySettings) ShouldStoreMessages() bool {
	return p.AllowMessageStorage
}

func (p *PrivacySettings) ShouldEncryptData() bool {
	return p.DataEncrypted
}

func (p *PrivacySettings) GetRetentionPeriod() time.Duration {
	return time.Duration(p.MessageRetentionDays) * 24 * time.Hour
}
