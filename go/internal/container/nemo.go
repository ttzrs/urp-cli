// Package container NeMo GPU container commands.
package container

import (
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/joss/urp/internal/audit"
)

// GPUStatus represents GPU availability information
type GPUStatus struct {
	Available   bool
	DeviceCount int
	Reason      string
}

// CheckGPU returns detailed GPU availability status
func (m *Manager) CheckGPU() *GPUStatus {
	status := &GPUStatus{}

	// Try nvidia-smi first (most reliable)
	out, err := m.runner.Run(m.ctx, "nvidia-smi", "--query-gpu=count", "--format=csv,noheader")
	if err == nil {
		status.Available = true
		// Parse device count
		var count int
		if _, err := fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &count); err == nil {
			status.DeviceCount = count
		} else {
			status.DeviceCount = 1 // At least one if nvidia-smi works
		}
		return status
	}

	// nvidia-smi failed, check why
	if _, err := osexec.LookPath("nvidia-smi"); err != nil {
		status.Reason = "nvidia-smi not found (NVIDIA drivers not installed)"
		return status
	}

	// nvidia-smi exists but failed - likely no GPU
	status.Reason = "no NVIDIA GPU detected"
	return status
}

// hasNvidiaGPU checks if NVIDIA GPU drivers are available
func (m *Manager) hasNvidiaGPU() bool {
	return m.CheckGPU().Available
}

// LaunchNeMo starts a NeMo GPU container for ML tasks.
// Called by worker to delegate GPU-intensive operations.
// GPU support is auto-detected; falls back to CPU mode if unavailable.
// Returns container name for subsequent exec commands.
func (m *Manager) LaunchNeMo(projectPath string, containerName string) (string, error) {
	startTime := time.Now()

	if m.runtime == RuntimeNone {
		return "", fmt.Errorf("no container runtime found")
	}

	// When running inside a worker container, use URP_HOST_PATH for the actual host path
	// Otherwise filepath.Abs("/workspace") returns container path, not host path
	hostPath := os.Getenv("URP_HOST_PATH")
	if hostPath == "" {
		var err error
		hostPath, err = filepath.Abs(projectPath)
		if err != nil {
			return "", fmt.Errorf("invalid path: %w", err)
		}
	}

	absPath := hostPath
	projectName := filepath.Base(absPath)

	if containerName == "" {
		containerName = fmt.Sprintf("urp-nemo-%s", projectName)
	}

	// Set project for scoped resources
	m.project = projectName
	networkName := NetworkName(m.project)
	memgraphName := MemgraphName(m.project)

	// Kill existing if any
	m.runQuiet("rm", "-f", containerName)

	// Get env file path
	envFile := ResolveEnvFile()

	args := []string{
		"run", "-d",
		"--name", containerName,
		"--network", networkName,
		"--security-opt", "label=disable",          // SELinux compatibility
		"--security-opt", "no-new-privileges",      // Prevent privilege escalation
		"--cap-drop", "ALL",                        // Drop all capabilities (NeMo doesn't need them)
		"--user", "1000:1000",                      // Run as non-root
	}

	// Auto-detect GPU availability with graceful fallback
	gpuMode := "cpu"
	if m.hasNvidiaGPU() {
		args = append(args, "--gpus", "all")
		args = append(args, "--shm-size", "16g") // Shared memory for PyTorch
		gpuMode = "gpu"
	}

	// Volumes and environment
	args = append(args,
		"-v", fmt.Sprintf("%s:/workspace:rw", absPath),
		"-v", fmt.Sprintf("%s:/var/lib/urp/vector", VectorVolume(m.project)),
		"-v", fmt.Sprintf("%s:/etc/urp/.env:ro", envFile),
		"-e", fmt.Sprintf("NEO4J_URI=bolt://%s:7687", memgraphName),
		"-e", fmt.Sprintf("URP_GPU_MODE=%s", gpuMode), // Expose GPU mode to container
		"-w", "/workspace",
		NeMoImage,
		"tail", "-f", "/dev/null", // Stay alive for exec
	)

	_, runErr := m.run(args...)
	if runErr != nil {
		audit.NeMoEvent("launch", containerName, projectName, time.Since(startTime), runErr)
		return "", fmt.Errorf("failed to launch NeMo: %w", runErr)
	}

	audit.NeMoEvent("launch", containerName, projectName, time.Since(startTime), nil)
	return containerName, nil
}

// ExecNeMo runs a command in the NeMo container.
func (m *Manager) ExecNeMo(containerName string, command string) (string, error) {
	return m.run("exec", containerName, "/bin/bash", "-c", command)
}

// KillNeMo stops and removes a NeMo container.
func (m *Manager) KillNeMo(containerName string) error {
	startTime := time.Now()
	_, err := m.run("rm", "-f", containerName)
	audit.NeMoEvent("kill", containerName, "", time.Since(startTime), err)
	return err
}
