// Package logging provides structured JSON logging for URP components.
package logging

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Level represents log severity
type Level string

const (
	LevelDebug Level = "debug"
	LevelInfo  Level = "info"
	LevelWarn  Level = "warn"
	LevelError Level = "error"
)

// Event represents a structured log event
type Event struct {
	Timestamp string                 `json:"ts"`
	Level     Level                  `json:"level"`
	Component string                 `json:"component"`
	Event     string                 `json:"event"`
	Worker    string                 `json:"worker,omitempty"`
	Project   string                 `json:"project,omitempty"`
	Duration  int64                  `json:"duration_ms,omitempty"`
	Error     string                 `json:"error,omitempty"`
	Extra     map[string]interface{} `json:"extra,omitempty"`
}

// Logger provides structured logging
type Logger struct {
	component string
	project   string
	worker    string
}

// New creates a new logger for a component
func New(component string) *Logger {
	return &Logger{
		component: component,
		project:   os.Getenv("URP_PROJECT"),
		worker:    os.Getenv("URP_WORKER_ID"),
	}
}

// WithProject sets the project context
func (l *Logger) WithProject(project string) *Logger {
	return &Logger{
		component: l.component,
		project:   project,
		worker:    l.worker,
	}
}

// WithWorker sets the worker context
func (l *Logger) WithWorker(worker string) *Logger {
	return &Logger{
		component: l.component,
		project:   l.project,
		worker:    worker,
	}
}

// log emits a structured log event
func (l *Logger) log(level Level, event string, extra map[string]interface{}, err error) {
	e := Event{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Level:     level,
		Component: l.component,
		Event:     event,
		Project:   l.project,
		Worker:    l.worker,
		Extra:     extra,
	}

	if err != nil {
		e.Error = err.Error()
	}

	data, _ := json.Marshal(e)
	fmt.Fprintln(os.Stderr, string(data))
}

// Debug logs a debug event
func (l *Logger) Debug(event string, extra map[string]interface{}) {
	l.log(LevelDebug, event, extra, nil)
}

// Info logs an info event
func (l *Logger) Info(event string, extra map[string]interface{}) {
	l.log(LevelInfo, event, extra, nil)
}

// Warn logs a warning event
func (l *Logger) Warn(event string, extra map[string]interface{}, err error) {
	l.log(LevelWarn, event, extra, err)
}

// Error logs an error event
func (l *Logger) Error(event string, extra map[string]interface{}, err error) {
	l.log(LevelError, event, extra, err)
}

// TimedEvent logs an event with duration
func (l *Logger) TimedEvent(event string, start time.Time, extra map[string]interface{}) {
	e := Event{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Level:     LevelInfo,
		Component: l.component,
		Event:     event,
		Project:   l.project,
		Worker:    l.worker,
		Duration:  time.Since(start).Milliseconds(),
		Extra:     extra,
	}

	data, _ := json.Marshal(e)
	fmt.Fprintln(os.Stderr, string(data))
}

// SpawnEvent logs a container spawn event
func SpawnEvent(workerName, project string, success bool, duration time.Duration, err error) {
	e := Event{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Level:     LevelInfo,
		Component: "container",
		Event:     "spawn",
		Worker:    workerName,
		Project:   project,
		Duration:  duration.Milliseconds(),
		Extra: map[string]interface{}{
			"success": success,
		},
	}

	if err != nil {
		e.Level = LevelError
		e.Error = err.Error()
	}

	data, _ := json.Marshal(e)
	fmt.Fprintln(os.Stderr, string(data))
}

// NeMoEvent logs a NeMo container event
func NeMoEvent(event, containerName, project string, duration time.Duration, err error) {
	e := Event{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Level:     LevelInfo,
		Component: "nemo",
		Event:     event,
		Project:   project,
		Duration:  duration.Milliseconds(),
		Extra: map[string]interface{}{
			"container": containerName,
		},
	}

	if err != nil {
		e.Level = LevelError
		e.Error = err.Error()
	}

	data, _ := json.Marshal(e)
	fmt.Fprintln(os.Stderr, string(data))
}

// HealthEvent logs a health check event
func HealthEvent(workerName, status string, healthy bool) {
	level := LevelInfo
	if !healthy {
		level = LevelWarn
	}

	e := Event{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Level:     level,
		Component: "health",
		Event:     "check",
		Worker:    workerName,
		Extra: map[string]interface{}{
			"status":  status,
			"healthy": healthy,
		},
	}

	data, _ := json.Marshal(e)
	fmt.Fprintln(os.Stderr, string(data))
}
