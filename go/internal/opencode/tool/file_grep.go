package tool

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/joss/urp/internal/opencode/domain"
)

type Grep struct {
	workDir string
}

func NewGrep(workDir string) *Grep {
	return &Grep{workDir: workDir}
}

func (g *Grep) Info() domain.Tool {
	return domain.Tool{
		ID:          "grep",
		Name:        "grep",
		Description: "Search file contents using ripgrep. Supports regex, output modes, context lines, and pagination.",
		Parameters: domain.JSONSchema{
			"type": "object",
			"properties": map[string]any{
				"pattern": map[string]any{
					"type":        "string",
					"description": "Regex pattern to search",
				},
				"path": map[string]any{
					"type":        "string",
					"description": "File or directory to search",
				},
				"glob": map[string]any{
					"type":        "string",
					"description": "Filter files by glob (e.g., *.go)",
				},
				"type": map[string]any{
					"type":        "string",
					"description": "File type to search (e.g., go, py, js, rust)",
				},
				"case_insensitive": map[string]any{
					"type":        "boolean",
					"description": "Case insensitive search (-i)",
				},
				"output_mode": map[string]any{
					"type":        "string",
					"enum":        []string{"content", "files_with_matches", "count"},
					"description": "Output mode: content (default), files_with_matches (-l), count (-c)",
				},
				"context_before": map[string]any{
					"type":        "integer",
					"description": "Lines to show before match (-B)",
				},
				"context_after": map[string]any{
					"type":        "integer",
					"description": "Lines to show after match (-A)",
				},
				"context": map[string]any{
					"type":        "integer",
					"description": "Lines to show before and after match (-C)",
				},
				"multiline": map[string]any{
					"type":        "boolean",
					"description": "Enable multiline mode (-U --multiline-dotall)",
				},
				"head_limit": map[string]any{
					"type":        "integer",
					"description": "Limit output to first N lines/entries",
				},
				"offset": map[string]any{
					"type":        "integer",
					"description": "Skip first N lines/entries",
				},
			},
			"required": []string{"pattern"},
		},
	}
}

func (g *Grep) Execute(ctx context.Context, args map[string]any) (*Result, error) {
	pattern, ok := args["pattern"].(string)
	if !ok || pattern == "" {
		return &Result{Error: ErrInvalidArgs}, ErrInvalidArgs
	}

	searchPath := g.workDir
	if p, ok := args["path"].(string); ok && p != "" {
		searchPath = p
	}

	rgArgs := []string{"--color=never"}

	// Output mode
	outputMode := "content"
	if mode, ok := args["output_mode"].(string); ok && mode != "" {
		outputMode = mode
	}

	switch outputMode {
	case "files_with_matches":
		rgArgs = append(rgArgs, "-l")
	case "count":
		rgArgs = append(rgArgs, "-c")
	default:
		rgArgs = append(rgArgs, "-n") // Show line numbers for content mode
	}

	// Case insensitive
	if ci, ok := args["case_insensitive"].(bool); ok && ci {
		rgArgs = append(rgArgs, "-i")
	}

	// File type filter
	if fileType, ok := args["type"].(string); ok && fileType != "" {
		rgArgs = append(rgArgs, "--type", fileType)
	}

	// Glob filter
	if glob, ok := args["glob"].(string); ok && glob != "" {
		rgArgs = append(rgArgs, "--glob", glob)
	}

	// Context lines (only for content mode)
	if outputMode == "content" {
		if c, ok := getInt(args, "context"); ok && c > 0 {
			rgArgs = append(rgArgs, "-C", fmt.Sprintf("%d", c))
		} else {
			if b, ok := getInt(args, "context_before"); ok && b > 0 {
				rgArgs = append(rgArgs, "-B", fmt.Sprintf("%d", b))
			}
			if a, ok := getInt(args, "context_after"); ok && a > 0 {
				rgArgs = append(rgArgs, "-A", fmt.Sprintf("%d", a))
			}
		}
	}

	// Multiline mode
	if ml, ok := args["multiline"].(bool); ok && ml {
		rgArgs = append(rgArgs, "-U", "--multiline-dotall")
	}

	rgArgs = append(rgArgs, pattern, searchPath)

	cmd := exec.CommandContext(ctx, "rg", rgArgs...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	cmd.Run() // Ignore error - rg returns non-zero for no matches

	output := stdout.String()
	if output == "" && stderr.Len() > 0 {
		output = stderr.String()
	}
	if output == "" {
		output = "No matches found"
	}

	// Apply pagination (offset and head_limit)
	lines := strings.Split(output, "\n")
	offset, _ := getInt(args, "offset")
	headLimit, hasLimit := getInt(args, "head_limit")

	if offset > 0 && offset < len(lines) {
		lines = lines[offset:]
	}
	if hasLimit && headLimit > 0 && headLimit < len(lines) {
		lines = lines[:headLimit]
	}
	output = strings.Join(lines, "\n")

	// Truncate if too long
	if len(output) > 30000 {
		output = output[:30000] + "\n... (output truncated)"
	}

	return &Result{
		Title:  fmt.Sprintf("Grep %s", pattern),
		Output: output,
		Metadata: map[string]any{
			"pattern":     pattern,
			"path":        searchPath,
			"output_mode": outputMode,
		},
	}, nil
}

// getInt extracts an integer from args, handling both int and float64 (JSON numbers)
func getInt(args map[string]any, key string) (int, bool) {
	if v, ok := args[key]; ok {
		switch n := v.(type) {
		case int:
			return n, true
		case float64:
			return int(n), true
		}
	}
	return 0, false
}

var _ Executor = (*Grep)(nil)
