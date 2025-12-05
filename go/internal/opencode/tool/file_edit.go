package tool

import (
	"context"
	"fmt"

	"github.com/joss/urp/internal/opencode/domain"
)

type Edit struct{}

func NewEdit() *Edit { return &Edit{} }

func (e *Edit) Info() domain.Tool {
	return domain.Tool{
		ID:          "edit",
		Name:        "edit",
		Description: "Edit a file by replacing exact string matches. Use old_string and new_string for precise edits.",
		Parameters: domain.JSONSchema{
			"type": "object",
			"properties": map[string]any{
				"file_path": map[string]any{
					"type":        "string",
					"description": "Absolute path to the file",
				},
				"old_string": map[string]any{
					"type":        "string",
					"description": "Exact string to find and replace",
				},
				"new_string": map[string]any{
					"type":        "string",
					"description": "Replacement string",
				},
				"replace_all": map[string]any{
					"type":        "boolean",
					"description": "Replace all occurrences (default: false)",
				},
			},
			"required": []string{"file_path", "old_string", "new_string"},
		},
	}
}

func (e *Edit) Execute(ctx context.Context, args map[string]any) (*Result, error) {
	filePath, ok := args["file_path"].(string)
	if !ok || filePath == "" {
		return &Result{Error: ErrInvalidArgs}, ErrInvalidArgs
	}

	oldStr, ok := args["old_string"].(string)
	if !ok {
		return &Result{Error: ErrInvalidArgs}, ErrInvalidArgs
	}

	newStr, ok := args["new_string"].(string)
	if !ok {
		return &Result{Error: ErrInvalidArgs}, ErrInvalidArgs
	}

	replaceAll := false
	if ra, ok := args["replace_all"].(bool); ok {
		replaceAll = ra
	}

	count, err := editFile(filePath, oldStr, newStr, replaceAll)
	if err != nil {
		return &Result{
			Title:  fmt.Sprintf("Edit %s", filePath),
			Output: err.Error(),
			Error:  err,
		}, nil
	}

	return &Result{
		Title:  fmt.Sprintf("Edit %s", filePath),
		Output: fmt.Sprintf("Replaced %d occurrence(s) in %s", count, filePath),
		Metadata: map[string]any{
			"path":         filePath,
			"replacements": count,
		},
	}, nil
}

var _ Executor = (*Edit)(nil)
