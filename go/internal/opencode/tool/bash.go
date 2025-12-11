package tool

import (
	"context"
	"strings"
	"time"

	"github.com/joss/urp/internal/opencode/domain"
)

type Bash struct {
	workDir  string
	timeout  time.Duration
	executor CommandExecutor
}

func NewBash(workDir string) *Bash {
	return &Bash{
		workDir:  workDir,
		timeout:  2 * time.Minute,
		executor: &LocalExecutor{},
	}
}

// SetExecutor sets the command executor (e.g. for remote execution)
func (b *Bash) SetExecutor(exec CommandExecutor) {
	b.executor = exec
}

func (b *Bash) Info() domain.Tool {
	desc := "Execute bash commands in a persistent shell."
	if b.executor.IsRemote() {
		desc += " (Runs in ISOLATED WORKER container)"
	} else {
		desc += " (Runs LOCALLY on host)"
	}
	
	return domain.Tool{
		ID:          "bash",
		Name:        "bash",
		Description: desc + " Use for git, npm, docker, and other CLI operations. DO NOT use for file operations - use dedicated tools instead.",
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

	output, err := b.executor.Execute(ctx, b.workDir, command)

	// Truncate if too long
	if len(output) > 30000 {
		output = output[:30000] + "\n... (output truncated)"
	}

	result := &Result{
		Title:  truncateTitle(command),
		Output: output,
		Metadata: map[string]any{
			"command":  command,
			// "exitCode": cmd.ProcessState.ExitCode(), // Not available in interface yet
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
