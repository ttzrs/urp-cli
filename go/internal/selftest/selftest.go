// Package selftest provides runtime environment validation and self-diagnostics.
package selftest

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"golang.org/x/term"
)

// Environment describes the runtime environment.
type Environment struct {
	HasTTY         bool
	Runtime        string // docker, podman, or none
	RuntimeVersion string
	MemgraphUp     bool
	NetworkExists  bool
	ImagesExist    map[string]bool
	Warnings       []string
	Errors         []string
}

// Check performs a complete environment validation.
func Check() *Environment {
	env := &Environment{
		ImagesExist: make(map[string]bool),
	}

	// TTY detection
	env.HasTTY = term.IsTerminal(int(os.Stdin.Fd()))

	// Container runtime
	env.detectRuntime()

	// Check infrastructure if runtime available
	if env.Runtime != "none" {
		env.checkInfrastructure()
		env.checkImages()
	}

	return env
}

func (e *Environment) detectRuntime() {
	// Check docker first
	if path, err := exec.LookPath("docker"); err == nil {
		cmd := exec.Command(path, "--version")
		if out, err := cmd.Output(); err == nil {
			e.Runtime = "docker"
			e.RuntimeVersion = strings.TrimSpace(string(out))
			return
		}
	}

	// Fall back to podman
	if path, err := exec.LookPath("podman"); err == nil {
		cmd := exec.Command(path, "--version")
		if out, err := cmd.Output(); err == nil {
			e.Runtime = "podman"
			e.RuntimeVersion = strings.TrimSpace(string(out))
			return
		}
	}

	e.Runtime = "none"
	e.Errors = append(e.Errors, "No container runtime found (docker or podman)")
}

func (e *Environment) checkInfrastructure() {
	// Check network
	cmd := exec.Command(e.Runtime, "network", "ls", "--filter", "name=urp-network", "--format", "{{.Name}}")
	if out, err := cmd.Output(); err == nil && strings.Contains(string(out), "urp-network") {
		e.NetworkExists = true
	}

	// Check memgraph
	cmd = exec.Command(e.Runtime, "ps", "--filter", "name=urp-memgraph", "--format", "{{.Status}}")
	if out, err := cmd.Output(); err == nil && strings.Contains(string(out), "Up") {
		e.MemgraphUp = true
	}
}

func (e *Environment) checkImages() {
	images := []string{"urp:latest", "urp:master", "urp:worker"}
	for _, img := range images {
		cmd := exec.Command(e.Runtime, "image", "inspect", img)
		if err := cmd.Run(); err == nil {
			e.ImagesExist[img] = true
		} else {
			e.ImagesExist[img] = false
			e.Warnings = append(e.Warnings, fmt.Sprintf("Image %s not found", img))
		}
	}
}

// IsHealthy returns true if the environment can run URP.
func (e *Environment) IsHealthy() bool {
	return len(e.Errors) == 0 && e.Runtime != "none"
}

// CanLaunchMaster returns true if master containers can be launched.
func (e *Environment) CanLaunchMaster() bool {
	return e.IsHealthy() && e.ImagesExist["urp:master"]
}

// CanSpawnWorker returns true if worker containers can be spawned from master.
func (e *Environment) CanSpawnWorker() bool {
	return e.IsHealthy() && e.ImagesExist["urp:worker"]
}

// CanLaunchStandalone returns true if standalone containers can be launched.
func (e *Environment) CanLaunchStandalone() bool {
	return e.IsHealthy() && e.ImagesExist["urp:latest"]
}

// Summary returns a human-readable summary.
func (e *Environment) Summary() string {
	var sb strings.Builder

	sb.WriteString("URP ENVIRONMENT CHECK\n")
	sb.WriteString(strings.Repeat("─", 40) + "\n")

	// TTY
	ttyStatus := "No (detached mode will be used)"
	if e.HasTTY {
		ttyStatus = "Yes (interactive mode available)"
	}
	sb.WriteString(fmt.Sprintf("TTY:          %s\n", ttyStatus))

	// Runtime
	if e.Runtime == "none" {
		sb.WriteString("Runtime:      NOT FOUND\n")
	} else {
		sb.WriteString(fmt.Sprintf("Runtime:      %s\n", e.RuntimeVersion))
	}

	// Infrastructure
	netStatus := "Missing"
	if e.NetworkExists {
		netStatus = "OK"
	}
	sb.WriteString(fmt.Sprintf("Network:      %s\n", netStatus))

	memStatus := "Not running"
	if e.MemgraphUp {
		memStatus = "Running"
	}
	sb.WriteString(fmt.Sprintf("Memgraph:     %s\n", memStatus))

	// Images
	sb.WriteString("Images:\n")
	for img, exists := range e.ImagesExist {
		status := "Missing"
		if exists {
			status = "OK"
		}
		sb.WriteString(fmt.Sprintf("  %-12s %s\n", img, status))
	}

	// Warnings
	if len(e.Warnings) > 0 {
		sb.WriteString("\nWarnings:\n")
		for _, w := range e.Warnings {
			sb.WriteString(fmt.Sprintf("  ⚠ %s\n", w))
		}
	}

	// Errors
	if len(e.Errors) > 0 {
		sb.WriteString("\nErrors:\n")
		for _, err := range e.Errors {
			sb.WriteString(fmt.Sprintf("  ✗ %s\n", err))
		}
	}

	// Overall status
	sb.WriteString("\n")
	if e.IsHealthy() {
		sb.WriteString("Status: HEALTHY\n")
	} else {
		sb.WriteString("Status: UNHEALTHY - fix errors above\n")
	}

	return sb.String()
}

// QuickCheck returns a one-line status suitable for non-verbose output.
func (e *Environment) QuickCheck() string {
	if !e.IsHealthy() {
		return fmt.Sprintf("Environment unhealthy: %s", strings.Join(e.Errors, "; "))
	}

	mode := "detached"
	if e.HasTTY {
		mode = "interactive"
	}

	infra := "infra:down"
	if e.MemgraphUp && e.NetworkExists {
		infra = "infra:up"
	}

	return fmt.Sprintf("runtime:%s mode:%s %s", e.Runtime, mode, infra)
}
