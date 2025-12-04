package container

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// SpawnRequirements holds validation results for spawning workers
type SpawnRequirements struct {
	DockerSocket bool
	ProjectPath  bool
	EnvFile      bool
	SELinux      string // "enforcing", "permissive", "disabled", "unknown"
	Errors       []string
}

// IsValid returns true if all requirements are met
func (r *SpawnRequirements) IsValid() bool {
	return len(r.Errors) == 0
}

// ValidateSpawnRequirements checks environment before spawning a worker
func (m *Manager) ValidateSpawnRequirements(projectPath string) *SpawnRequirements {
	req := &SpawnRequirements{
		SELinux: detectSELinux(),
	}

	// Check docker.sock exists on host
	if _, err := os.Stat("/var/run/docker.sock"); err == nil {
		req.DockerSocket = true
	} else {
		req.Errors = append(req.Errors, "docker.sock not found: workers won't be able to control NeMo")
	}

	// Check project path exists
	absPath, err := filepath.Abs(projectPath)
	if err != nil {
		req.Errors = append(req.Errors, fmt.Sprintf("invalid project path: %s", projectPath))
	} else if _, err := os.Stat(absPath); os.IsNotExist(err) {
		req.Errors = append(req.Errors, fmt.Sprintf("project path does not exist: %s", absPath))
	} else {
		req.ProjectPath = true
	}

	// Check .env file exists
	homeDir := os.Getenv("URP_HOST_HOME")
	if homeDir == "" {
		homeDir, _ = os.UserHomeDir()
	}
	envFile := filepath.Join(homeDir, ".urp-go", ".env")
	if _, err := os.Stat(envFile); os.IsNotExist(err) {
		req.Errors = append(req.Errors, fmt.Sprintf("env file not found: %s (create with ANTHROPIC_API_KEY)", envFile))
	} else {
		req.EnvFile = true
	}

	return req
}

// HealthCheckResult holds worker health status
type HealthCheckResult struct {
	Running      bool
	DockerAccess bool
	Caps         string
	Error        string
}

// VerifyWorkerHealth checks if spawned worker is functional
func (m *Manager) VerifyWorkerHealth(containerName string, timeout time.Duration) *HealthCheckResult {
	result := &HealthCheckResult{}
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		// Check container is running
		out, err := m.run("ps", "-q", "--filter", fmt.Sprintf("name=^%s$", containerName))
		if err != nil || strings.TrimSpace(out) == "" {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		result.Running = true

		// Check docker access inside container
		out, err = m.run("exec", containerName, "sh", "-c", "echo $URP_DOCKER_HEALTHY")
		if err == nil && strings.TrimSpace(out) == "true" {
			result.DockerAccess = true
		}

		// Get capabilities
		out, err = m.run("exec", containerName, "sh", "-c", "echo $URP_WORKER_CAPS")
		if err == nil {
			result.Caps = strings.TrimSpace(out)
		}

		return result
	}

	result.Error = fmt.Sprintf("worker %s failed to start within %v", containerName, timeout)
	return result
}

// detectSELinux returns the current SELinux mode
func detectSELinux() string {
	out, err := exec.Command("getenforce").Output()
	if err != nil {
		// getenforce not found or failed - likely not a SELinux system
		return "unknown"
	}
	mode := strings.TrimSpace(strings.ToLower(string(out)))
	switch mode {
	case "enforcing", "permissive", "disabled":
		return mode
	default:
		return "unknown"
	}
}

// IsSELinuxEnforcing returns true if SELinux is in enforcing mode
func IsSELinuxEnforcing() bool {
	return detectSELinux() == "enforcing"
}

// NeedsSELinuxWorkaround returns true if we need special handling for docker socket
func (m *Manager) NeedsSELinuxWorkaround() bool {
	// Only Podman on SELinux enforcing needs :Z labels
	// Docker with --security-opt label=disable works
	return m.runtime == RuntimePodman && IsSELinuxEnforcing()
}

// WorkerHealth represents the health status of a worker container
type WorkerHealth struct {
	Name    string
	Status  string // "healthy", "unhealthy", "starting", "exited"
	Health  string // Docker HEALTHCHECK status if available
	Running bool
}

// CheckWorkerHealth returns the health status of a specific worker
func (m *Manager) CheckWorkerHealth(containerName string) *WorkerHealth {
	wh := &WorkerHealth{Name: containerName}

	// Check if running
	out, err := m.run("ps", "-a", "--filter", fmt.Sprintf("name=^%s$", containerName), "--format", "{{.Status}}")
	if err != nil || strings.TrimSpace(out) == "" {
		wh.Status = "not_found"
		return wh
	}

	status := strings.TrimSpace(out)
	wh.Running = strings.HasPrefix(status, "Up")

	// Parse health status from docker ps output
	if strings.Contains(status, "(healthy)") {
		wh.Status = "healthy"
		wh.Health = "healthy"
	} else if strings.Contains(status, "(unhealthy)") {
		wh.Status = "unhealthy"
		wh.Health = "unhealthy"
	} else if strings.Contains(status, "(starting)") {
		wh.Status = "starting"
		wh.Health = "starting"
	} else if strings.HasPrefix(status, "Exited") {
		wh.Status = "exited"
	} else if wh.Running {
		wh.Status = "running" // No healthcheck defined
	}

	return wh
}

// ListUnhealthyWorkers returns workers that are unhealthy or exited
func (m *Manager) ListUnhealthyWorkers(projectName string) []*WorkerHealth {
	var unhealthy []*WorkerHealth

	// List all workers for this project
	out, err := m.run("ps", "-a", "--filter", fmt.Sprintf("name=urp-%s-w", projectName), "--format", "{{.Names}}")
	if err != nil {
		return unhealthy
	}

	for _, name := range strings.Split(strings.TrimSpace(out), "\n") {
		if name == "" {
			continue
		}
		health := m.CheckWorkerHealth(name)
		if health.Status == "unhealthy" || health.Status == "exited" {
			unhealthy = append(unhealthy, health)
		}
	}

	return unhealthy
}

// RestartWorker stops and starts a worker container
func (m *Manager) RestartWorker(containerName string) error {
	_, err := m.run("restart", containerName)
	if err != nil {
		return fmt.Errorf("failed to restart worker %s: %w", containerName, err)
	}
	return nil
}

// MonitorAndRestartUnhealthy checks for unhealthy workers and restarts them
// Returns the names of workers that were restarted
func (m *Manager) MonitorAndRestartUnhealthy(projectName string) []string {
	var restarted []string

	unhealthy := m.ListUnhealthyWorkers(projectName)
	for _, w := range unhealthy {
		if err := m.RestartWorker(w.Name); err == nil {
			restarted = append(restarted, w.Name)
		}
	}

	return restarted
}
