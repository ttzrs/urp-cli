package selftest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFormatUptime(t *testing.T) {
	tests := []struct {
		duration time.Duration
		expected string
	}{
		{30 * time.Second, "30s"},
		{5*time.Minute + 30*time.Second, "5m30s"},
		{2*time.Hour + 15*time.Minute + 30*time.Second, "2h15m30s"},
		{3*24*time.Hour + 5*time.Hour + 30*time.Minute, "3d5h30m"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			got := formatUptime(tt.duration)
			if got != tt.expected {
				t.Errorf("formatUptime(%v) = %s, want %s", tt.duration, got, tt.expected)
			}
		})
	}
}

func TestLastError(t *testing.T) {
	// Clear any existing error
	ClearLastError()

	if got := GetLastError(); got != "" {
		t.Errorf("expected empty string after clear, got %s", got)
	}

	// Set an error
	SetLastError(context.DeadlineExceeded)
	if got := GetLastError(); got != "context deadline exceeded" {
		t.Errorf("expected 'context deadline exceeded', got %s", got)
	}

	// Nil error should not change state
	SetLastError(nil)
	if got := GetLastError(); got != "context deadline exceeded" {
		t.Errorf("nil error should not change state, got %s", got)
	}

	// Clear again
	ClearLastError()
	if got := GetLastError(); got != "" {
		t.Errorf("expected empty string after clear, got %s", got)
	}
}

func TestCheckHealth(t *testing.T) {
	ctx := context.Background()
	status := CheckHealth(ctx)

	if status == nil {
		t.Fatal("CheckHealth returned nil")
	}

	if status.Timestamp == "" {
		t.Error("timestamp should not be empty")
	}

	if status.Uptime == "" {
		t.Error("uptime should not be empty")
	}

	// Should have components
	if len(status.Components) == 0 {
		t.Error("should have at least one component")
	}

	// Status should be one of valid values
	validStatuses := map[string]bool{"healthy": true, "degraded": true, "unhealthy": true}
	if !validStatuses[status.Status] {
		t.Errorf("invalid status: %s", status.Status)
	}
}

func TestHealthHandler(t *testing.T) {
	handler := HealthHandler()

	req := httptest.NewRequest("GET", "/health", nil)
	rec := httptest.NewRecorder()

	handler(rec, req)

	// Should return JSON
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}

	// Should be parseable JSON
	var status HealthStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Errorf("failed to parse response: %v", err)
	}

	// Basic structure check
	if status.Timestamp == "" {
		t.Error("timestamp should not be empty")
	}
}

func TestQuickHealthHandler(t *testing.T) {
	handler := QuickHealthHandler()

	req := httptest.NewRequest("GET", "/health/quick", nil)
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	if rec.Body.String() != "ok" {
		t.Errorf("expected 'ok', got %s", rec.Body.String())
	}
}

func TestComponentStatusValues(t *testing.T) {
	// Test that component statuses are valid
	ctx := context.Background()
	status := CheckHealth(ctx)

	validComponentStatuses := map[string]bool{"ok": true, "degraded": true, "error": true}

	for name, comp := range status.Components {
		if !validComponentStatuses[comp.Status] {
			t.Errorf("component %s has invalid status: %s", name, comp.Status)
		}

		// Latency should be non-negative
		if comp.Latency < 0 {
			t.Errorf("component %s has negative latency: %d", name, comp.Latency)
		}
	}
}

func TestCheckDocker(t *testing.T) {
	ctx := context.Background()
	status := checkDocker(ctx)

	// Should not panic and return valid status
	validStatuses := map[string]bool{"ok": true, "degraded": true, "error": true}
	if !validStatuses[status.Status] {
		t.Errorf("invalid docker status: %s", status.Status)
	}
}

func TestCheckVectorStore(t *testing.T) {
	ctx := context.Background()
	status := checkVectorStore(ctx)

	// Vector store check is lightweight, should always be ok
	if status.Status != "ok" {
		t.Errorf("expected 'ok', got %s", status.Status)
	}
}
