package tool

import (
	"bytes"
	"context"
	"os/exec"
)

// CommandExecutor abstracts the execution of shell commands.
// This allows switching between local execution (os/exec) and remote execution (workers).
type CommandExecutor interface {
	// Execute runs a command and returns combined stdout/stderr and error.
	Execute(ctx context.Context, dir string, command string) (string, error)
	
	// IsRemote returns true if execution happens in a separate environment (e.g. container).
	IsRemote() bool
}

// LocalExecutor implements CommandExecutor using local os/exec.
type LocalExecutor struct{}

func (l *LocalExecutor) Execute(ctx context.Context, dir string, command string) (string, error) {
	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	cmd.Dir = dir

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

	return output, err
}

func (l *LocalExecutor) IsRemote() bool { return false }
