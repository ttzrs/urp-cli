package specs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// Engine handles the Spec-Driven Development workflow.
// It ports the logic from the Python Specify CLI.
type Engine struct {
	workDir string
}

// NewEngine creates a new spec engine.
func NewEngine(workDir string) *Engine {
	return &Engine{
		workDir: workDir,
	}
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
