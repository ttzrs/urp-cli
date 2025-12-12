package harness

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// TaskList manages JSON-based task tracking.
// Tasks have a 'passes' status flag (only field agent modifies).
// Prevents agents from editing test descriptions or steps - ensures test integrity.
// Format: [{"id": "auth", "description": "Implement user authentication", "steps": [...], "passes": false}, ...]
type TaskList struct {
	workDir  string
	taskPath string
	mu       sync.RWMutex // Protect concurrent reads/writes
}

// NewTaskList creates a new TaskList manager for a workspace.
func NewTaskList(workDir string) *TaskList {
	return &TaskList{
		workDir:  workDir,
		taskPath: filepath.Join(workDir, ".urp", "tasks.json"),
	}
}

// Load reads all tasks from tasks.json.
// Returns empty slice if file doesn't exist (first run).
func (t *TaskList) Load() ([]Task, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	data, err := os.ReadFile(t.taskPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []Task{}, nil // File doesn't exist yet (first run)
		}
		return nil, fmt.Errorf("failed to read tasks.json: %w", err)
	}

	var tasks []Task
	if err := json.Unmarshal(data, &tasks); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tasks.json: %w", err)
	}

	return tasks, nil
}

// Save writes tasks to tasks.json with pretty formatting.
// Creates file if it doesn't exist.
func (t *TaskList) Save(tasks []Task) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	data, err := json.MarshalIndent(tasks, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal tasks: %w", err)
	}

	if err := os.WriteFile(t.taskPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write tasks.json: %w", err)
	}

	return nil
}

// MarkComplete updates a task's 'passes' flag to true.
// ONLY field that agent can modify - test integrity is preserved.
func (t *TaskList) MarkComplete(taskID string) error {
	tasks, err := t.Load()
	if err != nil {
		return err
	}

	found := false
	for i, task := range tasks {
		if task.ID == taskID {
			tasks[i].Passes = true
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("task not found: %s", taskID)
	}

	return t.Save(tasks)
}

// MarkFailed updates a task's 'passes' flag to false.
func (t *TaskList) MarkFailed(taskID string) error {
	tasks, err := t.Load()
	if err != nil {
		return err
	}

	found := false
	for i, task := range tasks {
		if task.ID == taskID {
			tasks[i].Passes = false
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("task not found: %s", taskID)
	}

	return t.Save(tasks)
}

// GetTask retrieves a single task by ID.
func (t *TaskList) GetTask(taskID string) (*Task, error) {
	tasks, err := t.Load()
	if err != nil {
		return nil, err
	}

	for _, task := range tasks {
		if task.ID == taskID {
			return &task, nil
		}
	}

	return nil, fmt.Errorf("task not found: %s", taskID)
}

// GetPassed returns all tasks with passes=true.
func (t *TaskList) GetPassed() ([]Task, error) {
	tasks, err := t.Load()
	if err != nil {
		return nil, err
	}

	var passed []Task
	for _, task := range tasks {
		if task.Passes {
			passed = append(passed, task)
		}
	}

	return passed, nil
}

// GetFailed returns all tasks with passes=false.
func (t *TaskList) GetFailed() ([]Task, error) {
	tasks, err := t.Load()
	if err != nil {
		return nil, err
	}

	var failed []Task
	for _, task := range tasks {
		if !task.Passes {
			failed = append(failed, task)
		}
	}

	return failed, nil
}

// Progress returns (passed_count, total_count) for status reporting.
func (t *TaskList) Progress() (int, int, error) {
	tasks, err := t.Load()
	if err != nil {
		return 0, 0, err
	}

	passed := 0
	for _, task := range tasks {
		if task.Passes {
			passed++
		}
	}

	return passed, len(tasks), nil
}

// SetTasks replaces entire task list (destructive).
// Used when agent completes setup and defines new task set.
func (t *TaskList) SetTasks(tasks []Task) error {
	return t.Save(tasks)
}

// AddTask appends a new task to the list.
func (t *TaskList) AddTask(task Task) error {
	tasks, err := t.Load()
	if err != nil {
		return err
	}

	// Check for duplicate ID
	for _, existing := range tasks {
		if existing.ID == task.ID {
			return fmt.Errorf("task ID already exists: %s", task.ID)
		}
	}

	tasks = append(tasks, task)
	return t.Save(tasks)
}

// Summary returns a human-readable summary of task status.
// Example: "3/5 tasks passing (auth, database, api - remaining: validation, deployment)"
func (t *TaskList) Summary() (string, error) {
	tasks, err := t.Load()
	if err != nil {
		return "", err
	}

	passed := 0
	failedIDs := []string{}
	passedIDs := []string{}

	for _, task := range tasks {
		if task.Passes {
			passed++
			passedIDs = append(passedIDs, task.ID)
		} else {
			failedIDs = append(failedIDs, task.ID)
		}
	}

	if len(tasks) == 0 {
		return "No tasks defined", nil
	}

	summary := fmt.Sprintf("%d/%d tasks passing", passed, len(tasks))

	if len(passedIDs) > 0 {
		summary += fmt.Sprintf(" (completed: %v)", passedIDs)
	}

	if len(failedIDs) > 0 {
		summary += fmt.Sprintf(" (remaining: %v)", failedIDs)
	}

	return summary, nil
}
