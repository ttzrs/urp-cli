package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func TestLoggerCreation(t *testing.T) {
	// Set env vars for testing
	os.Setenv("URP_PROJECT", "test-project")
	os.Setenv("URP_WORKER_ID", "w1")
	defer os.Unsetenv("URP_PROJECT")
	defer os.Unsetenv("URP_WORKER_ID")

	logger := New("test-component")

	if logger.component != "test-component" {
		t.Errorf("expected component 'test-component', got '%s'", logger.component)
	}
	if logger.project != "test-project" {
		t.Errorf("expected project 'test-project', got '%s'", logger.project)
	}
	if logger.worker != "w1" {
		t.Errorf("expected worker 'w1', got '%s'", logger.worker)
	}
}

func TestLoggerWithProject(t *testing.T) {
	logger := New("component").WithProject("my-project")

	if logger.project != "my-project" {
		t.Errorf("expected project 'my-project', got '%s'", logger.project)
	}
}

func TestLoggerWithWorker(t *testing.T) {
	logger := New("component").WithWorker("worker-5")

	if logger.worker != "worker-5" {
		t.Errorf("expected worker 'worker-5', got '%s'", logger.worker)
	}
}

func TestEventSerialization(t *testing.T) {
	event := Event{
		Timestamp: "2024-01-01T00:00:00Z",
		Level:     LevelInfo,
		Component: "test",
		Event:     "test_event",
		Project:   "proj",
		Worker:    "w1",
		Duration:  100,
		Error:     "",
		Extra: map[string]interface{}{
			"key": "value",
		},
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("failed to marshal event: %v", err)
	}

	// Verify JSON structure
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal event: %v", err)
	}

	if parsed["level"] != "info" {
		t.Errorf("expected level 'info', got '%v'", parsed["level"])
	}
	if parsed["component"] != "test" {
		t.Errorf("expected component 'test', got '%v'", parsed["component"])
	}
	if parsed["duration_ms"].(float64) != 100 {
		t.Errorf("expected duration_ms 100, got '%v'", parsed["duration_ms"])
	}
}

func TestSpawnEventSuccess(t *testing.T) {
	// Capture stderr
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	SpawnEvent("worker-1", "test-project", true, 500*time.Millisecond, nil)

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Verify JSON output
	var event Event
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &event); err != nil {
		t.Fatalf("failed to parse output as JSON: %v (output: %s)", err, output)
	}

	if event.Level != LevelInfo {
		t.Errorf("expected level 'info', got '%s'", event.Level)
	}
	if event.Component != "container" {
		t.Errorf("expected component 'container', got '%s'", event.Component)
	}
	if event.Event != "spawn" {
		t.Errorf("expected event 'spawn', got '%s'", event.Event)
	}
	if event.Worker != "worker-1" {
		t.Errorf("expected worker 'worker-1', got '%s'", event.Worker)
	}
	if event.Duration != 500 {
		t.Errorf("expected duration 500, got %d", event.Duration)
	}
}

func TestSpawnEventError(t *testing.T) {
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	SpawnEvent("worker-1", "test-project", false, 100*time.Millisecond,
		context.DeadlineExceeded)

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	var event Event
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &event); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}

	if event.Level != LevelError {
		t.Errorf("expected level 'error', got '%s'", event.Level)
	}
	if event.Error == "" {
		t.Error("expected error message to be set")
	}
}

func TestNeMoEvent(t *testing.T) {
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	NeMoEvent("launch", "nemo-container", "project", 2*time.Second, nil)

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	var event Event
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &event); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}

	if event.Component != "nemo" {
		t.Errorf("expected component 'nemo', got '%s'", event.Component)
	}
	if event.Event != "launch" {
		t.Errorf("expected event 'launch', got '%s'", event.Event)
	}
	if event.Duration != 2000 {
		t.Errorf("expected duration 2000ms, got %d", event.Duration)
	}
}

func TestHealthEvent(t *testing.T) {
	tests := []struct {
		name     string
		healthy  bool
		expected Level
	}{
		{"healthy", true, LevelInfo},
		{"unhealthy", false, LevelWarn},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldStderr := os.Stderr
			r, w, _ := os.Pipe()
			os.Stderr = w

			HealthEvent("worker-1", "running", tt.healthy)

			w.Close()
			os.Stderr = oldStderr

			var buf bytes.Buffer
			buf.ReadFrom(r)
			output := buf.String()

			var event Event
			if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &event); err != nil {
				t.Fatalf("failed to parse output: %v", err)
			}

			if event.Level != tt.expected {
				t.Errorf("expected level '%s', got '%s'", tt.expected, event.Level)
			}
		})
	}
}

// Mock GraphWriter for testing
type mockGraphWriter struct {
	writeCount int
	lastQuery  string
	lastParams map[string]any
	shouldFail bool
}

func (m *mockGraphWriter) ExecuteWrite(ctx context.Context, query string, params map[string]any) error {
	m.writeCount++
	m.lastQuery = query
	m.lastParams = params
	if m.shouldFail {
		return context.DeadlineExceeded
	}
	return nil
}

func TestGraphPersistence(t *testing.T) {
	mock := &mockGraphWriter{}
	SetGraphDriver(mock)
	defer SetGraphDriver(nil)

	PersistContainerEvent("spawn", "worker-1", "project", 100, true, "")

	if mock.writeCount != 1 {
		t.Errorf("expected 1 write, got %d", mock.writeCount)
	}
	if mock.lastParams["event"] != "spawn" {
		t.Errorf("expected event 'spawn', got '%v'", mock.lastParams["event"])
	}
	if mock.lastParams["container"] != "worker-1" {
		t.Errorf("expected container 'worker-1', got '%v'", mock.lastParams["container"])
	}
}

func TestGraphPersistenceWithNoDriver(t *testing.T) {
	SetGraphDriver(nil)

	// Should not panic
	PersistContainerEvent("spawn", "worker-1", "project", 100, true, "")
	PersistWorkerSpawn("worker-1", "project", true, 100, "")
	PersistNeMoEvent("launch", "nemo-1", "project", 100, "")
	PersistHealthCheck("worker-1", "running", true)
}
