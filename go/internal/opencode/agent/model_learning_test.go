package agent

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewModelLearningStore(t *testing.T) {
	store := NewModelLearningStore()
	if store == nil {
		t.Fatal("NewModelLearningStore returned nil")
	}
	if store.Count() != 0 {
		t.Errorf("Count() = %d, want 0", store.Count())
	}
}

func TestModelLearningStore_Record(t *testing.T) {
	store := NewModelLearningStore()

	outcome := &ModelOutcome{
		TaskID:      "task-1",
		TaskType:    "bugfix",
		Environment: "go",
		Complexity:  0.5,
		ModelID:     "test-model",
		Success:     true,
		Score:       0.8,
		Tokens:      1000,
		Cost:        0.05,
		Duration:    2 * time.Second,
	}

	store.Record(outcome)

	if store.Count() != 1 {
		t.Errorf("Count() = %d, want 1", store.Count())
	}

	// Check stats updated
	stats := store.GetStats("test-model")
	if stats == nil {
		t.Fatal("GetStats returned nil")
	}
	if stats.SampleCount != 1 {
		t.Errorf("SampleCount = %d, want 1", stats.SampleCount)
	}
	if stats.SuccessRate != 1.0 {
		t.Errorf("SuccessRate = %f, want 1.0", stats.SuccessRate)
	}
}

func TestModelLearningStore_RunningAverages(t *testing.T) {
	store := NewModelLearningStore()

	// Record 2 successful, 1 failed
	store.Record(&ModelOutcome{ModelID: "test", Success: true, Score: 0.8, Cost: 0.10})
	store.Record(&ModelOutcome{ModelID: "test", Success: true, Score: 0.9, Cost: 0.05})
	store.Record(&ModelOutcome{ModelID: "test", Success: false, Score: 0.3, Cost: 0.15})

	stats := store.GetStats("test")
	if stats == nil {
		t.Fatal("stats is nil")
	}

	// Success rate: 2/3 = 0.666...
	expectedSuccess := 2.0 / 3.0
	if diff := stats.SuccessRate - expectedSuccess; diff < -0.01 || diff > 0.01 {
		t.Errorf("SuccessRate = %f, want ~%f", stats.SuccessRate, expectedSuccess)
	}

	// Avg score: (0.8 + 0.9 + 0.3) / 3 = 0.666...
	expectedScore := (0.8 + 0.9 + 0.3) / 3.0
	if diff := stats.AvgScore - expectedScore; diff < -0.01 || diff > 0.01 {
		t.Errorf("AvgScore = %f, want ~%f", stats.AvgScore, expectedScore)
	}
}

func TestModelLearningStore_TaskStats(t *testing.T) {
	store := NewModelLearningStore()

	// Record outcomes for different task types
	store.Record(&ModelOutcome{ModelID: "model-a", TaskType: "bugfix", Environment: "go", Success: true, Score: 0.9})
	store.Record(&ModelOutcome{ModelID: "model-a", TaskType: "explore", Environment: "go", Success: true, Score: 0.7})
	store.Record(&ModelOutcome{ModelID: "model-b", TaskType: "bugfix", Environment: "go", Success: false, Score: 0.4})

	// Check task-specific stats
	taskStats := store.GetTaskStats("model-a", "bugfix", "go")
	if taskStats == nil {
		t.Fatal("GetTaskStats returned nil")
	}
	if taskStats.SampleCount != 1 {
		t.Errorf("TaskStats.SampleCount = %d, want 1", taskStats.SampleCount)
	}
	if taskStats.SuccessRate != 1.0 {
		t.Errorf("TaskStats.SuccessRate = %f, want 1.0", taskStats.SuccessRate)
	}
}

func TestModelLearningStore_GetBestModel(t *testing.T) {
	store := NewModelLearningStore()
	store.SetMinSamples(2) // Lower threshold for testing

	// Model A: good at bugfix
	for i := 0; i < 3; i++ {
		store.Record(&ModelOutcome{
			ModelID:     "model-a",
			TaskType:    "bugfix",
			Environment: "go",
			Success:     true,
			Score:       0.9,
			Cost:        0.10,
		})
	}

	// Model B: good at explore, bad at bugfix
	for i := 0; i < 3; i++ {
		store.Record(&ModelOutcome{
			ModelID:     "model-b",
			TaskType:    "explore",
			Environment: "go",
			Success:     true,
			Score:       0.95,
			Cost:        0.05,
		})
	}
	store.Record(&ModelOutcome{
		ModelID:     "model-b",
		TaskType:    "bugfix",
		Environment: "go",
		Success:     false,
		Score:       0.3,
		Cost:        0.10,
	})

	// For bugfix, model-a should be recommended
	best := store.GetBestModel("bugfix", "go", 0.5)
	if best != "model-a" {
		t.Errorf("GetBestModel(bugfix) = %s, want model-a", best)
	}

	// For explore, model-b should be recommended
	best = store.GetBestModel("explore", "go", 0.3)
	if best != "model-b" {
		t.Errorf("GetBestModel(explore) = %s, want model-b", best)
	}
}

func TestModelLearningStore_GetBestModel_MinSamples(t *testing.T) {
	store := NewModelLearningStore()
	// Default minSamples is 5

	// Only 2 samples - shouldn't recommend
	store.Record(&ModelOutcome{ModelID: "model-a", TaskType: "bugfix", Success: true, Score: 0.9})
	store.Record(&ModelOutcome{ModelID: "model-a", TaskType: "bugfix", Success: true, Score: 0.9})

	best := store.GetBestModel("bugfix", "go", 0.5)
	if best != "" {
		t.Errorf("GetBestModel with < minSamples should return empty, got %s", best)
	}
}

func TestModelLearningStore_Clear(t *testing.T) {
	store := NewModelLearningStore()

	store.Record(&ModelOutcome{ModelID: "test", Success: true})
	if store.Count() != 1 {
		t.Fatal("expected 1 outcome")
	}

	store.Clear()
	if store.Count() != 0 {
		t.Errorf("Count after Clear() = %d, want 0", store.Count())
	}

	stats := store.GetAllStats()
	if len(stats) != 0 {
		t.Errorf("GetAllStats after Clear() has %d entries", len(stats))
	}
}

func TestModelLearningStore_Persistence(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "learning.json")

	// Create store and record data
	store1 := NewModelLearningStoreWithPath(path)
	store1.Record(&ModelOutcome{
		ModelID:     "test-model",
		TaskType:    "bugfix",
		Environment: "go",
		Success:     true,
		Score:       0.85,
		Cost:        0.10,
	})

	// Verify file was created
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("learning file was not created")
	}

	// Create new store and load
	store2 := NewModelLearningStoreWithPath(path)
	if store2.Count() != 1 {
		t.Errorf("loaded Count() = %d, want 1", store2.Count())
	}

	stats := store2.GetStats("test-model")
	if stats == nil {
		t.Fatal("loaded stats is nil")
	}
	if stats.SuccessRate != 1.0 {
		t.Errorf("loaded SuccessRate = %f, want 1.0", stats.SuccessRate)
	}
}

func TestModelLearningStore_GetAllStats(t *testing.T) {
	store := NewModelLearningStore()

	store.Record(&ModelOutcome{ModelID: "model-a", Success: true})
	store.Record(&ModelOutcome{ModelID: "model-b", Success: true})
	store.Record(&ModelOutcome{ModelID: "model-a", Success: false})

	stats := store.GetAllStats()
	if len(stats) != 2 {
		t.Errorf("GetAllStats() has %d entries, want 2", len(stats))
	}

	if stats["model-a"] == nil {
		t.Error("missing stats for model-a")
	}
	if stats["model-a"].SampleCount != 2 {
		t.Errorf("model-a SampleCount = %d, want 2", stats["model-a"].SampleCount)
	}
}

func TestModelLearningStore_GetAllTaskStats(t *testing.T) {
	store := NewModelLearningStore()

	store.Record(&ModelOutcome{ModelID: "model-a", TaskType: "bugfix", Environment: "go", Success: true})
	store.Record(&ModelOutcome{ModelID: "model-a", TaskType: "explore", Environment: "go", Success: true})

	taskStats := store.GetAllTaskStats()
	if len(taskStats) != 2 {
		t.Errorf("GetAllTaskStats() has %d entries, want 2", len(taskStats))
	}
}

func TestModelLearningStore_MaxOutcomes(t *testing.T) {
	store := NewModelLearningStore()
	store.maxOutcomes = 5 // Set low for testing

	// Record more than max
	for i := 0; i < 10; i++ {
		store.Record(&ModelOutcome{ModelID: "test", Success: true})
	}

	if store.Count() != 5 {
		t.Errorf("Count() = %d, want 5 (maxOutcomes)", store.Count())
	}
}

func TestModelOutcome_Fields(t *testing.T) {
	outcome := &ModelOutcome{
		TaskID:      "task-123",
		TaskType:    "feature",
		Environment: "typescript",
		Complexity:  0.7,
		ModelID:     "gpt-4",
		Success:     true,
		Score:       0.92,
		Tokens:      5000,
		Cost:        0.25,
		Duration:    30 * time.Second,
		Timestamp:   time.Now(),
	}

	if outcome.TaskID != "task-123" {
		t.Error("TaskID mismatch")
	}
	if outcome.Cost != 0.25 {
		t.Error("Cost mismatch")
	}
}

func TestModelStats_Fields(t *testing.T) {
	stats := &ModelStats{
		ModelID:     "test",
		SuccessRate: 0.85,
		AvgScore:    0.9,
		AvgCost:     0.10,
		AvgDuration: 2500,
		SampleCount: 100,
	}

	if stats.SuccessRate != 0.85 {
		t.Error("SuccessRate mismatch")
	}
}
