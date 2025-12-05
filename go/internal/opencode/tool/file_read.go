package tool

import (
	"context"
	"fmt"

	"github.com/joss/urp/internal/opencode/domain"
)

type Read struct{}

func NewRead() *Read { return &Read{} }

func (r *Read) Info() domain.Tool {
	return domain.Tool{
		ID:          "read",
		Name:        "read",
		Description: "Read file contents. Supports text files, images, PDFs, and Jupyter notebooks.",
		Parameters: domain.JSONSchema{
			"type": "object",
			"properties": map[string]any{
				"file_path": map[string]any{
					"type":        "string",
					"description": "Absolute path to the file",
				},
				"offset": map[string]any{
					"type":        "number",
					"description": "Line number to start from (1-indexed)",
				},
				"limit": map[string]any{
					"type":        "number",
					"description": "Number of lines to read",
				},
			},
			"required": []string{"file_path"},
		},
	}
}

func (r *Read) Execute(ctx context.Context, args map[string]any) (*Result, error) {
	filePath, ok := args["file_path"].(string)
	if !ok || filePath == "" {
		return &Result{Error: ErrInvalidArgs}, ErrInvalidArgs
	}

	offset := 0
	if o, ok := args["offset"].(float64); ok {
		offset = int(o)
	}

	limit := 2000
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}

	content, err := readFileWithLines(filePath, offset, limit)
	if err != nil {
		return &Result{
			Title:  fmt.Sprintf("Read %s", filePath),
			Output: err.Error(),
			Error:  err,
		}, nil
	}

	return &Result{
		Title:  fmt.Sprintf("Read %s", filePath),
		Output: content,
		Metadata: map[string]any{
			"path":   filePath,
			"offset": offset,
			"limit":  limit,
		},
	}, nil
}

var _ Executor = (*Read)(nil)
