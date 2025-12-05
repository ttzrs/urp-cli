// Package main worker protocol commands.
package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/joss/urp/internal/config"
	"github.com/joss/urp/internal/protocol"
)

func workerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "worker",
		Short: "Worker protocol commands",
		Long:  "Commands for worker containers communicating with master via protocol",
	}

	// urp worker run - Run as protocol worker (reads stdin, writes stdout)
	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Run as protocol worker",
		Long: `Start worker in protocol mode, communicating with master via stdin/stdout.

The worker reads JSON Line messages from stdin and writes responses to stdout.
All logs go to stderr.

This is typically called by the worker-entrypoint.sh in Docker containers.`,
		Run: func(cmd *cobra.Command, args []string) {
			workerID := config.Env().WorkerID
			if workerID == "" {
				hostname, _ := os.Hostname()
				workerID = hostname
			}

			capsEnv := os.Getenv("URP_WORKER_CAPS")
			var caps []string
			if capsEnv != "" {
				caps = strings.Split(capsEnv, ",")
			}

			worker := protocol.NewWorker(workerID, caps)
			worker.SetHandler(workerTaskHandler)

			ctx := context.Background()
			if err := worker.Run(ctx); err != nil {
				fmt.Fprintf(os.Stderr, "Worker error: %v\n", err)
				os.Exit(1)
			}
		},
	}

	cmd.AddCommand(runCmd)
	return cmd
}

// workerTaskHandler handles tasks assigned to this worker
func workerTaskHandler(ctx context.Context, task *protocol.AssignTaskPayload, reporter *protocol.TaskReporter) error {
	reporter.Progress(0.1, "starting task")

	// Log to stderr (stdout is for protocol)
	fmt.Fprintf(os.Stderr, "[worker] Task %s: %s\n", task.TaskID, task.Description)

	// Create the git branch if specified
	if task.Branch != "" {
		reporter.Progress(0.2, "creating git branch")
		cmd := exec.CommandContext(ctx, "git", "checkout", "-b", task.Branch)
		cmd.Dir = "/workspace"
		if err := cmd.Run(); err != nil {
			// Branch might already exist, try checkout
			cmd = exec.CommandContext(ctx, "git", "checkout", task.Branch)
			cmd.Dir = "/workspace"
			if err := cmd.Run(); err != nil {
				reporter.Failed(fmt.Sprintf("failed to checkout branch: %v", err), 1)
				return err
			}
		}
		reporter.Stdout(fmt.Sprintf("On branch %s\n", task.Branch))
	}

	reporter.Progress(0.3, "executing command")

	// Execute the task description as a shell command
	// If it starts with "urp ", execute directly, otherwise use shell
	command := task.Description
	var cmd *exec.Cmd

	if strings.HasPrefix(command, "urp ") {
		// Direct urp command
		args := strings.Fields(command)[1:] // Remove "urp" prefix
		cmd = exec.CommandContext(ctx, "urp", args...)
	} else {
		// Shell command
		cmd = exec.CommandContext(ctx, "sh", "-c", command)
	}
	cmd.Dir = "/workspace"

	reporter.Progress(0.5, "running")
	output, err := cmd.CombinedOutput()

	if err != nil {
		reporter.Failed(fmt.Sprintf("command failed: %v\n%s", err, string(output)), 1)
		return err
	}

	reporter.Progress(1.0, "completed")
	reporter.Complete(strings.TrimSpace(string(output)), nil, "")
	return nil
}
