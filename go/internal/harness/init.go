// Package harness provides file-based session state management following Anthropic's
// long-running agent patterns. Enables cross-session resume via append-only logs and
// JSON task tracking (ephemeral, deleted with project).
package harness

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Task represents a single task with completion status.
type Task struct {
	ID          string   `json:"id"`
	Description string   `json:"description"`
	Steps       []string `json:"steps"`
	Passes      bool     `json:"passes"` // Only field agent modifies
}

// InitHarness initializes the .urp/ directory structure for a project.
// Creates:
// - .urp/progress.txt: append-only session progress log (ephemeral, deleted with project)
// - .urp/tasks.json: JSON task list with status (ephemeral, deleted with project)
// - .urp/init.sh: environment setup script (persistent, stays with project)
// - .urp/logs/: directory for session logs (persistent, audit trail)
//
// Should be called at agent startup via agent.Run().
func InitHarness(workDir string) error {
	urpDir := filepath.Join(workDir, ".urp")
	if err := os.MkdirAll(urpDir, 0755); err != nil {
		return fmt.Errorf("failed to create .urp directory: %w", err)
	}

	// Create progress.txt if not exists
	// Append-only session progress log (human-readable format)
	progressPath := filepath.Join(urpDir, "progress.txt")
	if !fileExists(progressPath) {
		header := "# URP Progress Log - DO NOT EDIT MANUALLY\n"
		header += fmt.Sprintf("# Created: %s\n", time.Now().Format(time.RFC3339))
		header += "# Format: [timestamp] Session <sessionID>: <entry>\n\n"
		if err := os.WriteFile(progressPath, []byte(header), 0644); err != nil {
			return fmt.Errorf("failed to create progress.txt: %w", err)
		}
	}

	// Create tasks.json if not exists
	// JSON task list with status flags (agent only modifies 'passes' field)
	taskPath := filepath.Join(urpDir, "tasks.json")
	if !fileExists(taskPath) {
		emptyTasks := []Task{}
		data, err := json.MarshalIndent(emptyTasks, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal empty tasks: %w", err)
		}
		if err := os.WriteFile(taskPath, data, 0644); err != nil {
			return fmt.Errorf("failed to create tasks.json: %w", err)
		}
	}

	// Create logs directory (persistent, for debugging and audit trail)
	logsDir := filepath.Join(urpDir, "logs")
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		return fmt.Errorf("failed to create logs directory: %w", err)
	}

	// Create init.sh template if not exists (persistent, stays with project)
	initShPath := filepath.Join(urpDir, "init.sh")
	if !fileExists(initShPath) {
		initShContent := `#!/bin/bash
# URP Project Initialization Script
# This script is persistent and stays with your project

# Set project-specific environment variables here
export URP_PROJECT="${URP_PROJECT:-.}"
export URP_SESSION_ID="${URP_SESSION_ID:-unknown}"

echo "URP Environment initialized for: $URP_PROJECT"
`
		if err := os.WriteFile(initShPath, []byte(initShContent), 0755); err != nil {
			return fmt.Errorf("failed to create init.sh: %w", err)
		}
	}

	return nil
}

// fileExists checks if a file exists at the given path.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
