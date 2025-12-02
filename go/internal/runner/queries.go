// Package runner provides event queries.
package runner

import (
	"context"
	"time"

	"github.com/joss/urp/internal/domain"
	"github.com/joss/urp/internal/graph"
)

// EventStore provides access to terminal events.
type EventStore struct {
	db graph.Driver
}

// NewEventStore creates a new event store.
func NewEventStore(db graph.Driver) *EventStore {
	return &EventStore{db: db}
}

// ListRecent returns recent terminal events.
func (s *EventStore) ListRecent(ctx context.Context, limit int, project string) ([]domain.Event, error) {
	projectClause := ""
	params := map[string]any{"limit": limit}

	if project != "" {
		projectClause = "WHERE e.project = $project"
		params["project"] = project
	}

	query := `
		MATCH (e:TerminalEvent)
		` + projectClause + `
		RETURN e.command as command,
		       e.cmd_base as cmd_base,
		       e.exit_code as exit_code,
		       e.duration_sec as duration_sec,
		       e.datetime as datetime,
		       e.cwd as cwd,
		       e.project as project
		ORDER BY e.timestamp DESC
		LIMIT $limit
	`

	records, err := s.db.Execute(ctx, query, params)
	if err != nil {
		return nil, err
	}

	events := make([]domain.Event, 0, len(records))
	for _, r := range records {
		event := domain.Event{
			Command:     getString(r, "command"),
			CmdBase:     getString(r, "cmd_base"),
			ExitCode:    getInt(r, "exit_code"),
			DurationSec: getFloat(r, "duration_sec"),
			Cwd:         getString(r, "cwd"),
			Project:     getString(r, "project"),
		}
		if dt := getString(r, "datetime"); dt != "" {
			if t, err := time.Parse(time.RFC3339, dt); err == nil {
				event.Timestamp = t
			}
		}
		events = append(events, event)
	}

	return events, nil
}

// ListErrors returns recent errors (conflicts).
func (s *EventStore) ListErrors(ctx context.Context, minutes int, project string) ([]domain.Conflict, error) {
	cutoff := time.Now().Add(-time.Duration(minutes) * time.Minute).Unix()

	projectClause := ""
	params := map[string]any{"cutoff": cutoff}

	if project != "" {
		projectClause = "AND e.project = $project"
		params["project"] = project
	}

	query := `
		MATCH (e:Conflict)
		WHERE e.timestamp > $cutoff ` + projectClause + `
		RETURN e.command as command,
		       e.cmd_base as cmd_base,
		       e.exit_code as exit_code,
		       e.stderr_preview as stderr_preview,
		       e.datetime as datetime,
		       e.cwd as cwd,
		       e.project as project
		ORDER BY e.timestamp DESC
	`

	records, err := s.db.Execute(ctx, query, params)
	if err != nil {
		return nil, err
	}

	conflicts := make([]domain.Conflict, 0, len(records))
	for _, r := range records {
		conflict := domain.Conflict{
			Event: domain.Event{
				Command:       getString(r, "command"),
				CmdBase:       getString(r, "cmd_base"),
				ExitCode:      getInt(r, "exit_code"),
				StderrPreview: getString(r, "stderr_preview"),
				Cwd:           getString(r, "cwd"),
				Project:       getString(r, "project"),
				IsConflict:    true,
			},
		}
		if dt := getString(r, "datetime"); dt != "" {
			if t, err := time.Parse(time.RFC3339, dt); err == nil {
				conflict.Timestamp = t
			}
		}
		conflicts = append(conflicts, conflict)
	}

	return conflicts, nil
}

// Helper functions for type conversion
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
		case int:
			return n
		case int64:
			return int(n)
		case float64:
			return int(n)
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
