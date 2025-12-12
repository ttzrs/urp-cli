package graph

import (
	"context"
	"fmt"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// ComputedState represents the "Rendered View" of the current system state.
type ComputedState struct {
	ActiveErrors  []string
	ModifiedFiles []string
}

// InitSession creates a new session node in the graph.
func (c *Client) InitSession(ctx context.Context, sessionID, goal string) error {
	query := `
		MERGE (s:Session {id: $id})
		SET s.goal = $goal, s.start_time = timestamp()
		RETURN s
	`
	params := map[string]any{
		"id":   sessionID,
		"goal": goal,
	}

	session := c.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: c.dbName})
	defer session.Close(ctx)

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		_, err := tx.Run(ctx, query, params)
		return nil, err
	})
	return err
}

// GetComputedState fetches the "Current Truth" for the compiler.
func (c *Client) GetComputedState(ctx context.Context, sessionID string) (*ComputedState, error) {
	query := `
		MATCH (s:Session {id: $id})
		// Optional: Active Errors
		OPTIONAL MATCH (s)-[:HAS_ERROR]->(e:Error {status: "active"})
		// Optional: Modified Files (last 5 interactions or flagged as modified)
		OPTIONAL MATCH (s)-[:HAS_FILE]->(f:File {status: "modified"})
		
		RETURN collect(DISTINCT e.msg) as errors, collect(DISTINCT f.path) as files
	`
	params := map[string]any{"id": sessionID}

	session := c.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: c.dbName})
	defer session.Close(ctx)

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, query, params)
		if err != nil {
			return nil, err
		}
		
		record, err := res.Single(ctx)
		if err != nil {
			return nil, err
		}

		state := &ComputedState{}

		// Parse Errors
		if errsRaw, ok := record.Get("errors"); ok && errsRaw != nil {
			for _, e := range errsRaw.([]any) {
				if eStr, ok := e.(string); ok {
					state.ActiveErrors = append(state.ActiveErrors, eStr)
				}
			}
		}

		// Parse Files
		if filesRaw, ok := record.Get("files"); ok && filesRaw != nil {
			for _, f := range filesRaw.([]any) {
				if fStr, ok := f.(string); ok {
					state.ModifiedFiles = append(state.ModifiedFiles, fStr)
				}
			}
		}

		return state, nil
	})

	if err != nil {
		return nil, err
	}

	return result.(*ComputedState), nil
}

// AddMockData injects some test nodes to verify retrieval
func (c *Client) AddMockData(ctx context.Context, sessionID string) error {
	query := `
		MATCH (s:Session {id: $id})
		CREATE (e:Error {msg: "Connection Refused on port 8080", status: "active"})
		CREATE (f:File {path: "/internal/compiler/compiler.go", status: "modified"})
		CREATE (s)-[:HAS_ERROR]->(e)
		CREATE (s)-[:HAS_FILE]->(f)
	`
	params := map[string]any{"id": sessionID}

	session := c.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: c.dbName})
	defer session.Close(ctx)

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		_, err := tx.Run(ctx, query, params)
		return nil, err
	})
	return err
}

// CreateTheoreticalRule persists a "Theorist" hypothesis into the graph.
// It maps to the architecture: (:Rule {status: "THEORETICAL"})
func (c *Client) CreateTheoreticalRule(ctx context.Context, source, proposition string, confidence float64) error {
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

	session := c.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: c.dbName})
	defer session.Close(ctx)

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		_, err := tx.Run(ctx, query, params)
		return nil, err
	})
	return err
}

// GetRelevantRules fetches rules that might help achieve the goal.
// In a full implementation, this would use Vector Search (LanceDB).
// For this prototype, we return recent rules.
func (c *Client) GetRelevantRules(ctx context.Context, goal string) ([]string, error) {
	// Simple retrieval: Get top 5 recent rules
	query := `
		MATCH (r:Rule)
		RETURN r.proposition as prop, r.status as status, r.confidence as conf
		ORDER BY r.created_at DESC
		LIMIT 5
	`
	
	session := c.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: c.dbName})
	defer session.Close(ctx)

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, query, nil)
		if err != nil {
			return nil, err
		}
		
		var rules []string
		for res.Next(ctx) {
			rec := res.Record()
			prop, _ := rec.Get("prop")
			status, _ := rec.Get("status")
			conf, _ := rec.Get("conf")
			
			ruleStr := fmt.Sprintf("[%s] %s (Conf: %.2f)", status, prop, conf)
			rules = append(rules, ruleStr)
		}
		return rules, nil
	})

	if err != nil {
		return nil, err
	}
	return result.([]string), nil
}
