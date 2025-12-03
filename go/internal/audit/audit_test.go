package audit

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestAuditEvent(t *testing.T) {
	event := &AuditEvent{
		EventID:   "test-123",
		Category:  CategoryCode,
		Operation: "ingest",
		StartedAt: time.Now(),
	}

	event.Complete(StatusSuccess, nil)

	if event.Status != StatusSuccess {
		t.Errorf("expected success, got %s", event.Status)
	}
	if event.Duration <= 0 {
		t.Error("expected positive duration")
	}
	if event.DurationMs < 0 {
		t.Error("expected non-negative duration_ms")
	}
}

func TestAuditEventWithError(t *testing.T) {
	event := &AuditEvent{
		EventID:   "test-456",
		Category:  CategoryGit,
		Operation: "ingest",
		StartedAt: time.Now(),
	}

	err := &testError{msg: "test error"}
	event.Complete(StatusError, err)

	if event.Status != StatusError {
		t.Errorf("expected error, got %s", event.Status)
	}
	if event.ErrorMessage != "test error" {
		t.Errorf("expected error message, got %s", event.ErrorMessage)
	}
}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }

func TestGitContext(t *testing.T) {
	ctx := GetGitContext()
	// Should not panic, may have empty values if not in git repo
	_ = ctx.CommitHash
	_ = ctx.Branch
	_ = ctx.IsDirty
}

func TestLogger(t *testing.T) {
	var buf bytes.Buffer
	f, _ := os.CreateTemp("", "audit-test-*.log")
	defer os.Remove(f.Name())

	logger := NewLogger(
		WithSession("test-session"),
		WithProject("test-project"),
		WithOutput(f),
	)

	event := logger.Start(CategoryCode, "test-op")
	time.Sleep(1 * time.Millisecond)
	err := logger.LogSuccess(event)
	if err != nil {
		t.Fatalf("LogSuccess failed: %v", err)
	}

	// Read back
	f.Seek(0, 0)
	var logged AuditEvent
	decoder := json.NewDecoder(f)
	if err := decoder.Decode(&logged); err != nil {
		// May fail if empty, check buffer
		f.Seek(0, 0)
		f.Read(buf.Bytes())
		t.Logf("Buffer: %s", buf.String())
	}

	if logged.SessionID != "test-session" {
		t.Errorf("expected test-session, got %s", logged.SessionID)
	}
	if logged.Category != CategoryCode {
		t.Errorf("expected code, got %s", logged.Category)
	}
}

func TestLoggerCommand(t *testing.T) {
	f, _ := os.CreateTemp("", "audit-cmd-*.log")
	defer os.Remove(f.Name())

	logger := NewLogger(
		WithSession("cmd-test"),
		WithOutput(f),
	)

	logger.LogCommand("urp code stats", 0, 100, nil)

	f.Seek(0, 0)
	var logged AuditEvent
	if err := json.NewDecoder(f).Decode(&logged); err == nil {
		if logged.Command != "urp code stats" {
			t.Errorf("expected command, got %s", logged.Command)
		}
		if logged.ExitCode != 0 {
			t.Errorf("expected exit 0, got %d", logged.ExitCode)
		}
	}
}

func TestCategories(t *testing.T) {
	cats := []Category{
		CategoryCode,
		CategoryOrchestrate,
		CategoryEvents,
		CategoryMemory,
		CategoryKnowledge,
		CategorySystem,
		CategoryGit,
		CategoryInfra,
	}

	for _, c := range cats {
		if c == "" {
			t.Error("empty category")
		}
	}
}

func TestStatuses(t *testing.T) {
	statuses := []Status{
		StatusSuccess,
		StatusError,
		StatusWarning,
		StatusTimeout,
	}

	for _, s := range statuses {
		if s == "" {
			t.Error("empty status")
		}
	}
}
