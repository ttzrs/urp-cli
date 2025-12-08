// Package agent provides task context management for multi-turn conversations
package agent

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// TaskContext maintains awareness of the current task across turns
type TaskContext struct {
	mu sync.RWMutex

	// Core task info
	Objective     string    // What the user originally asked for
	StartTime     time.Time // When the task started
	TurnCount     int       // How many LLM turns we've done
	ToolCallCount int       // How many tools executed

	// Progress tracking
	FilesRead    []string // Files we've examined
	FilesWritten []string // Files we've modified
	LastAction   string   // Last significant action taken
	Errors       []string // Errors encountered (for learning)

	// State
	Phase       TaskPhase // Current phase
	Confidence  float64   // How confident we are (0-1)
	NeedsReplan bool      // Whether we should reconsider approach
}

// TaskPhase represents the current phase of task execution
type TaskPhase int

const (
	PhaseUnderstanding TaskPhase = iota // Gathering info
	PhasePlanning                       // Deciding approach
	PhaseExecuting                      // Making changes
	PhaseVerifying                      // Checking results
	PhaseComplete                       // Done
)

func (p TaskPhase) String() string {
	switch p {
	case PhaseUnderstanding:
		return "understanding"
	case PhasePlanning:
		return "planning"
	case PhaseExecuting:
		return "executing"
	case PhaseVerifying:
		return "verifying"
	case PhaseComplete:
		return "complete"
	default:
		return "unknown"
	}
}

// NewTaskContext creates a task context from the user's initial prompt
func NewTaskContext(objective string) *TaskContext {
	return &TaskContext{
		Objective: objective,
		StartTime: time.Now(),
		Phase:     PhaseUnderstanding,
	}
}

// RecordTurn increments the turn counter
func (t *TaskContext) RecordTurn() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.TurnCount++
}

// RecordToolCall tracks tool usage
func (t *TaskContext) RecordToolCall(toolName, arg string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.ToolCallCount++
	t.LastAction = fmt.Sprintf("%s: %s", toolName, truncateArg(arg, 50))

	// Track files
	if strings.Contains(toolName, "read") || toolName == "grep" || toolName == "glob" {
		if arg != "" && !contains(t.FilesRead, arg) {
			t.FilesRead = append(t.FilesRead, arg)
		}
	}
	if strings.Contains(toolName, "write") || strings.Contains(toolName, "edit") {
		if arg != "" && !contains(t.FilesWritten, arg) {
			t.FilesWritten = append(t.FilesWritten, arg)
		}
		t.Phase = PhaseExecuting
	}
}

// RecordError tracks errors for learning
func (t *TaskContext) RecordError(err string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Errors = append(t.Errors, truncateArg(err, 100))
	t.NeedsReplan = true
	t.Confidence *= 0.8 // Reduce confidence on error
}

// SetPhase updates the current phase
func (t *TaskContext) SetPhase(phase TaskPhase) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Phase = phase
}

// SetConfidence updates confidence level
func (t *TaskContext) SetConfidence(c float64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Confidence = c
}

// BuildReminder creates a concise reminder for the LLM
func (t *TaskContext) BuildReminder() string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var b strings.Builder
	b.WriteString("<task-context>\n")

	// Objective (always include, truncated)
	objective := t.Objective
	if len(objective) > 150 {
		objective = objective[:147] + "..."
	}
	b.WriteString(fmt.Sprintf("OBJECTIVE: %s\n", objective))

	// Progress summary
	b.WriteString(fmt.Sprintf("PHASE: %s | TURNS: %d | TOOLS: %d\n",
		t.Phase.String(), t.TurnCount, t.ToolCallCount))

	// Files touched (compact)
	if len(t.FilesWritten) > 0 {
		b.WriteString(fmt.Sprintf("MODIFIED: %s\n", strings.Join(lastN(t.FilesWritten, 3), ", ")))
	}

	// Last action for continuity
	if t.LastAction != "" {
		b.WriteString(fmt.Sprintf("LAST: %s\n", t.LastAction))
	}

	// Errors (if any)
	if len(t.Errors) > 0 {
		b.WriteString(fmt.Sprintf("ERRORS: %d (avoid repeating)\n", len(t.Errors)))
	}

	// Guidance based on state - STRONG focus reminders
	if t.NeedsReplan {
		b.WriteString("⚠️ REPLAN: Previous approach failed. Consider alternative.\n")
	}
	if t.TurnCount > 5 && t.Phase != PhaseComplete {
		b.WriteString("⚠️ FOCUS: Stay on objective. Don't add features not requested.\n")
	}
	if t.ToolCallCount > 15 {
		b.WriteString("⚠️ WRAP UP: Many tools used. Complete and respond to user.\n")
	}

	// Core discipline reminder
	b.WriteString("RULES: Only do what was asked. No extra docs/tests unless requested.\n")

	b.WriteString("</task-context>")
	return b.String()
}

// Stats returns task statistics
func (t *TaskContext) Stats() map[string]interface{} {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return map[string]interface{}{
		"objective":    truncateArg(t.Objective, 50),
		"phase":        t.Phase.String(),
		"turns":        t.TurnCount,
		"tools":        t.ToolCallCount,
		"filesRead":    len(t.FilesRead),
		"filesWritten": len(t.FilesWritten),
		"errors":       len(t.Errors),
		"confidence":   t.Confidence,
		"duration":     time.Since(t.StartTime).String(),
	}
}

// Helper functions

func truncateArg(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func lastN(slice []string, n int) []string {
	if len(slice) <= n {
		return slice
	}
	return slice[len(slice)-n:]
}
