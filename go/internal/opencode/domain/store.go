package domain

import "context"

// SessionStore defines the interface for session persistence
// This interface lives in domain to satisfy Dependency Inversion Principle
type SessionStore interface {
	CreateSession(ctx context.Context, sess *Session) error
	GetSession(ctx context.Context, id string) (*Session, error)
	ListSessions(ctx context.Context, projectID string, limit int) ([]*Session, error)
	UpdateSession(ctx context.Context, sess *Session) error
	DeleteSession(ctx context.Context, id string) error
}

// MessageStore defines the interface for message persistence
type MessageStore interface {
	CreateMessage(ctx context.Context, msg *Message) error
	GetMessages(ctx context.Context, sessionID string) ([]*Message, error)
	UpdateMessage(ctx context.Context, msg *Message) error
	DeleteMessage(ctx context.Context, id string) error
}

// UsageStore handles usage tracking persistence
type UsageStore interface {
	GetUsage(ctx context.Context, sessionID string) (*SessionUsage, error)
	UpdateUsage(ctx context.Context, usage *SessionUsage) error
	GetTotalUsage(ctx context.Context) (*Usage, error)
}

// Store combines session, message, and usage storage
// Implementations can satisfy this or the individual interfaces
type Store interface {
	SessionStore
	MessageStore
	UsageStore
}
