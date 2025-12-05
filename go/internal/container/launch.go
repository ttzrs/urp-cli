// Package container launch commands for standalone and master containers.
package container

import (
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/term"
)

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
