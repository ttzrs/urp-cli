// Package alerts provides an alerting system that integrates with Claude hooks.
// Alerts are written to files that Claude hooks can read and inject into context.
package alerts

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/joss/urp/internal/config"
)

// Level represents alert severity
type Level string

const (
	LevelInfo     Level = "info"
	LevelWarning  Level = "warning"
	LevelError    Level = "error"
	LevelCritical Level = "critical"
)

// Alert represents a system alert
type Alert struct {
	ID        string    `json:"id"`
	Level     Level     `json:"level"`
	Component string    `json:"component"`
	Title     string    `json:"title"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
	Resolved  bool      `json:"resolved"`
	Context   map[string]interface{} `json:"context,omitempty"`
}

// Manager handles alert creation and persistence
type Manager struct {
	mu           sync.RWMutex
	alertDir     string
	alerts       []Alert
	maxAlerts    int
	maxAlertFiles int // Maximum number of alert JSON files to keep
}

var (
	globalManager *Manager
	once          sync.Once
)

// Global returns the global alert manager
func Global() *Manager {
	once.Do(func() {
		alertDir := os.Getenv("URP_ALERT_DIR")
		if alertDir == "" {
			alertDir = config.GetPaths().Alerts
		}
		globalManager = NewManager(alertDir)
	})
	return globalManager
}

// NewManager creates a new alert manager
func NewManager(alertDir string) *Manager {
	os.MkdirAll(alertDir, 0755)
	m := &Manager{
		alertDir:      alertDir,
		alerts:        make([]Alert, 0),
		maxAlerts:     100,
		maxAlertFiles: 100, // Keep max 100 alert JSON files
	}
	m.loadFromDisk()
	m.rotateOldFiles() // Clean up old files on startup
	return m
}

// loadFromDisk loads existing alerts from the active.json file
func (m *Manager) loadFromDisk() {
	data, err := os.ReadFile(filepath.Join(m.alertDir, "active.json"))
	if err != nil {
		return // No existing alerts
	}

	var summary struct {
		Alerts []Alert `json:"alerts"`
	}
	if err := json.Unmarshal(data, &summary); err != nil {
		return
	}

	m.alerts = summary.Alerts
}

// Send creates and persists a new alert
func (m *Manager) Send(level Level, component, title, message string, ctx map[string]interface{}) *Alert {
	m.mu.Lock()
	defer m.mu.Unlock()

	alert := Alert{
		ID:        fmt.Sprintf("alert-%d", time.Now().UnixNano()),
		Level:     level,
		Component: component,
		Title:     title,
		Message:   message,
		Timestamp: time.Now().UTC(),
		Context:   ctx,
	}

	m.alerts = append(m.alerts, alert)

	// Trim old alerts
	if len(m.alerts) > m.maxAlerts {
		m.alerts = m.alerts[len(m.alerts)-m.maxAlerts:]
	}

	// Persist to file for Claude hooks
	m.persistAlert(&alert)

	// Update active alerts file
	m.updateActiveAlerts()

	return &alert
}

// Resolve marks an alert as resolved
func (m *Manager) Resolve(alertID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := range m.alerts {
		if m.alerts[i].ID == alertID {
			m.alerts[i].Resolved = true
			break
		}
	}

	m.updateActiveAlerts()
}

// GetActive returns all unresolved alerts
func (m *Manager) GetActive() []Alert {
	m.mu.RLock()
	defer m.mu.RUnlock()

	active := make([]Alert, 0)
	for _, a := range m.alerts {
		if !a.Resolved {
			active = append(active, a)
		}
	}
	return active
}

// GetRecent returns the most recent alerts
func (m *Manager) GetRecent(count int) []Alert {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if count > len(m.alerts) {
		count = len(m.alerts)
	}
	return m.alerts[len(m.alerts)-count:]
}

// persistAlert writes a single alert to a file
func (m *Manager) persistAlert(alert *Alert) {
	filename := filepath.Join(m.alertDir, fmt.Sprintf("%s.json", alert.ID))
	data, _ := json.MarshalIndent(alert, "", "  ")
	os.WriteFile(filename, data, 0644)

	// Rotate old files periodically (every 10 alerts)
	if len(m.alerts)%10 == 0 {
		m.rotateOldFiles()
	}
}

// rotateOldFiles removes old alert JSON files beyond maxAlertFiles
func (m *Manager) rotateOldFiles() {
	entries, err := os.ReadDir(m.alertDir)
	if err != nil {
		return
	}

	// Collect alert files (alert-*.json)
	type alertFile struct {
		name    string
		modTime time.Time
	}
	var alertFiles []alertFile

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		// Only process alert-*.json files, skip active.json and claude-alerts.md
		if len(name) > 6 && name[:6] == "alert-" && filepath.Ext(name) == ".json" {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			alertFiles = append(alertFiles, alertFile{
				name:    name,
				modTime: info.ModTime(),
			})
		}
	}

	// If within limits, nothing to do
	if len(alertFiles) <= m.maxAlertFiles {
		return
	}

	// Sort by modification time (oldest first)
	for i := 0; i < len(alertFiles)-1; i++ {
		for j := i + 1; j < len(alertFiles); j++ {
			if alertFiles[i].modTime.After(alertFiles[j].modTime) {
				alertFiles[i], alertFiles[j] = alertFiles[j], alertFiles[i]
			}
		}
	}

	// Remove oldest files until within limit
	toRemove := len(alertFiles) - m.maxAlertFiles
	for i := 0; i < toRemove; i++ {
		os.Remove(filepath.Join(m.alertDir, alertFiles[i].name))
	}
}

// updateActiveAlerts writes active alerts to a summary file for hooks
func (m *Manager) updateActiveAlerts() {
	active := make([]Alert, 0)
	for _, a := range m.alerts {
		if !a.Resolved {
			active = append(active, a)
		}
	}

	summary := struct {
		Count     int       `json:"count"`
		Updated   time.Time `json:"updated"`
		Alerts    []Alert   `json:"alerts"`
		HasErrors bool      `json:"has_errors"`
	}{
		Count:   len(active),
		Updated: time.Now().UTC(),
		Alerts:  active,
	}

	for _, a := range active {
		if a.Level == LevelError || a.Level == LevelCritical {
			summary.HasErrors = true
			break
		}
	}

	data, _ := json.MarshalIndent(summary, "", "  ")
	os.WriteFile(filepath.Join(m.alertDir, "active.json"), data, 0644)

	// Also write human-readable format for Claude
	m.writeClaudeFormat(active)
}

// writeClaudeFormat writes alerts in a format optimized for Claude context injection
func (m *Manager) writeClaudeFormat(alerts []Alert) {
	if len(alerts) == 0 {
		os.WriteFile(filepath.Join(m.alertDir, "claude-alerts.md"), []byte("# System Status\n\n✓ No active alerts\n"), 0644)
		return
	}

	var content string
	content = "# ⚠️ ACTIVE SYSTEM ALERTS\n\n"
	content += fmt.Sprintf("**%d active alert(s)** - Last updated: %s\n\n", len(alerts), time.Now().UTC().Format(time.RFC3339))

	for _, a := range alerts {
		icon := "ℹ️"
		switch a.Level {
		case LevelWarning:
			icon = "⚠️"
		case LevelError:
			icon = "❌"
		case LevelCritical:
			icon = "🚨"
		}

		content += fmt.Sprintf("## %s [%s] %s\n\n", icon, a.Level, a.Title)
		content += fmt.Sprintf("**Component:** %s\n", a.Component)
		content += fmt.Sprintf("**Time:** %s\n\n", a.Timestamp.Format(time.RFC3339))
		content += fmt.Sprintf("%s\n\n", a.Message)

		if len(a.Context) > 0 {
			content += "**Context:**\n```json\n"
			ctx, _ := json.MarshalIndent(a.Context, "", "  ")
			content += string(ctx)
			content += "\n```\n\n"
		}

		content += "---\n\n"
	}

	os.WriteFile(filepath.Join(m.alertDir, "claude-alerts.md"), []byte(content), 0644)
}

// Convenience functions

// Info sends an info-level alert
func Info(component, title, message string) *Alert {
	return Global().Send(LevelInfo, component, title, message, nil)
}

// Warning sends a warning-level alert
func Warning(component, title, message string) *Alert {
	return Global().Send(LevelWarning, component, title, message, nil)
}

// Error sends an error-level alert
func Error(component, title, message string, ctx map[string]interface{}) *Alert {
	return Global().Send(LevelError, component, title, message, ctx)
}

// Critical sends a critical-level alert
func Critical(component, title, message string, ctx map[string]interface{}) *Alert {
	return Global().Send(LevelCritical, component, title, message, ctx)
}

// Resolve resolves an alert by ID
func Resolve(alertID string) {
	Global().Resolve(alertID)
}

// GetAlertDir returns the alert directory path
func GetAlertDir() string {
	return Global().alertDir
}
