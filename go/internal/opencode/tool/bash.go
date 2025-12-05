package tool

import (
	"bytes"
	"context"
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

var _ Executor = (*Bash)(nil)
