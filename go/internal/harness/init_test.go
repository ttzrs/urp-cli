package harness

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitHarness(t *testing.T) {
	// Create temporary directory
	tmpDir := t.TempDir()

	// Test InitHarness
	err := InitHarness(tmpDir)
	if err != nil {
		t.Fatalf("InitHarness failed: %v", err)
	}

	// Verify .urp directory exists
	urpDir := filepath.Join(tmpDir, ".urp")
	if _, err := os.Stat(urpDir); os.IsNotExist(err) {
		t.Errorf(".urp directory not created")
	}

	// Verify progress.txt exists
	progressPath := filepath.Join(urpDir, "progress.txt")
	if _, err := os.Stat(progressPath); os.IsNotExist(err) {
		t.Errorf("progress.txt not created")
	}

	// Verify tasks.json exists
	tasksPath := filepath.Join(urpDir, "tasks.json")
	if _, err := os.Stat(tasksPath); os.IsNotExist(err) {
		t.Errorf("tasks.json not created")
	}

	// Verify init.sh exists
	initPath := filepath.Join(urpDir, "init.sh")
	if _, err := os.Stat(initPath); os.IsNotExist(err) {
		t.Errorf("init.sh not created")
	}

	// Verify logs directory exists
	logsDir := filepath.Join(urpDir, "logs")
	if _, err := os.Stat(logsDir); os.IsNotExist(err) {
		t.Errorf("logs directory not created")
	}
}

func TestInitHarness_Idempotent(t *testing.T) {
	// Create temporary directory
	tmpDir := t.TempDir()

	// Initialize once
	err := InitHarness(tmpDir)
	if err != nil {
		t.Fatalf("First InitHarness failed: %v", err)
	}

	// Initialize again (should not fail or overwrite)
	err = InitHarness(tmpDir)
	if err != nil {
		t.Fatalf("Second InitHarness failed: %v", err)
	}

	// Verify files still exist
	urpDir := filepath.Join(tmpDir, ".urp")
	progressPath := filepath.Join(urpDir, "progress.txt")
	tasksPath := filepath.Join(urpDir, "tasks.json")

	if _, err := os.Stat(progressPath); os.IsNotExist(err) {
		t.Errorf("progress.txt lost after second InitHarness")
	}

	if _, err := os.Stat(tasksPath); os.IsNotExist(err) {
		t.Errorf("tasks.json lost after second InitHarness")
	}
}
