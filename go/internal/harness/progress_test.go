package harness

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProgressLogAppend(t *testing.T) {
	tmpDir := t.TempDir()
	InitHarness(tmpDir)

	pl := NewProgressLog(tmpDir)

	// Append entry
	err := pl.Append("session-1", "Started task implementation")
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	// Verify entry was written
	data, err := os.ReadFile(filepath.Join(tmpDir, ".urp", "progress.txt"))
	if err != nil {
		t.Fatalf("Failed to read progress.txt: %v", err)
	}

	if !strings.Contains(string(data), "session-1") {
		t.Errorf("Session ID not found in progress.txt")
	}

	if !strings.Contains(string(data), "Started task implementation") {
		t.Errorf("Entry content not found in progress.txt")
	}
}

func TestProgressLogReadRecent(t *testing.T) {
	tmpDir := t.TempDir()
	InitHarness(tmpDir)

	pl := NewProgressLog(tmpDir)

	// Append multiple entries
	sessionID := "session-1"
	entries := []string{"First entry", "Second entry", "Third entry"}

	for _, entry := range entries {
		if err := pl.Append(sessionID, entry); err != nil {
			t.Fatalf("Append failed: %v", err)
		}
	}

	// Read recent
	recent, err := pl.ReadRecent(2)
	if err != nil {
		t.Fatalf("ReadRecent failed: %v", err)
	}

	if len(recent) != 2 {
		t.Errorf("Expected 2 recent entries, got %d", len(recent))
	}

	// Last 2 should be "Second entry" and "Third entry"
	if !strings.Contains(recent[0], "Second entry") {
		t.Errorf("First recent entry incorrect")
	}
	if !strings.Contains(recent[1], "Third entry") {
		t.Errorf("Second recent entry incorrect")
	}
}

func TestProgressLogExtractFailures(t *testing.T) {
	tmpDir := t.TempDir()
	InitHarness(tmpDir)

	pl := NewProgressLog(tmpDir)

	// Append entries with different patterns
	pl.Append("s1", "Tried approach A, failed with error Y")
	pl.Append("s1", "Implementation succeeded")
	pl.Append("s1", "Code panicked during execution")
	pl.Append("s1", "Timeout waiting for response")

	failures, err := pl.ExtractFailures()
	if err != nil {
		t.Fatalf("ExtractFailures failed: %v", err)
	}

	if len(failures) != 3 {
		t.Errorf("Expected 3 failures, got %d", len(failures))
	}

	// Verify all failures contain failure patterns
	for _, failure := range failures {
		hasPattern := strings.Contains(strings.ToLower(failure), "failed") ||
			strings.Contains(strings.ToLower(failure), "panic") ||
			strings.Contains(strings.ToLower(failure), "timeout")
		if !hasPattern {
			t.Errorf("Failure doesn't contain expected pattern: %s", failure)
		}
	}
}
