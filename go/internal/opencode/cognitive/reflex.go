// Package cognitive implements the "nervous system" for the agent
// It handles automatic context enrichment on errors (trauma)
package cognitive

import (
	"context"
	"fmt"
	"strings"

	"github.com/joss/urp/internal/graph"
)

// Reflex handles automatic context enrichment when errors occur
type Reflex struct {
	db       *graph.Memgraph
	workDir  string
	maxDepth int
}

// TraumaContext is the enriched context injected after an error
type TraumaContext struct {
	Error          string
	RelatedFiles   []string
	SimilarErrors  []SimilarError
	GraphNeighbors []string
}

// SimilarError represents a past error and its solution (from wisdom)
type SimilarError struct {
	Error    string
	Solution string
	Score    float64
}

// NewReflex creates a new reflex handler
func NewReflex(db *graph.Memgraph, workDir string) *Reflex {
	return &Reflex{
		db:       db,
		workDir:  workDir,
		maxDepth: 2,
	}
}

// HandleTrauma processes an error and returns enriched context
// This is the "pain response" - automatically gather relevant context
func (r *Reflex) HandleTrauma(ctx context.Context, errorOutput string, currentFile string) (*TraumaContext, error) {
	tc := &TraumaContext{
		Error: truncateError(errorOutput, 500),
	}

	// 1. Extract file paths from error message
	tc.RelatedFiles = extractFilePaths(errorOutput)

	// 2. Query graph for related files (if db available)
	if r.db != nil && currentFile != "" {
		neighbors, err := r.getGraphNeighbors(ctx, currentFile)
		if err == nil {
			tc.GraphNeighbors = neighbors
		}
	}

	// 3. Future: Query vector store for similar errors
	// tc.SimilarErrors = r.vectorStore.Search(ctx, errorOutput, 3)

	return tc, nil
}

// FormatEmergencyContext formats the trauma context for injection into the LLM
func (r *Reflex) FormatEmergencyContext(tc *TraumaContext) string {
	var b strings.Builder

	b.WriteString("⚠️ ERROR CONTEXT (Auto-injected by URP Cognitive System)\n\n")

	// Error summary
	b.WriteString("**Error:**\n```\n")
	b.WriteString(tc.Error)
	b.WriteString("\n```\n\n")

	// Related files from error
	if len(tc.RelatedFiles) > 0 {
		b.WriteString("**Files mentioned in error:**\n")
		for _, f := range tc.RelatedFiles {
			b.WriteString(fmt.Sprintf("- `%s`\n", f))
		}
		b.WriteString("\n")
	}

	// Graph neighbors
	if len(tc.GraphNeighbors) > 0 {
		b.WriteString("**Related files (from code graph):**\n")
		for _, f := range tc.GraphNeighbors {
			b.WriteString(fmt.Sprintf("- `%s`\n", f))
		}
		b.WriteString("\n")
	}

	// Similar past errors
	if len(tc.SimilarErrors) > 0 {
		b.WriteString("**Similar past errors (from wisdom):**\n")
		for _, se := range tc.SimilarErrors {
			b.WriteString(fmt.Sprintf("- Error: %s\n  Solution: %s\n", se.Error, se.Solution))
		}
		b.WriteString("\n")
	}

	return b.String()
}

// getGraphNeighbors queries Memgraph for files related to the error
func (r *Reflex) getGraphNeighbors(ctx context.Context, filePath string) ([]string, error) {
	if r.db == nil {
		return nil, nil
	}

	query := `
		MATCH (f:File {path: $path})-[*1..2]-(related:File)
		WHERE related.path <> $path
		RETURN DISTINCT related.path AS path
		LIMIT 10
	`

	records, err := r.db.Execute(ctx, query, map[string]any{"path": filePath})
	if err != nil {
		return nil, err
	}

	var paths []string
	for _, record := range records {
		if p, ok := record["path"].(string); ok {
			paths = append(paths, p)
		}
	}

	return paths, nil
}

// extractFilePaths extracts file paths from error output
func extractFilePaths(errorOutput string) []string {
	paths := make(map[string]bool)
	lines := strings.Split(errorOutput, "\n")

	for _, line := range lines {
		// Look for common file path patterns
		// Go: /path/to/file.go:123:45
		// Python: File "/path/to/file.py", line 123
		// Node: at /path/to/file.js:123:45

		words := strings.Fields(line)
		for _, word := range words {
			// Strip common prefixes/suffixes
			word = strings.TrimPrefix(word, "File")
			word = strings.TrimPrefix(word, "at")
			word = strings.Trim(word, `"'(),`)

			// Check if it looks like a file path
			if isFilePath(word) {
				// Remove line:col suffix
				if idx := strings.Index(word, ":"); idx > 0 {
					if isDigit(word[idx+1:]) {
						word = word[:idx]
					}
				}
				paths[word] = true
			}
		}
	}

	result := make([]string, 0, len(paths))
	for p := range paths {
		result = append(result, p)
	}
	return result
}

// isFilePath checks if a string looks like a file path
func isFilePath(s string) bool {
	if len(s) < 3 {
		return false
	}

	// Must contain a path separator and an extension
	hasSlash := strings.Contains(s, "/")
	hasDot := strings.Contains(s, ".")

	// Common extensions
	extensions := []string{".go", ".py", ".js", ".ts", ".rs", ".java", ".c", ".cpp", ".h"}
	hasExt := false
	for _, ext := range extensions {
		if strings.Contains(s, ext) {
			hasExt = true
			break
		}
	}

	return hasSlash && hasDot && hasExt
}

// isDigit checks if string starts with a digit
func isDigit(s string) bool {
	if len(s) == 0 {
		return false
	}
	return s[0] >= '0' && s[0] <= '9'
}

// truncateError truncates error output to max length
func truncateError(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
