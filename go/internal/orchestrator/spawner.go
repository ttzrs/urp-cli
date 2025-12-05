package orchestrator

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/joss/urp/internal/protocol"
)

// SpawnWorker spawns a new worker process (local subprocess).
func (o *Orchestrator) SpawnWorker(ctx context.Context, workerID string) error {
	// Start urp worker run as subprocess
	cmd := exec.CommandContext(ctx, "urp", "worker", "run")
	cmd.Env = append(cmd.Environ(), fmt.Sprintf("URP_WORKER_ID=%s", workerID))

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start worker: %w", err)
	}

	readyCh := make(chan struct{}, 1)

	o.mu.Lock()
	o.workers[workerID] = &workerConn{
		id:      workerID,
		cmd:     cmd,
		stdin:   stdin,
		stdout:  stdout,
		readyCh: readyCh,
	}
	o.mu.Unlock()

	// Register with master
	o.master.AddWorker(workerID, stdout, stdin)

	// Start handling messages from this worker
	go func() {
		o.master.HandleWorker(ctx, workerID)
		// Cleanup when done
		o.mu.Lock()
		delete(o.workers, workerID)
		o.mu.Unlock()
	}()

	return nil
}

// SpawnWorkerContainer spawns a worker in a Docker/Podman container.
// The container runs in protocol mode, communicating via stdin/stdout.
func (o *Orchestrator) SpawnWorkerContainer(ctx context.Context, workerID, projectPath string) error {
	// Detect container runtime
	runtime := detectContainerRuntime()
	if runtime == "" {
		return fmt.Errorf("no container runtime found (docker/podman)")
	}

	// Build container args for protocol mode
	// Use --entrypoint to bypass the shell entrypoint and run urp directly
	args := []string{
		"run", "-i", "--rm",
		"--name", workerID,
		"--network", "urp-network",
		"-v", projectPath + ":/workspace:rw,z",
		"-v", "urp_vector:/var/lib/urp/vector:z",
		"-e", "URP_WORKER_ID=" + workerID,
		"-e", "NEO4J_URI=bolt://urp-memgraph:7687",
		"-e", "URP_WORKER=1",
		"-w", "/workspace",
		"--entrypoint", "/usr/local/bin/urp", // Bypass shell entrypoint
		"urp:latest",
		"worker", "run",
	}

	cmd := exec.CommandContext(ctx, runtime, args...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}

	// Stderr goes to os.Stderr for debugging
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start container: %w", err)
	}

	readyCh := make(chan struct{}, 1)

	o.mu.Lock()
	o.workers[workerID] = &workerConn{
		id:      workerID,
		cmd:     cmd,
		stdin:   stdin,
		stdout:  stdout,
		readyCh: readyCh,
	}
	o.mu.Unlock()

	// Register with master
	o.master.AddWorker(workerID, stdout, stdin)

	// Start handling messages from this worker
	go func() {
		o.master.HandleWorker(ctx, workerID)
		// Cleanup when done
		o.mu.Lock()
		delete(o.workers, workerID)
		o.mu.Unlock()
		// Kill container if still running
		exec.Command(runtime, "rm", "-f", workerID).Run()
	}()

	return nil
}

// detectContainerRuntime finds the available container runtime.
func detectContainerRuntime() string {
	// Check which runtime has urp-memgraph running (must match infra)
	// This ensures workers can reach the database on the same network
	if out, err := exec.Command("docker", "ps", "-q", "-f", "name=urp-memgraph").Output(); err == nil && len(out) > 0 {
		return "docker"
	}
	if out, err := exec.Command("podman", "ps", "-q", "-f", "name=urp-memgraph").Output(); err == nil && len(out) > 0 {
		return "podman"
	}
	// Fallback: prefer podman for rootless
	if _, err := exec.LookPath("podman"); err == nil {
		return "podman"
	}
	if _, err := exec.LookPath("docker"); err == nil {
		return "docker"
	}
	return ""
}

// SpawnWorkerInline creates a worker that executes in the same process (for testing/simple tasks).
func (o *Orchestrator) SpawnWorkerInline(ctx context.Context, workerID string, handler protocol.TaskHandler) error {
	// Create pipes for communication
	masterToWorkerR, masterToWorkerW := io.Pipe()
	workerToMasterR, workerToMasterW := io.Pipe()

	readyCh := make(chan struct{}, 1)

	// Create worker
	worker := protocol.NewWorkerWithIO(workerID, []string{"inline"}, masterToWorkerR, workerToMasterW)
	worker.SetHandler(handler)

	// Register with master
	o.master.AddWorker(workerID, workerToMasterR, masterToWorkerW)

	o.mu.Lock()
	o.workers[workerID] = &workerConn{
		id:      workerID,
		stdin:   masterToWorkerW,
		stdout:  workerToMasterR,
		readyCh: readyCh,
	}
	o.mu.Unlock()

	// Start worker and master handler
	go worker.Run(ctx)
	go func() {
		o.master.HandleWorker(ctx, workerID)
		o.mu.Lock()
		delete(o.workers, workerID)
		o.mu.Unlock()
	}()

	return nil
}
