package tool

import (
	"context"
	"fmt"
	"strings"

	"github.com/joss/urp/internal/opencode/domain"
)

type Write struct{}

func NewWrite() *Write { return &Write{} }

func (w *Write) Info() domain.Tool {
	return domain.Tool{
		ID:          "write",
		Name:        "write",
		Description: "Write content to a file. Creates parent directories if needed. Overwrites existing files.",
		Parameters: domain.JSONSchema{
			"type": "object",
			"properties": map[string]any{
				"file_path": map[string]any{
					"type":        "string",
					"description": "Absolute path to the file",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "Content to write",
				},
			},
			"required": []string{"file_path", "content"},
		},
	}
}

func (w *Write) Execute(ctx context.Context, args map[string]any) (*Result, error) {
	filePath, ok := args["file_path"].(string)
	if !ok || filePath == "" {
		return &Result{Error: ErrInvalidArgs}, ErrInvalidArgs
	}

	content, ok := args["content"].(string)
	if !ok {
		return &Result{Error: ErrInvalidArgs}, ErrInvalidArgs
	}

	if err := writeFile(filePath, content); err != nil {
		return &Result{
			Title:  fmt.Sprintf("Write %s", filePath),
			Output: err.Error(),
			Error:  err,
		}, nil
	}

	lines := strings.Count(content, "\n") + 1
	return &Result{
		Title:  fmt.Sprintf("Write %s", filePath),
		Output: fmt.Sprintf("Wrote %d lines to %s", lines, filePath),
		Metadata: map[string]any{
			"path":  filePath,
			"lines": lines,
			"bytes": len(content),
		},
	}, nil
}

var _ Executor = (*Write)(nil)
