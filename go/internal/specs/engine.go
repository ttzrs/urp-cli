package specs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/joss/urp/internal/graph"
)

// Engine handles the Spec-Driven Development workflow.
// It ports the logic from the Python Specify CLI.
type Engine struct {
	workDir string
	db      *graph.Memgraph // Optional graph database for context enrichment
}

// NewEngine creates a new spec engine.
func NewEngine(workDir string) *Engine {
	return &Engine{
		workDir: workDir,
	}
}

// WithDB attaches a graph database for context enrichment
func (e *Engine) WithDB(db *graph.Memgraph) *Engine {
	e.db = db
	return e
}

// InitProject initializes a new spec-driven project.
func (e *Engine) InitProject(ctx context.Context, name string) error {
	// TODO: Implement template download and extraction
	// For now, we'll create the basic structure
	
	baseDir := filepath.Join(e.workDir, name)
	if name == "." {
		baseDir = e.workDir
	}

	dirs := []string{
		".specify/memory",
		".specify/templates",
		".specify/scripts",
		"specs",
	}

	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(baseDir, d), 0755); err != nil {
			return fmt.Errorf("create dir %s: %w", d, err)
		}
	}

	// Create constitution
	const constitution = `# Project Constitution

1. **Code Quality**: All code must be clean, readable, and follow established patterns.
2. **Testing**: New features must include tests.
3. **Architecture**: Respect the Clean Architecture layers.
`
	if err := os.WriteFile(filepath.Join(baseDir, ".specify/memory/constitution.md"), []byte(constitution), 0644); err != nil {
		return fmt.Errorf("write constitution: %w", err)
	}

	return nil
}

// ListSpecs returns all specs in the project.
func (e *Engine) ListSpecs(ctx context.Context) ([]string, error) {
	specsDir := filepath.Join(e.workDir, "specs")
	entries, err := os.ReadDir(specsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var specs []string
	for _, entry := range entries {
		if entry.IsDir() {
			specs = append(specs, entry.Name())
		}
	}
	return specs, nil
}

// ReadSpec reads the spec.md content for a given spec name.
func (e *Engine) ReadSpec(ctx context.Context, name string) (string, error) {
	specPath := filepath.Join(e.workDir, "specs", name, "spec.md")
	content, err := os.ReadFile(specPath)
	if err != nil {
		return "", fmt.Errorf("read spec %s: %w", name, err)
	}
	return string(content), nil
}

// ReadConstitution reads the project constitution.
func (e *Engine) ReadConstitution(ctx context.Context) (string, error) {
	constPath := filepath.Join(e.workDir, ".specify/memory/constitution.md")
	content, err := os.ReadFile(constPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read constitution: %w", err)
	}
	return string(content), nil
}

// ParseSpec reads and parses a spec with frontmatter
func (e *Engine) ParseSpec(ctx context.Context, name string) (*Spec, error) {
	specPath := filepath.Join(e.workDir, "specs", name, "spec.md")
	return ParseSpec(specPath)
}

// EnrichContext expands the spec's context using the graph database
// Returns: explicit files + related files from graph neighborhood
func (e *Engine) EnrichContext(ctx context.Context, spec *Spec, depth int) ([]string, error) {
	if depth <= 0 {
		depth = 1
	}

	// Start with explicit context from frontmatter
	files := make([]string, 0, len(spec.Context))
	for _, f := range spec.Context {
		// Resolve relative to workdir
		if !filepath.IsAbs(f) {
			f = filepath.Join(e.workDir, f)
		}
		files = append(files, f)
	}

	// If no graph DB, return explicit context only
	if e.db == nil {
		return files, nil
	}

	// Expand context using graph neighborhood
	enriched := make(map[string]bool)
	for _, f := range files {
		enriched[f] = true
	}

	for _, f := range files {
		neighbors, err := e.getNeighborhood(ctx, f, depth)
		if err != nil {
			continue // Skip on error
		}
		for _, n := range neighbors {
			enriched[n] = true
		}
	}

	// Convert to slice
	result := make([]string, 0, len(enriched))
	for f := range enriched {
		result = append(result, f)
	}

	return result, nil
}

// getNeighborhood queries the graph for related files
func (e *Engine) getNeighborhood(ctx context.Context, filePath string, depth int) ([]string, error) {
	if e.db == nil {
		return nil, nil
	}

	// Query files connected via CALLS, IMPORTS, CONTAINS within depth
	// Simpler query that works with Memgraph
	query := `
		MATCH (start:File {path: $path})-[*1..2]-(related:File)
		WHERE related.path <> $path
		RETURN DISTINCT related.path AS path
		LIMIT 20
	`

	records, err := e.db.Execute(ctx, query, map[string]any{"path": filePath})
	if err != nil {
		return nil, err
	}

	var paths []string
	for _, record := range records {
		if p, ok := record["path"]; ok {
			if path, ok := p.(string); ok {
				paths = append(paths, path)
			}
		}
	}

	return paths, nil
}

// BuildPrompt creates the structured prompt for the agent
func (e *Engine) BuildPrompt(ctx context.Context, spec *Spec, constitution string) (string, error) {
	var b strings.Builder

	// Header
	b.WriteString("# SPECIFICATION\n\n")
	if spec.Title != "" {
		b.WriteString(fmt.Sprintf("**Title**: %s\n", spec.Title))
	}
	if spec.ID != "" {
		b.WriteString(fmt.Sprintf("**ID**: %s\n", spec.ID))
	}
	if spec.Status != "" {
		b.WriteString(fmt.Sprintf("**Status**: %s\n", spec.Status))
	}
	if spec.Owner != "" {
		b.WriteString(fmt.Sprintf("**Owner**: %s\n", spec.Owner))
	}
	if spec.Type != "" {
		b.WriteString(fmt.Sprintf("**Type**: %s\n", spec.Type))
	}
	b.WriteString("\n")

	// Body
	b.WriteString("## Description\n\n")
	b.WriteString(spec.RawBody)
	b.WriteString("\n\n")

	// Requirements
	if len(spec.Requirements) > 0 {
		b.WriteString("## Requirements (Definition of Done)\n\n")
		b.WriteString(spec.FormatRequirements())
		b.WriteString("\n")

		// Progress
		progress := spec.Progress()
		b.WriteString(fmt.Sprintf("**Progress**: %.0f%% complete\n\n", progress))
	}

	// Plan
	if len(spec.Plan) > 0 {
		b.WriteString("## Implementation Plan\n\n")
		b.WriteString(spec.FormatPlan())
		b.WriteString("\n")
	}

	// Enriched context
	contextFiles, _ := e.EnrichContext(ctx, spec, 2)
	if len(contextFiles) > 0 {
		b.WriteString("## Context Files\n\n")
		b.WriteString("These files are relevant to this specification:\n\n")
		for _, f := range contextFiles {
			// Show relative path if possible
			rel, err := filepath.Rel(e.workDir, f)
			if err == nil {
				f = rel
			}
			b.WriteString(fmt.Sprintf("- `%s`\n", f))
		}
		b.WriteString("\n")
	}

	// Constitution
	if constitution != "" {
		b.WriteString("---\n\n")
		b.WriteString("## Project Constitution\n\n")
		b.WriteString(constitution)
		b.WriteString("\n")
	}

	// Instructions
	b.WriteString("---\n\n")
	b.WriteString("## Instructions\n\n")
	b.WriteString("1. Read and analyze the context files listed above\n")
	b.WriteString("2. Implement each uncompleted requirement\n")
	b.WriteString("3. Run tests to verify: `go test ./...`\n")
	b.WriteString("4. Fix any issues before marking complete\n")
	b.WriteString("\nStart now.\n")

	return b.String(), nil
}
