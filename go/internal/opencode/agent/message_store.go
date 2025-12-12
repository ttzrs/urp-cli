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

// EstimateTokens returns a rough estimate of tokens in all messages.
// Uses ~4 chars per token heuristic.
func (s *MessageStore) EstimateTokens() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	total := 0
	for _, msg := range s.messages {
		for _, part := range msg.Parts {
			switch p := part.(type) {
			case domain.TextPart:
				total += len(p.Text) / 4
			case domain.ToolCallPart:
				total += len(p.Result) / 4
				// Args can also be large
				for _, v := range p.Args {
					if str, ok := v.(string); ok {
						total += len(str) / 4
					}
				}
			}
		}
	}
	return total
}

// TruncateIfNeeded removes oldest messages if tokens exceed maxTokens.
// Keeps at least minMessages messages. Returns true if truncation occurred.
func (s *MessageStore) TruncateIfNeeded(maxTokens int, minMessages int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if len(s.messages) <= minMessages {
		return false
	}
	
	// Calculate current tokens
	total := 0
	for _, msg := range s.messages {
		for _, part := range msg.Parts {
			switch p := part.(type) {
			case domain.TextPart:
				total += len(p.Text) / 4
			case domain.ToolCallPart:
				total += len(p.Result) / 4
			}
		}
	}
	
	if total <= maxTokens {
		return false
	}
	
	// Remove oldest messages until under limit (keep minMessages)
	for total > maxTokens && len(s.messages) > minMessages {
		// Remove first message
		removed := s.messages[0]
		s.messages = s.messages[1:]
		
		// Subtract removed tokens
		for _, part := range removed.Parts {
			switch p := part.(type) {
			case domain.TextPart:
				total -= len(p.Text) / 4
			case domain.ToolCallPart:
				total -= len(p.Result) / 4
			}
		}
	}
	
	return true
}
