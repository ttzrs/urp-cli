package orchestrator

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/joss/urp/internal/protocol"
)

func TestOrchestratorParallelTasks(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	orch := New()
	defer orch.Shutdown()

	// Track execution
	var executed int32

	// Create a simple handler that simulates work
	handler := func(ctx context.Context, task *protocol.AssignTaskPayload, reporter *protocol.TaskReporter) error {
		atomic.AddInt32(&executed, 1)
		reporter.Progress(0.5, "working on "+task.Description)
		time.Sleep(10 * time.Millisecond) // Simulate work
		reporter.Complete(fmt.Sprintf("Result for: %s", task.Description), nil, "")
		return nil
	}

	// Define tasks
	tasks := []TaskDefinition{
		{ID: "task-1", Description: "Find dead code"},
		{ID: "task-2", Description: "Find cycles"},
		{ID: "task-3", Description: "Find hotspots"},
	}

	// Execute in parallel
	results, err := orch.ExecuteTasksParallel(ctx, tasks, handler)
	if err != nil {
		t.Fatalf("ExecuteTasksParallel: %v", err)
	}

	// Verify all executed
	if int(executed) != len(tasks) {
		t.Errorf("Expected %d tasks executed, got %d", len(tasks), executed)
	}

	// Verify results
	if len(results) != len(tasks) {
		t.Errorf("Expected %d results, got %d", len(tasks), len(results))
	}

	for _, task := range tasks {
		result, ok := results[task.ID]
		if !ok {
			t.Errorf("Missing result for task %s", task.ID)
			continue
		}
		if !result.Success {
			t.Errorf("Task %s failed: %s", task.ID, result.Error)
		}
		if result.Output == "" {
			t.Errorf("Task %s has empty output", task.ID)
		}
		t.Logf("Task %s: %s", task.ID, result.Output)
	}
}

func TestOrchestratorTaskFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	orch := New()
	defer orch.Shutdown()

	// Handler that fails
	handler := func(ctx context.Context, task *protocol.AssignTaskPayload, reporter *protocol.TaskReporter) error {
		reporter.Failed("intentional failure", 1)
		return fmt.Errorf("intentional failure")
	}

	tasks := []TaskDefinition{
		{ID: "fail-task", Description: "This will fail"},
	}

	results, err := orch.ExecuteTasksParallel(ctx, tasks, handler)
	if err != nil {
		t.Fatalf("ExecuteTasksParallel: %v", err)
	}

	result := results["fail-task"]
	if result == nil {
		t.Fatal("Expected result for fail-task")
	}
	if result.Success {
		t.Error("Expected task to fail")
	}
	if result.Error == "" {
		t.Error("Expected error message")
	}
	t.Logf("Task failed as expected: %s", result.Error)
}

func TestOrchestratorCallbacks(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	orch := New()
	defer orch.Shutdown()

	var (
		startedCount   int32
		completedCount int32
	)

	orch.OnTaskStarted = func(workerID, taskID string) {
		atomic.AddInt32(&startedCount, 1)
		t.Logf("Task started: %s on %s", taskID, workerID)
	}

	orch.OnTaskComplete = func(workerID, taskID string, result *TaskResult) {
		atomic.AddInt32(&completedCount, 1)
		t.Logf("Task completed: %s on %s", taskID, workerID)
	}

	handler := func(ctx context.Context, task *protocol.AssignTaskPayload, reporter *protocol.TaskReporter) error {
		reporter.Complete("done", nil, "")
		return nil
	}

	tasks := []TaskDefinition{
		{ID: "cb-task-1", Description: "Task 1"},
		{ID: "cb-task-2", Description: "Task 2"},
	}

	_, err := orch.ExecuteTasksParallel(ctx, tasks, handler)
	if err != nil {
		t.Fatalf("ExecuteTasksParallel: %v", err)
	}

	started := atomic.LoadInt32(&startedCount)
	completed := atomic.LoadInt32(&completedCount)

	if started != 2 {
		t.Errorf("Expected 2 started callbacks, got %d", started)
	}
	if completed != 2 {
		t.Errorf("Expected 2 completed callbacks, got %d", completed)
	}
}

func TestOrchestratorMixedResults(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	orch := New()
	defer orch.Shutdown()

	// Handler: odd tasks fail, even succeed
	handler := func(ctx context.Context, task *protocol.AssignTaskPayload, reporter *protocol.TaskReporter) error {
		// Extract task number from ID
		var num int
		fmt.Sscanf(task.TaskID, "mixed-%d", &num)

		if num%2 == 1 {
			reporter.Failed("odd task fails", 1)
			return fmt.Errorf("odd task")
		}

		reporter.Complete(fmt.Sprintf("Task %d succeeded", num), nil, "")
		return nil
	}

	tasks := []TaskDefinition{
		{ID: "mixed-1", Description: "Odd (fail)"},
		{ID: "mixed-2", Description: "Even (success)"},
		{ID: "mixed-3", Description: "Odd (fail)"},
		{ID: "mixed-4", Description: "Even (success)"},
	}

	results, err := orch.ExecuteTasksParallel(ctx, tasks, handler)
	if err != nil {
		t.Fatalf("ExecuteTasksParallel: %v", err)
	}

	successes := 0
	failures := 0
	for _, r := range results {
		if r.Success {
			successes++
		} else {
			failures++
		}
	}

	if successes != 2 {
		t.Errorf("Expected 2 successes, got %d", successes)
	}
	if failures != 2 {
		t.Errorf("Expected 2 failures, got %d", failures)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Advanced Tests: Stress, Concurrency, Race Conditions
// ─────────────────────────────────────────────────────────────────────────────

func TestOrchestratorStress(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	orch := New()
	defer orch.Shutdown()

	const numTasks = 50
	var completed int32

	// Handler with random delays
	handler := func(ctx context.Context, task *protocol.AssignTaskPayload, reporter *protocol.TaskReporter) error {
		// Random delay 1-20ms
		delay := time.Duration(1+rand.Intn(20)) * time.Millisecond
		time.Sleep(delay)

		atomic.AddInt32(&completed, 1)
		reporter.Complete(fmt.Sprintf("Task %s done in %v", task.TaskID, delay), nil, "")
		return nil
	}

	// Generate tasks
	tasks := make([]TaskDefinition, numTasks)
	for i := 0; i < numTasks; i++ {
		tasks[i] = TaskDefinition{
			ID:          fmt.Sprintf("stress-%d", i),
			Description: fmt.Sprintf("Stress task %d", i),
		}
	}

	start := time.Now()
	results, err := orch.ExecuteTasksParallel(ctx, tasks, handler)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("ExecuteTasksParallel: %v", err)
	}

	if int(completed) != numTasks {
		t.Errorf("Expected %d completed, got %d", numTasks, completed)
	}

	if len(results) != numTasks {
		t.Errorf("Expected %d results, got %d", numTasks, len(results))
	}

	// Check all succeeded
	for id, r := range results {
		if !r.Success {
			t.Errorf("Task %s failed: %s", id, r.Error)
		}
	}

	t.Logf("Completed %d tasks in %v (%.1f tasks/sec)", numTasks, elapsed, float64(numTasks)/elapsed.Seconds())
}

func TestOrchestratorWorkerPool(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	orch := New()
	defer orch.Shutdown()

	const numTasks = 20
	const poolSize = 4

	var (
		maxConcurrent int32
		current       int32
		mu            sync.Mutex
	)

	// Handler that tracks concurrency
	handler := func(ctx context.Context, task *protocol.AssignTaskPayload, reporter *protocol.TaskReporter) error {
		c := atomic.AddInt32(&current, 1)

		mu.Lock()
		if c > maxConcurrent {
			maxConcurrent = c
		}
		mu.Unlock()

		time.Sleep(10 * time.Millisecond) // Simulate work

		atomic.AddInt32(&current, -1)
		reporter.Complete(fmt.Sprintf("Task %s done", task.TaskID), nil, "")
		return nil
	}

	// Generate more tasks than pool size
	tasks := make([]TaskDefinition, numTasks)
	for i := 0; i < numTasks; i++ {
		tasks[i] = TaskDefinition{
			ID:          fmt.Sprintf("pool-%d", i),
			Description: fmt.Sprintf("Pool task %d", i),
		}
	}

	results, err := orch.ExecuteWithWorkerPool(ctx, tasks, poolSize, handler)
	if err != nil {
		t.Fatalf("ExecuteWithWorkerPool: %v", err)
	}

	if len(results) != numTasks {
		t.Errorf("Expected %d results, got %d", numTasks, len(results))
	}

	// Verify all succeeded
	for id, r := range results {
		if !r.Success {
			t.Errorf("Task %s failed: %s", id, r.Error)
		}
	}

	// Max concurrency should be <= pool size
	if maxConcurrent > int32(poolSize) {
		t.Errorf("Max concurrency %d exceeded pool size %d", maxConcurrent, poolSize)
	}

	t.Logf("Pool test: %d tasks with pool=%d, max concurrent=%d", numTasks, poolSize, maxConcurrent)
}

func TestOrchestratorConcurrentCallbacks(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	orch := New()
	defer orch.Shutdown()

	const numTasks = 10
	var (
		startedMap   = make(map[string]bool)
		completedMap = make(map[string]bool)
		mu           sync.Mutex
	)

	orch.OnTaskStarted = func(workerID, taskID string) {
		mu.Lock()
		startedMap[taskID] = true
		mu.Unlock()
	}

	orch.OnTaskComplete = func(workerID, taskID string, result *TaskResult) {
		mu.Lock()
		completedMap[taskID] = true
		mu.Unlock()
	}

	handler := func(ctx context.Context, task *protocol.AssignTaskPayload, reporter *protocol.TaskReporter) error {
		time.Sleep(5 * time.Millisecond)
		reporter.Complete("done", nil, "")
		return nil
	}

	tasks := make([]TaskDefinition, numTasks)
	for i := 0; i < numTasks; i++ {
		tasks[i] = TaskDefinition{
			ID:          fmt.Sprintf("cb-%d", i),
			Description: fmt.Sprintf("Callback task %d", i),
		}
	}

	_, err := orch.ExecuteTasksParallel(ctx, tasks, handler)
	if err != nil {
		t.Fatalf("ExecuteTasksParallel: %v", err)
	}

	// Verify all callbacks fired
	mu.Lock()
	defer mu.Unlock()

	for _, task := range tasks {
		if !startedMap[task.ID] {
			t.Errorf("Task %s never started", task.ID)
		}
		if !completedMap[task.ID] {
			t.Errorf("Task %s never completed", task.ID)
		}
	}
}

func TestOrchestratorListWorkersDeterministic(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	orch := New()
	defer orch.Shutdown()

	handler := func(ctx context.Context, task *protocol.AssignTaskPayload, reporter *protocol.TaskReporter) error {
		reporter.Complete("done", nil, "")
		return nil
	}

	// Spawn workers in random order
	workerIDs := []string{"worker-c", "worker-a", "worker-b"}
	for _, id := range workerIDs {
		if err := orch.SpawnWorkerInline(ctx, id, handler); err != nil {
			t.Fatalf("SpawnWorkerInline: %v", err)
		}
	}

	// Wait for workers
	time.Sleep(50 * time.Millisecond)

	// ListWorkers should return sorted
	list := orch.ListWorkers()
	expected := []string{"worker-a", "worker-b", "worker-c"}

	if len(list) != len(expected) {
		t.Fatalf("Expected %d workers, got %d", len(expected), len(list))
	}

	for i, id := range list {
		if id != expected[i] {
			t.Errorf("Position %d: expected %s, got %s", i, expected[i], id)
		}
	}
}

func TestOrchestratorEmptyTasks(t *testing.T) {
	ctx := context.Background()

	orch := New()
	defer orch.Shutdown()

	handler := func(ctx context.Context, task *protocol.AssignTaskPayload, reporter *protocol.TaskReporter) error {
		t.Error("Handler should not be called for empty tasks")
		return nil
	}

	// Empty task list
	results, err := orch.ExecuteTasksParallel(ctx, []TaskDefinition{}, handler)
	if err != nil {
		t.Fatalf("ExecuteTasksParallel: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("Expected 0 results, got %d", len(results))
	}

	// Empty pool
	results, err = orch.ExecuteWithWorkerPool(ctx, []TaskDefinition{}, 4, handler)
	if err != nil {
		t.Fatalf("ExecuteWithWorkerPool: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("Expected 0 results, got %d", len(results))
	}
}

func TestOrchestratorContextCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	orch := New()
	defer orch.Shutdown()

	// Handler that takes too long
	handler := func(ctx context.Context, task *protocol.AssignTaskPayload, reporter *protocol.TaskReporter) error {
		select {
		case <-ctx.Done():
			reporter.Failed("cancelled", 1)
			return ctx.Err()
		case <-time.After(5 * time.Second):
			reporter.Complete("done", nil, "")
			return nil
		}
	}

	tasks := []TaskDefinition{
		{ID: "slow-task", Description: "This is slow"},
	}

	_, err := orch.ExecuteTasksParallel(ctx, tasks, handler)
	if err == nil {
		t.Error("Expected timeout error")
	}
}

func TestOrchestratorRaceConditions(t *testing.T) {
	// Run with -race flag: go test -race ./...
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	orch := New()
	defer orch.Shutdown()

	const numTasks = 20
	var wg sync.WaitGroup

	handler := func(ctx context.Context, task *protocol.AssignTaskPayload, reporter *protocol.TaskReporter) error {
		// Random very short delay
		time.Sleep(time.Duration(rand.Intn(5)) * time.Millisecond)
		reporter.Complete("done", nil, "")
		return nil
	}

	tasks := make([]TaskDefinition, numTasks)
	for i := 0; i < numTasks; i++ {
		tasks[i] = TaskDefinition{
			ID:          fmt.Sprintf("race-%d", i),
			Description: fmt.Sprintf("Race task %d", i),
		}
	}

	// Concurrent operations on orchestrator
	wg.Add(3)

	// Execute tasks
	go func() {
		defer wg.Done()
		_, _ = orch.ExecuteTasksParallel(ctx, tasks, handler)
	}()

	// Concurrent ListWorkers calls
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_ = orch.ListWorkers()
			time.Sleep(time.Millisecond)
		}
	}()

	// Concurrent WorkerCount calls
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_ = orch.WorkerCount()
			time.Sleep(time.Millisecond)
		}
	}()

	wg.Wait()
}

func TestOrchestratorWorkerPoolSmallerThanTasks(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	orch := New()
	defer orch.Shutdown()

	const numTasks = 10
	const poolSize = 2

	var executed int32

	handler := func(ctx context.Context, task *protocol.AssignTaskPayload, reporter *protocol.TaskReporter) error {
		atomic.AddInt32(&executed, 1)
		time.Sleep(10 * time.Millisecond)
		reporter.Complete(fmt.Sprintf("Done: %s", task.TaskID), nil, "")
		return nil
	}

	tasks := make([]TaskDefinition, numTasks)
	for i := 0; i < numTasks; i++ {
		tasks[i] = TaskDefinition{
			ID:          fmt.Sprintf("small-pool-%d", i),
			Description: fmt.Sprintf("Task %d", i),
		}
	}

	results, err := orch.ExecuteWithWorkerPool(ctx, tasks, poolSize, handler)
	if err != nil {
		t.Fatalf("ExecuteWithWorkerPool: %v", err)
	}

	if int(executed) != numTasks {
		t.Errorf("Expected %d executed, got %d", numTasks, executed)
	}

	if len(results) != numTasks {
		t.Errorf("Expected %d results, got %d", numTasks, len(results))
	}

	// All should succeed
	for id, r := range results {
		if !r.Success {
			t.Errorf("Task %s failed: %s", id, r.Error)
		}
	}

	t.Logf("Processed %d tasks with %d workers", numTasks, poolSize)
}

// Benchmark parallel execution
func BenchmarkOrchestratorParallel(b *testing.B) {
	handler := func(ctx context.Context, task *protocol.AssignTaskPayload, reporter *protocol.TaskReporter) error {
		reporter.Complete("done", nil, "")
		return nil
	}

	for i := 0; i < b.N; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		orch := New()

		tasks := []TaskDefinition{
			{ID: "b-1", Description: "Task 1"},
			{ID: "b-2", Description: "Task 2"},
			{ID: "b-3", Description: "Task 3"},
			{ID: "b-4", Description: "Task 4"},
		}

		_, _ = orch.ExecuteTasksParallel(ctx, tasks, handler)

		orch.Shutdown()
		cancel()
	}
}

// Benchmark worker pool
func BenchmarkOrchestratorPool(b *testing.B) {
	handler := func(ctx context.Context, task *protocol.AssignTaskPayload, reporter *protocol.TaskReporter) error {
		reporter.Complete("done", nil, "")
		return nil
	}

	for i := 0; i < b.N; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		orch := New()

		tasks := make([]TaskDefinition, 10)
		for j := 0; j < 10; j++ {
			tasks[j] = TaskDefinition{ID: fmt.Sprintf("bp-%d", j), Description: "Task"}
		}

		_, _ = orch.ExecuteWithWorkerPool(ctx, tasks, 3, handler)

		orch.Shutdown()
		cancel()
	}
}
