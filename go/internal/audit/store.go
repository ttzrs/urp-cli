// Package audit store for Memgraph persistence.
package audit

import (
	"context"
	"fmt"
	"time"

	"github.com/joss/urp/internal/graph"
)

// Store persists audit events to Memgraph.
type Store struct {
	db        graph.Driver
	sessionID string
}

// NewStore creates a new audit store.
func NewStore(db graph.Driver, sessionID string) *Store {
	return &Store{db: db, sessionID: sessionID}
}

// Save persists an audit event to the graph.
func (s *Store) Save(ctx context.Context, event *AuditEvent) error {
	query := `
		MERGE (sess:Session {session_id: $session_id})
		CREATE (e:AuditEvent {
			event_id: $event_id,
			category: $category,
			operation: $operation,
			command: $command,
			status: $status,
			exit_code: $exit_code,
			error_message: $error_message,
			output_size: $output_size,
			started_at: $started_at,
			completed_at: $completed_at,
			duration_ms: $duration_ms,
			commit_hash: $commit_hash,
			commit_short: $commit_short,
			branch: $branch,
			is_dirty: $is_dirty,
			author: $author,
			repo_root: $repo_root,
			project: $project,
			worker_id: $worker_id
		})
		CREATE (sess)-[:LOGGED]->(e)
	`

	return s.db.ExecuteWrite(ctx, query, map[string]any{
		"session_id":    s.sessionID,
		"event_id":      event.EventID,
		"category":      string(event.Category),
		"operation":     event.Operation,
		"command":       event.Command,
		"status":        string(event.Status),
		"exit_code":     event.ExitCode,
		"error_message": event.ErrorMessage,
		"output_size":   event.OutputSize,
		"started_at":    event.StartedAt.UTC().Format(time.RFC3339),
		"completed_at":  event.CompletedAt.UTC().Format(time.RFC3339),
		"duration_ms":   event.DurationMs,
		"commit_hash":   event.Git.CommitHash,
		"commit_short":  event.Git.CommitShort,
		"branch":        event.Git.Branch,
		"is_dirty":      event.Git.IsDirty,
		"author":        event.Git.Author,
		"repo_root":     event.Git.RepoRoot,
		"project":       event.Project,
		"worker_id":     event.WorkerID,
	})
}

// QueryFilter defines filters for querying audit events.
type QueryFilter struct {
	Category   Category
	Status     Status
	Operation  string
	Since      time.Time
	Until      time.Time
	CommitHash string
	Limit      int
}

// Query retrieves audit events matching the filter.
func (s *Store) Query(ctx context.Context, filter QueryFilter) ([]AuditEvent, error) {
	// Build dynamic WHERE clauses
	conditions := []string{"sess.session_id = $session_id"}
	params := map[string]any{"session_id": s.sessionID}

	if filter.Category != "" {
		conditions = append(conditions, "e.category = $category")
		params["category"] = string(filter.Category)
	}
	if filter.Status != "" {
		conditions = append(conditions, "e.status = $status")
		params["status"] = string(filter.Status)
	}
	if filter.Operation != "" {
		conditions = append(conditions, "e.operation CONTAINS $operation")
		params["operation"] = filter.Operation
	}
	if !filter.Since.IsZero() {
		conditions = append(conditions, "e.started_at >= $since")
		params["since"] = filter.Since.UTC().Format(time.RFC3339)
	}
	if !filter.Until.IsZero() {
		conditions = append(conditions, "e.started_at <= $until")
		params["until"] = filter.Until.UTC().Format(time.RFC3339)
	}
	if filter.CommitHash != "" {
		conditions = append(conditions, "(e.commit_hash = $commit OR e.commit_short = $commit)")
		params["commit"] = filter.CommitHash
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	params["limit"] = limit

	whereClause := ""
	for i, cond := range conditions {
		if i == 0 {
			whereClause = "WHERE " + cond
		} else {
			whereClause += " AND " + cond
		}
	}

	query := fmt.Sprintf(`
		MATCH (sess:Session)-[:LOGGED]->(e:AuditEvent)
		%s
		RETURN e.event_id as event_id,
		       e.category as category,
		       e.operation as operation,
		       e.command as command,
		       e.status as status,
		       e.exit_code as exit_code,
		       e.error_message as error_message,
		       e.output_size as output_size,
		       e.started_at as started_at,
		       e.completed_at as completed_at,
		       e.duration_ms as duration_ms,
		       e.commit_hash as commit_hash,
		       e.commit_short as commit_short,
		       e.branch as branch,
		       e.is_dirty as is_dirty,
		       e.project as project,
		       e.worker_id as worker_id
		ORDER BY e.started_at DESC
		LIMIT $limit
	`, whereClause)

	records, err := s.db.Execute(ctx, query, params)
	if err != nil {
		return nil, err
	}

	var events []AuditEvent
	for _, r := range records {
		event := AuditEvent{
			EventID:      getString(r, "event_id"),
			Category:     Category(getString(r, "category")),
			Operation:    getString(r, "operation"),
			Command:      getString(r, "command"),
			Status:       Status(getString(r, "status")),
			ExitCode:     getInt(r, "exit_code"),
			ErrorMessage: getString(r, "error_message"),
			OutputSize:   getInt(r, "output_size"),
			DurationMs:   getInt64(r, "duration_ms"),
			Project:      getString(r, "project"),
			WorkerID:     getString(r, "worker_id"),
			Git: GitContext{
				CommitHash:  getString(r, "commit_hash"),
				CommitShort: getString(r, "commit_short"),
				Branch:      getString(r, "branch"),
				IsDirty:     getBool(r, "is_dirty"),
			},
		}

		// Parse times
		if started := getString(r, "started_at"); started != "" {
			if t, err := time.Parse(time.RFC3339, started); err == nil {
				event.StartedAt = t
			}
		}
		if completed := getString(r, "completed_at"); completed != "" {
			if t, err := time.Parse(time.RFC3339, completed); err == nil {
				event.CompletedAt = t
			}
		}
		event.Duration = time.Duration(event.DurationMs) * time.Millisecond

		events = append(events, event)
	}

	return events, nil
}

// GetErrors returns recent error events.
func (s *Store) GetErrors(ctx context.Context, limit int) ([]AuditEvent, error) {
	return s.Query(ctx, QueryFilter{
		Status: StatusError,
		Limit:  limit,
	})
}

// GetStats returns audit statistics.
func (s *Store) GetStats(ctx context.Context) (map[string]any, error) {
	query := `
		MATCH (sess:Session {session_id: $session_id})-[:LOGGED]->(e:AuditEvent)
		RETURN count(e) as total,
		       sum(CASE WHEN e.status = 'success' THEN 1 ELSE 0 END) as success,
		       sum(CASE WHEN e.status = 'error' THEN 1 ELSE 0 END) as errors,
		       sum(CASE WHEN e.status = 'warning' THEN 1 ELSE 0 END) as warnings,
		       sum(CASE WHEN e.status = 'timeout' THEN 1 ELSE 0 END) as timeouts,
		       avg(e.duration_ms) as avg_duration_ms,
		       max(e.duration_ms) as max_duration_ms,
		       min(e.started_at) as first_event,
		       max(e.started_at) as last_event
	`

	records, err := s.db.Execute(ctx, query, map[string]any{
		"session_id": s.sessionID,
	})
	if err != nil {
		return nil, err
	}

	if len(records) == 0 {
		return map[string]any{
			"total":          0,
			"success":        0,
			"errors":         0,
			"warnings":       0,
			"timeouts":       0,
			"avg_duration_ms": 0,
		}, nil
	}

	r := records[0]
	return map[string]any{
		"total":           getInt(r, "total"),
		"success":         getInt(r, "success"),
		"errors":          getInt(r, "errors"),
		"warnings":        getInt(r, "warnings"),
		"timeouts":        getInt(r, "timeouts"),
		"avg_duration_ms": getFloat(r, "avg_duration_ms"),
		"max_duration_ms": getInt64(r, "max_duration_ms"),
		"first_event":     getString(r, "first_event"),
		"last_event":      getString(r, "last_event"),
	}, nil
}

// GetStatsByCategory returns stats grouped by category.
func (s *Store) GetStatsByCategory(ctx context.Context) (map[string]map[string]any, error) {
	query := `
		MATCH (sess:Session {session_id: $session_id})-[:LOGGED]->(e:AuditEvent)
		RETURN e.category as category,
		       count(e) as total,
		       sum(CASE WHEN e.status = 'success' THEN 1 ELSE 0 END) as success,
		       sum(CASE WHEN e.status = 'error' THEN 1 ELSE 0 END) as errors,
		       avg(e.duration_ms) as avg_duration_ms
		ORDER BY total DESC
	`

	records, err := s.db.Execute(ctx, query, map[string]any{
		"session_id": s.sessionID,
	})
	if err != nil {
		return nil, err
	}

	stats := make(map[string]map[string]any)
	for _, r := range records {
		cat := getString(r, "category")
		if cat == "" {
			continue
		}
		stats[cat] = map[string]any{
			"total":           getInt(r, "total"),
			"success":         getInt(r, "success"),
			"errors":          getInt(r, "errors"),
			"avg_duration_ms": getFloat(r, "avg_duration_ms"),
		}
	}

	return stats, nil
}

// GetEventsByCommit returns events for a specific commit.
func (s *Store) GetEventsByCommit(ctx context.Context, commitHash string) ([]AuditEvent, error) {
	return s.Query(ctx, QueryFilter{
		CommitHash: commitHash,
		Limit:      100,
	})
}

// Helper functions
func getString(r graph.Record, key string) string {
	if v, ok := r[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func getInt(r graph.Record, key string) int {
	if v, ok := r[key]; ok {
		switch n := v.(type) {
		case int64:
			return int(n)
		case int:
			return n
		case float64:
			return int(n)
		}
	}
	return 0
}

func getInt64(r graph.Record, key string) int64 {
	if v, ok := r[key]; ok {
		switch n := v.(type) {
		case int64:
			return n
		case int:
			return int64(n)
		case float64:
			return int64(n)
		}
	}
	return 0
}

func getFloat(r graph.Record, key string) float64 {
	if v, ok := r[key]; ok {
		switch n := v.(type) {
		case float64:
			return n
		case int64:
			return float64(n)
		case int:
			return float64(n)
		}
	}
	return 0
}

func getBool(r graph.Record, key string) bool {
	if v, ok := r[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}
