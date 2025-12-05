// Package cognitive provides AI-like reasoning capabilities.
package cognitive

import (
	"context"
	"math"
	"sort"

	"github.com/joss/urp/internal/graph"
	"github.com/joss/urp/internal/vector"
)

// WisdomService finds similar past errors and solutions.
type WisdomService struct {
	db       graph.Driver
	vectors  vector.Store
	embedder vector.Embedder
}

// NewWisdomService creates a new wisdom service.
func NewWisdomService(db graph.Driver) *WisdomService {
	return &WisdomService{
		db:       db,
		vectors:  vector.Default(),
		embedder: vector.GetDefaultEmbedder(),
	}
}

// NewWisdomServiceWithVectors creates a wisdom service with custom vector store.
func NewWisdomServiceWithVectors(db graph.Driver, store vector.Store, embedder vector.Embedder) *WisdomService {
	return &WisdomService{
		db:       db,
		vectors:  store,
		embedder: embedder,
	}
}

// Match represents a similar past error.
type Match struct {
	Command    string  `json:"command"`
	Error      string  `json:"error"`
	Time       string  `json:"time"`
	Project    string  `json:"project"`
	Similarity float64 `json:"similarity"`
	Solution   string  `json:"solution,omitempty"`
}

// ConsultWisdom finds similar past errors using vector similarity.
// Falls back to Jaccard keyword matching if vectors unavailable.
func (w *WisdomService) ConsultWisdom(ctx context.Context, errorMsg string, threshold float64, project string) ([]Match, error) {
	// Try vector search first
	if w.vectors != nil && w.embedder != nil {
		matches, err := w.vectorSearch(ctx, errorMsg, threshold, project)
		if err == nil && len(matches) > 0 {
			return matches, nil
		}
		// Fall through to Jaccard if vector search fails
	}

	return w.jaccardSearch(ctx, errorMsg, threshold, project)
}

// vectorSearch uses embedding-based similarity search.
func (w *WisdomService) vectorSearch(ctx context.Context, errorMsg string, threshold float64, project string) ([]Match, error) {
	// Generate embedding for query
	queryVec, err := w.embedder.Embed(ctx, errorMsg)
	if err != nil {
		return nil, err
	}

	// Search vector store
	results, err := w.vectors.Search(ctx, queryVec, 10, "error")
	if err != nil {
		return nil, err
	}

	var matches []Match
	for _, r := range results {
		sim := float64(r.Score)
		if sim < threshold {
			continue
		}

		// Filter by project if specified
		if project != "" {
			if p, ok := r.Entry.Metadata["project"]; ok && p != project {
				continue
			}
		}

		matches = append(matches, Match{
			Command:    r.Entry.Metadata["command"],
			Error:      r.Entry.Text,
			Time:       r.Entry.Metadata["time"],
			Project:    r.Entry.Metadata["project"],
			Similarity: math.Round(sim*1000) / 1000,
		})
	}

	// Find solutions for matches
	for i := range matches {
		w.findSolution(ctx, &matches[i])
	}

	return matches, nil
}

// jaccardSearch uses keyword-based Jaccard similarity (fallback).
func (w *WisdomService) jaccardSearch(ctx context.Context, errorMsg string, threshold float64, project string) ([]Match, error) {
	projectClause := ""
	params := map[string]any{
		"error": errorMsg,
	}

	if project != "" {
		projectClause = "AND e.project = $project"
		params["project"] = project
	}

	// Query recent conflicts
	query := `
		MATCH (e:Conflict)
		WHERE e.stderr_preview IS NOT NULL ` + projectClause + `
		RETURN e.command as command,
		       e.stderr_preview as error,
		       e.datetime as time,
		       e.project as project
		ORDER BY e.timestamp DESC
		LIMIT 100
	`

	records, err := w.db.Execute(ctx, query, params)
	if err != nil {
		return nil, err
	}

	// Calculate similarity using simple text matching
	var matches []Match
	errorWords := tokenize(errorMsg)

	for _, r := range records {
		pastError := graph.GetString(r, "error")
		if pastError == "" {
			continue
		}

		// Calculate Jaccard similarity on tokens
		pastWords := tokenize(pastError)
		sim := jaccardSimilarity(errorWords, pastWords)

		if sim >= threshold {
			matches = append(matches, Match{
				Command:    graph.GetString(r, "command"),
				Error:      pastError,
				Time:       graph.GetString(r, "time"),
				Project:    graph.GetString(r, "project"),
				Similarity: math.Round(sim*1000) / 1000,
			})
		}
	}

	// Sort by similarity descending
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Similarity > matches[j].Similarity
	})

	// Return top 5
	if len(matches) > 5 {
		matches = matches[:5]
	}

	// Try to find solutions for matched errors
	for i := range matches {
		w.findSolution(ctx, &matches[i])
	}

	return matches, nil
}

// IndexError stores an error in the vector database for future matching.
func (w *WisdomService) IndexError(ctx context.Context, errorMsg, command, project string) error {
	if w.vectors == nil || w.embedder == nil {
		return nil // Silently skip if no vector store
	}

	vec, err := w.embedder.Embed(ctx, errorMsg)
	if err != nil {
		return err
	}

	entry := vector.VectorEntry{
		Text:   errorMsg,
		Vector: vec,
		Kind:   "error",
		Metadata: map[string]string{
			"command": command,
			"project": project,
		},
	}

	return w.vectors.Add(ctx, entry)
}

func (w *WisdomService) findSolution(ctx context.Context, match *Match) {
	// Look for Solution nodes that resolve this error
	query := `
		MATCH (s:Solution)-[:RESOLVES]->(c:Conflict)
		WHERE c.stderr_preview CONTAINS $error
		RETURN s.description as solution
		LIMIT 1
	`

	records, err := w.db.Execute(ctx, query, map[string]any{
		"error": match.Error[:min(50, len(match.Error))],
	})
	if err != nil || len(records) == 0 {
		return
	}

	if sol := graph.GetString(records[0], "solution"); sol != "" {
		match.Solution = sol
	}
}

// tokenize splits text into lowercase words.
func tokenize(text string) map[string]bool {
	words := make(map[string]bool)
	current := ""

	for _, r := range text {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			current += string(r)
		} else {
			if len(current) > 2 {
				words[toLowerCase(current)] = true
			}
			current = ""
		}
	}
	if len(current) > 2 {
		words[toLowerCase(current)] = true
	}

	return words
}

func toLowerCase(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 32
		}
		result[i] = c
	}
	return string(result)
}

// jaccardSimilarity calculates Jaccard index between two sets.
func jaccardSimilarity(a, b map[string]bool) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 0
	}

	intersection := 0
	for word := range a {
		if b[word] {
			intersection++
		}
	}

	union := len(a)
	for word := range b {
		if !a[word] {
			union++
		}
	}

	if union == 0 {
		return 0
	}

	return float64(intersection) / float64(union)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
