package model

import "time"

type AIJobStatus string

const (
	AIJobStatusPending    AIJobStatus = "pending"
	AIJobStatusProcessing AIJobStatus = "processing"
	AIJobStatusCompleted  AIJobStatus = "completed"
	AIJobStatusFailed     AIJobStatus = "failed"
)

type AIJob struct {
	ID            string
	Status        AIJobStatus
	SessionID     string
	UserMessageID string
	Retries       int
	LastError     string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}
