// Package agent provides the main AI agent implementation.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// LogLevel controls which events are logged
type LogLevel int

const (
	LogLevelOff LogLevel = iota
	LogLevelError
	LogLevelWarn
	LogLevelInfo
	LogLevelDebug
)

// LogEntry is a structured JSON log entry for agent operations
type LogEntry struct {
	Timestamp   time.Time `json:"timestamp"`
	Level       string    `json:"level"`
	Type        string    `json:"type"`
	SessionID   string    `json:"session_id,omitempty"`
	Model       string    `json:"model,omitempty"`
	DurationMs  int64     `json:"duration_ms,omitempty"`
	Error       string    `json:"error,omitempty"`

	// LLM call details
	InputTokens  int     `json:"input_tokens,omitempty"`
	OutputTokens int     `json:"output_tokens,omitempty"`
	ThinkTokens  int     `json:"think_tokens,omitempty"`
	CacheRead    int     `json:"cache_read,omitempty"`
	CacheWrite   int     `json:"cache_write,omitempty"`
	TotalCost    float64 `json:"total_cost,omitempty"`

	// Tool call details
	ToolName   string         `json:"tool_name,omitempty"`
	ToolArgs   map[string]any `json:"tool_args,omitempty"`
	ToolResult string         `json:"tool_result,omitempty"`

	// Extra fields
	Extra map[string]any `json:"extra,omitempty"`
}

// AgentLogger provides structured JSON logging for agent operations
type AgentLogger struct {
	mu        sync.Mutex
	level     LogLevel
	output    io.Writer
	sessionID string
	model     string
}

// NewAgentLogger creates a new structured logger
func NewAgentLogger(opts ...LoggerOption) *AgentLogger {
	l := &AgentLogger{
		level:  LogLevelInfo,
		output: io.Discard, // Default: no logging
	}
	for _, opt := range opts {
		opt(l)
	}
	return l
}

// LoggerOption configures the AgentLogger
type LoggerOption func(*AgentLogger)

// WithLogLevel sets the logging level
func WithLogLevel(level LogLevel) LoggerOption {
	return func(l *AgentLogger) {
		l.level = level
	}
}

// WithLogOutput sets the output destination
func WithLogOutput(w io.Writer) LoggerOption {
	return func(l *AgentLogger) {
		l.output = w
	}
}

// WithLogFile sets output to a file (creates if not exists)
func WithLogFile(path string) LoggerOption {
	return func(l *AgentLogger) {
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "agent logger: failed to open log file: %v\n", err)
			return
		}
		l.output = f
	}
}

// WithSessionID sets the session ID for all entries
func WithSessionID(id string) LoggerOption {
	return func(l *AgentLogger) {
		l.sessionID = id
	}
}

// WithModel sets the model name for all entries
func WithModel(model string) LoggerOption {
	return func(l *AgentLogger) {
		l.model = model
	}
}

// SetSession updates the session context
func (l *AgentLogger) SetSession(sessionID, model string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sessionID = sessionID
	l.model = model
}

// log writes a structured log entry
func (l *AgentLogger) log(entry LogEntry) {
	if l.output == nil || l.output == io.Discard {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// Fill in context
	entry.Timestamp = time.Now()
	if entry.SessionID == "" {
		entry.SessionID = l.sessionID
	}
	if entry.Model == "" {
		entry.Model = l.model
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	fmt.Fprintf(l.output, "%s\n", data)
}

// LLMCall logs an LLM API call with usage stats
func (l *AgentLogger) LLMCall(ctx context.Context, model string, durationMs int64, inputTokens, outputTokens, thinkTokens, cacheRead, cacheWrite int, totalCost float64, err error) {
	if l.level < LogLevelInfo {
		return
	}

	entry := LogEntry{
		Level:        "info",
		Type:         "llm_call",
		Model:        model,
		DurationMs:   durationMs,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		ThinkTokens:  thinkTokens,
		CacheRead:    cacheRead,
		CacheWrite:   cacheWrite,
		TotalCost:    totalCost,
	}
	if err != nil {
		entry.Level = "error"
		entry.Error = err.Error()
	}
	l.log(entry)
}

// ToolCall logs a tool execution
func (l *AgentLogger) ToolCall(ctx context.Context, toolName string, args map[string]any, durationMs int64, result string, err error) {
	if l.level < LogLevelInfo {
		return
	}

	// Truncate result for logging
	if len(result) > 500 {
		result = result[:497] + "..."
	}

	entry := LogEntry{
		Level:      "info",
		Type:       "tool_call",
		ToolName:   toolName,
		ToolArgs:   sanitizeArgs(args),
		DurationMs: durationMs,
		ToolResult: result,
	}
	if err != nil {
		entry.Level = "error"
		entry.Error = err.Error()
	}
	l.log(entry)
}

// SessionStart logs session creation
func (l *AgentLogger) SessionStart(ctx context.Context, sessionID, model string) {
	if l.level < LogLevelInfo {
		return
	}
	l.log(LogEntry{
		Level:     "info",
		Type:      "session_start",
		SessionID: sessionID,
		Model:     model,
	})
}

// SessionEnd logs session completion
func (l *AgentLogger) SessionEnd(ctx context.Context, sessionID string, totalTokens int, totalCost float64) {
	if l.level < LogLevelInfo {
		return
	}
	l.log(LogEntry{
		Level:     "info",
		Type:      "session_end",
		SessionID: sessionID,
		Extra: map[string]any{
			"total_tokens": totalTokens,
			"total_cost":   totalCost,
		},
	})
}

// Error logs an error event
func (l *AgentLogger) Error(ctx context.Context, eventType string, err error, extra map[string]any) {
	if l.level < LogLevelError {
		return
	}
	l.log(LogEntry{
		Level: "error",
		Type:  eventType,
		Error: err.Error(),
		Extra: extra,
	})
}

// Warn logs a warning event
func (l *AgentLogger) Warn(ctx context.Context, eventType string, msg string, extra map[string]any) {
	if l.level < LogLevelWarn {
		return
	}
	l.log(LogEntry{
		Level: "warn",
		Type:  eventType,
		Error: msg,
		Extra: extra,
	})
}

// Debug logs a debug event
func (l *AgentLogger) Debug(ctx context.Context, eventType string, extra map[string]any) {
	if l.level < LogLevelDebug {
		return
	}
	l.log(LogEntry{
		Level: "debug",
		Type:  eventType,
		Extra: extra,
	})
}

// sanitizeArgs removes sensitive data from tool arguments
func sanitizeArgs(args map[string]any) map[string]any {
	if args == nil {
		return nil
	}

	safe := make(map[string]any, len(args))
	for k, v := range args {
		// Redact potentially sensitive fields
		switch k {
		case "content", "text", "body":
			if s, ok := v.(string); ok && len(s) > 200 {
				safe[k] = s[:197] + "..."
			} else {
				safe[k] = v
			}
		case "password", "secret", "token", "key", "api_key":
			safe[k] = "[REDACTED]"
		default:
			safe[k] = v
		}
	}
	return safe
}

// Global default logger (disabled by default)
var defaultLogger = NewAgentLogger()

// SetDefaultLogger sets the package-level default logger
func SetDefaultLogger(l *AgentLogger) {
	defaultLogger = l
}

// DefaultLogger returns the package-level logger
func DefaultLogger() *AgentLogger {
	return defaultLogger
}
