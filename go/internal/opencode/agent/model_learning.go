// Package agent provides model learning for intelligent routing
package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ModelOutcome records the result of using a model for a task
type ModelOutcome struct {
	TaskID      string        `json:"task_id"`
	TaskType    string        `json:"task_type"`
	Environment string        `json:"environment"`
	Complexity  float64       `json:"complexity"`
	ModelID     string        `json:"model_id"`
	Success     bool          `json:"success"`
	Score       float64       `json:"score"`  // Quality score 0-1
	Tokens      int           `json:"tokens"` // Actual tokens used
	Cost        float64       `json:"cost"`   // Actual cost
	Duration    time.Duration `json:"duration"`
	Timestamp   time.Time     `json:"timestamp"`
}

// ModelStats holds aggregated statistics for a model
type ModelStats struct {
	ModelID     string  `json:"model_id"`
	SuccessRate float64 `json:"success_rate"`
	AvgScore    float64 `json:"avg_score"`
	AvgCost     float64 `json:"avg_cost"`
	AvgDuration float64 `json:"avg_duration_ms"`
	SampleCount int     `json:"sample_count"`
}

// TaskModelStats holds stats for a specific model + task type combination
type TaskModelStats struct {
	ModelID     string  `json:"model_id"`
	TaskType    string  `json:"task_type"`
	Environment string  `json:"environment"`
	SuccessRate float64 `json:"success_rate"`
	AvgScore    float64 `json:"avg_score"`
	AvgCost     float64 `json:"avg_cost"`
	SampleCount int     `json:"sample_count"`
}

// ModelLearningStore learns from historical outcomes to improve model selection
type ModelLearningStore struct {
	mu           sync.RWMutex
	outcomes     []ModelOutcome
	stats        map[string]*ModelStats     // model_id -> stats
	taskStats    map[string]*TaskModelStats // "model_id:task_type:env" -> stats
	persistPath  string                     // Path to save learning data
	maxOutcomes  int                        // Max outcomes to keep in memory
	minSamples   int                        // Min samples before recommending
}

// NewModelLearningStore creates a new learning store
func NewModelLearningStore() *ModelLearningStore {
	return &ModelLearningStore{
		outcomes:    make([]ModelOutcome, 0),
		stats:       make(map[string]*ModelStats),
		taskStats:   make(map[string]*TaskModelStats),
		maxOutcomes: 1000, // Keep last 1000 outcomes
		minSamples:  5,    // Need 5 samples before recommending
	}
}

// NewModelLearningStoreWithPath creates a learning store with persistence
func NewModelLearningStoreWithPath(path string) *ModelLearningStore {
	s := NewModelLearningStore()
	s.persistPath = path
	s.Load() // Load existing data
	return s
}

// Record stores an outcome for learning
func (s *ModelLearningStore) Record(outcome *ModelOutcome) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if outcome.Timestamp.IsZero() {
		outcome.Timestamp = time.Now()
	}
	s.outcomes = append(s.outcomes, *outcome)
	s.updateStats(outcome)
	s.updateTaskStats(outcome)

	// Trim if too many outcomes
	if len(s.outcomes) > s.maxOutcomes {
		s.outcomes = s.outcomes[len(s.outcomes)-s.maxOutcomes:]
	}

	// Auto-save if path configured
	if s.persistPath != "" {
		s.saveUnlocked()
	}
}

// updateStats updates running statistics for a model
func (s *ModelLearningStore) updateStats(outcome *ModelOutcome) {
	stats, exists := s.stats[outcome.ModelID]
	if !exists {
		stats = &ModelStats{ModelID: outcome.ModelID}
		s.stats[outcome.ModelID] = stats
	}

	// Running average update
	n := float64(stats.SampleCount)
	if outcome.Success {
		stats.SuccessRate = (stats.SuccessRate*n + 1.0) / (n + 1)
	} else {
		stats.SuccessRate = (stats.SuccessRate * n) / (n + 1)
	}
	stats.AvgScore = (stats.AvgScore*n + outcome.Score) / (n + 1)
	stats.AvgCost = (stats.AvgCost*n + outcome.Cost) / (n + 1)
	stats.AvgDuration = (stats.AvgDuration*n + float64(outcome.Duration.Milliseconds())) / (n + 1)
	stats.SampleCount++
}

// updateTaskStats updates task-specific statistics
func (s *ModelLearningStore) updateTaskStats(outcome *ModelOutcome) {
	key := outcome.ModelID + ":" + outcome.TaskType + ":" + outcome.Environment
	stats, exists := s.taskStats[key]
	if !exists {
		stats = &TaskModelStats{
			ModelID:     outcome.ModelID,
			TaskType:    outcome.TaskType,
			Environment: outcome.Environment,
		}
		s.taskStats[key] = stats
	}

	n := float64(stats.SampleCount)
	if outcome.Success {
		stats.SuccessRate = (stats.SuccessRate*n + 1.0) / (n + 1)
	} else {
		stats.SuccessRate = (stats.SuccessRate * n) / (n + 1)
	}
	stats.AvgScore = (stats.AvgScore*n + outcome.Score) / (n + 1)
	stats.AvgCost = (stats.AvgCost*n + outcome.Cost) / (n + 1)
	stats.SampleCount++
}

// GetBestModel returns the historically best model for given criteria
func (s *ModelLearningStore) GetBestModel(taskType string, env string, complexity float64) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// First try task-specific stats (most specific)
	bestModel, found := s.findBestForTask(taskType, env)
	if found {
		return bestModel
	}

	// Fall back to global model stats
	return s.findBestOverall()
}

// findBestForTask finds best model for specific task type + environment
func (s *ModelLearningStore) findBestForTask(taskType, env string) (string, bool) {
	var bestModel string
	var bestScore float64

	for _, stats := range s.taskStats {
		// Match task type (environment is optional match)
		if stats.TaskType != taskType {
			continue
		}
		if stats.SampleCount < s.minSamples {
			continue
		}

		// Score: success rate + quality - cost penalty
		// Higher is better
		score := 0.5*stats.SuccessRate + 0.4*stats.AvgScore - 0.1*(stats.AvgCost*10)

		// Bonus for matching environment
		if stats.Environment == env {
			score += 0.1
		}

		if score > bestScore {
			bestScore = score
			bestModel = stats.ModelID
		}
	}

	return bestModel, bestModel != ""
}

// findBestOverall finds best model from global stats
func (s *ModelLearningStore) findBestOverall() string {
	var bestModel string
	var bestScore float64

	for modelID, stats := range s.stats {
		if stats.SampleCount < s.minSamples {
			continue
		}

		// Weight: success, quality, cost (inverted)
		score := 0.5*stats.SuccessRate + 0.4*stats.AvgScore - 0.1*(stats.AvgCost*10)
		if score > bestScore {
			bestScore = score
			bestModel = modelID
		}
	}

	return bestModel
}

// GetStats returns stats for a specific model
func (s *ModelLearningStore) GetStats(modelID string) *ModelStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if stats, ok := s.stats[modelID]; ok {
		copy := *stats
		return &copy
	}
	return nil
}

// GetTaskStats returns stats for a specific model + task type
func (s *ModelLearningStore) GetTaskStats(modelID, taskType, env string) *TaskModelStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	key := modelID + ":" + taskType + ":" + env
	if stats, ok := s.taskStats[key]; ok {
		copy := *stats
		return &copy
	}
	return nil
}

// GetAllStats returns all model statistics
func (s *ModelLearningStore) GetAllStats() map[string]*ModelStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]*ModelStats)
	for k, v := range s.stats {
		statsCopy := *v
		result[k] = &statsCopy
	}
	return result
}

// GetAllTaskStats returns all task-specific statistics
func (s *ModelLearningStore) GetAllTaskStats() []*TaskModelStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*TaskModelStats, 0, len(s.taskStats))
	for _, v := range s.taskStats {
		statsCopy := *v
		result = append(result, &statsCopy)
	}
	return result
}

// Clear resets the learning store
func (s *ModelLearningStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.outcomes = make([]ModelOutcome, 0)
	s.stats = make(map[string]*ModelStats)
	s.taskStats = make(map[string]*TaskModelStats)

	if s.persistPath != "" {
		s.saveUnlocked()
	}
}

// Count returns number of recorded outcomes
func (s *ModelLearningStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.outcomes)
}

// SetMinSamples sets minimum samples needed before recommending
func (s *ModelLearningStore) SetMinSamples(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.minSamples = n
}

// learningData is the serialized format for persistence
type learningData struct {
	Outcomes  []ModelOutcome              `json:"outcomes"`
	Stats     map[string]*ModelStats      `json:"stats"`
	TaskStats map[string]*TaskModelStats  `json:"task_stats"`
}

// Save persists the learning data to disk
func (s *ModelLearningStore) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveUnlocked()
}

func (s *ModelLearningStore) saveUnlocked() error {
	if s.persistPath == "" {
		return nil
	}

	data := learningData{
		Outcomes:  s.outcomes,
		Stats:     s.stats,
		TaskStats: s.taskStats,
	}

	// Ensure directory exists
	dir := filepath.Dir(s.persistPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.persistPath, jsonData, 0644)
}

// Load reads learning data from disk
func (s *ModelLearningStore) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.persistPath == "" {
		return nil
	}

	jsonData, err := os.ReadFile(s.persistPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No data yet
		}
		return err
	}

	var data learningData
	if err := json.Unmarshal(jsonData, &data); err != nil {
		return err
	}

	s.outcomes = data.Outcomes
	if s.outcomes == nil {
		s.outcomes = make([]ModelOutcome, 0)
	}
	s.stats = data.Stats
	if s.stats == nil {
		s.stats = make(map[string]*ModelStats)
	}
	s.taskStats = data.TaskStats
	if s.taskStats == nil {
		s.taskStats = make(map[string]*TaskModelStats)
	}

	return nil
}
