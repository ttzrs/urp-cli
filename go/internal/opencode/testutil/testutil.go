// Package testutil provides common test helpers and utilities.
package testutil

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/joss/urp/internal/opencode/domain"
	"github.com/joss/urp/internal/opencode/tool"
	"github.com/joss/urp/pkg/llm"
	"github.com/stretchr/testify/require"
)

// Contains reports whether substr is within s.
func Contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

// TempDir creates a temporary directory and returns its path.
// The directory is automatically cleaned up when the test ends.
func TempDir(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}

// WriteFile creates a file with the given content in the specified directory.
func WriteFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
	return path
}

// ReadFile reads the content of a file.
func ReadFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(content)
}

// SetEnv sets an environment variable and returns a cleanup function.
func SetEnv(t *testing.T, key, value string) {
	t.Helper()
	old := os.Getenv(key)
	require.NoError(t, os.Setenv(key, value))
	t.Cleanup(func() {
		if old == "" {
			os.Unsetenv(key)
		} else {
			os.Setenv(key, old)
		}
	})
}

// MockProvider simulates LLM responses for testing.
type MockProvider struct {
	responses [][]domain.StreamEvent
	callCount int
	mu        sync.Mutex
}

func NewMockProvider(responses ...[]domain.StreamEvent) *MockProvider {
	return &MockProvider{responses: responses}
}

func (m *MockProvider) ID() string   { return "mock" }
func (m *MockProvider) Name() string { return "Mock" }
func (m *MockProvider) Models() []domain.Model {
	return []domain.Model{{ID: "mock", Name: "Mock Model", ShortCode: "mck"}}
}

func (m *MockProvider) Chat(ctx context.Context, req *llm.ChatRequest) (<-chan domain.StreamEvent, error) {
	m.mu.Lock()
	idx := m.callCount
	m.callCount++
	m.mu.Unlock()

	events := make(chan domain.StreamEvent, 100)
	go func() {
		defer close(events)
		if idx < len(m.responses) {
			for _, event := range m.responses[idx] {
				events <- event
			}
		}
	}()
	return events, nil
}

func (m *MockProvider) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.callCount
}

// MockTool is a simple tool for testing.
type MockTool struct {
	ToolName   string
	ToolResult string
	Delay      time.Duration
	OnExecute  func(args map[string]any)
}

func NewMockTool(name string) *MockTool {
	return &MockTool{ToolName: name}
}

func (m *MockTool) WithResult(result string) *MockTool {
	m.ToolResult = result
	return m
}

func (m *MockTool) WithDelay(d time.Duration) *MockTool {
	m.Delay = d
	return m
}

func (m *MockTool) WithCallback(fn func(args map[string]any)) *MockTool {
	m.OnExecute = fn
	return m
}

func (m *MockTool) Info() domain.Tool {
	return domain.Tool{
		Name:        m.ToolName,
		Description: "Mock tool for testing",
		Parameters: domain.JSONSchema{
			"type": "object",
			"properties": map[string]any{
				"message": map[string]any{"type": "string"},
			},
		},
	}
}

func (m *MockTool) Execute(ctx context.Context, args map[string]any) (*tool.Result, error) {
	if m.Delay > 0 {
		select {
		case <-time.After(m.Delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	if m.OnExecute != nil {
		m.OnExecute(args)
	}

	if m.ToolResult != "" {
		return &tool.Result{Output: m.ToolResult}, nil
	}

	msg, _ := args["message"].(string)
	return &tool.Result{Output: msg}, nil
}

// TestSession creates a test session.
func TestSession(id, dir string) *domain.Session {
	if id == "" {
		id = "test-session"
	}
	if dir == "" {
		dir = "/tmp/test"
	}
	return &domain.Session{
		ID:        id,
		Directory: dir,
	}
}

// TextResponse creates a simple text response event sequence.
func TextResponse(text string) []domain.StreamEvent {
	return []domain.StreamEvent{
		{Type: domain.StreamEventText, Content: text},
		{Type: domain.StreamEventDone, Done: true},
	}
}

// ToolCallResponse creates a tool call event sequence.
func ToolCallResponse(toolID, name string, args map[string]any) []domain.StreamEvent {
	return []domain.StreamEvent{
		{Type: domain.StreamEventToolCall, Part: domain.ToolCallPart{
			ToolID: toolID,
			Name:   name,
			Args:   args,
		}},
		{Type: domain.StreamEventDone, Done: true},
	}
}

// DoneResponse creates a done-only response.
func DoneResponse() []domain.StreamEvent {
	return []domain.StreamEvent{
		{Type: domain.StreamEventDone, Done: true},
	}
}

// DrainEvents consumes all events from a channel.
func DrainEvents(events <-chan domain.StreamEvent) {
	for range events {
	}
}

// CollectEvents collects all events from a channel by type.
func CollectEvents(events <-chan domain.StreamEvent) (text string, toolCalls, toolDones int) {
	for event := range events {
		switch event.Type {
		case domain.StreamEventText:
			text += event.Content
		case domain.StreamEventToolCall:
			toolCalls++
		case domain.StreamEventToolDone:
			toolDones++
		}
	}
	return
}
