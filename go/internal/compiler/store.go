package compiler

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/joss/urp/internal/graph"
)

// Store provides graph data access for the compiler.
type Store struct {
	db graph.Driver
}

// NewStore creates a new compiler store.
func NewStore(db graph.Driver) *Store {
	return &Store{db: db}
}

// ComputedState represents the "Rendered View" of the current system state.
type ComputedState struct {
	ActiveErrors  []string
	ModifiedFiles []string
}

// InitSession creates a new session node in the graph.
func (s *Store) InitSession(ctx context.Context, sessionID, goal string) error {
	query := `
		MERGE (s:Session {id: $id})
		SET s.goal = $goal, s.start_time = timestamp()
		RETURN s
	`
	params := map[string]any{
		"id":   sessionID,
		"goal": goal,
	}

	return s.db.ExecuteWrite(ctx, query, params)
}

// GetComputedState fetches the "Current Truth" for the compiler.
func (s *Store) GetComputedState(ctx context.Context, sessionID string) (*ComputedState, error) {
	query := `
		MATCH (s:Session {id: $id})
		// Optional: Active Errors
		OPTIONAL MATCH (s)-[:HAS_ERROR]->(e:Error {status: "active"})
		// Optional: Modified Files (last 5 interactions or flagged as modified)
		OPTIONAL MATCH (s)-[:HAS_FILE]->(f:File {status: "modified"})
		
		RETURN collect(DISTINCT e.msg) as errors, collect(DISTINCT f.path) as files
	`
	params := map[string]any{"id": sessionID}

	records, err := s.db.Execute(ctx, query, params)
	if err != nil {
		return nil, err
	}
	
	if len(records) == 0 {
		return &ComputedState{}, nil
	}

	record := records[0]
	state := &ComputedState{}

	// Parse Errors
	if errsRaw, ok := record["errors"]; ok && errsRaw != nil {
		if errsSlice, ok := errsRaw.([]any); ok {
			for _, e := range errsSlice {
				if eStr, ok := e.(string); ok {
					state.ActiveErrors = append(state.ActiveErrors, eStr)
				}
			}
		}
	}

	// Parse Files
	if filesRaw, ok := record["files"]; ok && filesRaw != nil {
		if filesSlice, ok := filesRaw.([]any); ok {
			for _, f := range filesSlice {
				if fStr, ok := f.(string); ok {
					state.ModifiedFiles = append(state.ModifiedFiles, fStr)
				}
			}
		}
	}

	return state, nil
}

// AddMockData injects some test nodes to verify retrieval
func (s *Store) AddMockData(ctx context.Context, sessionID string) error {
	query := `
		MATCH (s:Session {id: $id})
		CREATE (e:Error {msg: "Connection Refused on port 8080", status: "active"})
		CREATE (f:File {path: "/internal/compiler/compiler.go", status: "modified"})
		CREATE (s)-[:HAS_ERROR]->(e)
		CREATE (s)-[:HAS_FILE]->(f)
	`
	params := map[string]any{"id": sessionID}
	return s.db.ExecuteWrite(ctx, query, params)
}

// CreateTheoreticalRule persists a "Theorist" hypothesis into the graph.
// It maps to the architecture: (:Rule {status: "THEORETICAL"})
func (s *Store) CreateTheoreticalRule(ctx context.Context, source, proposition string, confidence float64) error {
	query := `
		MERGE (doc:Source {name: $source})
		CREATE (r:Rule {
			proposition: $proposition,
			status: "THEORETICAL",
			confidence: $confidence,
			created_at: timestamp()
		})
		CREATE (r)-[:DERIVED_FROM]->(doc)
		RETURN r
	`
	params := map[string]any{
		"source":      source,
		"proposition": proposition,
		"confidence":  confidence,
	}

	return s.db.ExecuteWrite(ctx, query, params)
}

// GetRelevantRules fetches rules that might help achieve the goal.
func (s *Store) GetRelevantRules(ctx context.Context, goal string) ([]string, error) {
	// Simple retrieval: Get top 5 recent rules
	query := `
		MATCH (r:Rule)
		RETURN r.proposition as prop, r.status as status, r.confidence as conf, r.created_at as created_at
		ORDER BY r.created_at DESC
		LIMIT 5
	`
	
	records, err := s.db.Execute(ctx, query, nil)
	if err != nil {
		return nil, err
	}
	
	var rules []string
	for _, rec := range records {
		prop := getString(rec, "prop")
		status := getString(rec, "status")
		conf := getFloat(rec, "conf")
		
		ruleStr := fmt.Sprintf("[%s] %s (Conf: %.2f)", status, prop, conf)
		rules = append(rules, ruleStr)
	}
	return rules, nil
}

// Helper functions for type safety
func getString(r graph.Record, key string) string {
	if v, ok := r[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func getFloat(r graph.Record, key string) float64 {
	if v, ok := r[key]; ok {
		if f, ok := v.(float64); ok {
			return f
		}
	}
	return 0.0
}

func getInt64(r graph.Record, key string) int64 {
	if v, ok := r[key]; ok {
		if i, ok := v.(int64); ok {
			return i
		}
	}
	return 0
}

func getJSON(r graph.Record, key string, target any) error {
	if v, ok := r[key]; ok {
		if s, ok := v.(string); ok {
			return json.Unmarshal([]byte(s), target)
		}
	}
	return fmt.Errorf("key not found or not string: %s", key)
}

// Mock methods to support time operations if needed
func (s *Store) PruneOldSessions(ctx context.Context, maxAge time.Duration) error {
    // Implementation placeholder
    return nil
}
