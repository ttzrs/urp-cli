// Package cognitive provides solution learning.
package cognitive

import (
	"context"
	"fmt"
	"time"

	"github.com/joss/urp/internal/graph"
)

// LearningService consolidates successful command sequences.
type LearningService struct {
	db graph.Driver
}

// NewLearningService creates a new learning service.
func NewLearningService(db graph.Driver) *LearningService {
	return &LearningService{db: db}
}

// LearnResult contains the learning outcome.
type LearnResult struct {
	Success           bool     `json:"success"`
	SolutionID        string   `json:"solution_id,omitempty"`
	Description       string   `json:"description,omitempty"`
	CommandsLinked    int      `json:"commands_linked,omitempty"`
	ConflictsResolved int      `json:"conflicts_resolved,omitempty"`
	Error             string   `json:"error,omitempty"`
}

// ConsolidateLearning creates a Solution node from recent successful commands.
func (l *LearningService) ConsolidateLearning(ctx context.Context, description string, minutes int) (*LearnResult, error) {
	cutoff := time.Now().Add(-time.Duration(minutes) * time.Minute).Unix()

	// Find recent successful commands
	query := `
		MATCH (e:TerminalEvent)
		WHERE e.timestamp > $cutoff
		  AND NOT e:Conflict
		RETURN e.command as command,
		       e.timestamp as ts,
		       id(e) as node_id
		ORDER BY e.timestamp ASC
	`

	records, err := l.db.Execute(ctx, query, map[string]any{"cutoff": cutoff})
	if err != nil {
		return &LearnResult{Success: false, Error: err.Error()}, nil
	}

	if len(records) == 0 {
		return &LearnResult{
			Success: false,
			Error:   fmt.Sprintf("No successful commands in the last %d minutes", minutes),
		}, nil
	}

	// Create Solution node
	solutionID := fmt.Sprintf("sol_%d", time.Now().Unix())

	createQuery := `
		CREATE (s:Solution {
			id: $sol_id,
			description: $desc,
			created_at: $ts,
			command_count: $count
		})
	`

	err = l.db.ExecuteWrite(ctx, createQuery, map[string]any{
		"sol_id": solutionID,
		"desc":   description,
		"ts":     time.Now().Unix(),
		"count":  len(records),
	})
	if err != nil {
		return &LearnResult{Success: false, Error: err.Error()}, nil
	}

	// Link events to solution
	linkedCount := 0
	for _, r := range records {
		nodeID := getInt64(r, "node_id")
		linkQuery := `
			MATCH (e:TerminalEvent) WHERE id(e) = $eid
			MATCH (s:Solution {id: $sol_id})
			MERGE (e)-[:CONTRIBUTED_TO]->(s)
		`
		if err := l.db.ExecuteWrite(ctx, linkQuery, map[string]any{
			"eid":    nodeID,
			"sol_id": solutionID,
		}); err == nil {
			linkedCount++
		}
	}

	// Find and link any conflicts that preceded this success
	var firstTS int64
	if ts, ok := records[0]["ts"].(int64); ok {
		firstTS = ts
	}

	conflictQuery := `
		MATCH (c:Conflict)
		WHERE c.timestamp > $cutoff_before AND c.timestamp < $first_success
		RETURN id(c) as node_id
	`

	conflictRecords, _ := l.db.Execute(ctx, conflictQuery, map[string]any{
		"cutoff_before": cutoff - 300, // 5 minutes before cutoff
		"first_success": firstTS,
	})

	resolvedCount := 0
	for _, cr := range conflictRecords {
		nodeID := getInt64(cr, "node_id")
		resolveQuery := `
			MATCH (c:Conflict) WHERE id(c) = $cid
			MATCH (s:Solution {id: $sol_id})
			MERGE (s)-[:RESOLVES]->(c)
		`
		if err := l.db.ExecuteWrite(ctx, resolveQuery, map[string]any{
			"cid":    nodeID,
			"sol_id": solutionID,
		}); err == nil {
			resolvedCount++
		}
	}

	return &LearnResult{
		Success:           true,
		SolutionID:        solutionID,
		Description:       description,
		CommandsLinked:    linkedCount,
		ConflictsResolved: resolvedCount,
	}, nil
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
