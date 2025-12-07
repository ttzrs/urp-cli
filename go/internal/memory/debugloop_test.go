package memory

import (
	"testing"
)

func TestDefaultDebugLoopConfig(t *testing.T) {
	cfg := DefaultDebugLoopConfig()

	if cfg.MaxIterations != 10 {
		t.Errorf("Expected MaxIterations=10, got %d", cfg.MaxIterations)
	}
	if cfg.MinConfidence != 0.3 {
		t.Errorf("Expected MinConfidence=0.3, got %f", cfg.MinConfidence)
	}
	if !cfg.AutoPromoteSuccess {
		t.Error("Expected AutoPromoteSuccess=true")
	}
}

func TestNewDebugLoop(t *testing.T) {
	dl := NewDebugLoop(nil, nil, DebugLoopConfig{})

	if dl.config.MaxIterations != 10 {
		t.Error("Expected default MaxIterations")
	}
}

func TestStartSession(t *testing.T) {
	dl := NewDebugLoop(nil, nil, DefaultDebugLoopConfig())

	session := dl.StartSession("Test error", "ERR001", "parent-session")

	if session.ID == "" {
		t.Error("Expected session ID")
	}
	if session.Problem != "Test error" {
		t.Error("Wrong problem")
	}
	if session.State != StatePropose {
		t.Errorf("Expected StatePropose, got %s", session.State)
	}
	if session.MaxIterations != 10 {
		t.Error("Wrong MaxIterations")
	}
}

func TestGetSession(t *testing.T) {
	dl := NewDebugLoop(nil, nil, DefaultDebugLoopConfig())

	session := dl.StartSession("Test", "ERR", "s1")
	retrieved := dl.GetSession(session.ID)

	if retrieved == nil {
		t.Fatal("Expected to retrieve session")
	}
	if retrieved.ID != session.ID {
		t.Error("Wrong session retrieved")
	}

	// Non-existent session
	if dl.GetSession("fake-id") != nil {
		t.Error("Expected nil for non-existent session")
	}
}

func TestPropose(t *testing.T) {
	dl := NewDebugLoop(nil, nil, DefaultDebugLoopConfig())

	session := dl.StartSession("Test", "ERR", "s1")

	hypotheses := []Hypothesis{
		{Description: "Try fix A", Confidence: 0.8},
		{Description: "Try fix B", Confidence: 0.5},
		{Description: "Try fix C", Confidence: 0.9, Priority: 10},
	}

	err := dl.Propose(session.ID, hypotheses)
	if err != nil {
		t.Errorf("Propose failed: %v", err)
	}

	retrieved := dl.GetSession(session.ID)
	if len(retrieved.Hypotheses) != 3 {
		t.Errorf("Expected 3 hypotheses, got %d", len(retrieved.Hypotheses))
	}

	// Check sorting - highest priority/confidence first
	if retrieved.Hypotheses[0].Description != "Try fix C" {
		t.Error("Expected highest priority hypothesis first")
	}

	if retrieved.State != StateTest {
		t.Errorf("Expected StateTest after propose, got %s", retrieved.State)
	}
}

func TestGetNextHypothesis(t *testing.T) {
	dl := NewDebugLoop(nil, nil, DefaultDebugLoopConfig())

	session := dl.StartSession("Test", "ERR", "s1")
	dl.Propose(session.ID, []Hypothesis{
		{Description: "Fix A", Confidence: 0.5},
		{Description: "Fix B", Confidence: 0.2}, // Below threshold
	})

	next := dl.GetNextHypothesis(session.ID)
	if next == nil {
		t.Fatal("Expected a hypothesis")
	}
	if next.Description != "Fix A" {
		t.Errorf("Expected Fix A, got %s", next.Description)
	}
}

func TestRecordTestResult(t *testing.T) {
	dl := NewDebugLoop(nil, nil, DefaultDebugLoopConfig())

	session := dl.StartSession("Test", "ERR", "s1")
	dl.Propose(session.ID, []Hypothesis{
		{Description: "Fix A", Confidence: 0.8},
	})

	hyp := dl.GetNextHypothesis(session.ID)

	result := TestResult{
		Passed:   true,
		Output:   "Tests passed",
		Duration: 150.0,
	}

	err := dl.RecordTestResult(session.ID, hyp.ID, result)
	if err != nil {
		t.Errorf("RecordTestResult failed: %v", err)
	}

	retrieved := dl.GetSession(session.ID)
	if retrieved.Hypotheses[0].TestResult == nil {
		t.Fatal("Expected test result to be recorded")
	}
	if !retrieved.Hypotheses[0].TestResult.Passed {
		t.Error("Expected test to pass")
	}
	if retrieved.State != StateAnalyze {
		t.Error("Expected StateAnalyze after successful test")
	}
}

func TestAnalyze(t *testing.T) {
	dl := NewDebugLoop(nil, nil, DefaultDebugLoopConfig())

	session := dl.StartSession("Test", "ERR", "s1")
	dl.Propose(session.ID, []Hypothesis{
		{Description: "Fix A", Confidence: 0.8},
	})

	hyp := dl.GetNextHypothesis(session.ID)
	dl.RecordTestResult(session.ID, hyp.ID, TestResult{
		Passed:   true,
		Insights: []string{"Memory leak fixed"},
	})

	insights := dl.Analyze(session.ID)

	if len(insights) == 0 {
		t.Error("Expected insights from analysis")
	}

	retrieved := dl.GetSession(session.ID)
	if retrieved.State != StateRefine {
		t.Errorf("Expected StateRefine after analyze, got %s", retrieved.State)
	}
}

func TestRefine_Success(t *testing.T) {
	dl := NewDebugLoop(nil, nil, DefaultDebugLoopConfig())

	session := dl.StartSession("Test", "ERR", "s1")
	dl.Propose(session.ID, []Hypothesis{
		{Description: "Fix A", Confidence: 0.8},
	})

	hyp := dl.GetNextHypothesis(session.ID)
	dl.RecordTestResult(session.ID, hyp.ID, TestResult{Passed: true})
	dl.Analyze(session.ID)

	resolved, err := dl.Refine(session.ID, nil)
	if err != nil {
		t.Errorf("Refine failed: %v", err)
	}
	if !resolved {
		t.Error("Expected resolved=true")
	}

	retrieved := dl.GetSession(session.ID)
	if retrieved.State != StateResolved {
		t.Error("Expected StateResolved")
	}
	if retrieved.Resolution != "Fix A" {
		t.Error("Wrong resolution")
	}
}

func TestRefine_Continue(t *testing.T) {
	dl := NewDebugLoop(nil, nil, DefaultDebugLoopConfig())

	session := dl.StartSession("Test", "ERR", "s1")
	dl.Propose(session.ID, []Hypothesis{
		{Description: "Fix A", Confidence: 0.8},
	})

	hyp := dl.GetNextHypothesis(session.ID)
	dl.RecordTestResult(session.ID, hyp.ID, TestResult{Passed: false})
	dl.Analyze(session.ID)

	resolved, _ := dl.Refine(session.ID, []Hypothesis{
		{Description: "New Fix B", Confidence: 0.9},
	})

	if resolved {
		t.Error("Should not be resolved")
	}

	retrieved := dl.GetSession(session.ID)
	if retrieved.State != StateTest {
		t.Error("Should return to StateTest")
	}
	if len(retrieved.Hypotheses) != 2 {
		t.Errorf("Expected 2 hypotheses after refinement, got %d", len(retrieved.Hypotheses))
	}
}

func TestRefine_MaxIterations(t *testing.T) {
	cfg := DefaultDebugLoopConfig()
	cfg.MaxIterations = 2
	dl := NewDebugLoop(nil, nil, cfg)

	session := dl.StartSession("Test", "ERR", "s1")

	// Simulate iterations
	dl.Propose(session.ID, []Hypothesis{{Description: "A", Confidence: 0.8}})
	h := dl.GetNextHypothesis(session.ID)
	dl.RecordTestResult(session.ID, h.ID, TestResult{Passed: false})
	dl.Analyze(session.ID)
	dl.Refine(session.ID, nil) // iter 1

	dl.Propose(session.ID, []Hypothesis{{Description: "B", Confidence: 0.8}})
	h = dl.GetNextHypothesis(session.ID)
	dl.RecordTestResult(session.ID, h.ID, TestResult{Passed: false})
	dl.Analyze(session.ID)
	resolved, _ := dl.Refine(session.ID, nil) // iter 2

	if resolved {
		t.Error("Should not be resolved")
	}

	retrieved := dl.GetSession(session.ID)
	if retrieved.State != StateFailed {
		t.Errorf("Expected StateFailed after max iterations, got %s", retrieved.State)
	}
}

func TestMarkResolved(t *testing.T) {
	dl := NewDebugLoop(nil, nil, DefaultDebugLoopConfig())

	session := dl.StartSession("Test", "ERR", "s1")
	dl.MarkResolved(session.ID, "Manual fix")

	retrieved := dl.GetSession(session.ID)
	if retrieved.State != StateResolved {
		t.Error("Expected StateResolved")
	}
	if retrieved.Resolution != "Manual fix" {
		t.Error("Wrong resolution")
	}
}

func TestMarkFailed(t *testing.T) {
	dl := NewDebugLoop(nil, nil, DefaultDebugLoopConfig())

	session := dl.StartSession("Test", "ERR", "s1")
	dl.MarkFailed(session.ID)

	retrieved := dl.GetSession(session.ID)
	if retrieved.State != StateFailed {
		t.Error("Expected StateFailed")
	}
}

func TestGetActiveSessions(t *testing.T) {
	dl := NewDebugLoop(nil, nil, DefaultDebugLoopConfig())

	s1 := dl.StartSession("Test1", "ERR1", "p1")
	dl.StartSession("Test2", "ERR2", "p2")
	dl.MarkResolved(s1.ID, "Fixed")

	active := dl.GetActiveSessions()
	if len(active) != 1 {
		t.Errorf("Expected 1 active session, got %d", len(active))
	}
}

func TestDebugLoopStats(t *testing.T) {
	dl := NewDebugLoop(nil, nil, DefaultDebugLoopConfig())

	dl.StartSession("Test1", "ERR1", "p1")
	s2 := dl.StartSession("Test2", "ERR2", "p2")
	s3 := dl.StartSession("Test3", "ERR3", "p3")

	dl.MarkResolved(s2.ID, "Fixed")
	dl.MarkFailed(s3.ID)

	stats := dl.Stats()

	if stats["total_sessions"].(int) != 3 {
		t.Errorf("Expected 3 total sessions, got %v", stats["total_sessions"])
	}
	if stats["resolved"].(int) != 1 {
		t.Errorf("Expected 1 resolved, got %v", stats["resolved"])
	}
	if stats["failed"].(int) != 1 {
		t.Errorf("Expected 1 failed, got %v", stats["failed"])
	}
	if stats["active"].(int) != 1 {
		t.Errorf("Expected 1 active, got %v", stats["active"])
	}
}

func TestIntegrationWithLearner(t *testing.T) {
	learner := NewAutoLearner(nil, nil, DefaultAutoLearnConfig())
	cfg := DefaultDebugLoopConfig()
	cfg.AutoPromoteSuccess = true
	dl := NewDebugLoop(nil, learner, cfg)

	session := dl.StartSession("Null pointer bug", "NPE", "s1")
	dl.Propose(session.ID, []Hypothesis{
		{Description: "Add nil check", Confidence: 0.9},
	})

	hyp := dl.GetNextHypothesis(session.ID)
	dl.RecordTestResult(session.ID, hyp.ID, TestResult{Passed: true})
	dl.Analyze(session.ID)
	dl.Refine(session.ID, nil)

	// Check learner received the fix
	events := learner.GetPendingEvents(0)
	if len(events) != 1 {
		t.Fatalf("Expected learner to receive 1 event, got %d", len(events))
	}
	if events[0].EventType != "fix" {
		t.Error("Expected fix event")
	}
}

func BenchmarkDebugLoop(b *testing.B) {
	dl := NewDebugLoop(nil, nil, DefaultDebugLoopConfig())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		session := dl.StartSession("Test", "ERR", "s1")
		dl.Propose(session.ID, []Hypothesis{
			{Description: "Fix", Confidence: 0.8},
		})
		h := dl.GetNextHypothesis(session.ID)
		dl.RecordTestResult(session.ID, h.ID, TestResult{Passed: true})
		dl.Analyze(session.ID)
		dl.Refine(session.ID, nil)
	}
}
