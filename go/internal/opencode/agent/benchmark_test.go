package agent

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// BenchmarkChallenge defines a test scenario
type BenchmarkChallenge struct {
	Name        string
	Description string
	Objective   string
	Steps       []string // Expected steps to complete
	Traps       []string // Common mistakes to avoid
	InScope     []string // Files that should be touched
	OutScope    []string // Files that should NOT be touched
	MaxTurns    int
	MaxTools    int
}

// IterativeRefactorChallenge tests multi-step refactoring
var IterativeRefactorChallenge = BenchmarkChallenge{
	Name:        "iterative-refactor",
	Description: "Multi-file refactoring with interdependencies",
	Objective:   "Rename function 'getData' to 'fetchData' across the codebase without breaking imports",
	Steps: []string{
		"1. Find all files containing 'getData'",
		"2. Identify which are declarations vs calls",
		"3. Rename the function in declaration file",
		"4. Update all call sites",
		"5. Verify no broken references",
	},
	Traps: []string{
		"Creating backup files",
		"Adding documentation",
		"Refactoring unrelated code",
		"Running tests not asked for",
	},
	InScope:  []string{"data.go", "service.go", "handler.go"},
	OutScope: []string{"README.md", "main_test.go", "config.go"},
	MaxTurns: 8,
	MaxTools: 15,
}

// BugFixChallenge tests error diagnosis and surgical fix
var BugFixChallenge = BenchmarkChallenge{
	Name:        "surgical-bugfix",
	Description: "Find and fix a specific bug without side effects",
	Objective:   "Fix the nil pointer panic in processUser() when user.Email is empty",
	Steps: []string{
		"1. Locate processUser function",
		"2. Identify the nil check issue",
		"3. Add minimal fix",
		"4. Verify fix doesn't break other cases",
	},
	Traps: []string{
		"Adding excessive error handling everywhere",
		"Refactoring the whole function",
		"Adding logging",
		"Creating new helper functions",
	},
	InScope:  []string{"user.go"},
	OutScope: []string{"user_test.go", "models.go", "README.md"},
	MaxTurns: 5,
	MaxTools: 8,
}

// FeatureAddChallenge tests focused feature addition
var FeatureAddChallenge = BenchmarkChallenge{
	Name:        "focused-feature",
	Description: "Add a specific feature without scope creep",
	Objective:   "Add a 'LastLogin' timestamp field to the User struct",
	Steps: []string{
		"1. Find User struct definition",
		"2. Add LastLogin field",
		"3. Update any initialization code",
	},
	Traps: []string{
		"Adding migration scripts",
		"Creating API endpoints",
		"Writing tests",
		"Adding documentation",
		"Implementing login tracking logic",
	},
	InScope:  []string{"user.go"},
	OutScope: []string{"api.go", "database.go", "README.md", "user_test.go"},
	MaxTurns: 4,
	MaxTools: 6,
}

// TestTaskContextEfficiency validates the TaskContext system
func TestTaskContextEfficiency(t *testing.T) {
	challenges := []BenchmarkChallenge{
		IterativeRefactorChallenge,
		BugFixChallenge,
		FeatureAddChallenge,
	}

	for _, challenge := range challenges {
		t.Run(challenge.Name, func(t *testing.T) {
			metrics := simulateChallenge(t, challenge)

			// Validate metrics meet thresholds
			if metrics.CurrentTurn > challenge.MaxTurns {
				t.Errorf("Too many turns: %d > %d", metrics.CurrentTurn, challenge.MaxTurns)
			}
			if metrics.ToolCalls > challenge.MaxTools {
				t.Errorf("Too many tool calls: %d > %d", metrics.ToolCalls, challenge.MaxTools)
			}
			if len(metrics.ScopeCreep) > 0 {
				t.Errorf("Scope creep detected: %v", metrics.ScopeCreep)
			}
			if len(metrics.OutOfScope) > 0 {
				t.Errorf("Out of scope files modified: %v", metrics.OutOfScope)
			}

			// Log report
			t.Log(metrics.Report())
		})
	}
}

// simulateChallenge runs a simulated challenge and returns metrics
func simulateChallenge(t *testing.T, challenge BenchmarkChallenge) *TaskMetrics {
	metrics := NewTaskMetrics(challenge.Name, challenge.Objective)

	// Create task context
	tc := NewTaskContext(challenge.Objective)

	// Simulate turns
	// In a real test, this would call the actual agent
	// For now, we simulate expected behavior

	// Turn 1: Understanding
	tc.SetPhase(PhaseUnderstanding)
	tc.RecordTurn()
	tc.RecordToolCall("grep", "getData")
	tc.RecordToolCall("glob", "*.go")
	metrics.RecordTurn(1000, 500, 2, "understanding", "searching codebase", true)
	metrics.RecordTool("grep", false)
	metrics.RecordTool("glob", false)

	// Turn 2: Planning
	tc.SetPhase(PhasePlanning)
	tc.RecordTurn()
	metrics.RecordTurn(1200, 600, 0, "planning", "analyzing dependencies", true)

	// Turn 3-5: Executing
	tc.SetPhase(PhaseExecuting)
	for i, file := range challenge.InScope {
		tc.RecordTurn()
		tc.RecordToolCall("read", file)
		tc.RecordToolCall("edit", file)
		metrics.RecordTurn(1500, 800, 2, "executing", fmt.Sprintf("modifying %s", file), true)
		metrics.RecordTool("read", false)
		metrics.RecordTool("edit", false)
		metrics.RecordFileModified(file, true)

		if i >= 2 { // Limit turns
			break
		}
	}

	// Turn 6: Verifying
	tc.SetPhase(PhaseVerifying)
	tc.RecordTurn()
	tc.RecordToolCall("grep", "getData") // Verify no remaining references
	metrics.RecordTurn(800, 400, 1, "verifying", "checking references", true)
	metrics.RecordTool("grep", false)

	// Complete
	tc.SetPhase(PhaseComplete)
	metrics.Finish(true)

	return metrics
}

// TestTaskContextReminder validates the reminder generation
func TestTaskContextReminder(t *testing.T) {
	tc := NewTaskContext("Fix the authentication bug in login.go")

	// Simulate some activity
	tc.RecordTurn()
	tc.RecordToolCall("read", "login.go")
	tc.RecordToolCall("grep", "auth")
	tc.SetPhase(PhaseExecuting)
	tc.RecordTurn()
	tc.RecordToolCall("edit", "login.go")

	reminder := tc.BuildReminder()

	// Verify reminder contains key elements
	if !strings.Contains(reminder, "OBJECTIVE:") {
		t.Error("Reminder missing OBJECTIVE")
	}
	if !strings.Contains(reminder, "PHASE: executing") {
		t.Error("Reminder missing PHASE")
	}
	if !strings.Contains(reminder, "TURNS:") {
		t.Error("Reminder missing TURNS")
	}
	if !strings.Contains(reminder, "RULES:") {
		t.Error("Reminder missing RULES")
	}

	t.Logf("Generated reminder:\n%s", reminder)
}

// TestFocusWarnings validates focus warnings trigger correctly
func TestFocusWarnings(t *testing.T) {
	tc := NewTaskContext("Simple task")

	// Simulate 6 turns (should trigger focus warning)
	for i := 0; i < 6; i++ {
		tc.RecordTurn()
		tc.RecordToolCall("read", fmt.Sprintf("file%d.go", i))
	}

	reminder := tc.BuildReminder()

	if !strings.Contains(reminder, "FOCUS:") {
		t.Error("Should have focus warning after 5+ turns")
	}

	// Simulate 16 tool calls (should trigger wrap up warning)
	for i := 0; i < 10; i++ {
		tc.RecordToolCall("grep", fmt.Sprintf("pattern%d", i))
	}

	reminder = tc.BuildReminder()

	if !strings.Contains(reminder, "WRAP UP:") {
		t.Error("Should have wrap up warning after 15+ tools")
	}

	t.Logf("Warnings triggered:\n%s", reminder)
}

// TestErrorRecovery validates error tracking and replan
func TestErrorRecovery(t *testing.T) {
	tc := NewTaskContext("Task with errors")
	tc.SetConfidence(1.0)

	// Record an error
	tc.RecordError("file not found: config.yaml")

	// Verify state changed
	if !tc.NeedsReplan {
		t.Error("Should need replan after error")
	}
	if tc.Confidence >= 1.0 {
		t.Error("Confidence should decrease after error")
	}

	reminder := tc.BuildReminder()
	if !strings.Contains(reminder, "REPLAN:") {
		t.Error("Should have replan warning after error")
	}

	t.Logf("After error:\n%s", reminder)
}

// BenchmarkTaskContextOverhead measures the overhead of TaskContext
func BenchmarkTaskContextOverhead(b *testing.B) {
	b.Run("CreateContext", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = NewTaskContext("benchmark objective")
		}
	})

	b.Run("RecordTurn", func(b *testing.B) {
		tc := NewTaskContext("benchmark")
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			tc.RecordTurn()
		}
	})

	b.Run("RecordToolCall", func(b *testing.B) {
		tc := NewTaskContext("benchmark")
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			tc.RecordToolCall("read", "/path/to/file.go")
		}
	})

	b.Run("BuildReminder", func(b *testing.B) {
		tc := NewTaskContext("benchmark objective that is moderately long")
		tc.RecordTurn()
		tc.RecordToolCall("read", "file1.go")
		tc.RecordToolCall("edit", "file2.go")
		tc.SetPhase(PhaseExecuting)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = tc.BuildReminder()
		}
	})
}

// MockAgent simulates agent behavior for benchmark testing
type MockAgent struct {
	taskContext *TaskContext
	turns       int
	tools       int
}

func (m *MockAgent) SimulateTask(ctx context.Context, objective string, steps int) *TaskMetrics {
	metrics := NewTaskMetrics("mock-task", objective)
	m.taskContext = NewTaskContext(objective)

	phases := []TaskPhase{PhaseUnderstanding, PhasePlanning, PhaseExecuting, PhaseVerifying}

	for i := 0; i < steps; i++ {
		phase := phases[i%len(phases)]
		m.taskContext.SetPhase(phase)
		m.taskContext.RecordTurn()
		m.turns++

		// Simulate tool calls based on phase
		toolCount := 1
		if phase == PhaseUnderstanding {
			toolCount = 2
		} else if phase == PhaseExecuting {
			toolCount = 3
		}

		for j := 0; j < toolCount; j++ {
			m.taskContext.RecordToolCall("tool", fmt.Sprintf("arg%d", j))
			m.tools++
			metrics.RecordTool("tool", false)
		}

		metrics.RecordTurn(1000, 500, toolCount, phase.String(), fmt.Sprintf("step %d", i), true)

		// Check context reminder for focus
		reminder := m.taskContext.BuildReminder()
		if strings.Contains(reminder, "WRAP UP:") {
			// Agent should wrap up
			break
		}
	}

	m.taskContext.SetPhase(PhaseComplete)
	metrics.Finish(true)
	return metrics
}

func TestMockAgentWithTaskContext(t *testing.T) {
	mock := &MockAgent{}
	ctx := context.Background()

	// Run a 20-step task - should be stopped by wrap-up warning
	metrics := mock.SimulateTask(ctx, "Complex multi-step refactoring", 20)

	t.Logf("Turns executed: %d", mock.turns)
	t.Logf("Tools used: %d", mock.tools)
	t.Log(metrics.Report())

	// Verify wrap-up triggered before all 20 steps
	if mock.turns >= 20 {
		t.Error("TaskContext should have triggered wrap-up before 20 turns")
	}
}

// Integration test placeholder - requires actual LLM
func TestLiveAgentEfficiency(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping live agent test in short mode")
	}

	// This would be the real integration test
	// Requires: running urp-memgraph, ANTHROPIC_API_KEY

	t.Log("Live agent efficiency test would run here")
	t.Log("Use: go test -v -run TestLiveAgentEfficiency ./internal/opencode/agent/")
}
