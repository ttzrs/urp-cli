// Package agent provides task auditing and learning
package agent

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// TaskAuditor analyzes completed tasks and extracts learnings
type TaskAuditor struct {
	store StrategyStore // Persistence layer
}

// StrategyStore is the interface for strategy persistence
type StrategyStore interface {
	// Strategies
	SaveStrategy(ctx context.Context, s *Strategy) error
	GetStrategy(ctx context.Context, id string) (*Strategy, error)
	FindStrategies(ctx context.Context, taskType string) ([]*Strategy, error)
	FindBestStrategy(ctx context.Context, taskDesc, env string) (*StrategyHint, error)

	// Implementations
	SaveImpl(ctx context.Context, impl *StrategyImpl) error
	GetImpl(ctx context.Context, strategyID, env string) (*StrategyImpl, error)
	FindSimilarImpl(ctx context.Context, strategyID, targetEnv string) (*StrategyImpl, float64, error)

	// Failed attempts
	SaveFailedAttempt(ctx context.Context, fa *FailedAttempt) error
	GetFailedAttempts(ctx context.Context, strategyID, env string) ([]*FailedAttempt, error)
}

// TaskAnalysis is the result of auditing a completed task
type TaskAnalysis struct {
	TaskID      string
	Objective   string
	Environment string
	Success     bool

	// Extracted patterns
	StrategyUsed    string   // Detected strategy pattern
	ToolSequence    []string // Tools used in order
	EffectiveTools  []string // Tools that contributed to success
	WastedTools     []string // Tools that didn't help

	// Metrics
	TotalTokens     int
	TotalTurns      int
	TokenEfficiency float64 // Compared to baseline
	TurnEfficiency  float64

	// Issues
	ScopeCreep    []string // Actions outside objective
	Errors        []string // Errors encountered
	Recoveries    int      // Successful error recoveries

	// Recommendations
	SuggestedStrategy *Strategy
	Improvements      []string
}

// NewTaskAuditor creates an auditor with a strategy store
func NewTaskAuditor(store StrategyStore) *TaskAuditor {
	return &TaskAuditor{store: store}
}

// Audit analyzes a completed task and extracts learnings
func (a *TaskAuditor) Audit(ctx context.Context, metrics *TaskMetrics, tc *TaskContext) (*TaskAnalysis, error) {
	analysis := &TaskAnalysis{
		TaskID:      metrics.TaskID,
		Objective:   metrics.Objective,
		Success:     metrics.FinalSuccess,
		TotalTokens: metrics.InputTokens + metrics.OutputTokens + metrics.ThinkingTokens,
		TotalTurns:  metrics.CurrentTurn,
		Errors:      metrics.Errors,
		Recoveries:  metrics.Recoveries,
		ScopeCreep:  metrics.ScopeCreep,
	}

	// Detect environment from files touched
	analysis.Environment = a.detectEnvironment(metrics.FilesTouched)

	// Extract tool sequence
	analysis.ToolSequence = a.extractToolSequence(tc)

	// Classify tools
	analysis.EffectiveTools, analysis.WastedTools = a.classifyTools(tc, metrics)

	// Detect strategy pattern
	analysis.StrategyUsed = a.detectStrategy(analysis.ToolSequence)

	// Calculate efficiency
	analysis.TokenEfficiency = a.calculateTokenEfficiency(analysis)
	analysis.TurnEfficiency = a.calculateTurnEfficiency(analysis)

	// Generate recommendations
	analysis.Improvements = a.generateImprovements(analysis)

	return analysis, nil
}

// Learn updates the knowledge base based on task analysis
func (a *TaskAuditor) Learn(ctx context.Context, analysis *TaskAnalysis) error {
	if a.store == nil {
		return nil // No persistence configured
	}

	// 1. Update or create strategy
	strategy, err := a.store.GetStrategy(ctx, analysis.StrategyUsed)
	if err != nil || strategy == nil {
		// Create new strategy from this task
		strategy = a.createStrategyFromAnalysis(analysis)
		if err := a.store.SaveStrategy(ctx, strategy); err != nil {
			return fmt.Errorf("save strategy: %w", err)
		}
	} else {
		// Update existing strategy
		if analysis.Success {
			strategy.RecordSuccess()
		} else {
			strategy.RecordFailure()
		}
		strategy.LearnedFrom = append(strategy.LearnedFrom, analysis.TaskID)
		if err := a.store.SaveStrategy(ctx, strategy); err != nil {
			return fmt.Errorf("update strategy: %w", err)
		}
	}

	// 2. Update or create implementation
	impl, err := a.store.GetImpl(ctx, strategy.ID, analysis.Environment)
	if err != nil || impl == nil {
		impl = NewStrategyImpl(strategy.ID, analysis.Environment, analysis.EffectiveTools)
	}
	impl.RecordUsage(analysis.TotalTokens, analysis.TotalTurns, analysis.Success)

	// Add anti-patterns from wasted tools
	for _, wasted := range analysis.WastedTools {
		impl.AddAntiPattern(fmt.Sprintf("avoid %s for this task type", wasted))
	}

	if err := a.store.SaveImpl(ctx, impl); err != nil {
		return fmt.Errorf("save impl: %w", err)
	}

	// 3. Record failures
	if !analysis.Success {
		fa := &FailedAttempt{
			ID:             fmt.Sprintf("fail-%s-%d", analysis.TaskID, time.Now().Unix()),
			StrategyID:     strategy.ID,
			Environment:    analysis.Environment,
			TaskID:         analysis.TaskID,
			AttemptedTools: analysis.ToolSequence,
			FailureReason:  strings.Join(analysis.Errors, "; "),
			Suggestion:     strings.Join(analysis.Improvements, "; "),
			CreatedAt:      time.Now(),
		}
		if err := a.store.SaveFailedAttempt(ctx, fa); err != nil {
			return fmt.Errorf("save failed attempt: %w", err)
		}
	}

	return nil
}

// GetHint returns strategy suggestions before starting a task
func (a *TaskAuditor) GetHint(ctx context.Context, objective, environment string) (*StrategyHint, error) {
	if a.store == nil {
		return a.getBuiltinHint(objective, environment), nil
	}

	hint, err := a.store.FindBestStrategy(ctx, objective, environment)
	if err != nil || hint == nil {
		return a.getBuiltinHint(objective, environment), nil
	}

	// Enrich with failed attempts to avoid
	if hint.Strategy != nil {
		failures, _ := a.store.GetFailedAttempts(ctx, hint.Strategy.ID, environment)
		for _, f := range failures {
			hint.Avoid = append(hint.Avoid, f.FailureReason)
			if f.Suggestion != "" {
				hint.Suggestions = append(hint.Suggestions, f.Suggestion)
			}
		}
	}

	return hint, nil
}

// detectEnvironment identifies the programming environment from files
func (a *TaskAuditor) detectEnvironment(files []string) string {
	extCount := make(map[string]int)

	for _, f := range files {
		if idx := strings.LastIndex(f, "."); idx != -1 {
			ext := f[idx+1:]
			extCount[ext]++
		}
	}

	// Map extensions to environments
	extToEnv := map[string]string{
		"go":   "go",
		"py":   "python",
		"rs":   "rust",
		"js":   "js",
		"ts":   "ts",
		"java": "java",
		"kt":   "kotlin",
		"rb":   "ruby",
		"c":    "c",
		"cpp":  "cpp",
		"h":    "c",
	}

	bestEnv := "unknown"
	bestCount := 0

	for ext, count := range extCount {
		if env, ok := extToEnv[ext]; ok {
			if count > bestCount {
				bestCount = count
				bestEnv = env
			}
		}
	}

	return bestEnv
}

// extractToolSequence gets ordered tool usage from task context
func (a *TaskAuditor) extractToolSequence(tc *TaskContext) []string {
	if tc == nil {
		return nil
	}

	// FilesRead and FilesWritten contain tool info via LastAction
	// For now, return a simplified sequence
	var tools []string
	seen := make(map[string]bool)

	// Derive from files accessed
	for range tc.FilesRead {
		if !seen["read"] {
			tools = append(tools, "read")
			seen["read"] = true
		}
	}
	for range tc.FilesWritten {
		if !seen["edit"] {
			tools = append(tools, "edit")
			seen["edit"] = true
		}
	}

	return tools
}

// classifyTools separates effective from wasted tools
func (a *TaskAuditor) classifyTools(tc *TaskContext, metrics *TaskMetrics) (effective, wasted []string) {
	if tc == nil {
		return nil, nil
	}

	// Tools that led to written files are effective
	effectiveSet := make(map[string]bool)
	for _, f := range tc.FilesWritten {
		if strings.HasSuffix(f, ".go") {
			effectiveSet["edit"] = true
		}
	}

	// Tools used but didn't contribute
	wastedSet := make(map[string]bool)

	// If many files read but few written, some reads were wasted
	readRatio := float64(len(tc.FilesWritten)) / float64(len(tc.FilesRead)+1)
	if readRatio < 0.3 && len(tc.FilesRead) > 5 {
		wastedSet["excessive-reads"] = true
	}

	// Convert to slices
	for t := range effectiveSet {
		effective = append(effective, t)
	}
	for t := range wastedSet {
		wasted = append(wasted, t)
	}

	return effective, wasted
}

// detectStrategy identifies which strategy pattern was used
func (a *TaskAuditor) detectStrategy(toolSequence []string) string {
	seq := strings.Join(toolSequence, "-")

	// Pattern matching
	patterns := map[string][]string{
		"strat-grep-then-edit":   {"grep", "read", "edit"},
		"strat-read-before-edit": {"read", "edit"},
		"strat-narrow-search":    {"glob", "grep", "read"},
	}

	for id, pattern := range patterns {
		if a.matchesPattern(toolSequence, pattern) {
			return id
		}
	}

	// Default based on sequence
	if strings.Contains(seq, "edit") {
		return "strat-read-before-edit"
	}
	return "strat-unknown"
}

func (a *TaskAuditor) matchesPattern(sequence, pattern []string) bool {
	if len(sequence) < len(pattern) {
		return false
	}

	patternIdx := 0
	for _, tool := range sequence {
		if patternIdx < len(pattern) && tool == pattern[patternIdx] {
			patternIdx++
		}
	}

	return patternIdx == len(pattern)
}

// calculateTokenEfficiency compares to baseline
func (a *TaskAuditor) calculateTokenEfficiency(analysis *TaskAnalysis) float64 {
	// Baseline tokens by task type (estimated)
	baselines := map[string]int{
		"refactor": 5000,
		"bugfix":   3000,
		"feature":  6000,
		"explore":  2000,
	}

	taskType := a.inferTaskType(analysis.Objective)
	baseline, ok := baselines[taskType]
	if !ok {
		baseline = 4000
	}

	if analysis.TotalTokens == 0 {
		return 1.0
	}

	return float64(baseline) / float64(analysis.TotalTokens)
}

// calculateTurnEfficiency compares turns to baseline
func (a *TaskAuditor) calculateTurnEfficiency(analysis *TaskAnalysis) float64 {
	baselines := map[string]int{
		"refactor": 6,
		"bugfix":   4,
		"feature":  8,
		"explore":  3,
	}

	taskType := a.inferTaskType(analysis.Objective)
	baseline, ok := baselines[taskType]
	if !ok {
		baseline = 5
	}

	if analysis.TotalTurns == 0 {
		return 1.0
	}

	return float64(baseline) / float64(analysis.TotalTurns)
}

func (a *TaskAuditor) inferTaskType(objective string) string {
	obj := strings.ToLower(objective)

	if strings.Contains(obj, "refactor") || strings.Contains(obj, "rename") {
		return "refactor"
	}
	if strings.Contains(obj, "fix") || strings.Contains(obj, "bug") {
		return "bugfix"
	}
	if strings.Contains(obj, "add") || strings.Contains(obj, "implement") {
		return "feature"
	}
	if strings.Contains(obj, "find") || strings.Contains(obj, "search") {
		return "explore"
	}

	return "general"
}

// generateImprovements suggests how to do better next time
func (a *TaskAuditor) generateImprovements(analysis *TaskAnalysis) []string {
	var improvements []string

	// Token efficiency
	if analysis.TokenEfficiency < 0.7 {
		improvements = append(improvements, "Consider more targeted searches to reduce token usage")
	}

	// Turn efficiency
	if analysis.TurnEfficiency < 0.6 {
		improvements = append(improvements, "Plan approach before executing to reduce turns")
	}

	// Wasted tools
	if len(analysis.WastedTools) > 0 {
		improvements = append(improvements,
			fmt.Sprintf("Avoid: %s", strings.Join(analysis.WastedTools, ", ")))
	}

	// Scope creep
	if len(analysis.ScopeCreep) > 0 {
		improvements = append(improvements, "Stay focused on original objective")
	}

	// Error recovery
	if len(analysis.Errors) > 0 && analysis.Recoveries == 0 {
		improvements = append(improvements, "Develop better error recovery strategies")
	}

	return improvements
}

// createStrategyFromAnalysis creates a new strategy from task analysis
func (a *TaskAuditor) createStrategyFromAnalysis(analysis *TaskAnalysis) *Strategy {
	taskType := a.inferTaskType(analysis.Objective)

	return &Strategy{
		ID:          fmt.Sprintf("strat-learned-%d", time.Now().Unix()),
		Name:        fmt.Sprintf("%s-%s", taskType, analysis.Environment),
		Description: fmt.Sprintf("Learned from task: %s", truncateStr(analysis.Objective, 50)),
		Level:       "concrete",
		TaskType:    taskType,
		Steps:       analysis.ToolSequence,
		Tools:       analysis.EffectiveTools,
		Confidence:  0.5, // Start neutral
		UsageCount:  1,
		SuccessCount: boolToInt(analysis.Success),
		LearnedFrom: []string{analysis.TaskID},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
}

// getBuiltinHint returns a hint from builtin strategies
func (a *TaskAuditor) getBuiltinHint(objective, environment string) *StrategyHint {
	strategies := BuiltinStrategies()

	var bestStrategy *Strategy
	var bestScore float64

	for _, s := range strategies {
		score := s.MatchScore(objective)
		if score > bestScore {
			bestScore = score
			bestStrategy = s
		}
	}

	if bestStrategy == nil {
		return nil
	}

	hint := &StrategyHint{
		Strategy:   bestStrategy,
		Confidence: bestScore,
	}

	// Find implementation
	for _, impl := range BuiltinImplementations() {
		if impl.StrategyID == bestStrategy.ID {
			if impl.Environment == environment {
				hint.Impl = impl
				break
			}
			// Track for similarity
			sim := GetEnvSimilarity(impl.Environment, environment)
			if hint.SimilarImpl == nil || sim > GetEnvSimilarity(hint.SimilarImpl.Environment, environment) {
				hint.SimilarImpl = impl
				hint.TransferFrom = impl.Environment
				hint.Confidence *= sim
			}
		}
	}

	// Build suggestions
	if hint.Impl != nil {
		hint.Suggestions = append(hint.Suggestions, fmt.Sprintf("Use: %v", hint.Impl.Toolchain))
		hint.Avoid = hint.Impl.AntiPatterns
	} else if hint.SimilarImpl != nil {
		hint.Suggestions = append(hint.Suggestions,
			fmt.Sprintf("Transfer from %s: %v", hint.TransferFrom, hint.SimilarImpl.Toolchain))
		hint.Avoid = hint.SimilarImpl.AntiPatterns
	}

	return hint
}

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
