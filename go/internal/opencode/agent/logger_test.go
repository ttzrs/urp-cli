package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestAgentLogger_LLMCall(t *testing.T) {
	var buf bytes.Buffer
	logger := NewAgentLogger(
		WithLogLevel(LogLevelDebug),
		WithLogOutput(&buf),
		WithSessionID("test-session"),
		WithModel("test-model"),
	)

	ctx := context.Background()
	logger.LLMCall(ctx, "claude-3", 1500, 100, 200, 50, 80, 20, 0.005, nil)

	var entry LogEntry
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to unmarshal log entry: %v", err)
	}

	if entry.Type != "llm_call" {
		t.Errorf("expected type 'llm_call', got %q", entry.Type)
	}
	if entry.Model != "claude-3" {
		t.Errorf("expected model 'claude-3', got %q", entry.Model)
	}
	if entry.InputTokens != 100 {
		t.Errorf("expected input_tokens 100, got %d", entry.InputTokens)
	}
	if entry.OutputTokens != 200 {
		t.Errorf("expected output_tokens 200, got %d", entry.OutputTokens)
	}
	if entry.Level != "info" {
		t.Errorf("expected level 'info', got %q", entry.Level)
	}
}

func TestAgentLogger_ToolCall(t *testing.T) {
	var buf bytes.Buffer
	logger := NewAgentLogger(
		WithLogLevel(LogLevelInfo),
		WithLogOutput(&buf),
	)

	ctx := context.Background()
	args := map[string]any{
		"file_path": "/test/file.go",
		"content":   "some content",
	}
	logger.ToolCall(ctx, "write", args, 250, "success", nil)

	var entry LogEntry
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to unmarshal log entry: %v", err)
	}

	if entry.Type != "tool_call" {
		t.Errorf("expected type 'tool_call', got %q", entry.Type)
	}
	if entry.ToolName != "write" {
		t.Errorf("expected tool_name 'write', got %q", entry.ToolName)
	}
	if entry.DurationMs != 250 {
		t.Errorf("expected duration_ms 250, got %d", entry.DurationMs)
	}
}

func TestAgentLogger_Error(t *testing.T) {
	var buf bytes.Buffer
	logger := NewAgentLogger(
		WithLogLevel(LogLevelError),
		WithLogOutput(&buf),
	)

	ctx := context.Background()
	logger.Error(ctx, "tool_error", errors.New("something went wrong"), map[string]any{
		"tool": "bash",
	})

	var entry LogEntry
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to unmarshal log entry: %v", err)
	}

	if entry.Level != "error" {
		t.Errorf("expected level 'error', got %q", entry.Level)
	}
	if entry.Error != "something went wrong" {
		t.Errorf("expected error 'something went wrong', got %q", entry.Error)
	}
}

func TestSanitizeArgs(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]any
		expected map[string]any
	}{
		{
			name:     "nil input",
			input:    nil,
			expected: nil,
		},
		{
			name: "redacts password",
			input: map[string]any{
				"username": "user",
				"password": "secret123",
			},
			expected: map[string]any{
				"username": "user",
				"password": "[REDACTED]",
			},
		},
		{
			name: "truncates long content",
			input: map[string]any{
				"content": string(make([]byte, 300)),
			},
			expected: map[string]any{
				"content": string(make([]byte, 197)) + "...",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeArgs(tt.input)
			if tt.expected == nil {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
				return
			}

			for k, v := range tt.expected {
				if result[k] != v {
					t.Errorf("key %q: expected %q, got %q", k, v, result[k])
				}
			}
		})
	}
}

func TestLogLevel(t *testing.T) {
	var buf bytes.Buffer
	logger := NewAgentLogger(
		WithLogLevel(LogLevelWarn),
		WithLogOutput(&buf),
	)

	ctx := context.Background()

	// Info should not log at Warn level
	logger.LLMCall(ctx, "test", 100, 10, 10, 0, 0, 0, 0.001, nil)
	if buf.Len() > 0 {
		t.Error("info should not be logged at warn level")
	}

	// Error should log at Warn level
	logger.Error(ctx, "test_error", errors.New("error"), nil)
	if buf.Len() == 0 {
		t.Error("error should be logged at warn level")
	}
}
