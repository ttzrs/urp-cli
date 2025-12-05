// Package orchestrator persistence adapter for Memgraph.
package orchestrator

import (
	"context"
	"fmt"
	"time"

	"github.com/joss/urp/internal/graph"
)

// OrchestratorRun represents a persisted orchestration run.
type OrchestratorRun struct {
	RunID       string       `json:"run_id"`
	Description string       `json:"description"`
	Status      string       `json:"status"` // running, completed, failed
	TaskCount   int          `json:"task_count"`
	Succeeded   int          `json:"succeeded"`
	Failed      int          `json:"failed"`
	DurationMs  int64        `json:"duration_ms"`
	CreatedAt   string       `json:"created_at"`
	CompletedAt string       `json:"completed_at,omitempty"`
	Results     []TaskResult `json:"results,omitempty"`
}

// PersistenceStore handles orchestration result storage.
type PersistenceStore struct {
	db        graph.Driver
	sessionID string
}

// NewPersistenceStore creates a new persistence store.
func NewPersistenceStore(db graph.Driver, sessionID string) *PersistenceStore {
	return &PersistenceStore{db: db, sessionID: sessionID}
}

// StartRun creates a new orchestration run record.
func (p *PersistenceStore) StartRun(ctx context.Context, description string, taskCount int) (string, error) {
	runID := fmt.Sprintf("orch-run-%d", time.Now().UnixNano())
	now := time.Now().UTC().Format(time.RFC3339)

	query := `
		MERGE (sess:Session {session_id: $session_id})
		CREATE (run:OrchestratorRun {
			run_id: $run_id,
			description: $description,
			status: 'running',
			task_count: $task_count,
			succeeded: 0,
			failed: 0,
			created_at: $now
		})
		CREATE (sess)-[:STARTED_RUN {at: $now}]->(run)
	`

	err := p.db.ExecuteWrite(ctx, query, map[string]any{
		"session_id":  p.sessionID,
		"run_id":      runID,
		"description": description,
		"task_count":  taskCount,
		"now":         now,
	})
	if err != nil {
		return "", fmt.Errorf("failed to start run: %w", err)
	}

	return runID, nil
}

// RecordTaskResult saves a task result to the run.
func (p *PersistenceStore) RecordTaskResult(ctx context.Context, runID string, result *TaskResult) error {
	now := time.Now().UTC().Format(time.RFC3339)
	resultID := fmt.Sprintf("orch-result-%d", time.Now().UnixNano())

	status := "success"
	if !result.Success {
		status = "failure"
	}

	// Truncate output for storage (keep first 10KB)
	output := result.Output
	if len(output) > 10240 {
		output = output[:10240] + "\n... (truncated)"
	}

	query := `
		MATCH (run:OrchestratorRun {run_id: $run_id})
		CREATE (result:OrchestratorResult {
			result_id: $result_id,
			task_id: $task_id,
			worker_id: $worker_id,
			status: $status,
			output: $output,
			error: $error,
			duration_ms: $duration_ms,
			created_at: $now
		})
		CREATE (run)-[:HAS_RESULT]->(result)
		SET run.succeeded = run.succeeded + CASE WHEN $status = 'success' THEN 1 ELSE 0 END,
		    run.failed = run.failed + CASE WHEN $status = 'failure' THEN 1 ELSE 0 END
	`

	return p.db.ExecuteWrite(ctx, query, map[string]any{
		"run_id":      runID,
		"result_id":   resultID,
		"task_id":     result.TaskID,
		"worker_id":   result.WorkerID,
		"status":      status,
		"output":      output,
		"error":       result.Error,
		"duration_ms": result.Duration.Milliseconds(),
		"now":         now,
	})
}

// CompleteRun marks a run as completed.
func (p *PersistenceStore) CompleteRun(ctx context.Context, runID string, durationMs int64) error {
	now := time.Now().UTC().Format(time.RFC3339)

	query := `
		MATCH (run:OrchestratorRun {run_id: $run_id})
		SET run.status = CASE WHEN run.failed > 0 THEN 'failed' ELSE 'completed' END,
		    run.completed_at = $now,
		    run.duration_ms = $duration_ms
	`

	return p.db.ExecuteWrite(ctx, query, map[string]any{
		"run_id":      runID,
		"duration_ms": durationMs,
		"now":         now,
	})
}

// GetRun retrieves a run with its results.
func (p *PersistenceStore) GetRun(ctx context.Context, runID string) (*OrchestratorRun, error) {
	query := `
		MATCH (run:OrchestratorRun {run_id: $run_id})
		OPTIONAL MATCH (run)-[:HAS_RESULT]->(result:OrchestratorResult)
		RETURN run.run_id as run_id,
		       run.description as description,
		       run.status as status,
		       run.task_count as task_count,
		       run.succeeded as succeeded,
		       run.failed as failed,
		       run.duration_ms as duration_ms,
		       run.created_at as created_at,
		       run.completed_at as completed_at,
		       collect({
		           task_id: result.task_id,
		           worker_id: result.worker_id,
		           success: result.status = 'success',
		           output: result.output,
		           error: result.error,
		           duration_ms: result.duration_ms
		       }) as results
	`

	records, err := p.db.Execute(ctx, query, map[string]any{
		"run_id": runID,
	})
	if err != nil {
		return nil, err
	}

	if len(records) == 0 {
		return nil, fmt.Errorf("run not found: %s", runID)
	}

	r := records[0]
	run := &OrchestratorRun{
		RunID:       graph.GetString(r, "run_id"),
		Description: graph.GetString(r, "description"),
		Status:      graph.GetString(r, "status"),
		TaskCount:   graph.GetInt(r, "task_count"),
		Succeeded:   graph.GetInt(r, "succeeded"),
		Failed:      graph.GetInt(r, "failed"),
		DurationMs:  graph.GetInt64(r, "duration_ms"),
		CreatedAt:   graph.GetString(r, "created_at"),
		CompletedAt: graph.GetString(r, "completed_at"),
	}

	// Parse results
	if resultsRaw, ok := r["results"].([]any); ok {
		for _, res := range resultsRaw {
			if rm, ok := res.(map[string]any); ok {
				if rm["task_id"] == nil {
					continue
				}
				run.Results = append(run.Results, TaskResult{
					TaskID:   getStringFrom(rm, "task_id"),
					WorkerID: getStringFrom(rm, "worker_id"),
					Success:  getBoolFrom(rm, "success"),
					Output:   getStringFrom(rm, "output"),
					Error:    getStringFrom(rm, "error"),
					Duration: time.Duration(getInt64From(rm, "duration_ms")) * time.Millisecond,
				})
			}
		}
	}

	return run, nil
}

// ListRuns returns recent orchestration runs.
func (p *PersistenceStore) ListRuns(ctx context.Context, limit int) ([]OrchestratorRun, error) {
	query := `
		MATCH (sess:Session {session_id: $session_id})-[:STARTED_RUN]->(run:OrchestratorRun)
		RETURN run.run_id as run_id,
		       run.description as description,
		       run.status as status,
		       run.task_count as task_count,
		       run.succeeded as succeeded,
		       run.failed as failed,
		       run.duration_ms as duration_ms,
		       run.created_at as created_at,
		       run.completed_at as completed_at
		ORDER BY run.created_at DESC
		LIMIT $limit
	`

	records, err := p.db.Execute(ctx, query, map[string]any{
		"session_id": p.sessionID,
		"limit":      limit,
	})
	if err != nil {
		return nil, err
	}

	var runs []OrchestratorRun
	for _, r := range records {
		runs = append(runs, OrchestratorRun{
			RunID:       graph.GetString(r, "run_id"),
			Description: graph.GetString(r, "description"),
			Status:      graph.GetString(r, "status"),
			TaskCount:   graph.GetInt(r, "task_count"),
			Succeeded:   graph.GetInt(r, "succeeded"),
			Failed:      graph.GetInt(r, "failed"),
			DurationMs:  graph.GetInt64(r, "duration_ms"),
			CreatedAt:   graph.GetString(r, "created_at"),
			CompletedAt: graph.GetString(r, "completed_at"),
		})
	}

	return runs, nil
}

// GetStats returns aggregate statistics for orchestration runs.
func (p *PersistenceStore) GetStats(ctx context.Context) (map[string]any, error) {
	query := `
		MATCH (sess:Session {session_id: $session_id})-[:STARTED_RUN]->(run:OrchestratorRun)
		RETURN count(run) as total_runs,
		       sum(CASE WHEN run.status = 'completed' THEN 1 ELSE 0 END) as completed_runs,
		       sum(CASE WHEN run.status = 'failed' THEN 1 ELSE 0 END) as failed_runs,
		       sum(run.task_count) as total_tasks,
		       sum(run.succeeded) as total_succeeded,
		       sum(run.failed) as total_failed,
		       avg(run.duration_ms) as avg_duration_ms
	`

	records, err := p.db.Execute(ctx, query, map[string]any{
		"session_id": p.sessionID,
	})
	if err != nil {
		return nil, err
	}

	if len(records) == 0 {
		return map[string]any{
			"total_runs":      0,
			"completed_runs":  0,
			"failed_runs":     0,
			"total_tasks":     0,
			"total_succeeded": 0,
			"total_failed":    0,
			"avg_duration_ms": 0,
		}, nil
	}

	r := records[0]
	return map[string]any{
		"total_runs":      graph.GetInt(r, "total_runs"),
		"completed_runs":  graph.GetInt(r, "completed_runs"),
		"failed_runs":     graph.GetInt(r, "failed_runs"),
		"total_tasks":     graph.GetInt(r, "total_tasks"),
		"total_succeeded": graph.GetInt(r, "total_succeeded"),
		"total_failed":    graph.GetInt(r, "total_failed"),
		"avg_duration_ms": graph.GetFloat(r, "avg_duration_ms"),
	}, nil
}


func getStringFrom(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func getBoolFrom(m map[string]any, key string) bool {
	if v, ok := m[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

func getInt64From(m map[string]any, key string) int64 {
	if v, ok := m[key]; ok {
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

// PersistentOrchestrator wraps Orchestrator with automatic persistence.
type PersistentOrchestrator struct {
	*Orchestrator
	store *PersistenceStore
}

// NewPersistentOrchestrator creates an orchestrator with persistence.
func NewPersistentOrchestrator(db graph.Driver, sessionID string) *PersistentOrchestrator {
	orch := New()
	store := NewPersistenceStore(db, sessionID)
	return &PersistentOrchestrator{
		Orchestrator: orch,
		store:        store,
	}
}

// RecordResults persists execution results from a completed run.
func (po *PersistentOrchestrator) RecordResults(
	ctx context.Context,
	description string,
	results map[string]*TaskResult,
	duration time.Duration,
) (*OrchestratorRun, error) {
	// Start run record
	runID, err := po.store.StartRun(ctx, description, len(results))
	if err != nil {
		return nil, fmt.Errorf("failed to start run: %w", err)
	}

	// Record each result
	for _, result := range results {
		if err := po.store.RecordTaskResult(ctx, runID, result); err != nil {
			// Log but continue
			fmt.Printf("Warning: failed to record result for %s: %v\n", result.TaskID, err)
		}
	}

	// Complete run
	if err := po.store.CompleteRun(ctx, runID, duration.Milliseconds()); err != nil {
		return nil, fmt.Errorf("failed to complete run: %w", err)
	}

	// Return run summary
	return po.store.GetRun(ctx, runID)
}

// ListRuns returns recent orchestration runs.
func (po *PersistentOrchestrator) ListRuns(ctx context.Context, limit int) ([]OrchestratorRun, error) {
	return po.store.ListRuns(ctx, limit)
}

// GetRunStats returns aggregate statistics.
func (po *PersistentOrchestrator) GetRunStats(ctx context.Context) (map[string]any, error) {
	return po.store.GetStats(ctx)
}
