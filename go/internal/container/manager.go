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
	osexec "os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/joss/urp/internal/audit"
	urpexec "github.com/joss/urp/internal/exec"
	"golang.org/x/term"
)

const (
	MemgraphImage    = "memgraph/memgraph-platform:latest"
	URPImage         = "urp:latest"
	URPWorkerImage   = "urp:worker"
	URPMasterImage   = "urp:master"
	NeMoImage        = "nvcr.io/nvidia/nemo:24.07"
	URPConfigDir     = "~/.urp-go"
	URPEnvFile       = "~/.urp-go/.env"
)

// NetworkName returns the project-specific network name.
// Each project gets its own isolated network: urp-<project>-net
func NetworkName(project string) string {
	if project == "" {
		return "urp-default-net"
	}
	return fmt.Sprintf("urp-%s-net", project)
}

// MemgraphName returns the project-specific memgraph container name.
func MemgraphName(project string) string {
	if project == "" {
		return "urp-memgraph"
	}
	return fmt.Sprintf("urp-%s-memgraph", project)
}

// VolumeName returns a project-specific volume name.
func VolumeName(project, suffix string) string {
	if project == "" {
		return fmt.Sprintf("urp_%s", suffix)
	}
	return fmt.Sprintf("urp_%s_%s", project, suffix)
}

// Volume name helpers for common volumes
func SessionsVolume(project string) string { return VolumeName(project, "sessions") }
func ChromaVolume(project string) string   { return VolumeName(project, "chroma") }
func VectorVolume(project string) string   { return VolumeName(project, "vector") }

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
	project string // Project name for scoped resources
	runner  urpexec.Runner
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
		runner:  urpexec.NewOSRunner(),
	}
}

// NewManagerForProject creates a manager for a specific project.
func NewManagerForProject(ctx context.Context, project string) *Manager {
	return &Manager{
		runtime: detectRuntime(),
		ctx:     ctx,
		project: project,
		runner:  urpexec.NewOSRunner(),
	}
}

// NewManagerWithRunner creates a manager with a custom runner (for testing).
func NewManagerWithRunner(ctx context.Context, runner urpexec.Runner) *Manager {
	return &Manager{
		runtime: detectRuntime(),
		ctx:     ctx,
		runner:  runner,
	}
}

// Project returns the project name (empty string if not set).
func (m *Manager) Project() string {
	return m.project
}

// SetProject sets the project name for scoped resources.
func (m *Manager) SetProject(project string) {
	m.project = project
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
	if _, err := osexec.LookPath("docker"); err == nil {
		return RuntimeDocker
	}
	if _, err := osexec.LookPath("podman"); err == nil {
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
	out, err := m.runner.Run(m.ctx, string(m.runtime), args...)
	return strings.TrimSpace(string(out)), err
}

// runQuiet runs command, ignoring errors (for idempotent ops).
func (m *Manager) runQuiet(args ...string) string {
	out, _ := m.run(args...)
	return out
}

// Status returns current infrastructure status for the manager's project.
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

	networkName := NetworkName(m.project)
	memgraphName := MemgraphName(m.project)

	// Check network
	out, _ := m.run("network", "ls", "--format", "{{.Name}}")
	status.Network = strings.Contains(out, networkName)

	// Check memgraph
	out, _ = m.run("ps", "-a", "--filter", fmt.Sprintf("name=%s", memgraphName),
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

	// Check volumes (project-specific prefix)
	out, _ = m.run("volume", "ls", "--format", "{{.Name}}")
	prefix := "urp_"
	if m.project != "" {
		prefix = fmt.Sprintf("urp_%s_", m.project)
	}
	for _, name := range strings.Split(out, "\n") {
		if strings.HasPrefix(name, prefix) {
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
		if len(parts) < 4 || parts[1] == memgraphName {
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
// All resources are project-scoped. No ports are exposed to host.
func (m *Manager) StartInfra() error {
	if m.runtime == RuntimeNone {
		return fmt.Errorf("no container runtime found")
	}

	networkName := NetworkName(m.project)
	memgraphName := MemgraphName(m.project)

	// Create network
	m.runQuiet("network", "create", networkName)

	// Create volumes (project-scoped)
	m.runQuiet("volume", "create", SessionsVolume(m.project))
	m.runQuiet("volume", "create", ChromaVolume(m.project))
	m.runQuiet("volume", "create", VectorVolume(m.project))

	// Start memgraph if not running
	out, _ := m.run("ps", "-q", "--filter", fmt.Sprintf("name=%s", memgraphName))
	if out == "" {
		// Check if exists but stopped
		out, _ = m.run("ps", "-aq", "--filter", fmt.Sprintf("name=%s", memgraphName))
		if out != "" {
			// Start existing
			_, err := m.run("start", memgraphName)
			if err != nil {
				return fmt.Errorf("failed to start memgraph: %w", err)
			}
		} else {
			// Create and run - NO PORT MAPPINGS (network-only access)
			args := []string{
				"run", "-d",
				"--name", memgraphName,
				"--network", networkName,
				// No -p flags: containers access via network name, not host ports
				"-v", fmt.Sprintf("%s:/var/lib/memgraph", SessionsVolume(m.project)),
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
	memgraphName := MemgraphName(m.project)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		// Try to connect via bolt
		out, err := m.run("exec", memgraphName, "bash", "-c", "echo 'RETURN 1;' | mgconsole")
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

// CleanInfra removes all URP containers, volumes, and network for this project.
func (m *Manager) CleanInfra() error {
	if m.runtime == RuntimeNone {
		return fmt.Errorf("no container runtime found")
	}

	networkName := NetworkName(m.project)

	// Stop and remove containers for this project
	filter := "name=urp-"
	if m.project != "" {
		filter = fmt.Sprintf("name=urp-%s", m.project)
	}
	out, _ := m.run("ps", "-aq", "--filter", filter)
	if out != "" {
		ids := strings.Split(out, "\n")
		for _, id := range ids {
			if id != "" {
				m.runQuiet("rm", "-f", id)
			}
		}
	}

	// Remove volumes (project-scoped)
	m.runQuiet("volume", "rm", "-f", SessionsVolume(m.project))
	m.runQuiet("volume", "rm", "-f", ChromaVolume(m.project))
	m.runQuiet("volume", "rm", "-f", VectorVolume(m.project))

	// Remove network
	m.runQuiet("network", "rm", networkName)

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

	// Set project for scoped resources
	m.project = projectName
	networkName := NetworkName(m.project)
	memgraphName := MemgraphName(m.project)

	args := []string{
		"run", "-d",
		"--name", containerName,
		"--network", networkName,
		"--security-opt", "label=disable",
		"--security-opt", "no-new-privileges", // Prevent privilege escalation
		"-v", fmt.Sprintf("%s:/workspace:%s", absPath, mountOpt),
		"-v", fmt.Sprintf("%s:/var/lib/urp/vector", VectorVolume(m.project)),
		"-v", fmt.Sprintf("%s:/etc/urp/.env:ro", envFile),
		"-e", fmt.Sprintf("URP_PROJECT=%s", projectName),
		"-e", fmt.Sprintf("NEO4J_URI=bolt://%s:7687", memgraphName),
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
// Master includes Firefox for web GUI access to services.
func (m *Manager) LaunchMaster(projectPath string) (string, error) {
	if m.runtime == RuntimeNone {
		return "", fmt.Errorf("no container runtime found")
	}

	absPath, err := filepath.Abs(projectPath)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}

	projectName := filepath.Base(absPath)
	containerName := fmt.Sprintf("urp-master-%s", projectName)

	// Set project for scoped resources
	m.project = projectName
	networkName := NetworkName(m.project)
	memgraphName := MemgraphName(m.project)

	// Ensure infra
	status := m.Status()
	if status.Memgraph == nil || !strings.Contains(status.Memgraph.Status, "Up") {
		if err := m.StartInfra(); err != nil {
			return "", err
		}
	}

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

	// Alerts directory
	alertsDir := filepath.Join(homeDir, ".urp-go", "alerts")
	os.MkdirAll(alertsDir, 0755)

	args = append(args,
		"--name", containerName,
		"--network", networkName,
		// Disable SELinux for docker socket access
		"--security-opt", "label=disable",
		// Project: read-only
		"-v", fmt.Sprintf("%s:/workspace:ro", absPath),
		// Docker socket for spawning workers
		"-v", fmt.Sprintf("%s:/var/run/docker.sock", socketPath),
		// Vector store
		"-v", fmt.Sprintf("%s:/var/lib/urp/vector", VectorVolume(m.project)),
		// Alerts directory for Claude hooks
		"-v", fmt.Sprintf("%s:/var/lib/urp/alerts", alertsDir),
		// Env file
		"-v", fmt.Sprintf("%s:/etc/urp/.env:ro", envFile),
		// X11 socket for Firefox GUI (if available)
		"-v", "/tmp/.X11-unix:/tmp/.X11-unix:ro",
		// Environment
		"-e", fmt.Sprintf("URP_PROJECT=%s", projectName),
		"-e", fmt.Sprintf("URP_HOST_PATH=%s", absPath),
		"-e", fmt.Sprintf("URP_HOST_HOME=%s", homeDir),
		"-e", fmt.Sprintf("NEO4J_URI=bolt://%s:7687", memgraphName),
		"-e", fmt.Sprintf("URP_NETWORK=%s", networkName),
		"-e", "URP_MASTER=1",
		"-e", "URP_READ_ONLY=true",
		"-e", "TERM=xterm-256color",
		// X11 display for Firefox
		"-e", fmt.Sprintf("DISPLAY=%s", os.Getenv("DISPLAY")),
		"-w", "/workspace",
	)

	// Image - entrypoint handles TTY vs daemon mode
	args = append(args, URPMasterImage)

	cmd := osexec.CommandContext(m.ctx, string(m.runtime), args...)

	if hasTTY {
		// Interactive mode: attach stdin/stdout/stderr
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err = cmd.Run()
		if err != nil {
			// Exit code from claude is normal
			if exitErr, ok := err.(*osexec.ExitError); ok {
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
		checkOutput, _ := m.run("ps", "--filter", fmt.Sprintf("name=%s", containerName), "--format", "{{.Names}}")
		if !strings.Contains(checkOutput, containerName) {
			// Container didn't stay running, check logs
			logs, _ := m.run("logs", containerName)
			return "", fmt.Errorf("master container exited immediately (logs: %s)", logs)
		}
	}

	return containerName, nil
}

// SpawnWorker creates a worker container from inside a master.
// Worker has read-write access. Master sends instructions via urp ask.
func (m *Manager) SpawnWorker(projectPath string, workerNum int) (string, error) {
	startTime := time.Now()

	absPath, err := filepath.Abs(projectPath)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}

	projectName := filepath.Base(absPath)
	containerName := fmt.Sprintf("urp-%s-w%d", projectName, workerNum)

	// Set project for scoped resources
	m.project = projectName
	networkName := NetworkName(m.project)
	memgraphName := MemgraphName(m.project)

	// Validate requirements before spawning
	req := m.ValidateSpawnRequirements(projectPath)
	if !req.IsValid() {
		return "", fmt.Errorf("spawn requirements not met: %s", strings.Join(req.Errors, "; "))
	}

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
		"--network", networkName,
	)

	// Security: prevent privilege escalation
	args = append(args, "--security-opt", "no-new-privileges")

	// SELinux handling: use appropriate security options based on runtime and SELinux mode
	if m.NeedsSELinuxWorkaround() {
		// Podman with SELinux enforcing: use :Z labels instead of disabling
		args = append(args,
			"-v", "/var/run/docker.sock:/var/run/docker.sock:Z",
			"-v", fmt.Sprintf("%s:/workspace:rw:Z", absPath),
			"-v", fmt.Sprintf("%s:/var/lib/urp/vector:Z", VectorVolume(m.project)),
			"-v", fmt.Sprintf("%s:/etc/urp/.env:ro:Z", envFile),
		)
	} else {
		// Docker or SELinux permissive/disabled: disable labels for socket access
		args = append(args,
			"--security-opt", "label=disable",
			"-v", "/var/run/docker.sock:/var/run/docker.sock",
			"-v", fmt.Sprintf("%s:/workspace:rw", absPath),
			"-v", fmt.Sprintf("%s:/var/lib/urp/vector", VectorVolume(m.project)),
			"-v", fmt.Sprintf("%s:/etc/urp/.env:ro", envFile),
		)
	}

	// Environment variables
	args = append(args,
		"-e", fmt.Sprintf("URP_PROJECT=%s", projectName),
		"-e", fmt.Sprintf("URP_HOST_PATH=%s", absPath),
		"-e", fmt.Sprintf("URP_HOST_HOME=%s", homeDir),
		"-e", fmt.Sprintf("NEO4J_URI=bolt://%s:7687", memgraphName),
		"-e", fmt.Sprintf("URP_NETWORK=%s", networkName),
		"-e", fmt.Sprintf("URP_WORKER_ID=%d", workerNum),
		"-e", fmt.Sprintf("URP_SELINUX=%s", req.SELinux),
		"-e", "URP_READ_ONLY=false",
		"-e", "TERM=xterm-256color",
		"-w", "/workspace",
	)

	args = append(args, URPWorkerImage)

	cmd := osexec.CommandContext(m.ctx, string(m.runtime), args...)

	if hasTTY {
		// Interactive mode: attach stdin/stdout/stderr
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	err = cmd.Run()
	if err != nil {
		if exitErr, ok := err.(*osexec.ExitError); ok {
			if exitErr.ExitCode() == 0 {
				return containerName, nil
			}
		}
		return "", fmt.Errorf("worker exited: %w", err)
	}

	// Post-spawn health check (only for detached mode)
	if !hasTTY {
		health := m.VerifyWorkerHealth(containerName, 10*time.Second)
		if !health.Running {
			audit.SpawnEvent(containerName, projectName, false, time.Since(startTime), fmt.Errorf("failed to start: %s", health.Error))
			return "", fmt.Errorf("worker failed to start: %s", health.Error)
		}
		if !health.DockerAccess {
			// Warn but don't fail - worker might not need NeMo
			fmt.Fprintf(os.Stderr, "Warning: worker %s started but docker access unavailable (NeMo disabled)\n", containerName)
		}
	}

	audit.SpawnEvent(containerName, projectName, true, time.Since(startTime), nil)
	return containerName, nil
}


// ListWorkers returns all worker containers for a project.
func (m *Manager) ListWorkers(projectName string) []ContainerStatus {
	var workers []ContainerStatus

	filter := "name=urp-"
	if projectName != "" {
		filter = fmt.Sprintf("name=urp-%s", projectName)
	}
	memgraphName := MemgraphName(projectName)

	out, _ := m.run("ps", "-a", "--filter", filter,
		"--format", "{{.ID}}\t{{.Names}}\t{{.Image}}\t{{.Status}}")

	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, "\t")
		if len(parts) < 4 || parts[1] == memgraphName {
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

	cmd := osexec.Command(string(m.runtime), "exec", "-it", containerName, "/bin/bash")
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

	cmd := osexec.Command(string(m.runtime), "exec", containerName, "/bin/bash", "-c", command)
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
	homeDir := os.Getenv("URP_HOST_HOME")
	if homeDir == "" {
		homeDir, _ = os.UserHomeDir()
	}
	envFile := filepath.Join(homeDir, ".urp-go", ".env")

	args := []string{
		"run", "-d",
		"--name", containerName,
		"--network", networkName,
		"--security-opt", "label=disable", // SELinux compatibility
		"--security-opt", "no-new-privileges", // Prevent privilege escalation
		"--cap-drop", "ALL", // Drop all capabilities (NeMo doesn't need them)
		"--user", "1000:1000", // Run as non-root
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
