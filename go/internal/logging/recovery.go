// Package logging provides panic recovery with stack trace logging.
package logging

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime/debug"
	"time"

	"github.com/joss/urp/internal/alerts"
)

// RecoveryHandler handles panics with logging and optional alert
type RecoveryHandler struct {
	Component string
	OnPanic   func(err interface{}, stack string)
}

// NewRecoveryHandler creates a recovery handler for a component
func NewRecoveryHandler(component string) *RecoveryHandler {
	return &RecoveryHandler{
		Component: component,
	}
}

// Wrap executes fn with panic recovery
func (r *RecoveryHandler) Wrap(fn func()) {
	defer r.recover()
	fn()
}

// WrapError executes fn with panic recovery, returning error on panic
func (r *RecoveryHandler) WrapError(fn func() error) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			stack := string(debug.Stack())
			err = r.handlePanic(rec, stack)
		}
	}()
	return fn()
}

// recover handles a panic, logs it, and optionally sends alert
func (r *RecoveryHandler) recover() {
	if rec := recover(); rec != nil {
		stack := string(debug.Stack())
		r.handlePanic(rec, stack)
	}
}

// handlePanic processes the panic - logs, alerts, and calls custom handler
func (r *RecoveryHandler) handlePanic(rec interface{}, stack string) error {
	// Build error message
	errMsg := fmt.Sprintf("panic in %s: %v", r.Component, rec)
	ts := time.Now().UTC().Format(time.RFC3339)

	// Log to stderr immediately
	fmt.Fprintf(os.Stderr, "\n=== PANIC RECOVERED ===\n")
	fmt.Fprintf(os.Stderr, "Component: %s\n", r.Component)
	fmt.Fprintf(os.Stderr, "Error: %v\n", rec)
	fmt.Fprintf(os.Stderr, "Time: %s\n", ts)
	fmt.Fprintf(os.Stderr, "\nStack Trace:\n%s\n", stack)
	fmt.Fprintf(os.Stderr, "========================\n\n")

	// Log structured JSON event to stderr
	event := Event{
		Timestamp: ts,
		Level:     LevelError,
		Component: r.Component,
		Event:     "panic_recovered",
		Error:     fmt.Sprintf("%v", rec),
		Extra: map[string]interface{}{
			"stack":     stack,
			"recovered": true,
		},
	}
	eventJSON, _ := json.Marshal(event)
	fmt.Fprintf(os.Stderr, "%s\n", eventJSON)

	// Send critical alert
	alerts.Critical(r.Component, "Panic Recovered", errMsg, map[string]interface{}{
		"stack":     stack,
		"timestamp": ts,
	})

	// Call custom handler if set
	if r.OnPanic != nil {
		r.OnPanic(rec, stack)
	}

	return fmt.Errorf("%s", errMsg)
}

// SafeGo launches a goroutine with panic recovery
func SafeGo(component string, fn func()) {
	go func() {
		handler := NewRecoveryHandler(component)
		handler.Wrap(fn)
	}()
}

// SafeGoWithCallback launches a goroutine with panic recovery and callback
func SafeGoWithCallback(component string, fn func(), onPanic func(err interface{}, stack string)) {
	go func() {
		handler := NewRecoveryHandler(component)
		handler.OnPanic = onPanic
		handler.Wrap(fn)
	}()
}

// Must panics if err is not nil (for initialization)
func Must[T any](val T, err error) T {
	if err != nil {
		panic(err)
	}
	return val
}

// Recover is a simple defer-able recovery that logs panics
func Recover(component string) {
	if rec := recover(); rec != nil {
		stack := string(debug.Stack())
		handler := NewRecoveryHandler(component)
		handler.handlePanic(rec, stack)
	}
}
