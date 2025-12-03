package audit

import (
	"context"
	"testing"
	"time"
)

func TestNewHealer(t *testing.T) {
	healer := NewHealer()

	if healer == nil {
		t.Fatal("expected healer")
	}

	// Should have default rules
	if len(healer.rules) == 0 {
		t.Error("expected default rules")
	}
}

func TestHealerFindRule(t *testing.T) {
	healer := NewHealer()

	// High latency should match retry rule
	anomaly := &Anomaly{
		ID:        "test-1",
		Type:      MetricLatency,
		Category:  CategoryCode,
		Operation: "ingest",
		Level:     AnomalyHigh,
	}

	rule := healer.FindRule(anomaly)
	if rule == nil {
		t.Fatal("expected matching rule")
	}

	if rule.Action != ActionRetry {
		t.Errorf("expected retry action, got %s", rule.Action)
	}
}

func TestHealerFindRuleCritical(t *testing.T) {
	healer := NewHealer()

	// Critical latency should match escalate rule
	anomaly := &Anomaly{
		ID:        "test-2",
		Type:      MetricLatency,
		Category:  CategoryCode,
		Operation: "slow",
		Level:     AnomalyCritical,
	}

	rule := healer.FindRule(anomaly)
	if rule == nil {
		t.Fatal("expected matching rule")
	}

	if rule.Action != ActionEscalate {
		t.Errorf("expected escalate action, got %s", rule.Action)
	}
}

func TestHealerFindRuleGit(t *testing.T) {
	healer := NewHealer()

	// Git errors should match notify rule
	anomaly := &Anomaly{
		ID:       "test-3",
		Type:     MetricErrorRate,
		Category: CategoryGit,
		Level:    AnomalyMedium,
	}

	rule := healer.FindRule(anomaly)
	if rule == nil {
		t.Fatal("expected matching rule")
	}

	if rule.Action != ActionNotify {
		t.Errorf("expected notify action, got %s", rule.Action)
	}
}

func TestHealerCanHealCooldown(t *testing.T) {
	healer := NewHealer()

	anomaly := &Anomaly{
		ID:        "test-cool",
		Type:      MetricLatency,
		Category:  CategoryCode,
		Level:     AnomalyHigh,
	}

	rule := healer.FindRule(anomaly)
	if rule == nil {
		t.Fatal("expected rule")
	}

	// First check should pass
	canHeal, reason := healer.CanHeal(anomaly, rule)
	if !canHeal {
		t.Errorf("expected can heal, got blocked: %s", reason)
	}

	// Simulate a healing action
	healer.mu.Lock()
	key := ruleKey(anomaly, rule)
	healer.cooldowns[key] = time.Now()
	healer.mu.Unlock()

	// Second check should be blocked by cooldown
	canHeal, reason = healer.CanHeal(anomaly, rule)
	if canHeal {
		t.Error("expected blocked by cooldown")
	}

	if reason == "" {
		t.Error("expected reason for blocking")
	}
}

func TestHealerCanHealMaxRetries(t *testing.T) {
	healer := NewHealer()

	anomaly := &Anomaly{
		ID:        "test-retry",
		Type:      MetricLatency,
		Category:  CategoryCode,
		Level:     AnomalyHigh,
	}

	rule := healer.FindRule(anomaly)
	if rule == nil {
		t.Fatal("expected rule")
	}

	// Exhaust retries
	healer.mu.Lock()
	healer.retryCount[anomaly.ID] = rule.MaxRetries
	healer.mu.Unlock()

	canHeal, reason := healer.CanHeal(anomaly, rule)
	if canHeal {
		t.Error("expected blocked by max retries")
	}

	if reason == "" {
		t.Error("expected reason")
	}
}

func TestHealerHeal(t *testing.T) {
	healer := NewHealer()

	anomaly := &Anomaly{
		ID:          "test-heal",
		Type:        MetricLatency,
		Category:    CategoryCode,
		Operation:   "test",
		Level:       AnomalyHigh,
		Description: "test anomaly",
	}

	result, err := healer.Heal(context.Background(), anomaly)
	if err != nil {
		t.Fatalf("heal failed: %v", err)
	}

	if result == nil {
		t.Fatal("expected result")
	}

	if result.AnomalyID != anomaly.ID {
		t.Errorf("expected anomaly ID %s, got %s", anomaly.ID, result.AnomalyID)
	}

	if result.Action != ActionRetry {
		t.Errorf("expected retry action, got %s", result.Action)
	}
}

func TestHealerHealNoRule(t *testing.T) {
	healer := NewHealer()

	// Anomaly with no matching rule
	anomaly := &Anomaly{
		ID:       "test-no-rule",
		Type:     MetricFrequency, // No rule for this
		Category: CategoryMemory,
		Level:    AnomalyLow,
	}

	_, err := healer.Heal(context.Background(), anomaly)
	if err == nil {
		t.Error("expected error for no matching rule")
	}
}

func TestHealerHealAll(t *testing.T) {
	healer := NewHealer()

	anomalies := []Anomaly{
		{
			ID:       "a1",
			Type:     MetricLatency,
			Category: CategoryCode,
			Level:    AnomalyHigh,
		},
		{
			ID:       "a2",
			Type:     MetricErrorRate,
			Category: CategoryGit,
			Level:    AnomalyMedium,
		},
	}

	results := healer.HealAll(context.Background(), anomalies)

	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}

func TestAddRule(t *testing.T) {
	healer := NewHealer()
	initialCount := len(healer.rules)

	healer.AddRule(RemediationRule{
		Name:       "custom-rule",
		Category:   CategoryKnowledge,
		Action:     ActionSkip,
		Cooldown:   1 * time.Minute,
		Pattern: func(a *Anomaly) bool {
			return a.Category == CategoryKnowledge
		},
	})

	if len(healer.rules) != initialCount+1 {
		t.Error("expected rule to be added")
	}
}

func TestRemediationActions(t *testing.T) {
	actions := []RemediationAction{
		ActionRetry,
		ActionRollback,
		ActionRestart,
		ActionNotify,
		ActionSkip,
		ActionEscalate,
		ActionClearCache,
		ActionKill,
	}

	for _, a := range actions {
		if a == "" {
			t.Error("empty action")
		}
	}
}

func TestRuleKey(t *testing.T) {
	anomaly := &Anomaly{
		Category:  CategoryCode,
		Operation: "ingest",
	}
	rule := &RemediationRule{
		Name: "test-rule",
	}

	key := ruleKey(anomaly, rule)
	expected := "test-rule:code:ingest"

	if key != expected {
		t.Errorf("expected %s, got %s", expected, key)
	}
}

func TestGenerateHealingID(t *testing.T) {
	id1 := generateHealingID()
	id2 := generateHealingID()

	if id1 == id2 {
		t.Error("IDs should be unique")
	}

	if id1[:5] != "heal-" {
		t.Errorf("expected heal- prefix, got %s", id1[:5])
	}
}
