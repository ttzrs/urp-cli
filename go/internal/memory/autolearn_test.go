package memory

import (
	"testing"
)

func TestDefaultAutoLearnConfig(t *testing.T) {
	cfg := DefaultAutoLearnConfig()

	if cfg.MinConfidenceToStore != 0.5 {
		t.Errorf("Expected MinConfidenceToStore=0.5, got %f", cfg.MinConfidenceToStore)
	}
	if cfg.MinSuccessToPromote != 3 {
		t.Errorf("Expected MinSuccessToPromote=3, got %d", cfg.MinSuccessToPromote)
	}
}

func TestNewAutoLearner(t *testing.T) {
	learner := NewAutoLearner(nil, nil, AutoLearnConfig{})

	if learner.config.MinConfidenceToStore != 0.5 {
		t.Error("Expected default MinConfidenceToStore")
	}
	if learner.config.MinSuccessToPromote != 3 {
		t.Error("Expected default MinSuccessToPromote")
	}
}

func TestRecordSuccess(t *testing.T) {
	learner := NewAutoLearner(nil, nil, DefaultAutoLearnConfig())

	learner.RecordSuccess("Fixed null pointer", "Added nil check", "session1", map[string]string{
		"file": "main.go",
	})

	events := learner.GetPendingEvents(0)
	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}

	if events[0].EventType != "success" {
		t.Errorf("Expected type 'success', got %s", events[0].EventType)
	}
	if events[0].Solution != "Added nil check" {
		t.Errorf("Wrong solution")
	}
	if events[0].Confidence != 0.7 {
		t.Errorf("Expected confidence 0.7, got %f", events[0].Confidence)
	}
}

func TestRecordFailure(t *testing.T) {
	learner := NewAutoLearner(nil, nil, DefaultAutoLearnConfig())

	learner.RecordFailure("Build error", "Wrong import", "session1", nil)

	events := learner.GetPendingEvents(0)
	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}

	if events[0].EventType != "failure" {
		t.Errorf("Expected type 'failure', got %s", events[0].EventType)
	}
	if events[0].Confidence != 0.3 {
		t.Errorf("Expected confidence 0.3, got %f", events[0].Confidence)
	}
}

func TestRecordFix(t *testing.T) {
	learner := NewAutoLearner(nil, nil, DefaultAutoLearnConfig())

	learner.RecordFix("Import cycle", "Moved to internal package", "session1", nil)

	events := learner.GetPendingEvents(0)
	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}

	if events[0].EventType != "fix" {
		t.Errorf("Expected type 'fix', got %s", events[0].EventType)
	}
	if events[0].Confidence != 0.8 {
		t.Errorf("Expected confidence 0.8, got %f", events[0].Confidence)
	}
}

func TestRecordPattern(t *testing.T) {
	learner := NewAutoLearner(nil, nil, DefaultAutoLearnConfig())

	learner.RecordPattern("singleton", "Database connection singleton pattern", "session1", nil)

	events := learner.GetPendingEvents(0)
	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}

	if events[0].EventType != "pattern" {
		t.Errorf("Expected type 'pattern', got %s", events[0].EventType)
	}
	if events[0].Context["pattern_name"] != "singleton" {
		t.Errorf("Expected pattern_name 'singleton'")
	}
}

func TestEventMerging(t *testing.T) {
	learner := NewAutoLearner(nil, nil, DefaultAutoLearnConfig())

	// Same description should merge
	learner.RecordSuccess("Fixed null pointer bug", "Added nil check", "session1", nil)
	learner.RecordSuccess("Fixed null pointer bug", "Added nil check", "session2", nil)

	events := learner.GetPendingEvents(0)
	if len(events) != 1 {
		t.Fatalf("Expected events to merge, got %d", len(events))
	}

	if events[0].SuccessCount != 2 {
		t.Errorf("Expected SuccessCount=2, got %d", events[0].SuccessCount)
	}
}

func TestConfidenceFiltering(t *testing.T) {
	learner := NewAutoLearner(nil, nil, DefaultAutoLearnConfig())

	learner.RecordSuccess("High confidence", "solution", "s1", nil) // 0.7
	learner.RecordFailure("Low confidence", "bad", "s2", nil)       // 0.3

	highConf := learner.GetPendingEvents(0.5)
	if len(highConf) != 1 {
		t.Errorf("Expected 1 high confidence event, got %d", len(highConf))
	}
	if highConf[0].Description != "High confidence" {
		t.Error("Wrong event filtered")
	}
}

func TestConfirmSuccess(t *testing.T) {
	learner := NewAutoLearner(nil, nil, DefaultAutoLearnConfig())

	learner.RecordSuccess("My fix", "solution", "s1", nil)
	learner.ConfirmSuccess("My fix")
	learner.ConfirmSuccess("My fix")

	events := learner.GetPendingEvents(0)
	if events[0].SuccessCount != 3 {
		t.Errorf("Expected SuccessCount=3, got %d", events[0].SuccessCount)
	}

	// Confidence should increase
	if events[0].Confidence <= 0.7 {
		t.Errorf("Expected confidence to increase, got %f", events[0].Confidence)
	}
}

func TestConfirmFailure(t *testing.T) {
	learner := NewAutoLearner(nil, nil, DefaultAutoLearnConfig())

	learner.RecordSuccess("My fix", "solution", "s1", nil)
	learner.ConfirmFailure("My fix")

	events := learner.GetPendingEvents(0)
	if events[0].FailureCount != 1 {
		t.Errorf("Expected FailureCount=1, got %d", events[0].FailureCount)
	}

	// Confidence should decrease
	if events[0].Confidence >= 0.7 {
		t.Errorf("Expected confidence to decrease, got %f", events[0].Confidence)
	}
}

func TestGetPromotionCandidates(t *testing.T) {
	learner := NewAutoLearner(nil, nil, DefaultAutoLearnConfig())

	// Not enough successes
	learner.RecordFix("Fix 1", "sol1", "s1", nil)

	candidates := learner.GetPromotionCandidates()
	if len(candidates) != 0 {
		t.Errorf("Expected no candidates with only 1 success")
	}

	// Add more confirmations
	learner.ConfirmSuccess("Fix 1")
	learner.ConfirmSuccess("Fix 1") // Now 3 successes

	candidates = learner.GetPromotionCandidates()
	if len(candidates) != 1 {
		t.Errorf("Expected 1 candidate with 3 successes, got %d", len(candidates))
	}
}

func TestPromoteEvent(t *testing.T) {
	learner := NewAutoLearner(nil, nil, DefaultAutoLearnConfig())

	learner.RecordFix("My fix", "solution", "s1", nil)

	_, err := learner.PromoteEvent("My fix")
	if err != nil {
		t.Errorf("Promote failed: %v", err)
	}

	// Should be marked as promoted
	learner.mu.RLock()
	for _, e := range learner.pendingLearning {
		if e.Description == "My fix" {
			if !e.WasPromoted {
				t.Error("Event should be marked as promoted")
			}
			if e.PromotedAt == nil {
				t.Error("PromotedAt should be set")
			}
		}
	}
	learner.mu.RUnlock()

	// Promoted events should not appear in pending
	events := learner.GetPendingEvents(0)
	if len(events) != 0 {
		t.Errorf("Promoted event should not be in pending")
	}
}

func TestSuggestRelated(t *testing.T) {
	learner := NewAutoLearner(nil, nil, DefaultAutoLearnConfig())

	learner.RecordFix("Error in main.go", "Fix 1", "s1", map[string]string{
		"file":       "main.go",
		"error_type": "nil_pointer",
	})
	learner.RecordFix("Error in util.go", "Fix 2", "s1", map[string]string{
		"file":       "util.go",
		"error_type": "nil_pointer",
	})
	learner.RecordFix("Other error", "Fix 3", "s1", map[string]string{
		"file":       "other.go",
		"error_type": "type_mismatch",
	})

	// Suggest by file
	suggestions := learner.SuggestRelated("main.go", "")
	if len(suggestions) != 1 {
		t.Errorf("Expected 1 suggestion for main.go, got %d", len(suggestions))
	}

	// Suggest by error type
	suggestions = learner.SuggestRelated("", "nil_pointer")
	if len(suggestions) != 2 {
		t.Errorf("Expected 2 suggestions for nil_pointer, got %d", len(suggestions))
	}
}

func TestAutoLearnerStats(t *testing.T) {
	learner := NewAutoLearner(nil, nil, DefaultAutoLearnConfig())

	learner.RecordSuccess("s1", "sol", "s1", nil)
	learner.RecordSuccess("s2", "sol", "s1", nil)
	learner.RecordFailure("f1", "bad", "s1", nil)

	stats := learner.Stats()

	if stats["total_events"].(int) != 3 {
		t.Errorf("Expected 3 total events, got %v", stats["total_events"])
	}
	if stats["pending_events"].(int) != 3 {
		t.Errorf("Expected 3 pending events, got %v", stats["pending_events"])
	}
	if stats["total_success"].(int) != 2 {
		t.Errorf("Expected 2 total success, got %v", stats["total_success"])
	}
	if stats["total_failure"].(int) != 1 {
		t.Errorf("Expected 1 total failure, got %v", stats["total_failure"])
	}
}

func TestClear(t *testing.T) {
	learner := NewAutoLearner(nil, nil, DefaultAutoLearnConfig())

	learner.RecordSuccess("s1", "sol", "s1", nil)
	learner.RecordSuccess("s2", "sol", "s1", nil)

	learner.Clear()

	events := learner.GetPendingEvents(0)
	if len(events) != 0 {
		t.Error("Expected no events after clear")
	}
}

func TestMaxPendingEvents(t *testing.T) {
	cfg := DefaultAutoLearnConfig()
	cfg.MaxPendingEvents = 3
	learner := NewAutoLearner(nil, nil, cfg)

	learner.RecordSuccess("event1", "sol", "s1", nil)
	learner.RecordSuccess("event2", "sol", "s1", nil)
	learner.RecordSuccess("event3", "sol", "s1", nil)
	learner.RecordSuccess("event4", "sol", "s1", nil) // Should evict event1

	events := learner.GetPendingEvents(0)
	if len(events) != 3 {
		t.Errorf("Expected 3 events (max), got %d", len(events))
	}

	// event1 should be evicted
	for _, e := range events {
		if e.Description == "event1" {
			t.Error("event1 should have been evicted")
		}
	}
}

func BenchmarkRecordSuccess(b *testing.B) {
	learner := NewAutoLearner(nil, nil, DefaultAutoLearnConfig())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		learner.RecordSuccess("benchmark event", "solution", "session", nil)
	}
}

func BenchmarkGetPendingEvents(b *testing.B) {
	learner := NewAutoLearner(nil, nil, DefaultAutoLearnConfig())

	for i := 0; i < 100; i++ {
		learner.RecordSuccess("event"+string(rune(i)), "sol", "s1", nil)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		learner.GetPendingEvents(0.5)
	}
}
