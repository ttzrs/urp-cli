package tool

import (
	"context"
	"fmt"
	"strings"

	"github.com/joss/urp/internal/opencode/domain"
	"github.com/joss/urp/internal/vector"
)

// CodeSearch performs semantic search over indexed code
type CodeSearch struct {
	store    vector.VectorSearcher // ISP: only needs search operations
	embedder vector.Embedder
}

// NewCodeSearch creates a new CodeSearch tool
func NewCodeSearch(store vector.VectorSearcher, embedder vector.Embedder) *CodeSearch {
	return &CodeSearch{
		store:    store,
		embedder: embedder,
	}
}

func (c *CodeSearch) Info() domain.Tool {
	return domain.Tool{
		Name:        "code_search",
		Description: "Search code semantically using vector embeddings",
		Parameters: domain.JSONSchema{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Natural language query to search for",
				},
				"limit": map[string]any{
					"type":        "integer",
					"description": "Maximum results to return (default: 5)",
				},
				"kind": map[string]any{
					"type":        "string",
					"description": "Filter by kind: code, error, solution, knowledge",
					"enum":        []string{"code", "error", "solution", "knowledge", ""},
				},
			},
			"required": []string{"query"},
		},
	}
}

func (c *CodeSearch) Execute(ctx context.Context, args map[string]any) (*Result, error) {
	query, _ := args["query"].(string)
	if query == "" {
		return &Result{Error: fmt.Errorf("query is required")}, nil
	}

	limit := 5
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}

	kind, _ := args["kind"].(string)

	// Check if store/embedder are configured
	if c.store == nil || c.embedder == nil {
		return &Result{
			Error: fmt.Errorf("code search not configured (vector store or embedder missing)"),
		}, nil
	}

	// Generate embedding for query
	embedding, err := c.embedder.Embed(ctx, query)
	if err != nil {
		return &Result{Error: fmt.Errorf("embed query: %w", err)}, nil
	}

	// Search
	results, err := c.store.Search(ctx, embedding, limit, kind)
	if err != nil {
		return &Result{Error: fmt.Errorf("search: %w", err)}, nil
	}

	if len(results) == 0 {
		return &Result{
			Title:  "Code search",
			Output: "No matching code found",
		}, nil
	}

	// Format results
	var sb strings.Builder
	for i, r := range results {
		sb.WriteString(fmt.Sprintf("%d. [%.2f] %s\n", i+1, r.Score, r.Entry.Kind))
		if file, ok := r.Entry.Metadata["file"]; ok {
			sb.WriteString(fmt.Sprintf("   File: %s", file))
			if line, ok := r.Entry.Metadata["line"]; ok {
				sb.WriteString(fmt.Sprintf(":%s", line))
			}
			sb.WriteString("\n")
		}
		// Truncate long text
		text := r.Entry.Text
		if len(text) > 200 {
			text = text[:197] + "..."
		}
		sb.WriteString(fmt.Sprintf("   %s\n\n", text))
	}

	return &Result{
		Title:  fmt.Sprintf("Code search: %d results", len(results)),
		Output: sb.String(),
		Metadata: map[string]any{
			"query":   query,
			"count":   len(results),
			"kind":    kind,
			"results": results,
		},
	}, nil
}
