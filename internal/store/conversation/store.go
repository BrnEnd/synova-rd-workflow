package conversation

import (
	"context"

	"synova-rd-workflow/internal/domain"
)

// Store defines the persistence contract for conversation sessions and messages.
type Store interface {
	GetOrCreateSession(ctx context.Context, phoneNumber string) (domain.Session, error)
	GetRecentMessages(ctx context.Context, sessionID string, limit int) ([]domain.Message, error)
	SaveMessage(ctx context.Context, msg domain.Message) error
}
