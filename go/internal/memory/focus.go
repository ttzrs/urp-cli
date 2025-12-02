// Package memory provides focused context loading.
package memory

import (
	"context"
	"fmt"
	"strings"

	"github.com/joss/urp/internal/graph"
)

// FocusResult contains the focused context.
type FocusResult struct {
	Target   string            `json:"target"`
	Entities []FocusEntity     `json:"entities"`
	Edges    []FocusEdge       `json:"edges"`
	Summary  map[string]int    `json:"summary"`
	Rendered string            `json:"rendered,omitempty"`
}

// FocusEntity represents an entity in focus.
type FocusEntity struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	Path      string `json:"path"`
	Signature string `json:"signature,omitempty"`
	LineStart int    `json:"line_start,omitempty"`
}

// FocusEdge represents a relationship.
type FocusEdge struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Type     string `json:"type"`
}

// FocusService loads targeted context.
type FocusService struct {
	db graph.Driver
}

// NewFocusService creates a focus service.
func NewFocusService(db graph.Driver) *FocusService {
	return &FocusService{db: db}
}

// Focus loads entities and relationships around a target.
func (f *FocusService) Focus(ctx context.Context, target string, depth int) (*FocusResult, error) {
	if depth < 1 {
		depth = 1
	}
	if depth > 3 {
		depth = 3 // Limit to avoid token explosion
	}

	result := &FocusResult{
		Target:  target,
		Summary: make(map[string]int),
	}

	// Find the target entity
	targetQuery := `
		MATCH (e)
		WHERE e.name = $target
		   OR e.signature CONTAINS $target
		   OR e.path CONTAINS $target
		RETURN e.name as name,
		       labels(e)[0] as type,
		       e.path as path,
		       e.signature as signature,
		       e.line_start as line_start
		LIMIT 10
	`

	records, err := f.db.Execute(ctx, targetQuery, map[string]any{
		"target": target,
	})
	if err != nil {
		return nil, err
	}

	if len(records) == 0 {
		return result, fmt.Errorf("target '%s' not found", target)
	}

	// Add target entities
	for _, r := range records {
		result.Entities = append(result.Entities, FocusEntity{
			Name:      getString(r, "name"),
			Type:      getString(r, "type"),
			Path:      getString(r, "path"),
			Signature: getString(r, "signature"),
			LineStart: getInt(r, "line_start"),
		})
		result.Summary["targets"]++
	}

	// Find outgoing relationships (what target calls/uses)
	outQuery := `
		MATCH (source)-[r]->(target)
		WHERE source.name = $target
		   OR source.signature CONTAINS $target
		RETURN source.name as from_name,
		       type(r) as rel_type,
		       target.name as to_name,
		       labels(target)[0] as to_type,
		       target.path as to_path,
		       target.signature as to_signature
		LIMIT 50
	`

	outRecords, _ := f.db.Execute(ctx, outQuery, map[string]any{
		"target": target,
	})

	for _, r := range outRecords {
		result.Edges = append(result.Edges, FocusEdge{
			From: getString(r, "from_name"),
			To:   getString(r, "to_name"),
			Type: getString(r, "rel_type"),
		})

		// Add target entity if not already present
		toName := getString(r, "to_name")
		if toName != "" && !hasEntity(result.Entities, toName) {
			result.Entities = append(result.Entities, FocusEntity{
				Name:      toName,
				Type:      getString(r, "to_type"),
				Path:      getString(r, "to_path"),
				Signature: getString(r, "to_signature"),
			})
		}
		result.Summary["outgoing"]++
	}

	// Find incoming relationships (what calls target)
	inQuery := `
		MATCH (source)-[r]->(target)
		WHERE target.name = $target
		   OR target.signature CONTAINS $target
		RETURN source.name as from_name,
		       labels(source)[0] as from_type,
		       source.path as from_path,
		       type(r) as rel_type,
		       target.name as to_name
		LIMIT 50
	`

	inRecords, _ := f.db.Execute(ctx, inQuery, map[string]any{
		"target": target,
	})

	for _, r := range inRecords {
		result.Edges = append(result.Edges, FocusEdge{
			From: getString(r, "from_name"),
			To:   getString(r, "to_name"),
			Type: getString(r, "rel_type"),
		})

		// Add source entity
		fromName := getString(r, "from_name")
		if fromName != "" && !hasEntity(result.Entities, fromName) {
			result.Entities = append(result.Entities, FocusEntity{
				Name: fromName,
				Type: getString(r, "from_type"),
				Path: getString(r, "from_path"),
			})
		}
		result.Summary["incoming"]++
	}

	// Depth 2+: expand one level further
	if depth >= 2 && len(result.Entities) > 1 {
		for _, entity := range result.Entities[1:5] { // Limit expansion
			if entity.Name == "" || entity.Name == target {
				continue
			}
			f.expandEntity(ctx, &result.Entities, &result.Edges, entity.Name, result.Summary)
		}
	}

	// Render as pseudo-code
	result.Rendered = f.renderCode(result)

	return result, nil
}

func (f *FocusService) expandEntity(ctx context.Context, entities *[]FocusEntity, edges *[]FocusEdge, name string, summary map[string]int) {
	query := `
		MATCH (source)-[r]->(target)
		WHERE source.name = $name
		RETURN source.name as from_name,
		       type(r) as rel_type,
		       target.name as to_name,
		       labels(target)[0] as to_type,
		       target.path as to_path
		LIMIT 10
	`

	records, _ := f.db.Execute(ctx, query, map[string]any{
		"name": name,
	})

	for _, r := range records {
		*edges = append(*edges, FocusEdge{
			From: getString(r, "from_name"),
			To:   getString(r, "to_name"),
			Type: getString(r, "rel_type"),
		})

		toName := getString(r, "to_name")
		if toName != "" && !hasEntity(*entities, toName) {
			*entities = append(*entities, FocusEntity{
				Name: toName,
				Type: getString(r, "to_type"),
				Path: getString(r, "to_path"),
			})
			summary["expanded"]++
		}
	}
}

func hasEntity(entities []FocusEntity, name string) bool {
	for _, e := range entities {
		if e.Name == name {
			return true
		}
	}
	return false
}

func (f *FocusService) renderCode(result *FocusResult) string {
	var lines []string
	lines = append(lines, "// TOPOLOGY MAP (not real code)")
	lines = append(lines, fmt.Sprintf("// Focus: %s", result.Target))

	// Group by file
	byFile := make(map[string][]FocusEntity)
	for _, e := range result.Entities {
		path := e.Path
		if path == "" {
			path = "unknown"
		}
		byFile[path] = append(byFile[path], e)
	}

	// Build edge lookup
	edgeMap := make(map[string][]string)
	for _, edge := range result.Edges {
		edgeMap[edge.From] = append(edgeMap[edge.From], fmt.Sprintf("%s(%s)", edge.Type, edge.To))
	}

	for path, entities := range byFile {
		lines = append(lines, fmt.Sprintf("\nmodule '%s' {", shortPath(path)))

		for _, e := range entities {
			// Add dependency decorators
			if deps, ok := edgeMap[e.Name]; ok {
				for _, dep := range deps[:min(3, len(deps))] {
					lines = append(lines, fmt.Sprintf("  @%s", dep))
				}
			}

			// Render signature
			sig := e.Signature
			if sig == "" {
				sig = fmt.Sprintf("%s %s", strings.ToLower(e.Type), e.Name)
			}
			lines = append(lines, fmt.Sprintf("  %s { ... }", sig))
		}

		lines = append(lines, "}")
	}

	// Add summary
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("// Entities: %d, Relationships: %d",
		len(result.Entities), len(result.Edges)))

	return strings.Join(lines, "\n")
}

func shortPath(path string) string {
	if path == "" {
		return "?"
	}
	parts := strings.Split(strings.ReplaceAll(path, "\\", "/"), "/")
	if len(parts) > 2 {
		return strings.Join(parts[len(parts)-2:], "/")
	}
	return path
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
