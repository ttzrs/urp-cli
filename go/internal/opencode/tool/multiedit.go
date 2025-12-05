package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/joss/urp/internal/opencode/domain"
)

// MultiEdit performs multiple edits on a single file atomically
type MultiEdit struct {
	workDir string
}

func NewMultiEdit(workDir string) *MultiEdit {
	return &MultiEdit{workDir: workDir}
}

func (m *MultiEdit) Info() domain.Tool {
	return domain.Tool{
		Name:        "multi_edit",
		Description: "Apply multiple edits to a single file atomically. All edits succeed or none do.",
		Parameters: domain.JSONSchema{
			"type": "object",
			"properties": map[string]any{
				"file_path": map[string]any{
					"type":        "string",
					"description": "Absolute path to the file to edit",
				},
				"edits": map[string]any{
					"type":        "array",
					"description": "Array of edit operations: [{old_string, new_string}, ...]",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"old_string": map[string]any{"type": "string"},
							"new_string": map[string]any{"type": "string"},
						},
					},
				},
			},
			"required": []string{"file_path", "edits"},
		},
	}
}

// EditOp represents a single edit operation
type EditOp struct {
	OldString string `json:"old_string"`
	NewString string `json:"new_string"`
}

func (m *MultiEdit) Execute(ctx context.Context, args map[string]any) (*Result, error) {
	filePath, _ := args["file_path"].(string)
	editsRaw, _ := args["edits"].([]any)

	if filePath == "" {
		return &Result{Error: fmt.Errorf("file_path is required")}, nil
	}

	// Resolve path
	if !filepath.IsAbs(filePath) {
		filePath = filepath.Join(m.workDir, filePath)
	}

	// Parse edits
	var edits []EditOp
	for i, e := range editsRaw {
		switch v := e.(type) {
		case map[string]any:
			old, _ := v["old_string"].(string)
			new, _ := v["new_string"].(string)
			if old == "" {
				return &Result{Error: fmt.Errorf("edit %d: old_string is required", i)}, nil
			}
			edits = append(edits, EditOp{OldString: old, NewString: new})
		case string:
			// Try JSON parsing
			var op EditOp
			if err := json.Unmarshal([]byte(v), &op); err != nil {
				return &Result{Error: fmt.Errorf("edit %d: invalid format", i)}, nil
			}
			edits = append(edits, op)
		default:
			return &Result{Error: fmt.Errorf("edit %d: unexpected type %T", i, e)}, nil
		}
	}

	if len(edits) == 0 {
		return &Result{Error: fmt.Errorf("at least one edit is required")}, nil
	}

	// Read file
	content, err := os.ReadFile(filePath)
	if err != nil {
		return &Result{Error: fmt.Errorf("read file: %w", err)}, nil
	}

	// Validate all edits first (atomic check)
	original := string(content)
	for i, edit := range edits {
		if !strings.Contains(original, edit.OldString) {
			return &Result{
				Error: fmt.Errorf("edit %d failed: old_string not found in file (may have been modified by earlier edit in same batch)", i),
			}, nil
		}
		count := strings.Count(original, edit.OldString)
		if count > 1 {
			return &Result{
				Error: fmt.Errorf("edit %d failed: old_string appears %d times, must be unique", i, count),
			}, nil
		}
	}

	// Apply all edits
	result := original
	for _, edit := range edits {
		result = strings.Replace(result, edit.OldString, edit.NewString, 1)
	}

	// Write atomically (write to temp, then rename)
	dir := filepath.Dir(filePath)
	tmpFile, err := os.CreateTemp(dir, ".multiedit-*")
	if err != nil {
		return &Result{Error: fmt.Errorf("create temp file: %w", err)}, nil
	}
	tmpPath := tmpFile.Name()

	if _, err := tmpFile.WriteString(result); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return &Result{Error: fmt.Errorf("write temp file: %w", err)}, nil
	}
	tmpFile.Close()

	// Preserve permissions
	info, err := os.Stat(filePath)
	if err == nil {
		os.Chmod(tmpPath, info.Mode())
	}

	// Atomic rename
	if err := os.Rename(tmpPath, filePath); err != nil {
		os.Remove(tmpPath)
		return &Result{Error: fmt.Errorf("rename: %w", err)}, nil
	}

	// Count lines changed
	oldLines := strings.Count(original, "\n")
	newLines := strings.Count(result, "\n")
	diff := newLines - oldLines

	diffStr := ""
	if diff > 0 {
		diffStr = fmt.Sprintf("+%d lines", diff)
	} else if diff < 0 {
		diffStr = fmt.Sprintf("%d lines", diff)
	} else {
		diffStr = "no line count change"
	}

	return &Result{
		Title:  fmt.Sprintf("Applied %d edits to %s", len(edits), filepath.Base(filePath)),
		Output: fmt.Sprintf("Successfully applied %d edits to %s (%s)", len(edits), filePath, diffStr),
		Metadata: map[string]any{
			"file_path":   filePath,
			"edit_count":  len(edits),
			"line_diff":   diff,
			"bytes_before": len(original),
			"bytes_after":  len(result),
		},
	}, nil
}
