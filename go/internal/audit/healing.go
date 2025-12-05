// Package audit auto-healing and remediation.
package audit

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// RemediationAction defines a corrective action.
type RemediationAction string

const (
	ActionRetry      RemediationAction = "retry"
	ActionRollback   RemediationAction = "rollback"
	ActionRestart    RemediationAction = "restart"
	ActionNotify     RemediationAction = "notify"
	ActionSkip       RemediationAction = "skip"
	ActionEscalate   RemediationAction = "escalate"
	ActionClearCache RemediationAction = "clear_cache"
	ActionKill       RemediationAction = "kill"
)

// RemediationResult captures outcome of a healing attempt.
type RemediationResult struct {
	ID          string            `json:"id"`
	AnomalyID   string            `json:"anomaly_id"`
	Action      RemediationAction `json:"action"`
	Success     bool              `json:"success"`
	Message     string            `json:"message"`
	RollbackRef string            `json:"rollback_ref,omitempty"`
	AttemptedAt time.Time         `json:"attempted_at"`
	CompletedAt time.Time         `json:"completed_at"`
	DurationMs  int64             `json:"duration_ms"`
}

// RemediationRule maps patterns to actions.
type RemediationRule struct {
	Name        string            `json:"name"`
	Pattern     PatternMatcher    `json:"-"`
	Category    Category          `json:"category,omitempty"`
	Operation   string            `json:"operation,omitempty"`
	Level       AnomalyLevel      `json:"level,omitempty"`
	MetricType  MetricType        `json:"metric_type,omitempty"`
	Action      RemediationAction `json:"action"`
	MaxRetries  int               `json:"max_retries"`
	Cooldown    time.Duration     `json:"cooldown"`
	Description string            `json:"description"`
}

// PatternMatcher decides if a rule applies to an anomaly.
type PatternMatcher func(*Anomaly) bool

// Healer orchestrates auto-healing.
type Healer struct {
	mu               sync.Mutex
	rules            []RemediationRule
	cooldowns        map[string]time.Time // key -> last action time
	retryCount       map[string]int       // anomaly_id -> retry count
	maxGlobalRetries int
}

// NewHealer creates a healer with default rules.
func NewHealer() *Healer {
	h := &Healer{
		rules:            make([]RemediationRule, 0),
		cooldowns:        make(map[string]time.Time),
		retryCount:       make(map[string]int),
		maxGlobalRetries: 3,
	}

	h.RegisterDefaults()
	return h
}

// RegisterDefaults adds standard remediation rules.
func (h *Healer) RegisterDefaults() {
	// High latency → retry (might be transient)
	h.AddRule(RemediationRule{
		Name:        "high-latency-retry",
		MetricType:  MetricLatency,
		Level:       AnomalyHigh,
		Action:      ActionRetry,
		MaxRetries:  2,
		Cooldown:    30 * time.Second,
		Description: "Retry operations with high latency",
		Pattern: func(a *Anomaly) bool {
			return a.Type == MetricLatency && a.Level == AnomalyHigh
		},
	})

	// Critical latency → escalate
	h.AddRule(RemediationRule{
		Name:        "critical-latency-escalate",
		MetricType:  MetricLatency,
		Level:       AnomalyCritical,
		Action:      ActionEscalate,
		Cooldown:    5 * time.Minute,
		Description: "Escalate critical latency issues",
		Pattern: func(a *Anomaly) bool {
			return a.Type == MetricLatency && a.Level == AnomalyCritical
		},
	})

	// Code errors → rollback to last good commit
	h.AddRule(RemediationRule{
		Name:        "code-error-rollback",
		Category:    CategoryCode,
		Level:       AnomalyHigh,
		Action:      ActionRollback,
		Cooldown:    10 * time.Minute,
		Description: "Rollback code changes on persistent errors",
		Pattern: func(a *Anomaly) bool {
			return a.Category == CategoryCode &&
				(a.Level == AnomalyHigh || a.Level == AnomalyCritical)
		},
	})

	// Git errors → notify (don't auto-fix git)
	h.AddRule(RemediationRule{
		Name:        "git-error-notify",
		Category:    CategoryGit,
		Action:      ActionNotify,
		Cooldown:    1 * time.Minute,
		Description: "Notify on git operation failures",
		Pattern: func(a *Anomaly) bool {
			return a.Category == CategoryGit
		},
	})

	// System errors → restart service
	h.AddRule(RemediationRule{
		Name:        "system-error-restart",
		Category:    CategorySystem,
		Level:       AnomalyCritical,
		Action:      ActionRestart,
		MaxRetries:  1,
		Cooldown:    5 * time.Minute,
		Description: "Restart system services on critical failures",
		Pattern: func(a *Anomaly) bool {
			return a.Category == CategorySystem && a.Level == AnomalyCritical
		},
	})

	// Output too large → clear cache
	h.AddRule(RemediationRule{
		Name:        "large-output-clear",
		MetricType:  MetricOutputSize,
		Level:       AnomalyHigh,
		Action:      ActionClearCache,
		Cooldown:    1 * time.Minute,
		Description: "Clear cache when output sizes spike",
		Pattern: func(a *Anomaly) bool {
			return a.Type == MetricOutputSize && a.Level >= AnomalyHigh
		},
	})
}

// AddRule registers a new remediation rule.
func (h *Healer) AddRule(rule RemediationRule) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.rules = append(h.rules, rule)
}

// FindRule returns the best matching rule for an anomaly.
func (h *Healer) FindRule(anomaly *Anomaly) *RemediationRule {
	h.mu.Lock()
	defer h.mu.Unlock()

	for i := range h.rules {
		rule := &h.rules[i]
		if rule.Pattern != nil && rule.Pattern(anomaly) {
			return rule
		}
	}
	return nil
}

// CanHeal checks if we can apply healing (respects cooldowns and retries).
func (h *Healer) CanHeal(anomaly *Anomaly, rule *RemediationRule) (bool, string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	key := ruleKey(anomaly, rule)

	// Check cooldown
	if lastAction, ok := h.cooldowns[key]; ok {
		if time.Since(lastAction) < rule.Cooldown {
			remaining := rule.Cooldown - time.Since(lastAction)
			return false, fmt.Sprintf("cooldown: %v remaining", remaining.Round(time.Second))
		}
	}

	// Check retries
	if rule.MaxRetries > 0 {
		if count := h.retryCount[anomaly.ID]; count >= rule.MaxRetries {
			return false, fmt.Sprintf("max retries (%d) exceeded", rule.MaxRetries)
		}
	}

	return true, ""
}

// Heal attempts to remediate an anomaly.
func (h *Healer) Heal(ctx context.Context, anomaly *Anomaly) (*RemediationResult, error) {
	rule := h.FindRule(anomaly)
	if rule == nil {
		return nil, fmt.Errorf("no matching rule for anomaly %s", anomaly.ID)
	}

	canHeal, reason := h.CanHeal(anomaly, rule)
	if !canHeal {
		return &RemediationResult{
			ID:          generateHealingID(),
			AnomalyID:   anomaly.ID,
			Action:      rule.Action,
			Success:     false,
			Message:     reason,
			AttemptedAt: time.Now(),
			CompletedAt: time.Now(),
		}, nil
	}

	result := &RemediationResult{
		ID:          generateHealingID(),
		AnomalyID:   anomaly.ID,
		Action:      rule.Action,
		AttemptedAt: time.Now(),
	}

	// Execute remediation (implementations in remediation.go)
	var err error
	switch rule.Action {
	case ActionRetry:
		err = h.executeRetry(ctx, anomaly)
	case ActionRollback:
		result.RollbackRef, err = h.executeRollback(ctx, anomaly)
	case ActionRestart:
		err = h.executeRestart(ctx, anomaly)
	case ActionNotify:
		err = h.executeNotify(ctx, anomaly)
	case ActionClearCache:
		err = h.executeClearCache(ctx, anomaly)
	case ActionEscalate:
		err = h.executeEscalate(ctx, anomaly)
	case ActionKill:
		err = h.executeKill(ctx, anomaly)
	case ActionSkip:
		// Do nothing
	default:
		err = fmt.Errorf("unknown action: %s", rule.Action)
	}

	result.CompletedAt = time.Now()
	result.DurationMs = result.CompletedAt.Sub(result.AttemptedAt).Milliseconds()

	if err != nil {
		result.Success = false
		result.Message = err.Error()
	} else {
		result.Success = true
		result.Message = fmt.Sprintf("applied %s successfully", rule.Action)
	}

	// Update tracking
	h.mu.Lock()
	key := ruleKey(anomaly, rule)
	h.cooldowns[key] = time.Now()
	if rule.MaxRetries > 0 {
		h.retryCount[anomaly.ID]++
	}
	h.mu.Unlock()

	return result, nil
}

// HealAll attempts to remediate multiple anomalies.
func (h *Healer) HealAll(ctx context.Context, anomalies []Anomaly) []RemediationResult {
	var results []RemediationResult

	for i := range anomalies {
		result, err := h.Heal(ctx, &anomalies[i])
		if err != nil {
			results = append(results, RemediationResult{
				ID:          generateHealingID(),
				AnomalyID:   anomalies[i].ID,
				Success:     false,
				Message:     err.Error(),
				AttemptedAt: time.Now(),
				CompletedAt: time.Now(),
			})
		} else if result != nil {
			results = append(results, *result)
		}
	}

	return results
}

// Helper functions

func ruleKey(anomaly *Anomaly, rule *RemediationRule) string {
	return fmt.Sprintf("%s:%s:%s", rule.Name, anomaly.Category, anomaly.Operation)
}

var healingCounter int64

func generateHealingID() string {
	healingCounter++
	return "heal-" + intToStr(healingCounter)
}
