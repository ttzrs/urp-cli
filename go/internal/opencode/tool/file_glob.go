package tool

import (
	"context"
	"fmt"
	"strings"

	"github.com/joss/urp/internal/opencode/domain"
)

type Glob struct {
	workDir string
}

func NewGlob(workDir string) *Glob {
	return &Glob{workDir: workDir}
}

func (g *Glob) Info() domain.Tool {
	return domain.Tool{
		ID:          "glob",
		Name:        "glob",
		Description: "Find files matching a glob pattern. Use **/*.go for recursive search.",
		Parameters: domain.JSONSchema{
			"type": "object",
			"properties": map[string]any{
				"pattern": map[string]any{
					"type":        "string",
					"description": "Glob pattern (e.g., **/*.ts, src/**/*.go)",
				},
				"path": map[string]any{
					"type":        "string",
					"description": "Base directory (default: working directory)",
				},
			},
			"required": []string{"pattern"},
		},
	}
}

func (g *Glob) Execute(ctx context.Context, args map[string]any) (*Result, error) {
	pattern, ok := args["pattern"].(string)
	if !ok || pattern == "" {
		return &Result{Error: ErrInvalidArgs}, ErrInvalidArgs
	}

	basePath := g.workDir
	if p, ok := args["path"].(string); ok && p != "" {
		basePath = p
	}

	matches, err := globFiles(basePath, pattern)
	if err != nil {
		return &Result{
			Title:  fmt.Sprintf("Glob %s", pattern),
			Output: err.Error(),
			Error:  err,
		}, nil
	}

	output := strings.Join(matches, "\n")
	if len(matches) == 0 {
		output = "No files found"
	}

	return &Result{
		Title:  fmt.Sprintf("Glob %s", pattern),
		Output: output,
		Metadata: map[string]any{
			"pattern": pattern,
			"count":   len(matches),
		},
	}, nil
}

var _ Executor = (*Glob)(nil)
