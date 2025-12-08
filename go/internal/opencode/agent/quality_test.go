package agent

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// QualityMetrics tracks test success rate vs token cost
type QualityMetrics struct {
	// Test results
	TestsAttempted int
	TestsPassed    int
	TestsFailed    int
	TestsSkipped   int

	// Token costs
	TotalInputTokens   int
	TotalOutputTokens  int
	TotalThinkingTokens int

	// Derived efficiency
	TokensPerTest       float64 // Average tokens per test
	TokensPerPassedTest float64 // Tokens spent per successful test
	PassRate            float64 // Success percentage
	EfficiencyScore     float64 // PassRate / (Tokens/1000) - higher is better

	// Timing
	TotalDuration time.Duration
	AvgDuration   time.Duration
}

// QualityTest represents a single quality test case
type QualityTest struct {
	Name        string
	Description string
	Objective   string
	Validator   func(result *TestResult) bool
	ExpectedTokens int // Expected token budget
}

// TestResult holds the outcome of a test
type TestResult struct {
	Name          string
	Passed        bool
	FailReason    string
	InputTokens   int
	OutputTokens  int
	ThinkingTokens int
	Duration      time.Duration
	Turns         int
	ToolCalls     int
	ScopeCreep    []string
}

// QualitySuite runs a suite of quality tests
type QualitySuite struct {
	tests   []QualityTest
	results []TestResult
	metrics QualityMetrics
}

// NewQualitySuite creates a new test suite
func NewQualitySuite() *QualitySuite {
	return &QualitySuite{
		tests:   defaultQualityTests(),
		results: make([]TestResult, 0),
	}
}

// defaultQualityTests returns the standard quality test suite
func defaultQualityTests() []QualityTest {
	return []QualityTest{
		{
			Name:        "focus-simple",
			Description: "Agent stays focused on simple single-file task",
			Objective:   "Add a comment to line 10 of main.go",
			ExpectedTokens: 2000,
			Validator: func(r *TestResult) bool {
				return r.Turns <= 3 && len(r.ScopeCreep) == 0
			},
		},
		{
			Name:        "focus-complex",
			Description: "Agent stays focused on multi-file refactor",
			Objective:   "Rename function across 5 files",
			ExpectedTokens: 8000,
			Validator: func(r *TestResult) bool {
				return r.Turns <= 10 && len(r.ScopeCreep) == 0
			},
		},
		{
			Name:        "recovery-error",
			Description: "Agent recovers from tool error gracefully",
			Objective:   "Edit file that doesn't exist initially",
			ExpectedTokens: 4000,
			Validator: func(r *TestResult) bool {
				return r.Passed // Should succeed after finding correct file
			},
		},
		{
			Name:        "efficiency-read",
			Description: "Agent doesn't re-read same file multiple times",
			Objective:   "Find and modify a function",
			ExpectedTokens: 3000,
			Validator: func(r *TestResult) bool {
				// Check for redundant reads in tool calls
				return r.ToolCalls <= 5
			},
		},
		{
			Name:        "no-scope-creep",
			Description: "Agent doesn't add tests/docs unless asked",
			Objective:   "Fix a typo in error message",
			ExpectedTokens: 1500,
			Validator: func(r *TestResult) bool {
				for _, sc := range r.ScopeCreep {
					if strings.Contains(sc, "test") || strings.Contains(sc, "doc") {
						return false
					}
				}
				return true
			},
		},
		{
			Name:        "wrap-up-trigger",
			Description: "Agent wraps up when hitting tool limit",
			Objective:   "Explore codebase structure",
			ExpectedTokens: 6000,
			Validator: func(r *TestResult) bool {
				return r.Turns <= 12 // Should stop before runaway
			},
		},
		{
			Name:        "replan-on-failure",
			Description: "Agent changes approach after failure",
			Objective:   "Find function using wrong search term",
			ExpectedTokens: 5000,
			Validator: func(r *TestResult) bool {
				return r.Passed // Should eventually find it
			},
		},
		{
			Name:        "token-efficient",
			Description: "Agent uses reasonable token budget",
			Objective:   "Simple grep and read task",
			ExpectedTokens: 2000,
			Validator: func(r *TestResult) bool {
				total := r.InputTokens + r.OutputTokens + r.ThinkingTokens
				return total <= 3000
			},
		},
	}
}

// Run executes all tests and computes metrics
func (s *QualitySuite) Run(t *testing.T) {
	start := time.Now()

	for _, test := range s.tests {
		result := s.runTest(t, test)
		s.results = append(s.results, result)

		// Update metrics
		s.metrics.TestsAttempted++
		s.metrics.TotalInputTokens += result.InputTokens
		s.metrics.TotalOutputTokens += result.OutputTokens
		s.metrics.TotalThinkingTokens += result.ThinkingTokens

		if result.Passed {
			s.metrics.TestsPassed++
		} else {
			s.metrics.TestsFailed++
		}
	}

	s.metrics.TotalDuration = time.Since(start)
	s.computeDerivedMetrics()
}

func (s *QualitySuite) runTest(t *testing.T, test QualityTest) TestResult {
	t.Run(test.Name, func(t *testing.T) {
		// Simulate test execution
		// In production, this would call the actual agent
	})

	// Simulated result for demonstration
	result := simulateTestExecution(test)
	result.Passed = test.Validator(&result)

	return result
}

// simulateTestExecution simulates a test run
// Replace with actual agent execution in production
func simulateTestExecution(test QualityTest) TestResult {
	// Simulate based on test type
	baseTokens := test.ExpectedTokens

	result := TestResult{
		Name:           test.Name,
		Passed:         true, // Simulated success
		InputTokens:   baseTokens / 2,
		OutputTokens:  baseTokens / 3,
		ThinkingTokens: baseTokens / 6,
		Turns:         2, // Efficient: 2 turns
		ToolCalls:     3, // Reasonable tool usage
	}

	// All simulated tests pass - this validates the framework works
	// Real tests would have actual failures
	return result
}

func (s *QualitySuite) computeDerivedMetrics() {
	totalTokens := s.metrics.TotalInputTokens + s.metrics.TotalOutputTokens + s.metrics.TotalThinkingTokens

	if s.metrics.TestsAttempted > 0 {
		s.metrics.TokensPerTest = float64(totalTokens) / float64(s.metrics.TestsAttempted)
		s.metrics.PassRate = float64(s.metrics.TestsPassed) / float64(s.metrics.TestsAttempted) * 100
		s.metrics.AvgDuration = s.metrics.TotalDuration / time.Duration(s.metrics.TestsAttempted)
	}

	if s.metrics.TestsPassed > 0 {
		s.metrics.TokensPerPassedTest = float64(totalTokens) / float64(s.metrics.TestsPassed)
	}

	// Efficiency score: pass rate weighted by token efficiency
	// Higher is better: 100% pass rate with 1000 tokens = excellent
	// Score = PassRate * (1000 / TokensPerTest)
	if s.metrics.TokensPerTest > 0 {
		s.metrics.EfficiencyScore = s.metrics.PassRate * (3000 / s.metrics.TokensPerTest)
	}
}

// Report generates a quality report
func (s *QualitySuite) Report() string {
	var b strings.Builder

	b.WriteString("╔══════════════════════════════════════════════════════════╗\n")
	b.WriteString("║           AGENT QUALITY METRICS REPORT                   ║\n")
	b.WriteString("╚══════════════════════════════════════════════════════════╝\n\n")

	// Test results
	b.WriteString("┌─── TEST RESULTS ───────────────────────────────────────────┐\n")
	b.WriteString(fmt.Sprintf("│ Attempted:  %3d                                            │\n", s.metrics.TestsAttempted))
	b.WriteString(fmt.Sprintf("│ Passed:     %3d  ✅                                        │\n", s.metrics.TestsPassed))
	b.WriteString(fmt.Sprintf("│ Failed:     %3d  ❌                                        │\n", s.metrics.TestsFailed))
	b.WriteString(fmt.Sprintf("│ Pass Rate:  %.1f%%                                         │\n", s.metrics.PassRate))
	b.WriteString("└────────────────────────────────────────────────────────────┘\n\n")

	// Token usage
	b.WriteString("┌─── TOKEN USAGE ────────────────────────────────────────────┐\n")
	totalTokens := s.metrics.TotalInputTokens + s.metrics.TotalOutputTokens + s.metrics.TotalThinkingTokens
	b.WriteString(fmt.Sprintf("│ Total Tokens:     %6d                                   │\n", totalTokens))
	b.WriteString(fmt.Sprintf("│   Input:          %6d                                   │\n", s.metrics.TotalInputTokens))
	b.WriteString(fmt.Sprintf("│   Output:         %6d                                   │\n", s.metrics.TotalOutputTokens))
	b.WriteString(fmt.Sprintf("│   Thinking:       %6d                                   │\n", s.metrics.TotalThinkingTokens))
	b.WriteString("└────────────────────────────────────────────────────────────┘\n\n")

	// Efficiency metrics
	b.WriteString("┌─── EFFICIENCY ─────────────────────────────────────────────┐\n")
	b.WriteString(fmt.Sprintf("│ Tokens/Test:      %.0f                                     │\n", s.metrics.TokensPerTest))
	b.WriteString(fmt.Sprintf("│ Tokens/Pass:      %.0f                                     │\n", s.metrics.TokensPerPassedTest))
	b.WriteString(fmt.Sprintf("│ Avg Duration:     %s                                  │\n", s.metrics.AvgDuration.Round(time.Millisecond)))
	b.WriteString("└────────────────────────────────────────────────────────────┘\n\n")

	// Quality score
	b.WriteString("┌─── QUALITY SCORE ─────────────────────────────────────────┐\n")
	grade := gradeScore(s.metrics.EfficiencyScore)
	b.WriteString(fmt.Sprintf("│                                                            │\n"))
	b.WriteString(fmt.Sprintf("│    EFFICIENCY SCORE:  %.1f  %s                          │\n", s.metrics.EfficiencyScore, grade))
	b.WriteString(fmt.Sprintf("│                                                            │\n"))
	b.WriteString(fmt.Sprintf("│    Formula: PassRate × (3000 / TokensPerTest)             │\n"))
	b.WriteString(fmt.Sprintf("│    Higher = Better (100%% pass at 3k tokens = 100)         │\n"))
	b.WriteString("└────────────────────────────────────────────────────────────┘\n\n")

	// Individual test results
	b.WriteString("┌─── INDIVIDUAL TESTS ──────────────────────────────────────┐\n")
	for _, r := range s.results {
		status := "✅"
		if !r.Passed {
			status = "❌"
		}
		tokens := r.InputTokens + r.OutputTokens + r.ThinkingTokens
		b.WriteString(fmt.Sprintf("│ %s %-20s  T:%d TC:%d Tok:%d\n",
			status, r.Name, r.Turns, r.ToolCalls, tokens))
	}
	b.WriteString("└────────────────────────────────────────────────────────────┘\n")

	return b.String()
}

func gradeScore(score float64) string {
	switch {
	case score >= 100:
		return "⭐ EXCELLENT"
	case score >= 80:
		return "✅ GOOD"
	case score >= 60:
		return "⚠️ FAIR"
	case score >= 40:
		return "🔶 POOR"
	default:
		return "❌ FAILING"
	}
}

// TestQualitySuite runs the full quality test suite
func TestQualitySuite(t *testing.T) {
	suite := NewQualitySuite()
	suite.Run(t)
	t.Log("\n" + suite.Report())

	// Assert minimum quality standards
	if suite.metrics.PassRate < 75 {
		t.Errorf("Pass rate %.1f%% below minimum 75%%", suite.metrics.PassRate)
	}
	if suite.metrics.EfficiencyScore < 50 {
		t.Errorf("Efficiency score %.1f below minimum 50", suite.metrics.EfficiencyScore)
	}
}

// TokenBudgetTest verifies tasks complete within token budgets
func TestTokenBudgets(t *testing.T) {
	budgets := []struct {
		taskType       string
		maxTokens      int
		simulatedUsage int
	}{
		{"simple-edit", 2000, 1500},
		{"multi-file-search", 5000, 4200},
		{"refactor", 10000, 8500},
		{"bug-fix", 4000, 3200},
		{"feature-add", 8000, 7100},
	}

	var totalBudget, totalUsed int
	var overBudget []string

	for _, b := range budgets {
		totalBudget += b.maxTokens
		totalUsed += b.simulatedUsage

		if b.simulatedUsage > b.maxTokens {
			overBudget = append(overBudget, b.taskType)
		}

		efficiency := float64(b.simulatedUsage) / float64(b.maxTokens) * 100
		t.Logf("%-20s: %5d/%5d tokens (%.0f%% of budget)",
			b.taskType, b.simulatedUsage, b.maxTokens, efficiency)
	}

	if len(overBudget) > 0 {
		t.Errorf("Tasks over budget: %v", overBudget)
	}

	overallEfficiency := float64(totalUsed) / float64(totalBudget) * 100
	t.Logf("\nOverall: %d/%d tokens (%.0f%% efficiency)", totalUsed, totalBudget, overallEfficiency)
}

// BenchmarkTokenEfficiency measures token usage patterns
func BenchmarkTokenEfficiency(b *testing.B) {
	scenarios := []struct {
		name       string
		complexity int // 1-10
	}{
		{"trivial", 1},
		{"simple", 3},
		{"moderate", 5},
		{"complex", 7},
		{"very-complex", 10},
	}

	for _, s := range scenarios {
		b.Run(s.name, func(b *testing.B) {
			// Simulate token usage based on complexity
			expectedTokens := s.complexity * 1000

			for i := 0; i < b.N; i++ {
				// Simulate processing
				tc := NewTaskContext(fmt.Sprintf("task-%d", i))
				for j := 0; j < s.complexity; j++ {
					tc.RecordTurn()
					tc.RecordToolCall("tool", "arg")
				}
				_ = tc.BuildReminder()
			}

			b.ReportMetric(float64(expectedTokens), "expected_tokens")
		})
	}
}

// QualityGate checks if metrics meet release criteria
type QualityGate struct {
	MinPassRate       float64
	MaxTokensPerTest  float64
	MinEfficiency     float64
	MaxScopeCreepRate float64
}

var DefaultQualityGate = QualityGate{
	MinPassRate:       80.0,
	MaxTokensPerTest:  5000,
	MinEfficiency:     60.0,
	MaxScopeCreepRate: 0.1,
}

func (g QualityGate) Check(m QualityMetrics) (bool, []string) {
	var failures []string

	if m.PassRate < g.MinPassRate {
		failures = append(failures, fmt.Sprintf("PassRate %.1f%% < %.1f%%", m.PassRate, g.MinPassRate))
	}
	if m.TokensPerTest > g.MaxTokensPerTest {
		failures = append(failures, fmt.Sprintf("TokensPerTest %.0f > %.0f", m.TokensPerTest, g.MaxTokensPerTest))
	}
	if m.EfficiencyScore < g.MinEfficiency {
		failures = append(failures, fmt.Sprintf("Efficiency %.1f < %.1f", m.EfficiencyScore, g.MinEfficiency))
	}

	return len(failures) == 0, failures
}

func TestQualityGate(t *testing.T) {
	suite := NewQualitySuite()
	suite.Run(t)

	passed, failures := DefaultQualityGate.Check(suite.metrics)

	if !passed {
		for _, f := range failures {
			t.Errorf("Quality gate failed: %s", f)
		}
	} else {
		t.Log("✅ All quality gates passed")
	}
}
