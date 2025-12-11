package bootstrap

import (
	"context"
	"fmt"
	"time"

	"github.com/joss/urp/internal/opencode/tool"
	"github.com/joss/urp/internal/orchestrator"
)

// RemoteExecutor implements tool.CommandExecutor via Orchestrator workers.
type RemoteExecutor struct {
	orch     *orchestrator.Orchestrator
	workerID string
}

func NewRemoteExecutor(orch *orchestrator.Orchestrator, workerID string) *RemoteExecutor {
	return &RemoteExecutor{
		orch:     orch,
		workerID: workerID,
	}
}

func (r *RemoteExecutor) Execute(ctx context.Context, dir string, command string) (string, error) {
	// Wait for worker to be ready (with timeout)
	readyCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	
	if err := r.orch.WaitForWorkerReady(readyCtx, r.workerID); err != nil {
		return "", fmt.Errorf("worker not ready: %w", err)
	}

	// Prepare command: cd to dir if specified
	fullCmd := command
	if dir != "" {
		// Map host dir to container dir if needed?
		// We assume dir is already mapped to /workspace or relative to it.
		// If dir is absolute host path, we might have issues unless it's the WorkDir.
		// For simplicity, we assume dir is "." or relative, or we force /workspace.
		// Actually, runAgent mounts WorkDir -> /workspace.
		// So we should run in /workspace.
		fullCmd = fmt.Sprintf("cd /workspace && %s", command)
	}

	taskID := fmt.Sprintf("task-%d", time.Now().UnixNano())
	if err := r.orch.AssignTask(taskID, r.workerID, fullCmd); err != nil {
		return "", fmt.Errorf("assign task: %w", err)
	}

	result, err := r.orch.WaitForTask(ctx, taskID)
	if err != nil {
		return "", fmt.Errorf("wait task: %w", err)
	}

	if !result.Success {
		return result.Output, fmt.Errorf("remote error: %s", result.Error)
	}

	return result.Output, nil
}

func (r *RemoteExecutor) IsRemote() bool { return true }

var _ tool.CommandExecutor = (*RemoteExecutor)(nil)
