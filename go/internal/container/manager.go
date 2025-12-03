// Package container manages Docker/Podman infrastructure for URP.
package container

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	NetworkName      = "urp-network"
	MemgraphName     = "urp-memgraph"
	SessionsVolume   = "urp_sessions"
	ChromaVolume     = "urp_chroma"
	VectorVolume     = "urp_vector"
	MemgraphImage    = "memgraph/memgraph-platform:latest"
	URPImage         = "urp:latest"
	URPMasterImage   = "urp:master"
	URPConfigDir     = "~/.urp-go"
	URPEnvFile       = "~/.urp-go/.env"
)

// Runtime represents detected container engine.
type Runtime string

const (
	RuntimeDocker  Runtime = "docker"
	RuntimePodman  Runtime = "podman"
	RuntimeNone    Runtime = ""
)

// Manager handles container orchestration.
type Manager struct {
	runtime Runtime
	ctx     context.Context
}

// ContainerStatus represents a running container.
type ContainerStatus struct {
	ID      string
	Name    string
	Image   string
	Status  string
	Ports   string
	Network string
}

// InfraStatus represents infrastructure state.
type InfraStatus struct {
	Runtime    Runtime
	Network    bool
	Memgraph   *ContainerStatus
	Volumes    []string
	Workers    []ContainerStatus
	Error      string
}

// NewManager creates a container manager, auto-detecting runtime.
func NewManager(ctx context.Context) *Manager {
	return &Manager{
		runtime: detectRuntime(),
		ctx:     ctx,
	}
}

func detectRuntime() Runtime {
	// Allow override via env var
	if override := os.Getenv("URP_RUNTIME"); override != "" {
		switch override {
		case "docker":
			return RuntimeDocker
		case "podman":
			return RuntimePodman
		}
	}
	// Prefer docker (more common), fall back to podman
	if _, err := exec.LookPath("docker"); err == nil {
		return RuntimeDocker
	}
	if _, err := exec.LookPath("podman"); err == nil {
		return RuntimePodman
	}
	return RuntimeNone
}

// Runtime returns the detected container runtime.
func (m *Manager) Runtime() Runtime {
	return m.runtime
}

// run executes a container command and returns output.
func (m *Manager) run(args ...string) (string, error) {
	if m.runtime == RuntimeNone {
		return "", fmt.Errorf("no container runtime found")
	}
	cmd := exec.CommandContext(m.ctx, string(m.runtime), args...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// runQuiet runs command, ignoring errors (for idempotent ops).
func (m *Manager) runQuiet(args ...string) string {
	out, _ := m.run(args...)
	return out
}

// Status returns current infrastructure status.
func (m *Manager) Status() *InfraStatus {
	status := &InfraStatus{
		Runtime: m.runtime,
		Volumes: []string{},
		Workers: []ContainerStatus{},
	}

	if m.runtime == RuntimeNone {
		status.Error = "no container runtime (docker/podman) found"
		return status
	}

	// Check network
	out, _ := m.run("network", "ls", "--format", "{{.Name}}")
	status.Network = strings.Contains(out, NetworkName)

	// Check memgraph
	out, _ = m.run("ps", "-a", "--filter", fmt.Sprintf("name=%s", MemgraphName),
		"--format", "{{.ID}}\t{{.Names}}\t{{.Image}}\t{{.Status}}\t{{.Ports}}")
	if out != "" {
		parts := strings.Split(out, "\t")
		if len(parts) >= 4 {
			status.Memgraph = &ContainerStatus{
				ID:     parts[0],
				Name:   parts[1],
				Image:  parts[2],
				Status: parts[3],
			}
			if len(parts) >= 5 {
				status.Memgraph.Ports = parts[4]
			}
		}
	}

	// Check volumes
	out, _ = m.run("volume", "ls", "--format", "{{.Name}}")
	for _, name := range strings.Split(out, "\n") {
		if strings.HasPrefix(name, "urp_") {
			status.Volumes = append(status.Volumes, name)
		}
	}

	// Check workers (urp-* containers excluding memgraph)
	out, _ = m.run("ps", "-a", "--filter", "name=urp-",
		"--format", "{{.ID}}\t{{.Names}}\t{{.Image}}\t{{.Status}}\t{{.Networks}}")
	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, "\t")
		if len(parts) < 4 || parts[1] == MemgraphName {
			continue
		}
		worker := ContainerStatus{
			ID:     parts[0],
			Name:   parts[1],
			Image:  parts[2],
			Status: parts[3],
		}
		if len(parts) >= 5 {
			worker.Network = parts[4]
		}
		status.Workers = append(status.Workers, worker)
	}

	return status
}

// StartInfra starts shared infrastructure (network, memgraph, volumes).
func (m *Manager) StartInfra() error {
	if m.runtime == RuntimeNone {
		return fmt.Errorf("no container runtime found")
	}

	// Create network
	m.runQuiet("network", "create", NetworkName)

	// Create volumes
	m.runQuiet("volume", "create", SessionsVolume)
	m.runQuiet("volume", "create", ChromaVolume)
	m.runQuiet("volume", "create", VectorVolume)

	// Start memgraph if not running
	out, _ := m.run("ps", "-q", "--filter", fmt.Sprintf("name=%s", MemgraphName))
	if out == "" {
		// Check if exists but stopped
		out, _ = m.run("ps", "-aq", "--filter", fmt.Sprintf("name=%s", MemgraphName))
		if out != "" {
			// Start existing
			_, err := m.run("start", MemgraphName)
			if err != nil {
				return fmt.Errorf("failed to start memgraph: %w", err)
			}
		} else {
			// Create and run
			args := []string{
				"run", "-d",
				"--name", MemgraphName,
				"--network", NetworkName,
				"-p", "7687:7687",
				"-p", "7444:7444",
				"-p", "3000:3000",
				"-v", fmt.Sprintf("%s:/var/lib/memgraph", SessionsVolume),
				"--restart", "unless-stopped",
				MemgraphImage,
			}
			_, err := m.run(args...)
			if err != nil {
				return fmt.Errorf("failed to create memgraph: %w", err)
			}
		}
	}

	// Wait for memgraph to be ready
	return m.waitForMemgraph(30 * time.Second)
}

func (m *Manager) waitForMemgraph(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		// Try to connect via bolt
		out, err := m.run("exec", MemgraphName, "bash", "-c", "echo 'RETURN 1;' | mgconsole")
		if err == nil && strings.Contains(out, "1") {
			return nil
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("memgraph not ready after %v", timeout)
}

// StopInfra stops all URP containers.
func (m *Manager) StopInfra() error {
	if m.runtime == RuntimeNone {
		return fmt.Errorf("no container runtime found")
	}

	// Stop all urp-* containers
	out, _ := m.run("ps", "-aq", "--filter", "name=urp-")
	if out != "" {
		ids := strings.Split(out, "\n")
		for _, id := range ids {
			if id != "" {
				m.runQuiet("stop", id)
			}
		}
	}
	return nil
}

// CleanInfra removes all URP containers, volumes, and network.
func (m *Manager) CleanInfra() error {
	if m.runtime == RuntimeNone {
		return fmt.Errorf("no container runtime found")
	}

	// Stop and remove containers
	out, _ := m.run("ps", "-aq", "--filter", "name=urp-")
	if out != "" {
		ids := strings.Split(out, "\n")
		for _, id := range ids {
			if id != "" {
				m.runQuiet("rm", "-f", id)
			}
		}
	}

	// Remove volumes
	m.runQuiet("volume", "rm", "-f", SessionsVolume)
	m.runQuiet("volume", "rm", "-f", ChromaVolume)
	m.runQuiet("volume", "rm", "-f", VectorVolume)

	// Remove network
	m.runQuiet("network", "rm", NetworkName)

	return nil
}

// LaunchWorker starts a worker container for a project.
func (m *Manager) LaunchWorker(projectPath string, readOnly bool) (string, error) {
	if m.runtime == RuntimeNone {
		return "", fmt.Errorf("no container runtime found")
	}

	// Ensure infra is running
	status := m.Status()
	if status.Memgraph == nil || !strings.Contains(status.Memgraph.Status, "Up") {
		if err := m.StartInfra(); err != nil {
			return "", err
		}
	}

	// Resolve project path
	absPath, err := filepath.Abs(projectPath)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}

	projectName := filepath.Base(absPath)
	containerName := fmt.Sprintf("urp-%s", projectName)
	if readOnly {
		containerName = fmt.Sprintf("urp-ro-%s", projectName)
	}

	// Check if already running
	out, _ := m.run("ps", "-q", "--filter", fmt.Sprintf("name=^%s$", containerName))
	if out != "" {
		return containerName, nil // Already running
	}

	// Mount mode
	mountOpt := "rw"
	if readOnly {
		mountOpt = "ro"
	}

	// Expand home directory for env file
	homeDir, _ := os.UserHomeDir()
	envFile := filepath.Join(homeDir, ".urp-go", ".env")

	args := []string{
		"run", "-d",
		"--name", containerName,
		"--network", NetworkName,
		"-v", fmt.Sprintf("%s:/workspace:%s,z", absPath, mountOpt),
		"-v", fmt.Sprintf("%s:/var/lib/urp/vector:z", VectorVolume),
		"-v", fmt.Sprintf("%s:/etc/urp/.env:ro,Z", envFile),
		"-e", fmt.Sprintf("URP_PROJECT=%s", projectName),
		"-e", fmt.Sprintf("NEO4J_URI=bolt://%s:7687", MemgraphName),
		"-e", fmt.Sprintf("URP_READ_ONLY=%v", readOnly),
		"-w", "/workspace",
		"--restart", "unless-stopped",
		URPImage,
		"tail", "-f", "/dev/null", // Keep alive for background worker
	}

	_, err = m.run(args...)
	if err != nil {
		return "", fmt.Errorf("failed to launch worker: %w", err)
	}

	return containerName, nil
}

// LaunchMaster starts a master container (read-only, can spawn workers).
// Now runs interactively with auto-ingest and Claude CLI.
func (m *Manager) LaunchMaster(projectPath string) (string, error) {
	if m.runtime == RuntimeNone {
		return "", fmt.Errorf("no container runtime found")
	}

	// Ensure infra
	status := m.Status()
	if status.Memgraph == nil || !strings.Contains(status.Memgraph.Status, "Up") {
		if err := m.StartInfra(); err != nil {
			return "", err
		}
	}

	absPath, err := filepath.Abs(projectPath)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}

	projectName := filepath.Base(absPath)
	containerName := fmt.Sprintf("urp-master-%s", projectName)

	// Stop existing if running
	m.runQuiet("rm", "-f", containerName)

	// Mount docker socket for spawning workers
	socketPath := "/var/run/docker.sock"
	if m.runtime == RuntimePodman {
		xdgRuntime := os.Getenv("XDG_RUNTIME_DIR")
		if xdgRuntime != "" {
			socketPath = fmt.Sprintf("%s/podman/podman.sock", xdgRuntime)
		}
	}

	// Expand home directory for env file
	homeDir, _ := os.UserHomeDir()
	envFile := filepath.Join(homeDir, ".urp-go", ".env")

	// Build args for interactive master with SELinux labels
	args := []string{
		"run", "-it", "--rm",
		"--name", containerName,
		"--network", NetworkName,
		// Project: read-only, shared SELinux label (:z)
		"-v", fmt.Sprintf("%s:/workspace:ro,z", absPath),
		// Docker socket for spawning workers
		"-v", fmt.Sprintf("%s:/var/run/docker.sock", socketPath),
		// Vector store: shared
		"-v", fmt.Sprintf("%s:/var/lib/urp/vector:z", VectorVolume),
		// Env file: private SELinux label (:Z)
		"-v", fmt.Sprintf("%s:/etc/urp/.env:ro,Z", envFile),
		// Environment
		"-e", fmt.Sprintf("URP_PROJECT=%s", projectName),
		"-e", fmt.Sprintf("NEO4J_URI=bolt://%s:7687", MemgraphName),
		"-e", "URP_MASTER=1",
		"-e", "URP_READ_ONLY=true",
		"-e", "TERM=xterm-256color",
		"-w", "/workspace",
		URPMasterImage,
	}

	// Run interactively (attach stdin/stdout/stderr)
	cmd := exec.CommandContext(m.ctx, string(m.runtime), args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = cmd.Run()
	if err != nil {
		// Exit code from claude is normal
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == 0 {
				return containerName, nil
			}
		}
		return "", fmt.Errorf("master exited: %w", err)
	}

	return containerName, nil
}

// SpawnWorker creates an ephemeral worker from inside a master container.
// Worker has read-write access and is removed when done (--rm).
// Communication happens via envelope protocol on stdin/stdout.
func (m *Manager) SpawnWorker(projectPath string, workerNum int) (string, error) {
	return m.SpawnWorkerForTask(projectPath, workerNum, "", "")
}

// SpawnWorkerForTask creates a worker with optional task/plan context.
// If taskID is provided, creates a git branch for the task.
func (m *Manager) SpawnWorkerForTask(projectPath string, workerNum int, planID, taskID string) (string, error) {
	absPath, err := filepath.Abs(projectPath)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}

	projectName := filepath.Base(absPath)
	containerName := fmt.Sprintf("urp-%s-w%d", projectName, workerNum)

	// Kill existing worker with same name (if any)
	m.runQuiet("rm", "-f", containerName)

	// Create git branch if task is specified
	branchName := ""
	if taskID != "" {
		branchName = fmt.Sprintf("urp/%s/task-%d", planID, workerNum)
		if err := m.createGitBranch(absPath, branchName); err != nil {
			return "", fmt.Errorf("failed to create branch: %w", err)
		}
	}

	// Expand home directory for env file
	homeDir, _ := os.UserHomeDir()
	envFile := filepath.Join(homeDir, ".urp-go", ".env")

	args := []string{
		"run", "-it", "--rm",
		"--name", containerName,
		"--network", NetworkName,
		// Project: read-write with SELinux shared label
		"-v", fmt.Sprintf("%s:/workspace:rw,z", absPath),
		// Vector store: shared
		"-v", fmt.Sprintf("%s:/var/lib/urp/vector:z", VectorVolume),
		// Env file: private SELinux label
		"-v", fmt.Sprintf("%s:/etc/urp/.env:ro,Z", envFile),
		// Environment
		"-e", fmt.Sprintf("URP_PROJECT=%s", projectName),
		"-e", fmt.Sprintf("NEO4J_URI=bolt://%s:7687", MemgraphName),
		"-e", fmt.Sprintf("URP_WORKER_ID=%d", workerNum),
		"-e", "URP_READ_ONLY=false",
		"-e", "TERM=xterm-256color",
		"-w", "/workspace",
	}

	// Add task context if provided
	if taskID != "" {
		args = append(args,
			"-e", fmt.Sprintf("URP_TASK_ID=%s", taskID),
			"-e", fmt.Sprintf("URP_PLAN_ID=%s", planID),
			"-e", fmt.Sprintf("URP_BRANCH=%s", branchName),
		)
	}

	args = append(args, URPImage)

	// Run interactively
	cmd := exec.CommandContext(m.ctx, string(m.runtime), args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = cmd.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == 0 {
				return containerName, nil
			}
		}
		return "", fmt.Errorf("worker exited: %w", err)
	}

	return containerName, nil
}

// createGitBranch creates and checks out a new branch from current HEAD.
func (m *Manager) createGitBranch(repoPath, branchName string) error {
	// Check if branch exists
	cmd := exec.Command("git", "-C", repoPath, "rev-parse", "--verify", branchName)
	if err := cmd.Run(); err == nil {
		// Branch exists, just checkout
		cmd = exec.Command("git", "-C", repoPath, "checkout", branchName)
		return cmd.Run()
	}

	// Create new branch from current HEAD
	cmd = exec.Command("git", "-C", repoPath, "checkout", "-b", branchName)
	return cmd.Run()
}

// SpawnWorkerBackground creates a detached worker (for automation).
func (m *Manager) SpawnWorkerBackground(projectPath string, workerNum int) (string, error) {
	absPath, err := filepath.Abs(projectPath)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}

	projectName := filepath.Base(absPath)
	containerName := fmt.Sprintf("urp-%s-w%d", projectName, workerNum)

	// Check if exists
	out, _ := m.run("ps", "-q", "--filter", fmt.Sprintf("name=^%s$", containerName))
	if out != "" {
		return containerName, nil
	}

	// Expand home directory for env file
	homeDir, _ := os.UserHomeDir()
	envFile := filepath.Join(homeDir, ".urp-go", ".env")

	args := []string{
		"run", "-d",
		"--name", containerName,
		"--network", NetworkName,
		"-v", fmt.Sprintf("%s:/workspace:rw,z", absPath),
		"-v", fmt.Sprintf("%s:/var/lib/urp/vector:z", VectorVolume),
		"-v", fmt.Sprintf("%s:/etc/urp/.env:ro,Z", envFile),
		"-e", fmt.Sprintf("URP_PROJECT=%s", projectName),
		"-e", fmt.Sprintf("NEO4J_URI=bolt://%s:7687", MemgraphName),
		"-e", fmt.Sprintf("URP_WORKER_ID=%d", workerNum),
		"-e", "URP_READ_ONLY=false",
		"-w", "/workspace",
		URPImage,
		"tail", "-f", "/dev/null", // Keep alive
	}

	_, err = m.run(args...)
	if err != nil {
		return "", fmt.Errorf("failed to spawn worker: %w", err)
	}

	return containerName, nil
}

// ListWorkers returns all worker containers for a project.
func (m *Manager) ListWorkers(projectName string) []ContainerStatus {
	var workers []ContainerStatus

	filter := "name=urp-"
	if projectName != "" {
		filter = fmt.Sprintf("name=urp-%s", projectName)
	}

	out, _ := m.run("ps", "-a", "--filter", filter,
		"--format", "{{.ID}}\t{{.Names}}\t{{.Image}}\t{{.Status}}")

	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, "\t")
		if len(parts) < 4 || parts[1] == MemgraphName {
			continue
		}
		// Skip master containers
		if strings.Contains(parts[1], "-master-") {
			continue
		}
		workers = append(workers, ContainerStatus{
			ID:     parts[0],
			Name:   parts[1],
			Image:  parts[2],
			Status: parts[3],
		})
	}

	return workers
}

// AttachWorker opens interactive shell in a worker.
func (m *Manager) AttachWorker(containerName string) error {
	if m.runtime == RuntimeNone {
		return fmt.Errorf("no container runtime found")
	}

	cmd := exec.Command(string(m.runtime), "exec", "-it", containerName, "/bin/bash")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// ExecInWorker runs a command in a worker container.
func (m *Manager) ExecInWorker(containerName string, command []string) (string, error) {
	args := append([]string{"exec", containerName}, command...)
	return m.run(args...)
}

// KillWorker stops and removes a worker container.
func (m *Manager) KillWorker(containerName string) error {
	_, err := m.run("rm", "-f", containerName)
	return err
}

// KillAllWorkers stops all worker containers for a project.
func (m *Manager) KillAllWorkers(projectName string) error {
	workers := m.ListWorkers(projectName)
	for _, w := range workers {
		m.runQuiet("rm", "-f", w.ID)
	}
	return nil
}

// Logs returns container logs.
func (m *Manager) Logs(containerName string, tail int) (string, error) {
	return m.run("logs", "--tail", fmt.Sprintf("%d", tail), containerName)
}
