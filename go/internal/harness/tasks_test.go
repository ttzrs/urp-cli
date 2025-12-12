package harness

import (
	"testing"
)

func TestTaskListLoad(t *testing.T) {
	tmpDir := t.TempDir()
	InitHarness(tmpDir)

	tl := NewTaskList(tmpDir)

	// Load empty tasks
	tasks, err := tl.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(tasks) != 0 {
		t.Errorf("Expected 0 tasks initially, got %d", len(tasks))
	}
}

func TestTaskListSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	InitHarness(tmpDir)

	tl := NewTaskList(tmpDir)

	// Create tasks
	tasks := []Task{
		{ID: "auth", Description: "Implement authentication", Steps: []string{"step1", "step2"}, Passes: false},
		{ID: "db", Description: "Setup database", Steps: []string{"step1"}, Passes: false},
	}

	// Save
	err := tl.Save(tasks)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Load
	loaded, err := tl.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(loaded) != 2 {
		t.Errorf("Expected 2 tasks, got %d", len(loaded))
	}

	if loaded[0].ID != "auth" || loaded[1].ID != "db" {
		t.Errorf("Task IDs don't match")
	}
}

func TestTaskListMarkComplete(t *testing.T) {
	tmpDir := t.TempDir()
	InitHarness(tmpDir)

	tl := NewTaskList(tmpDir)

	// Create tasks
	tasks := []Task{
		{ID: "auth", Description: "Implement authentication", Steps: []string{}, Passes: false},
		{ID: "db", Description: "Setup database", Steps: []string{}, Passes: false},
	}

	tl.Save(tasks)

	// Mark first task complete
	err := tl.MarkComplete("auth")
	if err != nil {
		t.Fatalf("MarkComplete failed: %v", err)
	}

	// Verify
	loaded, _ := tl.Load()
	if !loaded[0].Passes {
		t.Errorf("First task should be marked complete")
	}
	if loaded[1].Passes {
		t.Errorf("Second task should not be marked complete")
	}
}

func TestTaskListProgress(t *testing.T) {
	tmpDir := t.TempDir()
	InitHarness(tmpDir)

	tl := NewTaskList(tmpDir)

	// Create mixed tasks
	tasks := []Task{
		{ID: "auth", Description: "Auth", Steps: []string{}, Passes: true},
		{ID: "db", Description: "Database", Steps: []string{}, Passes: true},
		{ID: "api", Description: "API", Steps: []string{}, Passes: false},
		{ID: "ui", Description: "UI", Steps: []string{}, Passes: false},
	}

	tl.Save(tasks)

	// Check progress
	passed, total, err := tl.Progress()
	if err != nil {
		t.Fatalf("Progress failed: %v", err)
	}

	if passed != 2 || total != 4 {
		t.Errorf("Expected 2/4, got %d/%d", passed, total)
	}
}

func TestTaskListGetFailed(t *testing.T) {
	tmpDir := t.TempDir()
	InitHarness(tmpDir)

	tl := NewTaskList(tmpDir)

	tasks := []Task{
		{ID: "auth", Description: "Auth", Steps: []string{}, Passes: true},
		{ID: "db", Description: "Database", Steps: []string{}, Passes: false},
		{ID: "api", Description: "API", Steps: []string{}, Passes: false},
	}

	tl.Save(tasks)

	// Get failed
	failed, err := tl.GetFailed()
	if err != nil {
		t.Fatalf("GetFailed failed: %v", err)
	}

	if len(failed) != 2 {
		t.Errorf("Expected 2 failed tasks, got %d", len(failed))
	}

	if failed[0].ID != "db" || failed[1].ID != "api" {
		t.Errorf("Failed task IDs don't match")
	}
}

func TestTaskListSummary(t *testing.T) {
	tmpDir := t.TempDir()
	InitHarness(tmpDir)

	tl := NewTaskList(tmpDir)

	tasks := []Task{
		{ID: "auth", Description: "Auth", Steps: []string{}, Passes: true},
		{ID: "db", Description: "Database", Steps: []string{}, Passes: false},
		{ID: "api", Description: "API", Steps: []string{}, Passes: false},
	}

	tl.Save(tasks)

	summary, err := tl.Summary()
	if err != nil {
		t.Fatalf("Summary failed: %v", err)
	}

	if summary == "" {
		t.Errorf("Summary should not be empty")
	}

	// Verify summary contains expected info
	if !contains(summary, "1/3") {
		t.Errorf("Summary should contain '1/3': %s", summary)
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && s[0:len(substr)] == substr || len(s) > len(substr) && contains(s[1:], substr)
}
