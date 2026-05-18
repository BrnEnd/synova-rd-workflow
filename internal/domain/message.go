package domain

import "time"

// Session represents a conversation context tied to a phone number.
type Session struct {
	ID          string
	PhoneNumber string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Message represents a single turn in a conversation.
type Message struct {
	ID        string
	SessionID string
	Role      string // "user" | "assistant"
	Content   string
	Intent    string
	CreatedAt time.Time
}
