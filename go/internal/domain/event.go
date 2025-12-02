// Package domain defines terminal events and conflicts.
package domain

import "time"

// EventType classifies terminal events.
type EventType string

const (
	EventTerminal  EventType = "TerminalEvent"
	EventVCS       EventType = "VCSEvent"
	EventContainer EventType = "ContainerEvent"
	EventBuild     EventType = "BuildEvent"
	EventTest      EventType = "TestEvent"
)

// Event represents a terminal command execution (τ + Φ primitives).
type Event struct {
	ID            string    `json:"id"`
	Command       string    `json:"command"`
	CmdBase       string    `json:"cmd_base"`
	ExitCode      int       `json:"exit_code"`
	DurationSec   float64   `json:"duration_sec"`
	Cwd           string    `json:"cwd"`
	StdoutPreview string    `json:"stdout_preview,omitempty"`
	StderrPreview string    `json:"stderr_preview,omitempty"`
	Timestamp     time.Time `json:"timestamp"`
	Project       string    `json:"project"`
	Type          EventType `json:"type"`
	IsConflict    bool      `json:"is_conflict"` // ⊥ primitive
	Embedding     []float32 `json:"embedding,omitempty"`
}

// Conflict represents an error/failure (⊥ primitive).
type Conflict struct {
	Event
	Resolution string `json:"resolution,omitempty"`
	ResolvedAt string `json:"resolved_at,omitempty"`
}

// Solution represents a validated solution path.
type Solution struct {
	ID           string   `json:"id"`
	Description  string   `json:"description"`
	CreatedAt    string   `json:"created_at"`
	CommandCount int      `json:"command_count"`
	Commands     []string `json:"commands,omitempty"`
	Resolves     []string `json:"resolves,omitempty"` // Conflict IDs
}

// Session represents a terminal session context (T tensor).
type Session struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	StartedAt time.Time `json:"started_at"`
	Cwd       string    `json:"cwd"`
	Active    bool      `json:"active"`
	Project   string    `json:"project"`
}

// ClassifyCommand determines the event type from command base.
func ClassifyCommand(cmdBase string) EventType {
	switch cmdBase {
	case "git", "svn", "hg":
		return EventVCS
	case "docker", "podman", "kubectl", "k3s":
		return EventContainer
	case "npm", "pip", "cargo", "go", "make", "mvn", "gradle":
		return EventBuild
	case "pytest", "jest", "mocha", "vitest":
		return EventTest
	default:
		return EventTerminal
	}
}
