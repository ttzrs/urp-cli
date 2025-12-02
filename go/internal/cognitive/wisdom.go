// Package cognitive provides AI-like reasoning capabilities.
package cognitive

import (
	"context"
	"math"
	"sort"

	"github.com/joss/urp/internal/graph"
)

// WisdomService finds similar past errors and solutions.
type WisdomService struct {
	db graph.Driver
}

// NewWisdomService creates a new wisdom service.
func NewWisdomService(db graph.Driver) *WisdomService {
	return &WisdomService{db: db}
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

// ConsultWisdom finds similar past errors using text similarity.
// Without embeddings, falls back to keyword matching.
func (w *WisdomService) ConsultWisdom(ctx context.Context, errorMsg string, threshold float64, project string) ([]Match, error) {
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
		pastError := getString(r, "error")
		if pastError == "" {
			continue
		}

		// Calculate Jaccard similarity on tokens
		pastWords := tokenize(pastError)
		sim := jaccardSimilarity(errorWords, pastWords)

		if sim >= threshold {
			matches = append(matches, Match{
				Command:    getString(r, "command"),
				Error:      pastError,
				Time:       getString(r, "time"),
				Project:    getString(r, "project"),
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

	if sol := getString(records[0], "solution"); sol != "" {
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

func getString(r graph.Record, key string) string {
	if v, ok := r[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
