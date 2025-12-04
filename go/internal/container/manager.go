// Package container manages Docker/Podman infrastructure for URP.
//
// # Architecture
//
// URP supports two container modes:
//
// ## Master/Worker Flow (Primary)
//
// User launches master container (read-only workspace):
//
//	urp launch /path/to/project
//	  → LaunchMaster() → urp:master container
//	  → Opens Claude CLI for user interaction
//
// Master spawns workers (read-write workspace):
//
//	urp spawn        # from inside master
//	  → SpawnWorker() → urp:worker container
//	  → Worker enters daemon mode (stays alive)
//
// Master sends instructions via Claude CLI:
//
//	urp ask urp-proj-w1 "run tests and fix failures"
//	  → docker exec worker claude --print "..."
//	  → Worker's Claude CLI executes, reports to stdout
//
// All git/code operations happen in worker via Claude instructions.
// Master NEVER writes to workspace.
//
// ## Standalone Mode (Simple)
//
//	urp launch --worker /path/to/project
//	  → LaunchStandalone() → urp:latest container
//	  → For simple CLI access without orchestration
//
// # Images
//
//   - urp:master - Full + Claude CLI + docker-cli (spawns workers)
//   - urp:worker - Full + Claude CLI + dev tools (executes tasks)
//   - urp:latest - Full image (standalone use)
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

	"golang.org/x/term"
)

const (
	NetworkName      = "urp-network"
	MemgraphName     = "urp-memgraph"
	SessionsVolume   = "urp_sessions"
	ChromaVolume     = "urp_chroma"
	VectorVolume     = "urp_vector"
	MemgraphImage    = "memgraph/memgraph-platform:latest"
	URPImage         = "urp:latest"
	URPWorkerImage   = "urp:worker"
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

// LaunchStandalone starts a standalone URP container (no master/worker flow).
// Use this for simple CLI access or background daemon.
// For master/worker flow, use LaunchMaster() + SpawnWorker().
func (m *Manager) LaunchStandalone(projectPath string, readOnly bool) (string, error) {
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
		"--security-opt", "label=disable",
		"-v", fmt.Sprintf("%s:/workspace:%s", absPath, mountOpt),
		"-v", fmt.Sprintf("%s:/var/lib/urp/vector", VectorVolume),
		"-v", fmt.Sprintf("%s:/etc/urp/.env:ro", envFile),
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

	// Expand home directory for env file (resolve symlinks for Silverblue /var/home)
	homeDir, _ := os.UserHomeDir()
	if realHome, err := filepath.EvalSymlinks(homeDir); err == nil {
		homeDir = realHome
	}
	envFile := filepath.Join(homeDir, ".urp-go", ".env")

	// Check if we have a TTY available
	hasTTY := term.IsTerminal(int(os.Stdin.Fd()))

	args := []string{
		"run",
	}

	if hasTTY {
		// Interactive mode with TTY
		args = append(args, "-it", "--rm")
	} else {
		// Detached mode for non-TTY (e.g., Claude Code)
		// Don't use --rm so container persists for urp attach
		args = append(args, "-d")
	}

	args = append(args,
		"--name", containerName,
		"--network", NetworkName,
		// Disable SELinux for docker socket access
		"--security-opt", "label=disable",
		// Project: read-only
		"-v", fmt.Sprintf("%s:/workspace:ro", absPath),
		// Docker socket for spawning workers
		"-v", fmt.Sprintf("%s:/var/run/docker.sock", socketPath),
		// Vector store
		"-v", fmt.Sprintf("%s:/var/lib/urp/vector", VectorVolume),
		// Env file
		"-v", fmt.Sprintf("%s:/etc/urp/.env:ro", envFile),
		// Environment
		"-e", fmt.Sprintf("URP_PROJECT=%s", projectName),
		"-e", fmt.Sprintf("URP_HOST_PATH=%s", absPath),
		"-e", fmt.Sprintf("URP_HOST_HOME=%s", homeDir),
		"-e", fmt.Sprintf("NEO4J_URI=bolt://%s:7687", MemgraphName),
		"-e", "URP_MASTER=1",
		"-e", "URP_READ_ONLY=true",
		"-e", "TERM=xterm-256color",
		"-w", "/workspace",
	)

	// Image - entrypoint handles TTY vs daemon mode
	args = append(args, URPMasterImage)

	cmd := exec.CommandContext(m.ctx, string(m.runtime), args...)

	if hasTTY {
		// Interactive mode: attach stdin/stdout/stderr
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
	} else {
		// Detached mode: capture output to get container ID
		output, err := cmd.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("failed to start master: %w (output: %s)", err, string(output))
		}
		// Verify container is running
		time.Sleep(500 * time.Millisecond)
		checkCmd := exec.Command(string(m.runtime), "ps", "--filter", fmt.Sprintf("name=%s", containerName), "--format", "{{.Names}}")
		checkOutput, _ := checkCmd.Output()
		if !strings.Contains(string(checkOutput), containerName) {
			// Container didn't stay running, check logs
			logsCmd := exec.Command(string(m.runtime), "logs", containerName)
			logs, _ := logsCmd.CombinedOutput()
			return "", fmt.Errorf("master container exited immediately (logs: %s)", string(logs))
		}
	}

	return containerName, nil
}

// SpawnWorker creates a worker container from inside a master.
// Worker has read-write access. Master sends instructions via urp ask.
func (m *Manager) SpawnWorker(projectPath string, workerNum int) (string, error) {
	absPath, err := filepath.Abs(projectPath)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}

	projectName := filepath.Base(absPath)
	containerName := fmt.Sprintf("urp-%s-w%d", projectName, workerNum)

	// Kill existing worker with same name (if any)
	m.runQuiet("rm", "-f", containerName)

	// Use host home from env (when running inside master) or local home
	homeDir := os.Getenv("URP_HOST_HOME")
	if homeDir == "" {
		homeDir, _ = os.UserHomeDir()
	}
	envFile := filepath.Join(homeDir, ".urp-go", ".env")

	// Check if we have a TTY available
	hasTTY := term.IsTerminal(int(os.Stdin.Fd()))

	// Build args - use -it only if TTY available, otherwise -d for detached
	var runMode string
	if hasTTY {
		runMode = "-it"
	} else {
		runMode = "-d" // detached mode for non-TTY (e.g., Claude Code)
	}

	args := []string{"run", runMode}

	// Only use --rm for interactive mode; detached workers stay alive for urp ask
	if hasTTY {
		args = append(args, "--rm")
	}

	args = append(args,
		"--name", containerName,
		"--network", NetworkName,
		// Disable SELinux for container socket access
		"--security-opt", "label=disable",
		// Project: read-write
		"-v", fmt.Sprintf("%s:/workspace:rw", absPath),
		// Vector store
		"-v", fmt.Sprintf("%s:/var/lib/urp/vector", VectorVolume),
		// Env file
		"-v", fmt.Sprintf("%s:/etc/urp/.env:ro", envFile),
		// Environment
		"-e", fmt.Sprintf("URP_PROJECT=%s", projectName),
		"-e", fmt.Sprintf("NEO4J_URI=bolt://%s:7687", MemgraphName),
		"-e", fmt.Sprintf("URP_WORKER_ID=%d", workerNum),
		"-e", "URP_READ_ONLY=false",
		"-e", "TERM=xterm-256color",
		"-w", "/workspace",
	)

	args = append(args, URPWorkerImage)

	cmd := exec.CommandContext(m.ctx, string(m.runtime), args...)

	if hasTTY {
		// Interactive mode: attach stdin/stdout/stderr
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

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

// Exec runs a command in a container with live output.
func (m *Manager) Exec(containerName string, command string) error {
	if m.runtime == RuntimeNone {
		return fmt.Errorf("no container runtime found")
	}

	cmd := exec.Command(string(m.runtime), "exec", containerName, "/bin/bash", "-c", command)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
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
