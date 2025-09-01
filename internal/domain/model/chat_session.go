package model

import (
	"time"
)

type ChatSessionStatus string

const (
	ChatSessionActive   ChatSessionStatus = "active"
	ChatSessionFinished ChatSessionStatus = "finished"
)

// ChatMessage represents one message within a chat session.
type ChatMessage struct {
	SessionID string
	Role      string // "user" | "assistant" | "system"
	Content   string
	Tokens    int
	Timestamp time.Time
}

// ChatSession is the aggregate root for a running conversation with a model.
type ChatSession struct {
	ID        string
	UserID    string
	Model     string
	Status    ChatSessionStatus
	Messages  []ChatMessage
	CreatedAt time.Time
	UpdatedAt time.Time
}

func NewChatSession(id, userID, model string) *ChatSession {
	return &ChatSession{
		ID:        id,
		UserID:    userID,
		Model:     model,
		Status:    ChatSessionActive,
		Messages:  make([]ChatMessage, 0, 8),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

func (s *ChatSession) AddMessage(role, content string, tokens int) {
	s.Messages = append(s.Messages, ChatMessage{
		SessionID: s.ID,
		Role:      role,
		Content:   content,
		Tokens:    tokens,
		Timestamp: time.Now(),
	})
	s.UpdatedAt = time.Now()
}

func (s *ChatSession) GetRecentMessages(n int) []ChatMessage {
	if n <= 0 || len(s.Messages) <= n {
		return s.Messages
	}
	return s.Messages[len(s.Messages)-n:]
}
