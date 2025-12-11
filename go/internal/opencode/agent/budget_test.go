package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewBudgetTracker(t *testing.T) {
	b := NewBudgetTracker()
	if b == nil {
		t.Fatal("NewBudgetTracker returned nil")
	}
	if b.DailyLimit != 5.0 {
		t.Errorf("DailyLimit = %f, want 5.0", b.DailyLimit)
	}
	if b.SessionLimit != 1.0 {
		t.Errorf("SessionLimit = %f, want 1.0", b.SessionLimit)
	}
	if b.MaxPerTask != 0.20 {
		t.Errorf("MaxPerTask = %f, want 0.20", b.MaxPerTask)
	}
}

func TestBudgetTracker_CanAffordTask(t *testing.T) {
	b := NewBudgetTracker()

	// Should afford small cost
	if !b.CanAffordTask(0.10) {
		t.Error("should afford $0.10")
	}

	// Should not afford more than session limit
	if b.CanAffordTask(2.0) {
		t.Error("should not afford $2.0 (over session limit)")
	}

	// Should not afford more than daily limit
	if b.CanAffordTask(6.0) {
		t.Error("should not afford $6.0 (over daily limit)")
	}

	// Should not afford more than max per task
	if b.CanAffordTask(0.25) {
		t.Error("should not afford $0.25 (over max per task)")
	}
}

func TestBudgetTracker_Record(t *testing.T) {
	b := NewBudgetTracker()

	b.Record(0.10)

	session, daily := b.GetUsage()
	if session != 0.10 {
		t.Errorf("session usage = %f, want 0.10", session)
	}
	if daily != 0.10 {
		t.Errorf("daily usage = %f, want 0.10", daily)
	}
}

func TestBudgetTracker_RecordDetailed(t *testing.T) {
	b := NewBudgetTracker()

	b.RecordDetailed("test-model", 0.15, 1000, "bugfix")

	// Check model costs
	costs := b.GetModelCosts()
	if costs["test-model"] == nil {
		t.Fatal("model costs not tracked")
	}
	if costs["test-model"].TotalCost != 0.15 {
		t.Errorf("TotalCost = %f, want 0.15", costs["test-model"].TotalCost)
	}
	if costs["test-model"].CallCount != 1 {
		t.Errorf("CallCount = %d, want 1", costs["test-model"].CallCount)
	}
	if costs["test-model"].TotalTokens != 1000 {
		t.Errorf("TotalTokens = %d, want 1000", costs["test-model"].TotalTokens)
	}

	// Check history
	history := b.GetHistory(10)
	if len(history) != 1 {
		t.Errorf("history length = %d, want 1", len(history))
	}
}

func TestBudgetTracker_ResetSession(t *testing.T) {
	b := NewBudgetTracker()
	b.Record(0.50)

	session, _ := b.GetUsage()
	if session != 0.50 {
		t.Fatal("expected session usage of 0.50")
	}

	b.ResetSession()
	session, daily := b.GetUsage()
	if session != 0 {
		t.Errorf("session after reset = %f, want 0", session)
	}
	if daily != 0.50 {
		t.Errorf("daily after session reset = %f, want 0.50", daily)
	}
}

func TestBudgetTracker_GetRemaining(t *testing.T) {
	b := NewBudgetTrackerWithLimits(10.0, 2.0, 0.5)
	b.Record(0.50)

	session, daily := b.GetRemaining()
	if session != 1.50 {
		t.Errorf("session remaining = %f, want 1.50", session)
	}
	if daily != 9.50 {
		t.Errorf("daily remaining = %f, want 9.50", daily)
	}
}

func TestBudgetTracker_GetUsagePercent(t *testing.T) {
	b := NewBudgetTrackerWithLimits(10.0, 1.0, 0.5)
	b.Record(0.50)

	sessionPct, dailyPct := b.GetUsagePercent()
	if sessionPct != 50.0 {
		t.Errorf("session percent = %f, want 50.0", sessionPct)
	}
	if dailyPct != 5.0 {
		t.Errorf("daily percent = %f, want 5.0", dailyPct)
	}
}

func TestBudgetTracker_IsOverBudget(t *testing.T) {
	b := NewBudgetTrackerWithLimits(1.0, 0.5, 0.3)

	if b.IsOverBudget() {
		t.Error("should not be over budget initially")
	}

	b.Record(0.60) // Over session limit
	if !b.IsOverBudget() {
		t.Error("should be over budget after $0.60")
	}
}

func TestBudgetTracker_SetLimits(t *testing.T) {
	b := NewBudgetTracker()
	b.SetLimits(20.0, 5.0)

	if b.DailyLimit != 20.0 {
		t.Errorf("DailyLimit = %f, want 20.0", b.DailyLimit)
	}
	if b.SessionLimit != 5.0 {
		t.Errorf("SessionLimit = %f, want 5.0", b.SessionLimit)
	}
}

func TestBudgetTracker_SetMaxPerTask(t *testing.T) {
	b := NewBudgetTracker()
	b.SetMaxPerTask(0.50)

	if b.MaxPerTask != 0.50 {
		t.Errorf("MaxPerTask = %f, want 0.50", b.MaxPerTask)
	}

	// Should now afford $0.40
	if !b.CanAffordTask(0.40) {
		t.Error("should afford $0.40 with new max per task")
	}
}

func TestBudgetTracker_GetModelCosts(t *testing.T) {
	b := NewBudgetTracker()

	b.RecordDetailed("model-a", 0.10, 500, "explore")
	b.RecordDetailed("model-a", 0.15, 700, "explore")
	b.RecordDetailed("model-b", 0.05, 200, "explain")

	costs := b.GetModelCosts()
	if len(costs) != 2 {
		t.Errorf("expected 2 models, got %d", len(costs))
	}

	if costs["model-a"].TotalCost != 0.25 {
		t.Errorf("model-a TotalCost = %f, want 0.25", costs["model-a"].TotalCost)
	}
	if costs["model-a"].CallCount != 2 {
		t.Errorf("model-a CallCount = %d, want 2", costs["model-a"].CallCount)
	}
	// AvgCost should be 0.25/2 = 0.125
	if diff := costs["model-a"].AvgCost - 0.125; diff < -0.001 || diff > 0.001 {
		t.Errorf("model-a AvgCost = %f, want 0.125", costs["model-a"].AvgCost)
	}
}

func TestBudgetTracker_GetHistory(t *testing.T) {
	b := NewBudgetTracker()

	for i := 0; i < 10; i++ {
		b.Record(0.01)
	}

	// Get all
	all := b.GetHistory(0)
	if len(all) != 10 {
		t.Errorf("all history = %d, want 10", len(all))
	}

	// Get last 5
	last5 := b.GetHistory(5)
	if len(last5) != 5 {
		t.Errorf("last 5 = %d, want 5", len(last5))
	}

	// Get more than available
	many := b.GetHistory(100)
	if len(many) != 10 {
		t.Errorf("many = %d, want 10", len(many))
	}
}

func TestBudgetTracker_GetSummary(t *testing.T) {
	b := NewBudgetTrackerWithLimits(10.0, 2.0, 0.5)
	b.RecordDetailed("model-a", 0.50, 1000, "bugfix")

	summary := b.GetSummary()
	if summary == nil {
		t.Fatal("GetSummary returned nil")
	}

	if summary.DailyLimit != 10.0 {
		t.Errorf("DailyLimit = %f, want 10.0", summary.DailyLimit)
	}
	if summary.CurrentSession != 0.50 {
		t.Errorf("CurrentSession = %f, want 0.50", summary.CurrentSession)
	}
	if summary.SessionPercent != 25.0 {
		t.Errorf("SessionPercent = %f, want 25.0", summary.SessionPercent)
	}
	if summary.TotalCalls != 1 {
		t.Errorf("TotalCalls = %d, want 1", summary.TotalCalls)
	}
}

func TestBudgetTracker_Persistence(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "budget.json")

	// Create and record
	b1 := NewBudgetTrackerWithPath(path)
	b1.SetLimits(15.0, 3.0)
	b1.RecordDetailed("test-model", 0.25, 500, "explore")

	// Verify file created
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("budget file not created")
	}

	// Load in new instance
	b2 := NewBudgetTrackerWithPath(path)

	// Check limits restored
	if b2.DailyLimit != 15.0 {
		t.Errorf("loaded DailyLimit = %f, want 15.0", b2.DailyLimit)
	}
	if b2.SessionLimit != 3.0 {
		t.Errorf("loaded SessionLimit = %f, want 3.0", b2.SessionLimit)
	}

	// Check daily restored (same day)
	_, daily := b2.GetUsage()
	if daily != 0.25 {
		t.Errorf("loaded daily = %f, want 0.25", daily)
	}

	// Check model costs restored
	costs := b2.GetModelCosts()
	if costs["test-model"] == nil {
		t.Error("model costs not restored")
	}
}

func TestBudgetTracker_Alerts(t *testing.T) {
	b := NewBudgetTrackerWithLimits(1.0, 1.0, 1.0)

	var alerts []string
	b.SetAlertFunc(func(msg string) {
		alerts = append(alerts, msg)
	})

	// Record to trigger 80% alert
	b.Record(0.81)

	if len(alerts) == 0 {
		t.Error("expected alert at 80%")
	}

	// Record to exceed limit
	b.Record(0.25)

	if len(alerts) < 2 {
		t.Error("expected alert for exceeding limit")
	}
}

func TestCostRecord_Fields(t *testing.T) {
	record := CostRecord{
		ModelID:  "test",
		Cost:     0.15,
		Tokens:   1000,
		TaskType: "bugfix",
	}

	if record.ModelID != "test" {
		t.Error("ModelID mismatch")
	}
	if record.Cost != 0.15 {
		t.Error("Cost mismatch")
	}
}

func TestModelCosts_Fields(t *testing.T) {
	mc := ModelCosts{
		ModelID:     "test",
		TotalCost:   1.50,
		CallCount:   10,
		AvgCost:     0.15,
		TotalTokens: 5000,
	}

	if mc.AvgCost != 0.15 {
		t.Error("AvgCost mismatch")
	}
}

func TestBudgetSummary_Fields(t *testing.T) {
	summary := BudgetSummary{
		DailyLimit:     10.0,
		SessionLimit:   2.0,
		MaxPerTask:     0.50,
		CurrentDaily:   1.0,
		CurrentSession: 0.50,
		DailyPercent:   10.0,
		SessionPercent: 25.0,
		TotalCalls:     5,
	}

	if summary.DailyPercent != 10.0 {
		t.Error("DailyPercent mismatch")
	}
}
