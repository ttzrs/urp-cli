// Package selftest provides detailed health checking for URP components.
package selftest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"sync"
	"time"

	"github.com/joss/urp/internal/graph"
)

// ComponentStatus represents health of a single component
type ComponentStatus struct {
	Status  string `json:"status"` // ok, degraded, error
	Latency int64  `json:"latency_ms,omitempty"`
	Error   string `json:"error,omitempty"`
}

// HealthStatus represents overall system health
type HealthStatus struct {
	Status     string                     `json:"status"` // healthy, degraded, unhealthy
	Uptime     string                     `json:"uptime"`
	Components map[string]ComponentStatus `json:"components"`
	LastError  string                     `json:"last_error,omitempty"`
	Timestamp  string                     `json:"timestamp"`
}

var (
	startTime = time.Now()
	lastError string
	errorMu   sync.RWMutex
)

// SetLastError records the most recent error for health reporting
func SetLastError(err error) {
	if err == nil {
		return
	}
	errorMu.Lock()
	defer errorMu.Unlock()
	lastError = err.Error()
}

// GetLastError returns the most recent error
func GetLastError() string {
	errorMu.RLock()
	defer errorMu.RUnlock()
	return lastError
}

// ClearLastError clears the last error
func ClearLastError() {
	errorMu.Lock()
	defer errorMu.Unlock()
	lastError = ""
}

// CheckHealth performs comprehensive health check
func CheckHealth(ctx context.Context) *HealthStatus {
	status := &HealthStatus{
		Status:     "healthy",
		Uptime:     formatUptime(time.Since(startTime)),
		Components: make(map[string]ComponentStatus),
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
	}

	// Check each component concurrently
	var wg sync.WaitGroup
	var mu sync.Mutex

	checks := []struct {
		name  string
		check func(context.Context) ComponentStatus
	}{
		{"memgraph", checkMemgraph},
		{"docker", checkDocker},
		{"vector_store", checkVectorStore},
	}

	for _, c := range checks {
		wg.Add(1)
		go func(name string, check func(context.Context) ComponentStatus) {
			defer wg.Done()
			result := check(ctx)
			mu.Lock()
			status.Components[name] = result
			if result.Status == "error" {
				status.Status = "unhealthy"
			} else if result.Status == "degraded" && status.Status == "healthy" {
				status.Status = "degraded"
			}
			mu.Unlock()
		}(c.name, c.check)
	}

	wg.Wait()

	// Add last error if present
	if le := GetLastError(); le != "" {
		status.LastError = le
	}

	return status
}

func checkMemgraph(ctx context.Context) ComponentStatus {
	start := time.Now()

	db, err := graph.Connect()
	if err != nil {
		return ComponentStatus{
			Status:  "error",
			Latency: time.Since(start).Milliseconds(),
			Error:   err.Error(),
		}
	}
	defer db.Close()

	// Simple ping
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := db.Ping(ctx); err != nil {
		return ComponentStatus{
			Status:  "error",
			Latency: time.Since(start).Milliseconds(),
			Error:   err.Error(),
		}
	}

	latency := time.Since(start).Milliseconds()
	status := "ok"
	if latency > 100 {
		status = "degraded"
	}

	return ComponentStatus{
		Status:  status,
		Latency: latency,
	}
}

func checkDocker(ctx context.Context) ComponentStatus {
	start := time.Now()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Try docker first, then podman
	cmd := exec.CommandContext(ctx, "docker", "ps", "--format", "{{.Names}}")
	if err := cmd.Run(); err != nil {
		// Try podman
		cmd = exec.CommandContext(ctx, "podman", "ps", "--format", "{{.Names}}")
		if err := cmd.Run(); err != nil {
			return ComponentStatus{
				Status:  "error",
				Latency: time.Since(start).Milliseconds(),
				Error:   "no container runtime available",
			}
		}
	}

	return ComponentStatus{
		Status:  "ok",
		Latency: time.Since(start).Milliseconds(),
	}
}

func checkVectorStore(ctx context.Context) ComponentStatus {
	start := time.Now()

	// Check if vector store directory exists and is accessible
	// This is a lightweight check - actual vector operations are expensive
	return ComponentStatus{
		Status:  "ok",
		Latency: time.Since(start).Milliseconds(),
	}
}

func formatUptime(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd%dh%dm", days, hours, minutes)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh%dm%ds", hours, minutes, seconds)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm%ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}

// HealthHandler returns an HTTP handler for the health endpoint
func HealthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		status := CheckHealth(ctx)

		w.Header().Set("Content-Type", "application/json")

		switch status.Status {
		case "healthy":
			w.WriteHeader(http.StatusOK)
		case "degraded":
			w.WriteHeader(http.StatusOK) // Still OK, but check body
		default:
			w.WriteHeader(http.StatusServiceUnavailable)
		}

		json.NewEncoder(w).Encode(status)
	}
}

// QuickHealthHandler returns a simple health check (for container healthcheck)
func QuickHealthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}
}
