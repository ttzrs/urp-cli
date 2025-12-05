// Package cognitive provides novelty detection.
package cognitive

import (
	"context"

	"github.com/joss/urp/internal/graph"
	"github.com/joss/urp/internal/vector"
)

// NoveltyService detects unusual patterns in code.
type NoveltyService struct {
	db       graph.GraphReader // ISP: only needs read operations
	vectors  vector.Store
	embedder vector.Embedder
}

// NewNoveltyService creates a new novelty service.
func NewNoveltyService(db graph.GraphReader) *NoveltyService {
	return &NoveltyService{
		db:       db,
		vectors:  vector.Default(),
		embedder: vector.GetDefaultEmbedder(),
	}
}

// NewNoveltyServiceWithVectors creates a novelty service with custom vector store.
func NewNoveltyServiceWithVectors(db graph.GraphReader, store vector.Store, embedder vector.Embedder) *NoveltyService {
	return &NoveltyService{
		db:       db,
		vectors:  store,
		embedder: embedder,
	}
}

// NoveltyResult contains the novelty analysis.
type NoveltyResult struct {
	Novelty float64 `json:"novelty"`
	Level   string  `json:"level"`
	Message string  `json:"message"`
	Matches int     `json:"matches"`
}

// CheckNovelty analyzes how novel a piece of code is.
// Uses vector similarity when available, falls back to keyword matching.
func (n *NoveltyService) CheckNovelty(ctx context.Context, code string) (*NoveltyResult, error) {
	// Try vector search first
	if n.vectors != nil && n.embedder != nil {
		result, err := n.vectorNovelty(ctx, code)
		if err == nil && result != nil {
			return result, nil
		}
		// Fall through to Jaccard if vector search fails
	}

	return n.jaccardNovelty(ctx, code)
}

// vectorNovelty uses embedding-based similarity to detect novelty.
func (n *NoveltyService) vectorNovelty(ctx context.Context, code string) (*NoveltyResult, error) {
	// Generate embedding for the code
	codeVec, err := n.embedder.Embed(ctx, code)
	if err != nil {
		return nil, err
	}

	// Search for similar patterns in vector store
	results, err := n.vectors.Search(ctx, codeVec, 10, "code")
	if err != nil {
		return nil, err
	}

	if len(results) == 0 {
		return &NoveltyResult{
			Novelty: 1.0,
			Level:   "pioneer",
			Message: "No similar patterns in vector store. You are the pioneer.",
			Matches: 0,
		}, nil
	}

	// Calculate novelty from best matches
	// Higher similarity = lower novelty
	totalSim := float64(0)
	for _, r := range results {
		totalSim += float64(r.Score)
	}
	avgSim := totalSim / float64(len(results))

	result := &NoveltyResult{
		Novelty: 1.0 - avgSim,
		Matches: len(results),
	}

	// Clamp to 0-1
	if result.Novelty < 0 {
		result.Novelty = 0
	}
	if result.Novelty > 1 {
		result.Novelty = 1
	}

	// Classify
	switch {
	case result.Novelty < 0.3:
		result.Level = "safe"
		result.Message = "Standard pattern. Safe to proceed."
	case result.Novelty < 0.7:
		result.Level = "moderate"
		result.Message = "Some innovation. Review recommended."
	default:
		result.Level = "high"
		result.Message = "Novel pattern! Verify this is intentional."
	}

	return result, nil
}

// IndexCode stores code in the vector database for future novelty checks.
func (n *NoveltyService) IndexCode(ctx context.Context, code, signature, path string) error {
	if n.vectors == nil || n.embedder == nil {
		return nil // Silently skip if no vector store
	}

	vec, err := n.embedder.Embed(ctx, code)
	if err != nil {
		return err
	}

	entry := vector.VectorEntry{
		Text:   code,
		Vector: vec,
		Kind:   "code",
		Metadata: map[string]string{
			"signature": signature,
			"path":      path,
		},
	}

	return n.vectors.Add(ctx, entry)
}

// jaccardNovelty uses keyword-based similarity (fallback).
func (n *NoveltyService) jaccardNovelty(ctx context.Context, code string) (*NoveltyResult, error) {
	result := &NoveltyResult{
		Novelty: 0.5,
		Level:   "unknown",
		Message: "Could not calculate novelty",
	}

	// Extract keywords from the code
	keywords := tokenize(code)
	if len(keywords) == 0 {
		result.Novelty = 1.0
		result.Level = "pioneer"
		result.Message = "No recognizable patterns. You are the pioneer."
		return result, nil
	}

	// Search for similar function signatures in the graph
	query := `
		MATCH (f:Function)
		WHERE f.signature IS NOT NULL
		RETURN f.signature as signature, f.name as name
		LIMIT 200
	`

	records, err := n.db.Execute(ctx, query, nil)
	if err != nil {
		return result, err
	}

	if len(records) == 0 {
		result.Novelty = 1.0
		result.Level = "pioneer"
		result.Message = "No existing patterns. You are the pioneer."
		return result, nil
	}

	// Calculate average similarity to existing code
	totalSim := 0.0
	matches := 0

	for _, r := range records {
		sig := graph.GetString(r, "signature")
		if sig == "" {
			continue
		}

		sigWords := tokenize(sig)
		sim := jaccardSimilarity(keywords, sigWords)

		if sim > 0.1 {
			totalSim += sim
			matches++
		}
	}

	result.Matches = matches

	if matches == 0 {
		// No similar patterns found
		result.Novelty = 0.9
		result.Level = "high"
		result.Message = "Novel pattern! No similar code found in codebase."
		return result, nil
	}

	// Novelty is inverse of similarity
	avgSim := totalSim / float64(matches)
	result.Novelty = 1.0 - avgSim

	// Normalize to 0-1 range
	if result.Novelty < 0 {
		result.Novelty = 0
	}
	if result.Novelty > 1 {
		result.Novelty = 1
	}

	// Classify
	switch {
	case result.Novelty < 0.3:
		result.Level = "safe"
		result.Message = "Standard pattern. Safe to proceed."
	case result.Novelty < 0.7:
		result.Level = "moderate"
		result.Message = "Some innovation. Review recommended."
	default:
		result.Level = "high"
		result.Message = "Novel pattern! Verify this is intentional."
	}

	return result, nil
}
