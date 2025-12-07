// Package container worker lifecycle commands.
package container

import (
	"bufio"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/joss/urp/internal/audit"
	"golang.org/x/term"
)

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
	homeDir := ResolveHomeDir()
	// For mounts, always use the HOST path (not the container path)
	envFile := ResolveHostEnvFile()

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
		// SELinux enforcing: use :Z labels instead of disabling
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

// SpawnBrowserWorker creates a specialized worker with Chrome for browser automation.
// Two modes: observer (Chrome Full with GUI) or executor (headless).
func (m *Manager) SpawnBrowserWorker(projectPath string, workerNum int, headless bool) (string, error) {
	startTime := time.Now()

	absPath, err := filepath.Abs(projectPath)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}

	projectName := filepath.Base(absPath)
	mode := "observer"
	if headless {
		mode = "executor"
	}
	containerName := fmt.Sprintf("urp-%s-browser-%s-%d", projectName, mode, workerNum)

	m.project = projectName
	networkName := NetworkName(m.project)
	memgraphName := MemgraphName(m.project)

	// Kill existing with same name
	m.runQuiet("rm", "-f", containerName)

	homeDir := ResolveHomeDir()
	envFile := ResolveHostEnvFile()

	args := []string{"run"}

	if headless {
		// Headless mode: detached, no GUI
		args = append(args, "-d")
	} else {
		// Observer mode: needs X11/Wayland display
		args = append(args, "-d") // Still detached, but with display forwarding
	}

	args = append(args,
		"--name", containerName,
		"--network", networkName,
		"--shm-size=2g", // Chrome needs shared memory
	)

	// Security
	args = append(args, "--security-opt", "no-new-privileges")

	// Chrome-specific: seccomp profile and /dev/shm
	args = append(args,
		"--security-opt", "seccomp=unconfined", // Chrome requires this
	)

	// Display forwarding for observer mode (Chrome Full)
	if !headless {
		// Check for Wayland first, then X11
		waylandDisplay := os.Getenv("WAYLAND_DISPLAY")
		x11Display := os.Getenv("DISPLAY")
		xdgRuntimeDir := os.Getenv("XDG_RUNTIME_DIR")

		if waylandDisplay != "" && xdgRuntimeDir != "" {
			// Wayland display forwarding
			args = append(args,
				"-e", fmt.Sprintf("WAYLAND_DISPLAY=%s", waylandDisplay),
				"-e", fmt.Sprintf("XDG_RUNTIME_DIR=%s", xdgRuntimeDir),
				"-v", fmt.Sprintf("%s/%s:%s/%s", xdgRuntimeDir, waylandDisplay, xdgRuntimeDir, waylandDisplay),
			)
		} else if x11Display != "" {
			// X11 display forwarding
			args = append(args,
				"-e", fmt.Sprintf("DISPLAY=%s", x11Display),
				"-v", "/tmp/.X11-unix:/tmp/.X11-unix:rw",
			)
			// Try to add Xauthority if available
			if xauth := os.Getenv("XAUTHORITY"); xauth != "" {
				args = append(args,
					"-e", fmt.Sprintf("XAUTHORITY=%s", xauth),
					"-v", fmt.Sprintf("%s:%s:ro", xauth, xauth),
				)
			}
		}
	}

	// Volumes
	if m.NeedsSELinuxWorkaround() {
		args = append(args,
			"-v", "/var/run/docker.sock:/var/run/docker.sock:Z",
			"-v", fmt.Sprintf("%s:/workspace:rw:Z", absPath),
			"-v", fmt.Sprintf("%s:/var/lib/urp/vector:Z", VectorVolume(m.project)),
			"-v", fmt.Sprintf("%s:/etc/urp/.env:ro:Z", envFile),
		)
	} else {
		args = append(args,
			"--security-opt", "label=disable",
			"-v", "/var/run/docker.sock:/var/run/docker.sock",
			"-v", fmt.Sprintf("%s:/workspace:rw", absPath),
			"-v", fmt.Sprintf("%s:/var/lib/urp/vector", VectorVolume(m.project)),
			"-v", fmt.Sprintf("%s:/etc/urp/.env:ro", envFile),
		)
	}

	// Environment
	args = append(args,
		"-e", fmt.Sprintf("URP_PROJECT=%s", projectName),
		"-e", fmt.Sprintf("URP_HOST_PATH=%s", absPath),
		"-e", fmt.Sprintf("URP_HOST_HOME=%s", homeDir),
		"-e", fmt.Sprintf("NEO4J_URI=bolt://%s:7687", memgraphName),
		"-e", fmt.Sprintf("URP_NETWORK=%s", networkName),
		"-e", fmt.Sprintf("URP_BROWSER_WORKER_ID=%d", workerNum),
		"-e", fmt.Sprintf("URP_BROWSER_MODE=%s", mode),
		"-e", fmt.Sprintf("URP_BROWSER_HEADLESS=%t", headless),
		"-e", "TERM=xterm-256color",
		"-w", "/workspace",
	)

	// Use browser-capable image
	args = append(args, URPBrowserImage)

	cmd := osexec.CommandContext(m.ctx, string(m.runtime), args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to spawn browser worker: %w: %s", err, string(out))
	}

	// Health check
	health := m.VerifyWorkerHealth(containerName, 15*time.Second)
	if !health.Running {
		audit.SpawnEvent(containerName, projectName, false, time.Since(startTime), fmt.Errorf("browser worker failed: %s", health.Error))
		return "", fmt.Errorf("browser worker failed to start: %s", health.Error)
	}

	audit.SpawnEvent(containerName, projectName, true, time.Since(startTime), nil)
	return containerName, nil
}

// ListBrowserWorkers returns all browser worker containers for a project.
func (m *Manager) ListBrowserWorkers(projectName string) []ContainerStatus {
	var workers []ContainerStatus

	filter := fmt.Sprintf("name=urp-%s-browser", projectName)
	out, _ := m.run("ps", "-a", "--filter", filter,
		"--format", "{{.ID}}\t{{.Names}}\t{{.Image}}\t{{.Status}}")

	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, "\t")
		if len(parts) < 4 {
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
