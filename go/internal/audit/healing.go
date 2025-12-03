// Package audit auto-healing and remediation.
package audit

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/joss/urp/internal/graph"
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
	ID           string            `json:"id"`
	AnomalyID    string            `json:"anomaly_id"`
	Action       RemediationAction `json:"action"`
	Success      bool              `json:"success"`
	Message      string            `json:"message"`
	RollbackRef  string            `json:"rollback_ref,omitempty"` // git ref if rollback
	AttemptedAt  time.Time         `json:"attempted_at"`
	CompletedAt  time.Time         `json:"completed_at"`
	DurationMs   int64             `json:"duration_ms"`
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
	mu           sync.Mutex
	rules        []RemediationRule
	cooldowns    map[string]time.Time // key -> last action time
	retryCount   map[string]int       // anomaly_id -> retry count
	maxGlobalRetries int
}

// NewHealer creates a healer with default rules.
func NewHealer() *Healer {
	h := &Healer{
		rules:        make([]RemediationRule, 0),
		cooldowns:    make(map[string]time.Time),
		retryCount:   make(map[string]int),
		maxGlobalRetries: 3,
	}

	// Register default rules
	h.RegisterDefaults()

	return h
}

// RegisterDefaults adds standard remediation rules.
func (h *Healer) RegisterDefaults() {
	// High latency → retry (might be transient)
	h.AddRule(RemediationRule{
		Name:       "high-latency-retry",
		MetricType: MetricLatency,
		Level:      AnomalyHigh,
		Action:     ActionRetry,
		MaxRetries: 2,
		Cooldown:   30 * time.Second,
		Description: "Retry operations with high latency",
		Pattern: func(a *Anomaly) bool {
			return a.Type == MetricLatency && a.Level == AnomalyHigh
		},
	})

	// Critical latency → escalate
	h.AddRule(RemediationRule{
		Name:       "critical-latency-escalate",
		MetricType: MetricLatency,
		Level:      AnomalyCritical,
		Action:     ActionEscalate,
		Cooldown:   5 * time.Minute,
		Description: "Escalate critical latency issues",
		Pattern: func(a *Anomaly) bool {
			return a.Type == MetricLatency && a.Level == AnomalyCritical
		},
	})

	// Code errors → rollback to last good commit
	h.AddRule(RemediationRule{
		Name:       "code-error-rollback",
		Category:   CategoryCode,
		Level:      AnomalyHigh,
		Action:     ActionRollback,
		Cooldown:   10 * time.Minute,
		Description: "Rollback code changes on persistent errors",
		Pattern: func(a *Anomaly) bool {
			return a.Category == CategoryCode &&
				   (a.Level == AnomalyHigh || a.Level == AnomalyCritical)
		},
	})

	// Git errors → notify (don't auto-fix git)
	h.AddRule(RemediationRule{
		Name:       "git-error-notify",
		Category:   CategoryGit,
		Action:     ActionNotify,
		Cooldown:   1 * time.Minute,
		Description: "Notify on git operation failures",
		Pattern: func(a *Anomaly) bool {
			return a.Category == CategoryGit
		},
	})

	// System errors → restart service
	h.AddRule(RemediationRule{
		Name:       "system-error-restart",
		Category:   CategorySystem,
		Level:      AnomalyCritical,
		Action:     ActionRestart,
		MaxRetries: 1,
		Cooldown:   5 * time.Minute,
		Description: "Restart system services on critical failures",
		Pattern: func(a *Anomaly) bool {
			return a.Category == CategorySystem && a.Level == AnomalyCritical
		},
	})

	// Output too large → clear cache
	h.AddRule(RemediationRule{
		Name:       "large-output-clear",
		MetricType: MetricOutputSize,
		Level:      AnomalyHigh,
		Action:     ActionClearCache,
		Cooldown:   1 * time.Minute,
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

	// Execute remediation
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

// Remediation action implementations

func (h *Healer) executeRetry(ctx context.Context, anomaly *Anomaly) error {
	// Retry is typically handled by the caller
	// Here we just signal that a retry is requested
	return nil
}

func (h *Healer) executeRollback(ctx context.Context, anomaly *Anomaly) (string, error) {
	// Find last known good commit
	// For now, rollback to HEAD~1
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD~1")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("cannot find rollback target: %w", err)
	}

	target := strings.TrimSpace(string(out))

	// Soft reset to preserve working directory
	cmd = exec.CommandContext(ctx, "git", "reset", "--soft", target)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("rollback failed: %w", err)
	}

	return target, nil
}

func (h *Healer) executeRestart(ctx context.Context, anomaly *Anomaly) error {
	// Restart is operation-specific
	// Signal that a restart is needed
	return nil
}

func (h *Healer) executeNotify(ctx context.Context, anomaly *Anomaly) error {
	// Notification would go to logging/monitoring system
	fmt.Printf("[NOTIFY] Anomaly detected: %s - %s\n", anomaly.ID, anomaly.Description)
	return nil
}

func (h *Healer) executeClearCache(ctx context.Context, anomaly *Anomaly) error {
	// Cache clearing depends on the specific cache
	// This is a placeholder for cache-specific logic
	return nil
}

func (h *Healer) executeEscalate(ctx context.Context, anomaly *Anomaly) error {
	// Escalation would trigger alerts
	fmt.Printf("[ESCALATE] Critical anomaly: %s - %s (z-score: %.2f)\n",
		anomaly.ID, anomaly.Description, anomaly.ZScore)
	return nil
}

func (h *Healer) executeKill(ctx context.Context, anomaly *Anomaly) error {
	// Kill is dangerous - only for runaway processes
	return fmt.Errorf("kill action requires explicit approval")
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

// HealingStore persists remediation results.
type HealingStore struct {
	db        graph.Driver
	sessionID string
}

// NewHealingStore creates a healing store.
func NewHealingStore(db graph.Driver, sessionID string) *HealingStore {
	return &HealingStore{db: db, sessionID: sessionID}
}

// Save persists a remediation result.
func (s *HealingStore) Save(ctx context.Context, r *RemediationResult) error {
	query := `
		MERGE (sess:Session {session_id: $session_id})
		CREATE (h:Remediation {
			healing_id: $healing_id,
			anomaly_id: $anomaly_id,
			action: $action,
			success: $success,
			message: $message,
			rollback_ref: $rollback_ref,
			attempted_at: $attempted_at,
			completed_at: $completed_at,
			duration_ms: $duration_ms
		})
		CREATE (sess)-[:HEALED]->(h)
	`

	return s.db.ExecuteWrite(ctx, query, map[string]any{
		"session_id":   s.sessionID,
		"healing_id":   r.ID,
		"anomaly_id":   r.AnomalyID,
		"action":       string(r.Action),
		"success":      r.Success,
		"message":      r.Message,
		"rollback_ref": r.RollbackRef,
		"attempted_at": r.AttemptedAt.UTC().Format(time.RFC3339),
		"completed_at": r.CompletedAt.UTC().Format(time.RFC3339),
		"duration_ms":  r.DurationMs,
	})
}

// GetRecent retrieves recent remediation attempts.
func (s *HealingStore) GetRecent(ctx context.Context, limit int) ([]RemediationResult, error) {
	if limit <= 0 {
		limit = 50
	}

	query := `
		MATCH (sess:Session {session_id: $session_id})-[:HEALED]->(h:Remediation)
		RETURN h.healing_id as healing_id,
		       h.anomaly_id as anomaly_id,
		       h.action as action,
		       h.success as success,
		       h.message as message,
		       h.rollback_ref as rollback_ref,
		       h.attempted_at as attempted_at,
		       h.duration_ms as duration_ms
		ORDER BY h.attempted_at DESC
		LIMIT $limit
	`

	records, err := s.db.Execute(ctx, query, map[string]any{
		"session_id": s.sessionID,
		"limit":      limit,
	})
	if err != nil {
		return nil, err
	}

	var results []RemediationResult
	for _, r := range records {
		result := RemediationResult{
			ID:          getString(r, "healing_id"),
			AnomalyID:   getString(r, "anomaly_id"),
			Action:      RemediationAction(getString(r, "action")),
			Success:     getBool(r, "success"),
			Message:     getString(r, "message"),
			RollbackRef: getString(r, "rollback_ref"),
			DurationMs:  getInt64(r, "duration_ms"),
		}

		if attempted := getString(r, "attempted_at"); attempted != "" {
			if t, err := time.Parse(time.RFC3339, attempted); err == nil {
				result.AttemptedAt = t
			}
		}

		results = append(results, result)
	}

	return results, nil
}

// GetByAnomaly retrieves remediation attempts for a specific anomaly.
func (s *HealingStore) GetByAnomaly(ctx context.Context, anomalyID string) ([]RemediationResult, error) {
	query := `
		MATCH (sess:Session {session_id: $session_id})-[:HEALED]->(h:Remediation)
		WHERE h.anomaly_id = $anomaly_id
		RETURN h.healing_id as healing_id,
		       h.anomaly_id as anomaly_id,
		       h.action as action,
		       h.success as success,
		       h.message as message,
		       h.rollback_ref as rollback_ref,
		       h.attempted_at as attempted_at,
		       h.duration_ms as duration_ms
		ORDER BY h.attempted_at DESC
	`

	records, err := s.db.Execute(ctx, query, map[string]any{
		"session_id": s.sessionID,
		"anomaly_id": anomalyID,
	})
	if err != nil {
		return nil, err
	}

	var results []RemediationResult
	for _, r := range records {
		result := RemediationResult{
			ID:          getString(r, "healing_id"),
			AnomalyID:   getString(r, "anomaly_id"),
			Action:      RemediationAction(getString(r, "action")),
			Success:     getBool(r, "success"),
			Message:     getString(r, "message"),
			RollbackRef: getString(r, "rollback_ref"),
			DurationMs:  getInt64(r, "duration_ms"),
		}

		if attempted := getString(r, "attempted_at"); attempted != "" {
			if t, err := time.Parse(time.RFC3339, attempted); err == nil {
				result.AttemptedAt = t
			}
		}

		results = append(results, result)
	}

	return results, nil
}

// GetStats returns healing statistics.
func (s *HealingStore) GetStats(ctx context.Context) (map[string]any, error) {
	query := `
		MATCH (sess:Session {session_id: $session_id})-[:HEALED]->(h:Remediation)
		RETURN count(h) as total,
		       sum(CASE WHEN h.success = true THEN 1 ELSE 0 END) as success,
		       sum(CASE WHEN h.success = false THEN 1 ELSE 0 END) as failed,
		       avg(h.duration_ms) as avg_duration_ms
	`

	records, err := s.db.Execute(ctx, query, map[string]any{
		"session_id": s.sessionID,
	})
	if err != nil {
		return nil, err
	}

	if len(records) == 0 {
		return map[string]any{
			"total":   0,
			"success": 0,
			"failed":  0,
		}, nil
	}

	r := records[0]
	return map[string]any{
		"total":           getInt(r, "total"),
		"success":         getInt(r, "success"),
		"failed":          getInt(r, "failed"),
		"avg_duration_ms": getFloat(r, "avg_duration_ms"),
	}, nil
}
