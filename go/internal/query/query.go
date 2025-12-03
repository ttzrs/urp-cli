// Package query provides graph queries for code analysis.
package query

import (
	"context"
	"fmt"

	"github.com/joss/urp/internal/graph"
)

// Querier provides code analysis queries.
type Querier struct {
	db graph.Driver
}

// NewQuerier creates a new querier.
func NewQuerier(db graph.Driver) *Querier {
	return &Querier{db: db}
}

// Impact represents a function that would be affected by a change.
type Impact struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Distance  int    `json:"distance"`
	Signature string `json:"signature,omitempty"`
}

// FindImpact returns functions affected by changing the target (Φ inverse).
func (q *Querier) FindImpact(ctx context.Context, signature string, maxDepth int) ([]Impact, error) {
	// Note: Memgraph uses size() instead of length() for path length
	query := fmt.Sprintf(`
		MATCH (target)
		WHERE target.name CONTAINS $sig OR target.signature CONTAINS $sig
		MATCH path = (caller)-[:CALLS*1..%d]->(target)
		RETURN DISTINCT
		    caller.name as name,
		    caller.path as path,
		    caller.signature as signature,
		    size(path) as distance
		ORDER BY distance
		LIMIT 50
	`, maxDepth)

	records, err := q.db.Execute(ctx, query, map[string]any{"sig": signature})
	if err != nil {
		return nil, err
	}

	var impacts []Impact
	for _, r := range records {
		impacts = append(impacts, Impact{
			Name:      getString(r, "name"),
			Path:      getString(r, "path"),
			Signature: getString(r, "signature"),
			Distance:  getInt(r, "distance"),
		})
	}

	return impacts, nil
}

// Dependency represents a function dependency.
type Dependency struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Distance  int    `json:"distance"`
	Signature string `json:"signature,omitempty"`
}

// FindDeps returns dependencies of the target (Φ forward).
func (q *Querier) FindDeps(ctx context.Context, signature string, maxDepth int) ([]Dependency, error) {
	// Note: Memgraph uses size() instead of length() for path length
	query := fmt.Sprintf(`
		MATCH (source)
		WHERE source.name CONTAINS $sig OR source.signature CONTAINS $sig
		MATCH path = (source)-[:CALLS*1..%d]->(dep)
		RETURN DISTINCT
		    dep.name as name,
		    dep.path as path,
		    dep.signature as signature,
		    size(path) as distance
		ORDER BY distance
		LIMIT 50
	`, maxDepth)

	records, err := q.db.Execute(ctx, query, map[string]any{"sig": signature})
	if err != nil {
		return nil, err
	}

	var deps []Dependency
	for _, r := range records {
		deps = append(deps, Dependency{
			Name:      getString(r, "name"),
			Path:      getString(r, "path"),
			Signature: getString(r, "signature"),
			Distance:  getInt(r, "distance"),
		})
	}

	return deps, nil
}

// DeadCode represents unused code.
type DeadCode struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Type      string `json:"type"`
	Signature string `json:"signature,omitempty"`
}

// FindDeadCode returns functions that are never called (⊥ unused).
func (q *Querier) FindDeadCode(ctx context.Context) ([]DeadCode, error) {
	// Memgraph-compatible: use OPTIONAL MATCH + WHERE null check instead of NOT EXISTS pattern
	query := `
		MATCH (f:Function)
		WHERE NOT f.name STARTS WITH 'Test'
		  AND NOT f.name STARTS WITH 'main'
		  AND NOT f.name STARTS WITH 'init'
		OPTIONAL MATCH (caller)-[:CALLS]->(f)
		WITH f, caller
		WHERE caller IS NULL
		RETURN f.name as name,
		       f.path as path,
		       f.signature as signature,
		       'Function' as type
		ORDER BY f.path, f.name
		LIMIT 100
	`

	records, err := q.db.Execute(ctx, query, nil)
	if err != nil {
		return nil, err
	}

	var dead []DeadCode
	for _, r := range records {
		dead = append(dead, DeadCode{
			Name:      getString(r, "name"),
			Path:      getString(r, "path"),
			Type:      getString(r, "type"),
			Signature: getString(r, "signature"),
		})
	}

	return dead, nil
}

// Cycle represents a circular dependency.
type Cycle struct {
	Path []string `json:"path"`
}

// FindCycles returns circular dependencies (⊥ conflict).
func (q *Querier) FindCycles(ctx context.Context) ([]Cycle, error) {
	// Memgraph-compatible: extract path nodes in the query itself
	// Use UNWIND to extract node names from the path
	query := `
		MATCH p = (a:Function)-[:CALLS*2..5]->(a)
		WITH p, a
		UNWIND nodes(p) as node
		WITH p, a, collect(node.name) as names
		RETURN names as cycle
		LIMIT 20
	`

	records, err := q.db.Execute(ctx, query, nil)
	if err != nil {
		return nil, err
	}

	var cycles []Cycle
	for _, r := range records {
		if path, ok := r["cycle"].([]any); ok {
			var names []string
			for _, n := range path {
				if s, ok := n.(string); ok {
					names = append(names, s)
				}
			}
			cycles = append(cycles, Cycle{Path: names})
		}
	}

	return cycles, nil
}

// Hotspot represents a risky area of code.
type Hotspot struct {
	Path    string  `json:"path"`
	Commits int     `json:"commits"`
	Authors int     `json:"authors"`
	Score   float64 `json:"score"`
}

// FindHotspots returns high-churn areas (τ + Φ combined).
func (q *Querier) FindHotspots(ctx context.Context, days int) ([]Hotspot, error) {
	query := `
		MATCH (c:Commit)-[:TOUCHED]->(f:File)
		WHERE c.timestamp > $cutoff
		WITH f, count(DISTINCT c) as commits, count(DISTINCT c.author) as authors
		RETURN f.path as path,
		       commits,
		       authors,
		       (commits * 0.6 + authors * 0.4) as score
		ORDER BY score DESC
		LIMIT 20
	`

	cutoff := int64(0) // TODO: Calculate from days
	records, err := q.db.Execute(ctx, query, map[string]any{"cutoff": cutoff})
	if err != nil {
		return nil, err
	}

	var hotspots []Hotspot
	for _, r := range records {
		hotspots = append(hotspots, Hotspot{
			Path:    getString(r, "path"),
			Commits: getInt(r, "commits"),
			Authors: getInt(r, "authors"),
			Score:   getFloat(r, "score"),
		})
	}

	return hotspots, nil
}

// FileHistory represents a file's change history.
type FileHistory struct {
	Hash      string `json:"hash"`
	Author    string `json:"author"`
	Message   string `json:"message"`
	Timestamp string `json:"timestamp"`
}

// GetHistory returns file change history (τ sequence).
func (q *Querier) GetHistory(ctx context.Context, path string, limit int) ([]FileHistory, error) {
	query := `
		MATCH (c:Commit)-[:TOUCHED]->(f:File)
		WHERE f.path CONTAINS $path
		MATCH (a:Author)-[:AUTHORED]->(c)
		RETURN c.hash as hash,
		       a.name as author,
		       c.message as message,
		       c.datetime as timestamp
		ORDER BY c.timestamp DESC
		LIMIT $limit
	`

	records, err := q.db.Execute(ctx, query, map[string]any{
		"path":  path,
		"limit": limit,
	})
	if err != nil {
		return nil, err
	}

	var history []FileHistory
	for _, r := range records {
		history = append(history, FileHistory{
			Hash:      getString(r, "hash"),
			Author:    getString(r, "author"),
			Message:   getString(r, "message"),
			Timestamp: getString(r, "timestamp"),
		})
	}

	return history, nil
}

// GraphStats contains graph statistics.
type GraphStats struct {
	Files      int `json:"files"`
	Functions  int `json:"functions"`
	Structs    int `json:"structs"`
	Commits    int `json:"commits"`
	Authors    int `json:"authors"`
	Calls      int `json:"calls"`
	Events     int `json:"events"`
	Conflicts  int `json:"conflicts"`
}

// GetStats returns graph statistics.
// Uses a single batched query instead of 8 sequential queries.
func (q *Querier) GetStats(ctx context.Context) (*GraphStats, error) {
	// Single query that counts all node types and relationships
	query := `
		OPTIONAL MATCH (f:File) WITH count(f) as files
		OPTIONAL MATCH (fn:Function) WITH files, count(fn) as functions
		OPTIONAL MATCH (s:Struct) WITH files, functions, count(s) as structs
		OPTIONAL MATCH (c:Commit) WITH files, functions, structs, count(c) as commits
		OPTIONAL MATCH (a:Author) WITH files, functions, structs, commits, count(a) as authors
		OPTIONAL MATCH (e:TerminalEvent) WITH files, functions, structs, commits, authors, count(e) as events
		OPTIONAL MATCH (x:Conflict) WITH files, functions, structs, commits, authors, events, count(x) as conflicts
		OPTIONAL MATCH ()-[r:CALLS]->()
		RETURN files, functions, structs, commits, authors, events, conflicts, count(r) as calls
	`

	records, err := q.db.Execute(ctx, query, nil)
	if err != nil {
		return nil, fmt.Errorf("stats query failed: %w", err)
	}

	stats := &GraphStats{}
	if len(records) > 0 {
		r := records[0]
		stats.Files = getInt(r, "files")
		stats.Functions = getInt(r, "functions")
		stats.Structs = getInt(r, "structs")
		stats.Commits = getInt(r, "commits")
		stats.Authors = getInt(r, "authors")
		stats.Events = getInt(r, "events")
		stats.Conflicts = getInt(r, "conflicts")
		stats.Calls = getInt(r, "calls")
	}

	return stats, nil
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
