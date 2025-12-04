package alerts

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestNewManager(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)

	if m == nil {
		t.Fatal("NewManager returned nil")
	}

	if m.alertDir != dir {
		t.Errorf("expected alertDir %s, got %s", dir, m.alertDir)
	}
}

func TestSendAlert(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)

	alert := m.Send(LevelError, "test-component", "Test Alert", "This is a test", map[string]interface{}{
		"key": "value",
	})

	if alert == nil {
		t.Fatal("Send returned nil")
	}

	if alert.Level != LevelError {
		t.Errorf("expected level error, got %s", alert.Level)
	}

	if alert.Component != "test-component" {
		t.Errorf("expected component 'test-component', got %s", alert.Component)
	}

	// Check file was created
	alertFile := filepath.Join(dir, alert.ID+".json")
	if _, err := os.Stat(alertFile); os.IsNotExist(err) {
		t.Error("alert file was not created")
	}

	// Check active.json was updated
	activeFile := filepath.Join(dir, "active.json")
	if _, err := os.Stat(activeFile); os.IsNotExist(err) {
		t.Error("active.json was not created")
	}

	// Verify active.json content
	data, _ := os.ReadFile(activeFile)
	var summary struct {
		Count     int  `json:"count"`
		HasErrors bool `json:"has_errors"`
	}
	json.Unmarshal(data, &summary)

	if summary.Count != 1 {
		t.Errorf("expected count 1, got %d", summary.Count)
	}

	if !summary.HasErrors {
		t.Error("expected has_errors to be true for error-level alert")
	}
}

func TestResolveAlert(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)

	alert := m.Send(LevelWarning, "test", "Test", "Test message", nil)
	alertID := alert.ID

	// Should have 1 active alert
	active := m.GetActive()
	if len(active) != 1 {
		t.Errorf("expected 1 active alert, got %d", len(active))
	}

	// Resolve it
	m.Resolve(alertID)

	// Should have 0 active alerts
	active = m.GetActive()
	if len(active) != 0 {
		t.Errorf("expected 0 active alerts after resolve, got %d", len(active))
	}
}

func TestGetRecent(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)

	// Send 5 alerts
	for i := 0; i < 5; i++ {
		m.Send(LevelInfo, "test", "Test", "Message", nil)
	}

	// Get 3 most recent
	recent := m.GetRecent(3)
	if len(recent) != 3 {
		t.Errorf("expected 3 recent alerts, got %d", len(recent))
	}

	// Get more than available
	recent = m.GetRecent(10)
	if len(recent) != 5 {
		t.Errorf("expected 5 recent alerts (all), got %d", len(recent))
	}
}

func TestClaudeFormat(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)

	m.Send(LevelCritical, "container", "Worker Failed", "Worker-1 crashed unexpectedly", map[string]interface{}{
		"exit_code": 137,
		"oom":       true,
	})

	// Check claude-alerts.md was created
	claudeFile := filepath.Join(dir, "claude-alerts.md")
	data, err := os.ReadFile(claudeFile)
	if err != nil {
		t.Fatalf("claude-alerts.md not created: %v", err)
	}

	content := string(data)

	// Should contain alert info
	if !contains(content, "ACTIVE SYSTEM ALERTS") {
		t.Error("missing header")
	}

	if !contains(content, "Worker Failed") {
		t.Error("missing alert title")
	}

	if !contains(content, "container") {
		t.Error("missing component")
	}

	if !contains(content, "137") {
		t.Error("missing context data")
	}
}

func TestNoActiveAlerts(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)

	// Send and resolve
	alert := m.Send(LevelInfo, "test", "Test", "Message", nil)
	m.Resolve(alert.ID)

	// Check claude-alerts.md shows no alerts
	claudeFile := filepath.Join(dir, "claude-alerts.md")
	data, _ := os.ReadFile(claudeFile)

	if !contains(string(data), "No active alerts") {
		t.Error("should show 'No active alerts' when all resolved")
	}
}

func TestConvenienceFunctions(t *testing.T) {
	// Skip this test - convenience functions use sync.Once which can't be reset
	// The functions are simple wrappers, tested indirectly via Manager tests
	t.Skip("convenience functions use sync.Once, tested via Manager")
}

func TestLogRotation(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	m.maxAlertFiles = 5 // Low limit for testing

	// Create 10 alerts - should trigger rotation
	for i := 0; i < 10; i++ {
		m.Send(LevelInfo, "test", "Test", "Message", nil)
	}

	// Force rotation
	m.rotateOldFiles()

	// Count remaining alert files
	entries, _ := os.ReadDir(dir)
	var alertCount int
	for _, e := range entries {
		name := e.Name()
		if len(name) > 6 && name[:6] == "alert-" && filepath.Ext(name) == ".json" {
			alertCount++
		}
	}

	if alertCount > m.maxAlertFiles {
		t.Errorf("expected max %d alert files, got %d", m.maxAlertFiles, alertCount)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
