// Package audit healing persistence.
package audit

import (
	"context"
	"time"

	"github.com/joss/urp/internal/graph"
)

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

	return parseRemediationRecords(records), nil
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

	return parseRemediationRecords(records), nil
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
		"total":           graph.GetInt(r, "total"),
		"success":         graph.GetInt(r, "success"),
		"failed":          graph.GetInt(r, "failed"),
		"avg_duration_ms": graph.GetFloat(r, "avg_duration_ms"),
	}, nil
}

// parseRemediationRecords converts graph records to RemediationResult slice.
func parseRemediationRecords(records []graph.Record) []RemediationResult {
	var results []RemediationResult
	for _, r := range records {
		result := RemediationResult{
			ID:          graph.GetString(r, "healing_id"),
			AnomalyID:   graph.GetString(r, "anomaly_id"),
			Action:      RemediationAction(graph.GetString(r, "action")),
			Success:     graph.GetBool(r, "success"),
			Message:     graph.GetString(r, "message"),
			RollbackRef: graph.GetString(r, "rollback_ref"),
			DurationMs:  graph.GetInt64(r, "duration_ms"),
		}

		if attempted := graph.GetString(r, "attempted_at"); attempted != "" {
			if t, err := time.Parse(time.RFC3339, attempted); err == nil {
				result.AttemptedAt = t
			}
		}

		results = append(results, result)
	}
	return results
}
