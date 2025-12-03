// Package orchestrator implements multi-agent task orchestration.
// It coordinates the Master-Worker flow for automated task execution.
package orchestrator

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/joss/urp/internal/protocol"
)

// TaskDefinition defines a task to be executed by a worker.
type TaskDefinition struct {
	ID          string
	Description string
	Command     string        // urp command to execute (e.g., "code dead")
	Args        []string      // additional arguments
	Timeout     time.Duration // per-task timeout (0 = use context)
}

// TaskResult holds the result of a completed task.
type TaskResult struct {
	TaskID    string
	WorkerID  string
	Success   bool
	Output    string
	Error     string
	Duration  time.Duration
	StartedAt time.Time
}

// Orchestrator coordinates multi-agent task execution.
type Orchestrator struct {
	mu      sync.RWMutex
	master  *protocol.Master
	workers map[string]*workerConn
	results map[string]*TaskResult

	// Channels for coordination
	workerReady chan string
	taskDone    chan string

	// Callbacks
	OnTaskStarted  func(workerID, taskID string)
	OnTaskComplete func(workerID, taskID string, result *TaskResult)
	OnTaskFailed   func(workerID, taskID string, err error)
	OnWorkerReady  func(workerID string, caps []string)
}

type workerConn struct {
	id        string
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdout    io.ReadCloser
	caps      []string
	busy      bool
	currentID string
	readyCh   chan struct{} // signals when worker is ready
}

// New creates a new Orchestrator.
func New() *Orchestrator {
	o := &Orchestrator{
		workers:     make(map[string]*workerConn),
		results:     make(map[string]*TaskResult),
		workerReady: make(chan string, 100),
		taskDone:    make(chan string, 100),
	}

	o.master = protocol.NewMaster()

	// Wire up master callbacks
	o.master.OnWorkerReady = func(workerID string, caps []string) {
		o.mu.Lock()
		if w, ok := o.workers[workerID]; ok {
			w.caps = caps
			// Signal worker ready
			select {
			case w.readyCh <- struct{}{}:
			default:
			}
		}
		o.mu.Unlock()

		// Notify channel
		select {
		case o.workerReady <- workerID:
		default:
		}

		if o.OnWorkerReady != nil {
			o.OnWorkerReady(workerID, caps)
		}
	}

	o.master.OnTaskStarted = func(workerID, taskID, branch string) {
		o.mu.Lock()
		if result, ok := o.results[taskID]; ok {
			result.StartedAt = time.Now()
		}
		o.mu.Unlock()

		if o.OnTaskStarted != nil {
			o.OnTaskStarted(workerID, taskID)
		}
	}

	o.master.OnTaskComplete = func(workerID, taskID string, result *protocol.TaskCompletePayload) {
		o.mu.Lock()
		taskResult := &TaskResult{
			TaskID:   taskID,
			WorkerID: workerID,
			Success:  true,
			Output:   result.Output,
			Duration: time.Duration(result.Duration) * time.Millisecond,
		}
		o.results[taskID] = taskResult

		if w, ok := o.workers[workerID]; ok {
			w.busy = false
			w.currentID = ""
		}
		o.mu.Unlock()

		// Signal completion
		select {
		case o.taskDone <- taskID:
		default:
		}

		if o.OnTaskComplete != nil {
			o.OnTaskComplete(workerID, taskID, taskResult)
		}
	}

	o.master.OnTaskFailed = func(workerID, taskID string, result *protocol.TaskFailedPayload) {
		o.mu.Lock()
		taskResult := &TaskResult{
			TaskID:   taskID,
			WorkerID: workerID,
			Success:  false,
			Error:    result.Error,
		}
		o.results[taskID] = taskResult

		if w, ok := o.workers[workerID]; ok {
			w.busy = false
			w.currentID = ""
		}
		o.mu.Unlock()

		// Signal completion
		select {
		case o.taskDone <- taskID:
		default:
		}

		if o.OnTaskFailed != nil {
			o.OnTaskFailed(workerID, taskID, fmt.Errorf("%s", result.Error))
		}
	}

	return o
}

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

func detectContainerRuntime() string {
	// Prefer podman (rootless, SELinux-friendly)
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

// WaitForWorkerReady waits for a specific worker to be ready.
func (o *Orchestrator) WaitForWorkerReady(ctx context.Context, workerID string) error {
	o.mu.RLock()
	w, ok := o.workers[workerID]
	o.mu.RUnlock()

	if !ok {
		return fmt.Errorf("worker not found: %s", workerID)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-w.readyCh:
		return nil
	}
}

// AssignTask assigns a task to an available worker.
func (o *Orchestrator) AssignTask(taskID, workerID, description string) error {
	o.mu.Lock()
	w, ok := o.workers[workerID]
	if !ok {
		o.mu.Unlock()
		return fmt.Errorf("worker not found: %s", workerID)
	}
	if w.busy {
		o.mu.Unlock()
		return fmt.Errorf("worker %s is busy", workerID)
	}
	w.busy = true
	w.currentID = taskID

	// Pre-create result entry with StartedAt
	o.results[taskID] = &TaskResult{
		TaskID:    taskID,
		WorkerID:  workerID,
		StartedAt: time.Now(),
	}
	o.mu.Unlock()

	return o.master.AssignTask(workerID, &protocol.AssignTaskPayload{
		TaskID:      taskID,
		Description: description,
	})
}

// WaitForTask waits for a specific task to complete.
func (o *Orchestrator) WaitForTask(ctx context.Context, taskID string) (*TaskResult, error) {
	// First check if already done
	o.mu.RLock()
	if result, ok := o.results[taskID]; ok && (result.Success || result.Error != "") {
		o.mu.RUnlock()
		return result, nil
	}
	o.mu.RUnlock()

	// Wait for completion signal
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case doneID := <-o.taskDone:
			if doneID == taskID {
				o.mu.RLock()
				result := o.results[taskID]
				o.mu.RUnlock()
				return result, nil
			}
			// Put it back for other waiters
			select {
			case o.taskDone <- doneID:
			default:
			}
		}
	}
}

// WaitForAll waits for all given tasks to complete concurrently.
func (o *Orchestrator) WaitForAll(ctx context.Context, taskIDs []string) (map[string]*TaskResult, error) {
	results := make(map[string]*TaskResult)
	var mu sync.Mutex
	var wg sync.WaitGroup
	errCh := make(chan error, len(taskIDs))

	for _, id := range taskIDs {
		wg.Add(1)
		go func(taskID string) {
			defer wg.Done()
			result, err := o.WaitForTask(ctx, taskID)
			if err != nil {
				errCh <- fmt.Errorf("task %s: %w", taskID, err)
				return
			}
			mu.Lock()
			results[taskID] = result
			mu.Unlock()
		}(id)
	}

	wg.Wait()
	close(errCh)

	// Return first error if any
	for err := range errCh {
		return results, err
	}

	return results, nil
}

// GetIdleWorker returns an idle worker ID, or empty string if none available.
func (o *Orchestrator) GetIdleWorker() string {
	o.mu.RLock()
	defer o.mu.RUnlock()

	for id, w := range o.workers {
		if !w.busy {
			return id
		}
	}
	return ""
}

// ListWorkers returns all worker IDs in sorted order (deterministic).
func (o *Orchestrator) ListWorkers() []string {
	o.mu.RLock()
	defer o.mu.RUnlock()

	ids := make([]string, 0, len(o.workers))
	for id := range o.workers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// WorkerCount returns the number of workers.
func (o *Orchestrator) WorkerCount() int {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return len(o.workers)
}

// WorkerReadyCh returns the channel that receives worker IDs when they become ready.
func (o *Orchestrator) WorkerReadyCh() <-chan string {
	return o.workerReady
}

// Shutdown stops all workers gracefully.
func (o *Orchestrator) Shutdown() {
	o.master.ShutdownAll()

	o.mu.Lock()
	for _, w := range o.workers {
		if w.cmd != nil && w.cmd.Process != nil {
			w.cmd.Process.Kill()
		}
		if w.stdin != nil {
			w.stdin.Close()
		}
	}
	o.mu.Unlock()
}

// ─────────────────────────────────────────────────────────────────────────────
// High-level orchestration helpers
// ─────────────────────────────────────────────────────────────────────────────

// ExecuteTasksParallel spawns workers and executes tasks in parallel.
func (o *Orchestrator) ExecuteTasksParallel(ctx context.Context, tasks []TaskDefinition, handler protocol.TaskHandler) (map[string]*TaskResult, error) {
	if len(tasks) == 0 {
		return make(map[string]*TaskResult), nil
	}

	// Spawn workers for each task
	workerIDs := make([]string, len(tasks))
	for i := range tasks {
		workerID := fmt.Sprintf("worker-%d", i+1)
		workerIDs[i] = workerID
		if err := o.SpawnWorkerInline(ctx, workerID, handler); err != nil {
			return nil, fmt.Errorf("spawn worker %s: %w", workerID, err)
		}
	}

	// Wait for all workers to be ready (with timeout)
	readyCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	readyCount := 0
	for readyCount < len(workerIDs) {
		select {
		case <-readyCtx.Done():
			return nil, fmt.Errorf("timeout waiting for workers: got %d/%d ready", readyCount, len(workerIDs))
		case <-o.workerReady:
			readyCount++
		}
	}

	// Assign tasks to workers
	taskIDs := make([]string, len(tasks))
	for i, task := range tasks {
		taskIDs[i] = task.ID
		if err := o.AssignTask(task.ID, workerIDs[i], task.Description); err != nil {
			return nil, fmt.Errorf("assign task %s: %w", task.ID, err)
		}
	}

	// Wait for all tasks concurrently
	return o.WaitForAll(ctx, taskIDs)
}

// ExecuteWithWorkerPool executes tasks using a fixed pool of workers.
func (o *Orchestrator) ExecuteWithWorkerPool(ctx context.Context, tasks []TaskDefinition, poolSize int, handler protocol.TaskHandler) (map[string]*TaskResult, error) {
	if len(tasks) == 0 {
		return make(map[string]*TaskResult), nil
	}
	if poolSize <= 0 {
		poolSize = 1
	}
	if poolSize > len(tasks) {
		poolSize = len(tasks)
	}

	// Spawn worker pool
	workerIDs := make([]string, poolSize)
	for i := 0; i < poolSize; i++ {
		workerID := fmt.Sprintf("pool-worker-%d", i+1)
		workerIDs[i] = workerID
		if err := o.SpawnWorkerInline(ctx, workerID, handler); err != nil {
			return nil, fmt.Errorf("spawn worker %s: %w", workerID, err)
		}
	}

	// Wait for workers to be ready
	readyCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	readyCount := 0
	for readyCount < poolSize {
		select {
		case <-readyCtx.Done():
			return nil, fmt.Errorf("timeout waiting for workers")
		case <-o.workerReady:
			readyCount++
		}
	}

	// Execute tasks with work stealing
	results := make(map[string]*TaskResult)
	var mu sync.Mutex
	taskCh := make(chan TaskDefinition, len(tasks))
	var wg sync.WaitGroup

	// Queue all tasks
	for _, task := range tasks {
		taskCh <- task
	}
	close(taskCh)

	// Start worker goroutines
	for _, workerID := range workerIDs {
		wg.Add(1)
		go func(wID string) {
			defer wg.Done()
			for task := range taskCh {
				// Assign and wait
				if err := o.AssignTask(task.ID, wID, task.Description); err != nil {
					mu.Lock()
					results[task.ID] = &TaskResult{
						TaskID:  task.ID,
						Success: false,
						Error:   err.Error(),
					}
					mu.Unlock()
					continue
				}

				result, err := o.WaitForTask(ctx, task.ID)
				if err != nil {
					mu.Lock()
					results[task.ID] = &TaskResult{
						TaskID:  task.ID,
						Success: false,
						Error:   err.Error(),
					}
					mu.Unlock()
					continue
				}

				mu.Lock()
				results[task.ID] = result
				mu.Unlock()
			}
		}(workerID)
	}

	wg.Wait()
	return results, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Built-in task handlers for common operations
// ─────────────────────────────────────────────────────────────────────────────

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
