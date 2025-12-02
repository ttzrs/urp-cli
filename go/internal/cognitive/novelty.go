// Package cognitive provides novelty detection.
package cognitive

import (
	"context"

	"github.com/joss/urp/internal/graph"
)

// NoveltyService detects unusual patterns in code.
type NoveltyService struct {
	db graph.Driver
}

// NewNoveltyService creates a new novelty service.
func NewNoveltyService(db graph.Driver) *NoveltyService {
	return &NoveltyService{db: db}
}

// NoveltyResult contains the novelty analysis.
type NoveltyResult struct {
	Novelty float64 `json:"novelty"`
	Level   string  `json:"level"`
	Message string  `json:"message"`
	Matches int     `json:"matches"`
}

// CheckNovelty analyzes how novel a piece of code is.
// Without embeddings, uses keyword matching against existing code.
func (n *NoveltyService) CheckNovelty(ctx context.Context, code string) (*NoveltyResult, error) {
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
		sig := getString(r, "signature")
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
