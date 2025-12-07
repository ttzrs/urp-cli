// Package server provides HTTP server services for the desktop GUI.
// Implements DIP: high-level HTTP handlers depend on interfaces, not concrete implementations.
package server

import (
	"context"
	"sync"
	"time"
)

// AgentService defines the interface for agent operations.
// HTTP handlers depend on this interface (DIP), allowing easy mocking and testing.
type AgentService interface {
	// ProcessPrompt sends a prompt to the agent and returns the response.
	ProcessPrompt(ctx context.Context, req PromptRequest) (*PromptResponse, error)

	// StreamPrompt sends a prompt and streams responses via callback.
	StreamPrompt(ctx context.Context, req PromptRequest, callback StreamCallback) error
}

// PromptRequest contains the prompt and optional configuration.
type PromptRequest struct {
	Prompt    string `json:"prompt"`
	SessionID string `json:"sessionId,omitempty"`
}

// PromptResponse contains the agent's response.
type PromptResponse struct {
	Status    string `json:"status"`
	Response  string `json:"response,omitempty"`
	SessionID string `json:"sessionId,omitempty"`
	Timestamp int64  `json:"timestamp"`
	Error     string `json:"error,omitempty"`
}

// StreamCallback is called for each chunk of streaming response.
type StreamCallback func(event StreamEvent)

// StreamEvent represents a streaming response event.
type StreamEvent struct {
	Type      string      `json:"type"` // "start", "chunk", "done", "error"
	Data      interface{} `json:"data"`
	ID        string      `json:"id,omitempty"`
	Timestamp int64       `json:"timestamp"`
}

// FocusService defines the interface for focus operations.
type FocusService interface {
	// GetFocus returns the current focus state.
	GetFocus(ctx context.Context) (*FocusState, error)

	// SetFocus sets a new focus target.
	SetFocus(ctx context.Context, target string, depth int) (*FocusResult, error)
}

// FocusState represents the current focus.
type FocusState struct {
	Target string `json:"target"`
	Depth  int    `json:"depth"`
}

// FocusResult contains the result of a focus operation.
type FocusResult struct {
	Success  bool   `json:"success"`
	Entities int    `json:"entities"`
	Rendered string `json:"rendered"`
}

// ContextTracker tracks token and file usage.
type ContextTracker interface {
	// GetUsage returns current context window usage.
	GetUsage() ContextUsage

	// AddTokens adds to the token count.
	AddTokens(count int)

	// AddFile records a loaded file.
	AddFile(path string)

	// Reset clears all usage.
	Reset()
}

// ContextUsage represents context window usage.
type ContextUsage struct {
	Tokens int      `json:"tokens"`
	Files  int      `json:"files"`
	Paths  []string `json:"paths,omitempty"`
}

// ─────────────────────────────────────────────────────────────────────────────
// Default Implementations
// ─────────────────────────────────────────────────────────────────────────────

// DefaultContextTracker is a simple in-memory context tracker.
type DefaultContextTracker struct {
	mu     sync.RWMutex
	tokens int
	files  map[string]struct{}
}

// NewContextTracker creates a new context tracker.
func NewContextTracker() *DefaultContextTracker {
	return &DefaultContextTracker{
		files: make(map[string]struct{}),
	}
}

// GetUsage returns current usage.
func (t *DefaultContextTracker) GetUsage() ContextUsage {
	t.mu.RLock()
	defer t.mu.RUnlock()

	paths := make([]string, 0, len(t.files))
	for p := range t.files {
		paths = append(paths, p)
	}

	return ContextUsage{
		Tokens: t.tokens,
		Files:  len(t.files),
		Paths:  paths,
	}
}

// AddTokens adds to token count.
func (t *DefaultContextTracker) AddTokens(count int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.tokens += count
}

// AddFile records a file.
func (t *DefaultContextTracker) AddFile(path string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.files[path] = struct{}{}
}

// Reset clears usage.
func (t *DefaultContextTracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.tokens = 0
	t.files = make(map[string]struct{})
}

// ─────────────────────────────────────────────────────────────────────────────
// Stub Implementation (for serve.go until full agent integration)
// ─────────────────────────────────────────────────────────────────────────────

// StubAgentService provides a stub implementation for testing/development.
type StubAgentService struct{}

// ProcessPrompt returns a stub response.
func (s *StubAgentService) ProcessPrompt(ctx context.Context, req PromptRequest) (*PromptResponse, error) {
	return &PromptResponse{
		Status:    "stub",
		Response:  "Agent integration pending. Received: " + req.Prompt,
		Timestamp: time.Now().UnixMilli(),
	}, nil
}

// StreamPrompt streams stub responses.
func (s *StubAgentService) StreamPrompt(ctx context.Context, req PromptRequest, callback StreamCallback) error {
	// Send start
	callback(StreamEvent{
		Type:      "start",
		Data:      map[string]interface{}{"prompt": req.Prompt},
		Timestamp: time.Now().UnixMilli(),
	})

	// Simulate processing chunks
	chunks := []string{
		"Analyzing prompt...",
		"Searching codebase...",
		"Found relevant context.",
		"Generating response...",
	}

	for i, chunk := range chunks {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
			callback(StreamEvent{
				Type: "chunk",
				Data: map[string]interface{}{
					"content": chunk,
					"index":   i,
				},
				ID:        string(rune('0' + i)),
				Timestamp: time.Now().UnixMilli(),
			})
		}
	}

	// Send completion
	callback(StreamEvent{
		Type:      "done",
		Data:      map[string]interface{}{"totalChunks": len(chunks)},
		Timestamp: time.Now().UnixMilli(),
	})

	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Interface Compliance
// ─────────────────────────────────────────────────────────────────────────────

var (
	_ AgentService   = (*StubAgentService)(nil)
	_ ContextTracker = (*DefaultContextTracker)(nil)
)
