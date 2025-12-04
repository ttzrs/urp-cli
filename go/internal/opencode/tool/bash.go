package tool

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/joss/urp/internal/opencode/domain"
)

type Bash struct {
	workDir string
	timeout time.Duration
}

func NewBash(workDir string) *Bash {
	return &Bash{
		workDir: workDir,
		timeout: 2 * time.Minute,
	}
}

func (b *Bash) Info() domain.Tool {
	return domain.Tool{
		ID:          "bash",
		Name:        "bash",
		Description: "Execute bash commands in a persistent shell. Use for git, npm, docker, and other CLI operations. DO NOT use for file operations - use dedicated tools instead.",
		Parameters: domain.JSONSchema{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": "The bash command to execute",
				},
				"timeout": map[string]any{
					"type":        "number",
					"description": "Timeout in milliseconds (max 600000)",
				},
			},
			"required": []string{"command"},
		},
	}
}

func (b *Bash) Execute(ctx context.Context, args map[string]any) (*Result, error) {
	command, ok := args["command"].(string)
	if !ok || command == "" {
		return &Result{Error: ErrInvalidArgs}, ErrInvalidArgs
	}

	timeout := b.timeout
	if t, ok := args["timeout"].(float64); ok && t > 0 {
		timeout = time.Duration(t) * time.Millisecond
		if timeout > 10*time.Minute {
			timeout = 10 * time.Minute
		}
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	cmd.Dir = b.workDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	output := stdout.String()
	if stderr.Len() > 0 {
		if output != "" {
			output += "\n"
		}
		output += stderr.String()
	}

	// Truncate if too long
	if len(output) > 30000 {
		output = output[:30000] + "\n... (output truncated)"
	}

	result := &Result{
		Title:  truncateTitle(command),
		Output: output,
		Metadata: map[string]any{
			"command":  command,
			"exitCode": cmd.ProcessState.ExitCode(),
		},
	}

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			result.Output += "\n(command timed out)"
		}
		result.Error = err
	}

	return result, nil
}

func truncateTitle(s string) string {
	s = strings.Split(s, "\n")[0]
	if len(s) > 50 {
		return s[:47] + "..."
	}
	return s
}

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
		Description: "Search file contents using ripgrep. Supports regex patterns.",
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
				"case_insensitive": map[string]any{
					"type":        "boolean",
					"description": "Case insensitive search",
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

	rgArgs := []string{"-n", "--color=never"}

	if ci, ok := args["case_insensitive"].(bool); ok && ci {
		rgArgs = append(rgArgs, "-i")
	}

	if glob, ok := args["glob"].(string); ok && glob != "" {
		rgArgs = append(rgArgs, "--glob", glob)
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

	// Truncate if too long
	if len(output) > 30000 {
		output = output[:30000] + "\n... (output truncated)"
	}

	return &Result{
		Title:  fmt.Sprintf("Grep %s", pattern),
		Output: output,
		Metadata: map[string]any{
			"pattern": pattern,
			"path":    searchPath,
		},
	}, nil
}

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
