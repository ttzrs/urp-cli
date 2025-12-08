// Package agent provides adaptive learning strategies
package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// Strategy represents an abstract pattern that "rhymes" across environments
type Strategy struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Level       string    `json:"level"` // "abstract" | "concrete"
	TaskType    string    `json:"task_type"` // refactor, bugfix, feature, explore

	// Pattern definition
	Steps       []string  `json:"steps"`       // Ordered steps: ["grep target", "analyze deps", "edit"]
	Tools       []string  `json:"tools"`       // Tools typically used
	Preconditions []string `json:"preconditions"` // What must be true before using

	// Learning metrics
	Confidence   float64   `json:"confidence"`    // 0-1, how reliable
	UsageCount   int       `json:"usage_count"`
	SuccessCount int       `json:"success_count"`
	LearnedFrom  []string  `json:"learned_from"`  // Task IDs that taught this
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// StrategyImpl is an environment-specific implementation of a Strategy
type StrategyImpl struct {
	ID          string  `json:"id"`
	StrategyID  string  `json:"strategy_id"`  // Parent abstract strategy
	Environment string  `json:"environment"`  // go, python, rust, js, etc.

	// Implementation details
	Toolchain    []string `json:"toolchain"`     // Specific tools: ["go mod graph", "grep"]
	Commands     []string `json:"commands"`      // Specific commands that work
	AntiPatterns []string `json:"anti_patterns"` // What NOT to do in this env

	// Performance in this environment
	AvgTokens    float64 `json:"avg_tokens"`
	AvgTurns     float64 `json:"avg_turns"`
	SuccessRate  float64 `json:"success_rate"`
	SampleCount  int     `json:"sample_count"`

	// Metadata
	LastUsed  time.Time `json:"last_used"`
	CreatedAt time.Time `json:"created_at"`
}

// FailedAttempt records what didn't work (negative knowledge)
type FailedAttempt struct {
	ID            string    `json:"id"`
	StrategyID    string    `json:"strategy_id"`
	Environment   string    `json:"environment"`
	TaskID        string    `json:"task_id"`
	AttemptedTools []string `json:"attempted_tools"`
	FailureReason string    `json:"failure_reason"`
	Suggestion    string    `json:"suggestion"` // What to try instead
	CreatedAt     time.Time `json:"created_at"`
}

// StrategyHint is a suggestion for approaching a task
type StrategyHint struct {
	Strategy    *Strategy     `json:"strategy"`
	Impl        *StrategyImpl `json:"impl,omitempty"`        // Exact match
	SimilarImpl *StrategyImpl `json:"similar_impl,omitempty"` // From similar env

	Confidence    float64  `json:"confidence"`     // Adjusted for transfer
	TransferFrom  string   `json:"transfer_from"`  // If using similar env
	Suggestions   []string `json:"suggestions"`    // Specific hints
	Avoid         []string `json:"avoid"`          // Anti-patterns to avoid
}

// Environment similarity for transfer learning
var EnvSimilarity = map[string]map[string]float64{
	"go": {
		"rust":   0.70, // Similar: compiled, typed, systems
		"c":      0.60,
		"cpp":    0.55,
		"java":   0.50,
		"python": 0.30,
		"js":     0.25,
	},
	"rust": {
		"go":     0.70,
		"c":      0.80, // Very similar memory model
		"cpp":    0.75,
		"python": 0.20,
	},
	"python": {
		"ruby":   0.80, // Similar: dynamic, scripting
		"js":     0.60,
		"php":    0.50,
		"go":     0.30,
	},
	"js": {
		"ts":     0.95, // Almost identical
		"python": 0.55,
		"ruby":   0.50,
	},
	"ts": {
		"js":     0.95,
		"python": 0.50,
	},
	"java": {
		"kotlin": 0.85,
		"scala":  0.70,
		"go":     0.50,
		"cpp":    0.45,
	},
}

// GetEnvSimilarity returns similarity score between two environments
func GetEnvSimilarity(env1, env2 string) float64 {
	if env1 == env2 {
		return 1.0
	}
	if sims, ok := EnvSimilarity[env1]; ok {
		if sim, ok := sims[env2]; ok {
			return sim
		}
	}
	// Try reverse
	if sims, ok := EnvSimilarity[env2]; ok {
		if sim, ok := sims[env1]; ok {
			return sim
		}
	}
	return 0.1 // Default low similarity
}

// NewStrategy creates a new abstract strategy
func NewStrategy(name, taskType string, steps []string) *Strategy {
	id := generateStrategyID(name, taskType)
	return &Strategy{
		ID:          id,
		Name:        name,
		Level:       "abstract",
		TaskType:    taskType,
		Steps:       steps,
		Confidence:  0.5, // Start neutral
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
}

// NewStrategyImpl creates an environment-specific implementation
func NewStrategyImpl(strategyID, environment string, toolchain []string) *StrategyImpl {
	id := fmt.Sprintf("%s-%s", strategyID, environment)
	return &StrategyImpl{
		ID:          id,
		StrategyID:  strategyID,
		Environment: environment,
		Toolchain:   toolchain,
		SuccessRate: 0.5,
		CreatedAt:   time.Now(),
	}
}

// RecordSuccess updates metrics after successful use
func (s *Strategy) RecordSuccess() {
	s.UsageCount++
	s.SuccessCount++
	s.Confidence = float64(s.SuccessCount) / float64(s.UsageCount)
	s.UpdatedAt = time.Now()
}

// RecordFailure updates metrics after failed use
func (s *Strategy) RecordFailure() {
	s.UsageCount++
	s.Confidence = float64(s.SuccessCount) / float64(s.UsageCount)
	s.UpdatedAt = time.Now()
}

// RecordUsage updates implementation metrics
func (impl *StrategyImpl) RecordUsage(tokens, turns int, success bool) {
	impl.SampleCount++

	// Running average for tokens and turns
	impl.AvgTokens = (impl.AvgTokens*float64(impl.SampleCount-1) + float64(tokens)) / float64(impl.SampleCount)
	impl.AvgTurns = (impl.AvgTurns*float64(impl.SampleCount-1) + float64(turns)) / float64(impl.SampleCount)

	// Success rate
	if success {
		impl.SuccessRate = (impl.SuccessRate*float64(impl.SampleCount-1) + 1.0) / float64(impl.SampleCount)
	} else {
		impl.SuccessRate = (impl.SuccessRate * float64(impl.SampleCount-1)) / float64(impl.SampleCount)
	}

	impl.LastUsed = time.Now()
}

// AddAntiPattern records what not to do
func (impl *StrategyImpl) AddAntiPattern(pattern string) {
	for _, p := range impl.AntiPatterns {
		if p == pattern {
			return // Already exists
		}
	}
	impl.AntiPatterns = append(impl.AntiPatterns, pattern)
}

// MatchScore returns how well this strategy matches a task description
func (s *Strategy) MatchScore(taskDescription string) float64 {
	desc := strings.ToLower(taskDescription)
	score := 0.0

	// Match task type keywords
	typeKeywords := map[string][]string{
		"refactor": {"refactor", "rename", "move", "restructure", "reorganize"},
		"bugfix":   {"fix", "bug", "error", "crash", "panic", "nil", "null"},
		"feature":  {"add", "implement", "create", "new", "feature"},
		"explore":  {"find", "search", "where", "locate", "understand"},
	}

	if keywords, ok := typeKeywords[s.TaskType]; ok {
		for _, kw := range keywords {
			if strings.Contains(desc, kw) {
				score += 0.2
			}
		}
	}

	// Match strategy name keywords
	nameParts := strings.Split(strings.ToLower(s.Name), "-")
	for _, part := range nameParts {
		if strings.Contains(desc, part) {
			score += 0.1
		}
	}

	// Cap at confidence level
	if score > s.Confidence {
		score = s.Confidence
	}

	return score
}

// String returns a human-readable description
func (s *Strategy) String() string {
	return fmt.Sprintf("[%s] %s (%.0f%% confidence, %d uses)",
		s.TaskType, s.Name, s.Confidence*100, s.UsageCount)
}

// String returns implementation details
func (impl *StrategyImpl) String() string {
	return fmt.Sprintf("%s@%s: %.0f%% success, ~%.0f tokens, ~%.1f turns",
		impl.StrategyID, impl.Environment, impl.SuccessRate*100, impl.AvgTokens, impl.AvgTurns)
}

func generateStrategyID(name, taskType string) string {
	h := sha256.Sum256([]byte(name + taskType))
	return "strat-" + hex.EncodeToString(h[:8])
}

// BuiltinStrategies returns pre-defined strategies based on common patterns
func BuiltinStrategies() []*Strategy {
	return []*Strategy{
		{
			ID:          "strat-grep-then-edit",
			Name:        "grep-then-edit",
			Description: "Find occurrences first, then edit targeted files",
			Level:       "abstract",
			TaskType:    "refactor",
			Steps:       []string{"grep pattern", "analyze results", "edit files", "verify"},
			Tools:       []string{"grep", "read", "edit"},
			Confidence:  0.85,
			CreatedAt:   time.Now(),
		},
		{
			ID:          "strat-deps-first",
			Name:        "check-deps-first",
			Description: "Analyze dependencies before modifying",
			Level:       "abstract",
			TaskType:    "refactor",
			Steps:       []string{"find target", "analyze deps", "plan order", "edit leaf-to-root"},
			Tools:       []string{"grep", "read"},
			Preconditions: []string{"multi-file change", "function/type rename"},
			Confidence:  0.80,
			CreatedAt:   time.Now(),
		},
		{
			ID:          "strat-read-before-edit",
			Name:        "read-before-edit",
			Description: "Always read a file before editing it",
			Level:       "abstract",
			TaskType:    "bugfix",
			Steps:       []string{"read file", "understand context", "edit", "verify"},
			Tools:       []string{"read", "edit"},
			Confidence:  0.95,
			CreatedAt:   time.Now(),
		},
		{
			ID:          "strat-narrow-search",
			Name:        "narrow-search",
			Description: "Start broad, narrow down to specific file",
			Level:       "abstract",
			TaskType:    "explore",
			Steps:       []string{"glob pattern", "grep content", "read candidates", "confirm"},
			Tools:       []string{"glob", "grep", "read"},
			Confidence:  0.75,
			CreatedAt:   time.Now(),
		},
		{
			ID:          "strat-minimal-change",
			Name:        "minimal-change",
			Description: "Make smallest possible change to fix issue",
			Level:       "abstract",
			TaskType:    "bugfix",
			Steps:       []string{"locate issue", "understand root cause", "surgical edit", "verify"},
			Tools:       []string{"grep", "read", "edit"},
			Preconditions: []string{"specific bug", "known location"},
			Confidence:  0.88,
			CreatedAt:   time.Now(),
		},
	}
}

// BuiltinImplementations returns pre-defined implementations
func BuiltinImplementations() []*StrategyImpl {
	return []*StrategyImpl{
		// Go implementations
		{
			ID:          "strat-deps-first-go",
			StrategyID:  "strat-deps-first",
			Environment: "go",
			Toolchain:   []string{"go mod graph", "grep import", "guru referrers"},
			AntiPatterns: []string{"don't use go list -m all for local deps"},
			AvgTokens:   3200,
			AvgTurns:    4.5,
			SuccessRate: 0.88,
			SampleCount: 10,
			CreatedAt:   time.Now(),
		},
		// Python implementations
		{
			ID:          "strat-deps-first-python",
			StrategyID:  "strat-deps-first",
			Environment: "python",
			Toolchain:   []string{"pipdeptree", "grep import", "ast module"},
			AntiPatterns: []string{"don't rely on __init__.py order"},
			AvgTokens:   2800,
			AvgTurns:    4.0,
			SuccessRate: 0.85,
			SampleCount: 8,
			CreatedAt:   time.Now(),
		},
		// Rust implementations
		{
			ID:          "strat-deps-first-rust",
			StrategyID:  "strat-deps-first",
			Environment: "rust",
			Toolchain:   []string{"cargo tree", "grep use", "rust-analyzer"},
			AntiPatterns: []string{"cargo tree doesn't show internal deps well"},
			AvgTokens:   3500,
			AvgTurns:    5.0,
			SuccessRate: 0.82,
			SampleCount: 5,
			CreatedAt:   time.Now(),
		},
	}
}
