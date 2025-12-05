package tool

import (
	"context"
	"fmt"
	"strings"

	"github.com/joss/urp/internal/opencode/domain"
)

type LS struct {
	workDir string
}

func NewLS(workDir string) *LS {
	return &LS{workDir: workDir}
}

func (l *LS) Info() domain.Tool {
	return domain.Tool{
		ID:          "ls",
		Name:        "ls",
		Description: "List directory contents with file metadata.",
		Parameters: domain.JSONSchema{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Directory path",
				},
			},
			"required": []string{"path"},
		},
	}
}

func (l *LS) Execute(ctx context.Context, args map[string]any) (*Result, error) {
	path := l.workDir
	if p, ok := args["path"].(string); ok && p != "" {
		path = p
	}

	entries, err := listDir(path)
	if err != nil {
		return &Result{
			Title:  fmt.Sprintf("List %s", path),
			Output: err.Error(),
			Error:  err,
		}, nil
	}

	return &Result{
		Title:  fmt.Sprintf("List %s", path),
		Output: strings.Join(entries, "\n"),
		Metadata: map[string]any{
			"path":  path,
			"count": len(entries),
		},
	}, nil
}

var _ Executor = (*LS)(nil)
