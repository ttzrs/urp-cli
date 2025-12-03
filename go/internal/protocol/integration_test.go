package protocol

import (
	"context"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"
)

// TestFullTaskLifecycle tests complete task flow: assign → start → progress → complete
func TestFullTaskLifecycle(t *testing.T) {
	// Create bidirectional pipes
	masterToWorkerR, masterToWorkerW := io.Pipe()
	workerToMasterR, workerToMasterW := io.Pipe()

	// Track received messages
	var mu sync.Mutex
	received := make(map[MessageType]int)
	var lastComplete *TaskCompletePayload

	// Setup master
	master := NewMaster()
	master.OnWorkerReady = func(workerID string, caps []string) {
		mu.Lock()
		received[MsgReady]++
		mu.Unlock()
		t.Logf("Worker %s ready with caps: %v", workerID, caps)
	}
	master.OnTaskStarted = func(workerID, taskID, branch string) {
		mu.Lock()
		received[MsgTaskStarted]++
		mu.Unlock()
		t.Logf("Task %s started on %s (branch: %s)", taskID, workerID, branch)
	}
	master.OnTaskProgress = func(workerID, taskID string, progress float64, msg string) {
		mu.Lock()
		received[MsgTaskProgress]++
		mu.Unlock()
		t.Logf("Task %s progress: %.0f%% - %s", taskID, progress*100, msg)
	}
	master.OnTaskOutput = func(workerID, taskID, stream, data string) {
		mu.Lock()
		received[MsgTaskOutput]++
		mu.Unlock()
		t.Logf("Task %s [%s]: %s", taskID, stream, data)
	}
	master.OnTaskComplete = func(workerID, taskID string, result *TaskCompletePayload) {
		mu.Lock()
		received[MsgTaskComplete]++
		lastComplete = result
		mu.Unlock()
		t.Logf("Task %s complete: %s (duration: %dms)", taskID, result.Output, result.Duration)
	}

	// Add worker connection to master
	master.AddWorker("worker-1", workerToMasterR, masterToWorkerW)

	// Setup worker
	worker := NewWorkerWithIO("worker-1", []string{"go", "git", "test"},
		masterToWorkerR, workerToMasterW)

	taskDone := make(chan struct{})

	worker.SetHandler(func(ctx context.Context, task *AssignTaskPayload, reporter *TaskReporter) error {
		defer close(taskDone)

		// Simulate work
		reporter.Progress(0.25, "analyzing code")
		time.Sleep(10 * time.Millisecond)

		reporter.Stdout("Running tests...\n")
		reporter.Progress(0.50, "running tests")
		time.Sleep(10 * time.Millisecond)

		reporter.Stdout("Tests passed!\n")
		reporter.Progress(0.75, "formatting")
		time.Sleep(10 * time.Millisecond)

		reporter.Progress(1.0, "done")
		reporter.Complete("All tests passed", []string{"main.go", "main_test.go"}, "")

		return nil
	})

	// Start worker and master handler
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		worker.Run(ctx)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		master.HandleWorker(ctx, "worker-1")
	}()

	// Wait for worker ready
	time.Sleep(50 * time.Millisecond)

	// Assign task
	err := master.AssignTask("worker-1", &AssignTaskPayload{
		TaskID:      "task-lifecycle-test",
		PlanID:      "plan-test",
		Description: "Run full lifecycle test",
		Branch:      "urp/test/task-1",
	})
	if err != nil {
		t.Fatalf("AssignTask: %v", err)
	}

	// Wait for task completion
	select {
	case <-taskDone:
		t.Log("Task completed successfully")
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for task completion")
	}

	// Give master time to process final messages
	time.Sleep(100 * time.Millisecond)

	// Cancel and cleanup
	cancel()

	// Close pipes to unblock readers
	masterToWorkerW.Close()
	workerToMasterW.Close()

	// Verify messages received
	mu.Lock()
	defer mu.Unlock()

	if received[MsgReady] != 1 {
		t.Errorf("Expected 1 ready message, got %d", received[MsgReady])
	}
	if received[MsgTaskStarted] != 1 {
		t.Errorf("Expected 1 task_started message, got %d", received[MsgTaskStarted])
	}
	if received[MsgTaskProgress] < 3 {
		t.Errorf("Expected at least 3 progress messages, got %d", received[MsgTaskProgress])
	}
	if received[MsgTaskOutput] < 2 {
		t.Errorf("Expected at least 2 output messages, got %d", received[MsgTaskOutput])
	}
	if received[MsgTaskComplete] != 1 {
		t.Errorf("Expected 1 task_complete message, got %d", received[MsgTaskComplete])
	}

	if lastComplete == nil {
		t.Fatal("No complete message received")
	}
	if len(lastComplete.FilesChanged) != 2 {
		t.Errorf("Expected 2 files changed, got %d", len(lastComplete.FilesChanged))
	}
	if lastComplete.Duration <= 0 {
		t.Error("Expected positive duration")
	}
}

// TestTaskFailure tests task failure handling
func TestTaskFailure(t *testing.T) {
	masterToWorkerR, masterToWorkerW := io.Pipe()
	workerToMasterR, workerToMasterW := io.Pipe()

	var failureReceived *TaskFailedPayload
	var mu sync.Mutex

	master := NewMaster()
	master.OnTaskFailed = func(workerID, taskID string, result *TaskFailedPayload) {
		mu.Lock()
		failureReceived = result
		mu.Unlock()
	}

	master.AddWorker("worker-fail", workerToMasterR, masterToWorkerW)

	worker := NewWorkerWithIO("worker-fail", nil,
		masterToWorkerR, workerToMasterW)

	taskDone := make(chan struct{})

	worker.SetHandler(func(ctx context.Context, task *AssignTaskPayload, reporter *TaskReporter) error {
		defer close(taskDone)
		reporter.Progress(0.5, "working")
		reporter.Failed("compilation error: undefined variable", 1)
		return fmt.Errorf("task failed")
	})

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	go worker.Run(ctx)
	go master.HandleWorker(ctx, "worker-fail")

	time.Sleep(50 * time.Millisecond)

	master.AssignTask("worker-fail", &AssignTaskPayload{
		TaskID:      "task-fail-test",
		Description: "This will fail",
	})

	select {
	case <-taskDone:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Timeout")
	}

	time.Sleep(50 * time.Millisecond)
	cancel()
	masterToWorkerW.Close()
	workerToMasterW.Close()

	mu.Lock()
	defer mu.Unlock()

	if failureReceived == nil {
		t.Fatal("No failure received")
	}
	if failureReceived.Code != 1 {
		t.Errorf("Expected code 1, got %d", failureReceived.Code)
	}
	if failureReceived.Error == "" {
		t.Error("Expected error message")
	}
}

// TestTaskCancellation tests cancelling a running task
func TestTaskCancellation(t *testing.T) {
	masterToWorkerR, masterToWorkerW := io.Pipe()
	workerToMasterR, workerToMasterW := io.Pipe()

	master := NewMaster()
	master.AddWorker("worker-cancel", workerToMasterR, masterToWorkerW)

	worker := NewWorkerWithIO("worker-cancel", nil,
		masterToWorkerR, workerToMasterW)

	taskStarted := make(chan struct{})
	taskCancelled := make(chan struct{})

	worker.SetHandler(func(ctx context.Context, task *AssignTaskPayload, reporter *TaskReporter) error {
		close(taskStarted)
		reporter.Progress(0.1, "started")

		// Wait for cancellation
		select {
		case <-ctx.Done():
			close(taskCancelled)
			return ctx.Err()
		case <-time.After(5 * time.Second):
			return fmt.Errorf("should have been cancelled")
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go worker.Run(ctx)
	go master.HandleWorker(ctx, "worker-cancel")

	time.Sleep(50 * time.Millisecond)

	master.AssignTask("worker-cancel", &AssignTaskPayload{
		TaskID:      "task-cancel-test",
		Description: "Will be cancelled",
	})

	// Wait for task to start
	select {
	case <-taskStarted:
		t.Log("Task started")
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Task didn't start")
	}

	// Cancel the task
	master.CancelTask("worker-cancel", "task-cancel-test", "user requested")

	// Wait for cancellation
	select {
	case <-taskCancelled:
		t.Log("Task cancelled successfully")
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Task wasn't cancelled")
	}

	cancel()
	masterToWorkerW.Close()
	workerToMasterW.Close()
}

// TestMultipleWorkers tests master handling multiple workers
func TestMultipleWorkers(t *testing.T) {
	master := NewMaster()

	var readyCount int
	var mu sync.Mutex
	master.OnWorkerReady = func(workerID string, caps []string) {
		mu.Lock()
		readyCount++
		mu.Unlock()
	}

	// Create 3 workers
	workers := make([]*Worker, 3)

	for i := 0; i < 3; i++ {
		mToWR, mToWW := io.Pipe()
		wToMR, wToMW := io.Pipe()

		workerID := fmt.Sprintf("worker-%d", i)
		master.AddWorker(workerID, wToMR, mToWW)
		workers[i] = NewWorkerWithIO(workerID, []string{"go"}, mToWR, wToMW)

		workers[i].SetHandler(func(ctx context.Context, task *AssignTaskPayload, reporter *TaskReporter) error {
			reporter.Complete("done", nil, "")
			return nil
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Start all workers
	for i, w := range workers {
		go w.Run(ctx)
		go master.HandleWorker(ctx, fmt.Sprintf("worker-%d", i))
	}

	time.Sleep(100 * time.Millisecond)

	// Check all workers registered
	workerList := master.ListWorkers()
	if len(workerList) != 3 {
		t.Errorf("Expected 3 workers, got %d", len(workerList))
	}

	// Check all idle
	idle := master.ListIdleWorkers()
	if len(idle) != 3 {
		t.Errorf("Expected 3 idle workers, got %d", len(idle))
	}

	mu.Lock()
	if readyCount != 3 {
		t.Errorf("Expected 3 ready messages, got %d", readyCount)
	}
	mu.Unlock()

	cancel()
}

// TestPingPong tests health check
func TestPingPong(t *testing.T) {
	masterToWorkerR, masterToWorkerW := io.Pipe()
	workerToMasterR, workerToMasterW := io.Pipe()

	master := NewMaster()
	master.AddWorker("worker-ping", workerToMasterR, masterToWorkerW)

	worker := NewWorkerWithIO("worker-ping", nil,
		masterToWorkerR, workerToMasterW)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	go worker.Run(ctx)
	go master.HandleWorker(ctx, "worker-ping")

	time.Sleep(50 * time.Millisecond)

	// Send ping
	err := master.Ping("worker-ping")
	if err != nil {
		t.Fatalf("Ping failed: %v", err)
	}

	// Pong is handled internally, just verify no error
	time.Sleep(50 * time.Millisecond)

	cancel()
	masterToWorkerW.Close()
	workerToMasterW.Close()
}

// TestStreamingOutput tests OutputCollector with streaming
func TestStreamingOutputWithReporter(t *testing.T) {
	masterToWorkerR, masterToWorkerW := io.Pipe()
	workerToMasterR, workerToMasterW := io.Pipe()

	var outputMessages []string
	var mu sync.Mutex

	master := NewMaster()
	master.OnTaskOutput = func(workerID, taskID, stream, data string) {
		mu.Lock()
		outputMessages = append(outputMessages, fmt.Sprintf("[%s] %s", stream, data))
		mu.Unlock()
	}

	master.AddWorker("worker-stream", workerToMasterR, masterToWorkerW)

	worker := NewWorkerWithIO("worker-stream", nil,
		masterToWorkerR, workerToMasterW)

	taskDone := make(chan struct{})

	worker.SetHandler(func(ctx context.Context, task *AssignTaskPayload, reporter *TaskReporter) error {
		defer close(taskDone)

		// Use stream writers
		stdout := reporter.StdoutWriter()
		stderr := reporter.StderrWriter()

		fmt.Fprintln(stdout, "Starting process...")
		fmt.Fprintln(stderr, "Warning: deprecated API")
		fmt.Fprintln(stdout, "Process complete")

		reporter.Complete("done", nil, "")
		return nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	go worker.Run(ctx)
	go master.HandleWorker(ctx, "worker-stream")

	time.Sleep(50 * time.Millisecond)

	master.AssignTask("worker-stream", &AssignTaskPayload{
		TaskID: "task-stream",
	})

	select {
	case <-taskDone:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Timeout")
	}

	time.Sleep(50 * time.Millisecond)
	cancel()
	masterToWorkerW.Close()
	workerToMasterW.Close()

	mu.Lock()
	defer mu.Unlock()

	if len(outputMessages) < 3 {
		t.Errorf("Expected at least 3 output messages, got %d", len(outputMessages))
	}

	// Verify we got both stdout and stderr
	hasStdout := false
	hasStderr := false
	for _, msg := range outputMessages {
		if len(msg) > 8 && msg[:8] == "[stdout]" {
			hasStdout = true
		}
		if len(msg) > 8 && msg[:8] == "[stderr]" {
			hasStderr = true
		}
	}

	if !hasStdout {
		t.Error("Expected stdout messages")
	}
	if !hasStderr {
		t.Error("Expected stderr messages")
	}
}
