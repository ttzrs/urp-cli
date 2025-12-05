package planning

import (
	"testing"

	"github.com/joss/urp/internal/graph"
)

func TestPlanStatus(t *testing.T) {
	statuses := []PlanStatus{PlanPending, PlanInProgress, PlanCompleted, PlanFailed}
	for _, s := range statuses {
		if s == "" {
			t.Error("empty plan status")
		}
	}
}

func TestTaskStatus(t *testing.T) {
	statuses := []TaskStatus{
		TaskPending, TaskAssigned, TaskInProgress,
		TaskCompleted, TaskFailed, TaskBlocked,
	}
	for _, s := range statuses {
		if s == "" {
			t.Error("empty task status")
		}
	}
}

func TestPlanStruct(t *testing.T) {
	plan := Plan{
		PlanID:      "plan-123",
		Description: "Test plan",
		Status:      PlanPending,
		Tasks: []Task{
			{TaskID: "t1", Description: "Task 1", Status: TaskPending},
			{TaskID: "t2", Description: "Task 2", Status: TaskPending},
		},
	}

	if plan.PlanID != "plan-123" {
		t.Errorf("expected plan-123, got %s", plan.PlanID)
	}

	if len(plan.Tasks) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(plan.Tasks))
	}
}

func TestTaskStruct(t *testing.T) {
	task := Task{
		TaskID:      "task-1",
		PlanID:      "plan-1",
		Description: "Do something",
		Status:      TaskPending,
		Order:       1,
		DependsOn:   []string{"task-0"},
	}

	if task.TaskID != "task-1" {
		t.Error("task ID mismatch")
	}

	if len(task.DependsOn) != 1 {
		t.Error("expected 1 dependency")
	}
}

func TestResultStruct(t *testing.T) {
	result := Result{
		ResultID:     "r-1",
		TaskID:       "t-1",
		WorkerID:     "w-1",
		Status:       "success",
		Output:       "done",
		FilesChanged: []string{"main.go", "test.go"},
	}

	if result.Status != "success" {
		t.Error("expected success status")
	}

	if len(result.FilesChanged) != 2 {
		t.Errorf("expected 2 files changed, got %d", len(result.FilesChanged))
	}
}

func TestPRResultStruct(t *testing.T) {
	pr := PRResult{
		URL:        "https://github.com/user/repo/pull/1",
		Number:     1,
		Branch:     "feature-x",
		BaseBranch: "main",
	}

	if pr.URL == "" {
		t.Error("expected PR URL")
	}

	if pr.Number != 1 {
		t.Errorf("expected PR number 1, got %d", pr.Number)
	}
}

func TestGetStringHelper(t *testing.T) {
	record := map[string]any{
		"key1": "value1",
		"key2": 123,
	}

	if graph.GetString(record, "key1") != "value1" {
		t.Error("expected value1")
	}

	if graph.GetString(record, "key2") != "" {
		t.Error("expected empty string for non-string")
	}

	if graph.GetString(record, "missing") != "" {
		t.Error("expected empty string for missing")
	}
}

func TestGetIntHelper(t *testing.T) {
	record := map[string]any{
		"int":     int(42),
		"int64":   int64(64),
		"float64": float64(99.9),
		"string":  "not a number",
	}

	if graph.GetInt(record, "int") != 42 {
		t.Error("expected 42 for int")
	}

	if graph.GetInt(record, "int64") != 64 {
		t.Error("expected 64 for int64")
	}

	if graph.GetInt(record, "float64") != 99 {
		t.Error("expected 99 for float64")
	}

	if graph.GetInt(record, "string") != 0 {
		t.Error("expected 0 for string")
	}
}

func TestGetStringFromHelper(t *testing.T) {
	m := map[string]any{
		"name": "test",
		"num":  42,
	}

	if getStringFrom(m, "name") != "test" {
		t.Error("expected test")
	}

	if getStringFrom(m, "num") != "" {
		t.Error("expected empty for non-string")
	}
}

func TestGetIntFromHelper(t *testing.T) {
	m := map[string]any{
		"int":     int(10),
		"int64":   int64(20),
		"float64": float64(30.5),
	}

	if getIntFrom(m, "int") != 10 {
		t.Error("expected 10")
	}

	if getIntFrom(m, "int64") != 20 {
		t.Error("expected 20")
	}

	if getIntFrom(m, "float64") != 30 {
		t.Error("expected 30")
	}
}
