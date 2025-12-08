// Package agent tests learning system
package agent

import (
	"context"
	"testing"
	"time"
)

// MockStrategyStore for testing
type MockStrategyStore struct {
	strategies map[string]*Strategy
	impls      map[string]*StrategyImpl
	failures   map[string][]*FailedAttempt
}

func NewMockStrategyStore() *MockStrategyStore {
	return &MockStrategyStore{
		strategies: make(map[string]*Strategy),
		impls:      make(map[string]*StrategyImpl),
		failures:   make(map[string][]*FailedAttempt),
	}
}

func (m *MockStrategyStore) SaveStrategy(ctx context.Context, s *Strategy) error {
	m.strategies[s.ID] = s
	return nil
}

func (m *MockStrategyStore) GetStrategy(ctx context.Context, id string) (*Strategy, error) {
	return m.strategies[id], nil
}

func (m *MockStrategyStore) FindStrategies(ctx context.Context, taskType string) ([]*Strategy, error) {
	var result []*Strategy
	for _, s := range m.strategies {
		if s.TaskType == taskType || s.Level == "abstract" {
			result = append(result, s)
		}
	}
	return result, nil
}

func (m *MockStrategyStore) FindBestStrategy(ctx context.Context, taskDesc, env string) (*StrategyHint, error) {
	var best *Strategy
	var bestScore float64
	for _, s := range m.strategies {
		score := s.MatchScore(taskDesc)
		if score > bestScore {
			bestScore = score
			best = s
		}
	}
	if best == nil {
		return nil, nil
	}
	return &StrategyHint{Strategy: best, Confidence: bestScore}, nil
}

func (m *MockStrategyStore) SaveImpl(ctx context.Context, impl *StrategyImpl) error {
	m.impls[impl.ID] = impl
	return nil
}

func (m *MockStrategyStore) GetImpl(ctx context.Context, strategyID, env string) (*StrategyImpl, error) {
	key := strategyID + "-" + env
	return m.impls[key], nil
}

func (m *MockStrategyStore) FindSimilarImpl(ctx context.Context, strategyID, targetEnv string) (*StrategyImpl, float64, error) {
	var best *StrategyImpl
	var bestSim float64
	for _, impl := range m.impls {
		if impl.StrategyID == strategyID && impl.Environment != targetEnv {
			sim := GetEnvSimilarity(impl.Environment, targetEnv)
			if sim > bestSim {
				bestSim = sim
				best = impl
			}
		}
	}
	return best, bestSim, nil
}

func (m *MockStrategyStore) SaveFailedAttempt(ctx context.Context, fa *FailedAttempt) error {
	key := fa.StrategyID + "-" + fa.Environment
	m.failures[key] = append(m.failures[key], fa)
	return nil
}

func (m *MockStrategyStore) GetFailedAttempts(ctx context.Context, strategyID, env string) ([]*FailedAttempt, error) {
	key := strategyID + "-" + env
	return m.failures[key], nil
}

func TestStrategy(t *testing.T) {
	// Test strategy creation
	s := NewStrategy("grep-then-edit", "refactor", []string{"grep", "read", "edit"})

	if s.ID == "" {
		t.Error("Strategy ID should be generated")
	}
	if s.Confidence != 0.5 {
		t.Errorf("Initial confidence should be 0.5, got %f", s.Confidence)
	}

	// Test success recording
	s.RecordSuccess()
	s.RecordSuccess()
	s.RecordFailure()

	if s.UsageCount != 3 {
		t.Errorf("UsageCount should be 3, got %d", s.UsageCount)
	}
	if s.SuccessCount != 2 {
		t.Errorf("SuccessCount should be 2, got %d", s.SuccessCount)
	}
	// Confidence = 2/3 ≈ 0.666
	if s.Confidence < 0.66 || s.Confidence > 0.67 {
		t.Errorf("Confidence should be ~0.666, got %f", s.Confidence)
	}
}

func TestStrategyMatchScore(t *testing.T) {
	s := &Strategy{
		Name:       "grep-then-edit",
		TaskType:   "refactor",
		Confidence: 0.8,
	}

	// Should match refactor keywords
	score := s.MatchScore("refactor the getData function")
	if score < 0.1 {
		t.Errorf("Expected score > 0.1 for refactor task, got %f", score)
	}

	// Should not match well for unrelated task
	score2 := s.MatchScore("deploy the application")
	if score2 >= score {
		t.Errorf("Unrelated task should score lower: %f vs %f", score2, score)
	}
}

func TestEnvSimilarity(t *testing.T) {
	tests := []struct {
		env1, env2 string
		minSim     float64
	}{
		{"go", "go", 1.0},          // Same
		{"go", "rust", 0.7},        // High similarity
		{"go", "python", 0.2},      // Low similarity
		{"js", "ts", 0.9},          // Very high
		{"unknown1", "unknown2", 0}, // Default low
	}

	for _, tt := range tests {
		sim := GetEnvSimilarity(tt.env1, tt.env2)
		if sim < tt.minSim {
			t.Errorf("GetEnvSimilarity(%q, %q) = %f, want >= %f",
				tt.env1, tt.env2, sim, tt.minSim)
		}
	}
}

func TestStrategyImpl(t *testing.T) {
	impl := NewStrategyImpl("strat-1", "go", []string{"go mod graph", "grep"})

	// Record some usage
	impl.RecordUsage(3000, 4, true)
	impl.RecordUsage(4000, 5, true)
	impl.RecordUsage(5000, 6, false)

	if impl.SampleCount != 3 {
		t.Errorf("SampleCount should be 3, got %d", impl.SampleCount)
	}
	if impl.AvgTokens < 3000 || impl.AvgTokens > 5000 {
		t.Errorf("AvgTokens should be ~4000, got %f", impl.AvgTokens)
	}
	// Success rate should be 2/3
	if impl.SuccessRate < 0.6 || impl.SuccessRate > 0.7 {
		t.Errorf("SuccessRate should be ~0.666, got %f", impl.SuccessRate)
	}

	// Test anti-pattern
	impl.AddAntiPattern("don't use go list -m")
	impl.AddAntiPattern("don't use go list -m") // duplicate
	if len(impl.AntiPatterns) != 1 {
		t.Errorf("Should have 1 anti-pattern, got %d", len(impl.AntiPatterns))
	}
}

func TestTaskAuditor(t *testing.T) {
	store := NewMockStrategyStore()
	auditor := NewTaskAuditor(store)

	// Create metrics for a completed task
	metrics := NewTaskMetrics("task-1", "fix the login bug")
	metrics.RecordTurn(1000, 500, 2, "explore", "read auth.go", true)
	metrics.RecordTurn(800, 600, 1, "edit", "fixed nil check", true)
	metrics.FilesTouched = []string{"auth.go"}
	metrics.Finish(true)

	// Audit the task
	tc := &TaskContext{
		FilesRead:    []string{"auth.go", "config.go"},
		FilesWritten: []string{"auth.go"},
	}
	analysis, err := auditor.Audit(context.Background(), metrics, tc)
	if err != nil {
		t.Fatalf("Audit failed: %v", err)
	}

	if !analysis.Success {
		t.Error("Analysis should show success")
	}
	if analysis.TotalTokens == 0 {
		t.Error("TotalTokens should be > 0")
	}

	// Learn from it
	err = auditor.Learn(context.Background(), analysis)
	if err != nil {
		t.Fatalf("Learn failed: %v", err)
	}

	// Should have created a strategy
	if len(store.strategies) == 0 {
		t.Error("Should have created a strategy")
	}
}

func TestTaskAuditorHint(t *testing.T) {
	store := NewMockStrategyStore()

	// Seed a strategy
	store.strategies["strat-grep-then-edit"] = &Strategy{
		ID:         "strat-grep-then-edit",
		Name:       "grep-then-edit",
		TaskType:   "refactor",
		Steps:      []string{"grep", "read", "edit"},
		Confidence: 0.85,
	}

	auditor := NewTaskAuditor(store)

	// Get hint for a refactor task
	hint, err := auditor.GetHint(context.Background(), "refactor the login function", "go")
	if err != nil {
		t.Fatalf("GetHint failed: %v", err)
	}

	if hint == nil {
		t.Fatal("Expected hint, got nil")
	}
	if hint.Strategy == nil {
		t.Fatal("Expected strategy in hint")
	}
	if hint.Confidence <= 0 {
		t.Error("Confidence should be > 0")
	}
}

func TestBuiltinStrategies(t *testing.T) {
	strategies := BuiltinStrategies()
	if len(strategies) == 0 {
		t.Fatal("Should have builtin strategies")
	}

	// Check each strategy is valid
	for _, s := range strategies {
		if s.ID == "" {
			t.Error("Strategy missing ID")
		}
		if s.Name == "" {
			t.Error("Strategy missing Name")
		}
		if len(s.Steps) == 0 {
			t.Errorf("Strategy %s missing Steps", s.ID)
		}
	}
}

func TestBuiltinImplementations(t *testing.T) {
	impls := BuiltinImplementations()
	if len(impls) == 0 {
		t.Fatal("Should have builtin implementations")
	}

	// Check each impl is valid
	for _, impl := range impls {
		if impl.StrategyID == "" {
			t.Error("Impl missing StrategyID")
		}
		if impl.Environment == "" {
			t.Error("Impl missing Environment")
		}
		if len(impl.Toolchain) == 0 {
			t.Errorf("Impl %s missing Toolchain", impl.ID)
		}
	}
}

func TestSimilarTaskMetrics(t *testing.T) {
	similar := []SimilarTask{
		{Tokens: 3000, Turns: 4, Similarity: 0.9, StrategyUsed: "grep-then-edit"},
		{Tokens: 4000, Turns: 5, Similarity: 0.7, StrategyUsed: "grep-then-edit"},
		{Tokens: 5000, Turns: 6, Similarity: 0.5, StrategyUsed: "read-before-edit"},
	}

	// Test strategy extraction
	strat, conf := GetStrategyFromSimilar(similar)
	if strat == "" {
		t.Error("Should get a strategy")
	}
	if conf <= 0 {
		t.Error("Confidence should be > 0")
	}

	// Test expected metrics
	avgTokens, avgTurns := GetExpectedMetrics(similar)
	// Weighted average should favor higher similarity
	if avgTokens < 3000 || avgTokens > 5000 {
		t.Errorf("avgTokens %f out of expected range", avgTokens)
	}
	if avgTurns < 4 || avgTurns > 6 {
		t.Errorf("avgTurns %f out of expected range", avgTurns)
	}
}

func TestTaskMetrics(t *testing.T) {
	m := NewTaskMetrics("task-123", "fix the auth bug")

	// Record some turns
	m.RecordTurn(1000, 500, 2, "explore", "reading files", true)
	m.RecordTurn(800, 400, 1, "edit", "fixing bug", true)
	m.RecordTurn(500, 300, 0, "verify", "running tests", true)

	m.RecordFileModified("auth.go", true)
	m.RecordFileModified("readme.md", false) // Out of scope

	m.RecordError("test failed initially")
	m.RecordRecovery()

	m.Finish(true)

	// Check scores
	if m.Scores == nil {
		t.Fatal("Scores should be computed")
	}
	if m.Scores.FocusScore <= 0 {
		t.Error("FocusScore should be > 0")
	}
	if m.Scores.OverallScore <= 0 {
		t.Error("OverallScore should be > 0")
	}

	// Check report generation
	report := m.Report()
	if report == "" {
		t.Error("Report should not be empty")
	}
	if !containsStr(report, "TASK EFFICIENCY REPORT") {
		t.Error("Report missing header")
	}
}

func TestLearningAgentLifecycle(t *testing.T) {
	store := NewMockStrategyStore()

	// Seed builtin strategies
	for _, s := range BuiltinStrategies() {
		store.strategies[s.ID] = s
	}

	// Create a mock agent (nil is ok for this test)
	la, err := NewLearningAgent(nil, store, "")
	if err != nil {
		t.Fatalf("NewLearningAgent failed: %v", err)
	}
	defer la.Close()

	// Pre-task: get strategy hint
	hint, err := la.PreTask(context.Background(), "refactor the login function")
	if err != nil {
		t.Fatalf("PreTask failed: %v", err)
	}
	// Hint may be nil if no matching strategy, that's ok

	// Start task tracking
	la.StartTask("task-test-1", "refactor the login function")

	// Record some activity
	la.RecordTurn(1000, 500, 2, "explore", "reading files")
	la.RecordTurn(800, 400, 1, "edit", "refactoring")

	// Post-task: analyze and learn
	err = la.PostTask(context.Background(), true)
	if err != nil {
		t.Fatalf("PostTask failed: %v", err)
	}

	// After PostTask, metrics should be cleared
	if la.GetMetrics() != nil {
		t.Error("Metrics should be cleared after PostTask")
	}

	// Verify something was learned
	if len(store.strategies) == 0 && len(store.impls) == 0 {
		t.Error("Should have learned something")
	}

	_ = hint // used
}

func TestBuildEnhancedPrompt(t *testing.T) {
	store := NewMockStrategyStore()
	store.strategies["strat-test"] = &Strategy{
		ID:         "strat-test",
		Name:       "test-strategy",
		Steps:      []string{"step1", "step2"},
		Confidence: 0.8,
	}

	la, _ := NewLearningAgent(nil, store, "")

	// Set strategy hint manually
	la.strategyHint = &StrategyHint{
		Strategy: store.strategies["strat-test"],
		Confidence: 0.8,
		Suggestions: []string{"Use grep first"},
		Avoid: []string{"Don't over-read"},
	}

	base := "You are a helpful assistant."
	enhanced := la.BuildEnhancedPrompt(base)

	if enhanced == base {
		t.Error("Should have enhanced the prompt")
	}
	if !containsStr(enhanced, "learned-strategy") {
		t.Error("Should contain learned-strategy tag")
	}
	if !containsStr(enhanced, "test-strategy") {
		t.Error("Should contain strategy name")
	}
	if !containsStr(enhanced, "step1") {
		t.Error("Should contain steps")
	}
}

func TestTaskAuditorDetectEnvironment(t *testing.T) {
	auditor := &TaskAuditor{}

	tests := []struct {
		files    []string
		expected string
	}{
		{[]string{"main.go", "util.go", "test_test.go"}, "go"},
		{[]string{"app.py", "utils.py"}, "python"},
		{[]string{"lib.rs", "main.rs"}, "rust"},
		{[]string{"index.js", "app.js"}, "js"},
		{[]string{"index.ts", "config.ts"}, "ts"},
		{[]string{"readme.md"}, "unknown"},
	}

	for _, tt := range tests {
		env := auditor.detectEnvironment(tt.files)
		if env != tt.expected {
			t.Errorf("detectEnvironment(%v) = %q, want %q", tt.files, env, tt.expected)
		}
	}
}

func TestTaskAuditorInferTaskType(t *testing.T) {
	auditor := &TaskAuditor{}

	tests := []struct {
		objective string
		expected  string
	}{
		{"refactor the login function", "refactor"},
		{"rename getData to fetchData", "refactor"},
		{"fix the null pointer bug", "bugfix"},
		{"add user authentication", "feature"},
		{"implement dark mode", "feature"},
		{"find where errors are handled", "explore"},
		{"do something random", "general"},
	}

	for _, tt := range tests {
		taskType := auditor.inferTaskType(tt.objective)
		if taskType != tt.expected {
			t.Errorf("inferTaskType(%q) = %q, want %q", tt.objective, taskType, tt.expected)
		}
	}
}

func TestFailedAttemptRecording(t *testing.T) {
	store := NewMockStrategyStore()
	auditor := NewTaskAuditor(store)

	// Create metrics for a failed task
	metrics := NewTaskMetrics("task-fail", "fix the impossible bug")
	metrics.RecordTurn(2000, 1000, 5, "explore", "searching everywhere", true)
	metrics.RecordError("could not find root cause")
	metrics.Finish(false)

	tc := &TaskContext{
		FilesRead:    []string{"a.go", "b.go", "c.go"},
		FilesWritten: []string{},
	}

	// Audit and learn
	analysis, _ := auditor.Audit(context.Background(), metrics, tc)
	auditor.Learn(context.Background(), analysis)

	// Should have recorded failure
	var failureCount int
	for _, failures := range store.failures {
		failureCount += len(failures)
	}
	if failureCount == 0 {
		t.Error("Should have recorded at least one failed attempt")
	}
}

func TestStrategyImplPerformanceTracking(t *testing.T) {
	impl := NewStrategyImpl("strat-perf", "go", []string{"grep", "edit"})

	// Simulate 10 task completions
	for i := 0; i < 10; i++ {
		tokens := 2000 + i*100
		turns := 3 + (i % 3)
		success := i%4 != 0 // 75% success rate
		impl.RecordUsage(tokens, turns, success)
	}

	// Verify tracking
	if impl.SampleCount != 10 {
		t.Errorf("SampleCount = %d, want 10", impl.SampleCount)
	}
	if impl.AvgTokens < 2000 || impl.AvgTokens > 3000 {
		t.Errorf("AvgTokens = %f, out of range", impl.AvgTokens)
	}
	if impl.SuccessRate < 0.7 || impl.SuccessRate > 0.8 {
		t.Errorf("SuccessRate = %f, expected ~0.75", impl.SuccessRate)
	}
	if impl.LastUsed.Before(time.Now().Add(-time.Minute)) {
		t.Error("LastUsed should be recent")
	}
}
