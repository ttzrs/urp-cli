package cognitive

import (
	"context"
	"fmt"

	"github.com/joss/urp/internal/graph"
	"github.com/joss/urp/internal/vector"
)

// ContextOptimizer manages the hybrid search and spreading activation process.
type ContextOptimizer struct {
	graph  graph.GraphReader
	vector vector.VectorSearcher
}

// NewContextOptimizer creates a new context optimizer.
func NewContextOptimizer(g graph.GraphReader, v vector.VectorSearcher) *ContextOptimizer {
	return &ContextOptimizer{
		graph:  g,
		vector: v,
	}
}

// OptimizedFile represents a file selected by the cognitive process.
type OptimizedFile struct {
	Path   string  `json:"path"`
	Energy float64 `json:"energy"`
}

// GetOptimizedContext performs Hybrid Search + Spreading Activation using the high-performance Rust module.
func (c *ContextOptimizer) GetOptimizedContext(ctx context.Context, vectorEmbedding []float32) ([]OptimizedFile, error) {
	// 1. Vector Search (LanceDB)
	// Get top 20 semantic candidates to start the activation spreading
	results, err := c.vector.Search(ctx, vectorEmbedding, 20, "code")
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}

	if len(results) == 0 {
		return nil, nil
	}

	// Extract Paths from vector metadata
	var startPaths []string
	for _, r := range results {
		if path, ok := r.Entry.Metadata["path"]; ok {
			startPaths = append(startPaths, path)
		}
	}

	if len(startPaths) == 0 {
		return nil, nil
	}

	// 2. Spreading Activation (Memgraph + Rust)
	// We pass the paths, convert them to IDs in Cypher, and run the Rust module.
	// The Rust module 'libinferencia.optimize_context' takes [start_ids, decay, threshold].
	query := `
		MATCH (f:File) WHERE f.path IN $paths
		WITH collect(id(f)) as start_ids
		CALL libinferencia.optimize_context(start_ids, 0.8, 0.2) YIELD node_id, energy
		MATCH (target:File) WHERE id(target) = node_id
		RETURN target.path as path, energy
		ORDER BY energy DESC
		LIMIT 10
	`

	records, err := c.graph.Execute(ctx, query, map[string]any{"paths": startPaths})
	if err != nil {
		return nil, fmt.Errorf("spreading activation execution: %w", err)
	}

	var optimized []OptimizedFile
	for _, rec := range records {
		path, ok := rec["path"].(string)
		if !ok {
			continue
		}
		energy, _ := rec["energy"].(float64)

		optimized = append(optimized, OptimizedFile{
			Path:   path,
			Energy: energy,
		})
	}

	return optimized, nil
}
