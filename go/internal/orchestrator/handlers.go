package orchestrator

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/joss/urp/internal/protocol"
)

// URPCommandHandler creates a handler that executes urp commands.
func URPCommandHandler(command string, args ...string) protocol.TaskHandler {
	return func(ctx context.Context, _ *protocol.AssignTaskPayload, reporter *protocol.TaskReporter) error {
		reporter.Progress(0.1, "starting "+command)

		// Parse command (e.g., "code dead" -> ["code", "dead"])
		parts := strings.Fields(command)
		allArgs := append(parts, args...)

		// Execute urp command
		cmd := exec.CommandContext(ctx, "urp", allArgs...)
		output, err := cmd.CombinedOutput()

		if err != nil {
			reporter.Failed(fmt.Sprintf("command failed: %v\n%s", err, output), 1)
			return err
		}

		reporter.Progress(1.0, "done")
		reporter.Complete(string(output), nil, "")
		return nil
	}
}

// ShellCommandHandler creates a handler that executes shell commands.
func ShellCommandHandler(command string) protocol.TaskHandler {
	return func(ctx context.Context, _ *protocol.AssignTaskPayload, reporter *protocol.TaskReporter) error {
		reporter.Progress(0.1, "executing shell command")

		cmd := exec.CommandContext(ctx, "sh", "-c", command)
		output, err := cmd.CombinedOutput()

		if err != nil {
			reporter.Failed(fmt.Sprintf("command failed: %v\n%s", err, output), 1)
			return err
		}

		reporter.Progress(1.0, "done")
		reporter.Complete(string(output), nil, "")
		return nil
	}
}
