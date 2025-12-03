package protocol

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

func TestEnvelopeEncodeDecode(t *testing.T) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	dec := NewDecoder(&buf)

	// Send assign_task
	task := &AssignTaskPayload{
		TaskID:      "task-123",
		PlanID:      "plan-456",
		Description: "Test task",
		Branch:      "urp/plan-456/task-1",
	}

	if err := enc.Send(MsgAssignTask, task); err != nil {
		t.Fatalf("encode: %v", err)
	}

	// Decode
	env, err := dec.Decode()
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	if env.Type != MsgAssignTask {
		t.Errorf("expected type %s, got %s", MsgAssignTask, env.Type)
	}

	// Extract payload
	got, err := env.AsAssignTask()
	if err != nil {
		t.Fatalf("AsAssignTask: %v", err)
	}

	if got.TaskID != task.TaskID {
		t.Errorf("TaskID: expected %s, got %s", task.TaskID, got.TaskID)
	}
	if got.Branch != task.Branch {
		t.Errorf("Branch: expected %s, got %s", task.Branch, got.Branch)
	}
}

func TestMultipleMessages(t *testing.T) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf)

	// Send multiple messages
	messages := []MessageType{MsgPing, MsgPong, MsgReady}
	for _, m := range messages {
		enc.Send(m, nil)
	}

	// Decode all
	dec := NewDecoder(&buf)
	for i, expected := range messages {
		env, err := dec.Decode()
		if err != nil {
			t.Fatalf("decode %d: %v", i, err)
		}
		if env.Type != expected {
			t.Errorf("message %d: expected %s, got %s", i, expected, env.Type)
		}
	}

	// EOF
	_, err := dec.Decode()
	if err != io.EOF {
		t.Errorf("expected EOF, got %v", err)
	}
}

func TestWorkerMasterCommunication(t *testing.T) {
	// Create pipes for bidirectional communication
	masterToWorker := &bytes.Buffer{}
	workerToMaster := &bytes.Buffer{}

	// Create worker
	worker := NewWorkerWithIO("worker-1", []string{"go", "git"},
		masterToWorker, workerToMaster)

	taskReceived := make(chan *AssignTaskPayload, 1)

	worker.SetHandler(func(ctx context.Context, task *AssignTaskPayload, reporter *TaskReporter) error {
		taskReceived <- task
		reporter.Progress(0.5, "halfway")
		reporter.Complete("done", []string{"file.go"}, "")
		return nil
	})

	// Send task from "master"
	masterEnc := NewEncoder(masterToWorker)
	masterEnc.Send(MsgAssignTask, &AssignTaskPayload{
		TaskID:      "task-test",
		Description: "Test task",
	})

	// Run worker briefly
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	go worker.Run(ctx)

	// Wait for task
	select {
	case task := <-taskReceived:
		if task.TaskID != "task-test" {
			t.Errorf("expected task-test, got %s", task.TaskID)
		}
	case <-time.After(50 * time.Millisecond):
		t.Error("timeout waiting for task")
	}

	// Check worker output
	output := workerToMaster.String()
	if !strings.Contains(output, `"type":"ready"`) {
		t.Error("expected ready message")
	}
	if !strings.Contains(output, `"type":"task_started"`) {
		t.Error("expected task_started message")
	}
}

func TestStreamWriter(t *testing.T) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	reporter := &TaskReporter{
		enc:      enc,
		taskID:   "task-1",
		workerID: "worker-1",
	}

	// Use stream writer
	stdout := reporter.StdoutWriter()
	stdout.Write([]byte("hello world\n"))

	// Check output
	output := buf.String()
	if !strings.Contains(output, `"stream":"stdout"`) {
		t.Error("expected stdout stream")
	}
	if !strings.Contains(output, "hello world") {
		t.Error("expected hello world")
	}
}

func TestOutputCollector(t *testing.T) {
	collector := NewOutputCollector(nil, "stdout", false)

	collector.Write([]byte("line 1\n"))
	collector.Write([]byte("line 2\n"))

	expected := "line 1\nline 2\n"
	if collector.String() != expected {
		t.Errorf("expected %q, got %q", expected, collector.String())
	}
}

func TestMasterWorkerTracking(t *testing.T) {
	master := NewMaster()

	// Add workers
	var buf1, buf2 bytes.Buffer
	master.AddWorker("w1", strings.NewReader(""), &buf1)
	master.AddWorker("w2", strings.NewReader(""), &buf2)

	workers := master.ListWorkers()
	if len(workers) != 2 {
		t.Errorf("expected 2 workers, got %d", len(workers))
	}

	// All should be idle
	idle := master.ListIdleWorkers()
	if len(idle) != 2 {
		t.Errorf("expected 2 idle, got %d", len(idle))
	}

	// Assign task
	err := master.AssignTask("w1", &AssignTaskPayload{
		TaskID:      "task-1",
		Description: "Test",
	})
	if err != nil {
		t.Fatalf("AssignTask: %v", err)
	}

	// Now only 1 idle
	idle = master.ListIdleWorkers()
	if len(idle) != 1 {
		t.Errorf("expected 1 idle, got %d", len(idle))
	}

	// Check message was sent
	if !strings.Contains(buf1.String(), "task-1") {
		t.Error("expected task to be sent to w1")
	}

	// Remove worker
	master.RemoveWorker("w1")
	workers = master.ListWorkers()
	if len(workers) != 1 {
		t.Errorf("expected 1 worker after remove, got %d", len(workers))
	}
}
