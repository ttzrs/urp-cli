package agent

import (
	"context"
	"sync"

	"github.com/joss/urp/internal/opencode/domain"
)

// MessageCallback is called when a message should be persisted
type MessageCallback func(ctx context.Context, msg *domain.Message) error

// MessageStore handles message persistence and in-memory storage
// Thread-safe for concurrent access
type MessageStore struct {
	messages []domain.Message
	mu       sync.RWMutex
	callback MessageCallback
}

// NewMessageStore creates a new message store
func NewMessageStore() *MessageStore {
	return &MessageStore{}
}

// OnMessage sets the callback for external persistence
func (s *MessageStore) OnMessage(cb MessageCallback) {
	s.callback = cb
}

// Persist stores a message and triggers callback
func (s *MessageStore) Persist(ctx context.Context, msg *domain.Message) {
	s.mu.Lock()
	s.messages = append(s.messages, *msg)
	s.mu.Unlock()

	if s.callback != nil {
		s.callback(ctx, msg)
	}
}

// Messages returns a copy of all stored messages
func (s *MessageStore) Messages() []domain.Message {
	s.mu.RLock()
	defer s.mu.RUnlock()
	msgs := make([]domain.Message, len(s.messages))
	copy(msgs, s.messages)
	return msgs
}

// SetMessages replaces all messages (for compaction)
func (s *MessageStore) SetMessages(msgs []domain.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = make([]domain.Message, len(msgs))
	copy(s.messages, msgs)
}

// Count returns the number of stored messages
func (s *MessageStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.messages)
}

// Clear removes all messages
func (s *MessageStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = nil
}
