// Package audit provides structured logging and auditing for URP operations.
package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/joss/urp/internal/config"
)

// Logger provides audit logging capabilities.
type Logger struct {
	mu        sync.Mutex
	sessionID string
	project   string
	output    *os.File
	gitCtx    GitContext
	store     *Store // Persists events to graph
}

// LoggerOption configures the logger.
type LoggerOption func(*Logger)

// WithStore sets the graph store for persistence.
func WithStore(store *Store) LoggerOption {
	return func(l *Logger) {
		l.store = store
	}
}

// WithSession sets the session ID.
func WithSession(id string) LoggerOption {
	return func(l *Logger) {
		l.sessionID = id
	}
}

// WithProject sets the project name.
func WithProject(name string) LoggerOption {
	return func(l *Logger) {
		l.project = name
	}
}

// WithOutput sets the output file.
func WithOutput(f *os.File) LoggerOption {
	return func(l *Logger) {
		l.output = f
	}
}

// NewLogger creates a new audit logger.
func NewLogger(opts ...LoggerOption) *Logger {
	l := &Logger{
		sessionID: config.Env().SessionID,
		project:   config.Env().Project,
		output:    os.Stderr,
		gitCtx:    GetGitContext(),
	}

	for _, opt := range opts {
		opt(l)
	}

	if l.sessionID == "" {
		l.sessionID = fmt.Sprintf("sess-%d", time.Now().UnixNano())
	}

	return l
}

// Start begins tracking an operation.
func (l *Logger) Start(category Category, operation string) *AuditEvent {
	return &AuditEvent{
		EventID:   uuid.New().String(),
		Category:  category,
		Operation: operation,
		StartedAt: time.Now(),
		SessionID: l.sessionID,
		Project:   l.project,
		Git:       l.gitCtx,
	}
}

// StartWithCommand begins tracking a command operation.
func (l *Logger) StartWithCommand(category Category, operation, command string) *AuditEvent {
	event := l.Start(category, operation)
	event.Command = command
	return event
}

// Log writes a completed event to the output.
func (l *Logger) Log(event *AuditEvent) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Ensure timing is set
	if event.CompletedAt.IsZero() {
		event.CompletedAt = time.Now()
		event.Duration = event.CompletedAt.Sub(event.StartedAt)
		event.DurationMs = event.Duration.Milliseconds()
	}

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	_, err = fmt.Fprintf(l.output, "%s\n", data)

	// Persist to graph (blocking with timeout to ensure CLI persistence)
	if l.store != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		// We ignore error to not break the CLI flow, but we wait for it
		_ = l.store.Save(ctx, event)
	}

	return err
}

// LogSuccess logs a successful operation.
func (l *Logger) LogSuccess(event *AuditEvent) error {
	event.Complete(StatusSuccess, nil)
	return l.Log(event)
}

// LogError logs a failed operation.
func (l *Logger) LogError(event *AuditEvent, err error) error {
	event.Complete(StatusError, err)
	return l.Log(event)
}

// LogWarning logs a warning.
func (l *Logger) LogWarning(event *AuditEvent, msg string) error {
	event.Complete(StatusWarning, nil)
	event.ErrorMessage = msg
	return l.Log(event)
}

// Quick logs for simple operations

// LogOp logs a complete operation in one call.
func (l *Logger) LogOp(category Category, operation string, status Status, err error) {
	event := l.Start(category, operation)
	event.Complete(status, err)
	_ = l.Log(event)
}

// LogCommand logs a command execution.
func (l *Logger) LogCommand(command string, exitCode int, outputSize int, err error) {
	event := l.StartWithCommand(CategorySystem, "command", command)
	event.ExitCode = exitCode
	event.OutputSize = outputSize
	if err != nil {
		event.Complete(StatusError, err)
	} else if exitCode != 0 {
		event.Status = StatusError
		event.ErrorMessage = fmt.Sprintf("exit code %d", exitCode)
		event.CompletedAt = time.Now()
		event.Duration = event.CompletedAt.Sub(event.StartedAt)
		event.DurationMs = event.Duration.Milliseconds()
	} else {
		event.Complete(StatusSuccess, nil)
	}
	_ = l.Log(event)
}

// RefreshGitContext updates the git context (call after commits).
func (l *Logger) RefreshGitContext() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.gitCtx = GetGitContext()
}

// GetSessionID returns the current session ID.
func (l *Logger) GetSessionID() string {
	return l.sessionID
}

// Global logger instance
var (
	globalLogger *Logger
	globalOnce   sync.Once
)

// Global returns the global logger instance.
func Global() *Logger {
	globalOnce.Do(func() {
		globalLogger = NewLogger()
	})
	return globalLogger
}

// SetGlobal sets the global logger.
func SetGlobal(l *Logger) {
	globalLogger = l
}

// Convenience functions using global logger

// Op logs a quick operation.
func Op(category Category, operation string, status Status, err error) {
	Global().LogOp(category, operation, status, err)
}

// Command logs a command execution.
func Command(cmd string, exitCode int, outputSize int, err error) {
	Global().LogCommand(cmd, exitCode, outputSize, err)
}
