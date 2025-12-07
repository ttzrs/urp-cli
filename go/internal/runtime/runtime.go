// Package runtime provides system observability (vitals, topology, health).
// Implements DIP: Observer depends on Runtime interface, not concrete Docker calls.
package runtime

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Runtime defines the interface for container runtime operations.
// Any container runtime (Docker, Podman, etc.) must implement this interface.
// This enables DIP: high-level Observer depends on abstraction, not concrete Docker.
type Runtime interface {
	// Name returns the runtime name (e.g., "docker").
	Name() string

	// Stats returns container metrics.
	Stats(ctx context.Context) ([]RawContainerStats, error)

	// List returns running containers with network info.
	List(ctx context.Context) ([]RawContainerInfo, error)

	// ListAll returns all containers including stopped ones.
	ListAll(ctx context.Context) ([]RawContainerStatus, error)

	// Available returns true if the runtime is available.
	Available() bool
}

// RawContainerStats contains raw stats output from runtime.
type RawContainerStats struct {
	ID         string
	Name       string
	CPUPercent string // e.g., "1.23%"
	MemUsage   string // e.g., "123MiB / 1GiB"
	NetIO      string // e.g., "1.2kB / 3.4kB"
}

// RawContainerInfo contains raw container info from runtime.
type RawContainerInfo struct {
	ID       string
	Name     string
	Image    string
	Ports    string // e.g., "8080->80/tcp, 443->443/tcp"
	Networks string // e.g., "bridge,urp-network"
}

// RawContainerStatus contains raw container status from runtime.
type RawContainerStatus struct {
	ID     string
	Name   string
	Status string // e.g., "Up 2 hours", "Exited (1) 5 minutes ago"
	State  string // e.g., "running", "exited"
}

// ─────────────────────────────────────────────────────────────────────────────
// Docker Runtime Implementation
// ─────────────────────────────────────────────────────────────────────────────

// DockerRuntime implements Runtime for Docker.
type DockerRuntime struct {
	path string
}

// NewDockerRuntime creates a Docker runtime if available.
func NewDockerRuntime() *DockerRuntime {
	path, err := exec.LookPath("docker")
	if err != nil {
		return &DockerRuntime{path: ""}
	}
	return &DockerRuntime{path: path}
}

// Name returns "docker".
func (d *DockerRuntime) Name() string {
	return "docker"
}

// Available returns true if docker is installed.
func (d *DockerRuntime) Available() bool {
	return d.path != ""
}

// Stats returns container metrics via `docker stats`.
func (d *DockerRuntime) Stats(ctx context.Context) ([]RawContainerStats, error) {
	if !d.Available() {
		return nil, fmt.Errorf("Docker not found")
	}

	cmd := exec.CommandContext(ctx, d.path, "stats", "--no-stream",
		"--format", "{{.ID}}\t{{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}\t{{.NetIO}}")

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("docker stats failed: %w", err)
	}

	var stats []RawContainerStats
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, "\t")
		if len(parts) < 5 {
			continue
		}

		stats = append(stats, RawContainerStats{
			ID:         parts[0],
			Name:       parts[1],
			CPUPercent: parts[2],
			MemUsage:   parts[3],
			NetIO:      parts[4],
		})
	}

	return stats, nil
}

// List returns running containers via `docker ps`.
func (d *DockerRuntime) List(ctx context.Context) ([]RawContainerInfo, error) {
	if !d.Available() {
		return nil, fmt.Errorf("Docker not found")
	}

	cmd := exec.CommandContext(ctx, d.path, "ps",
		"--format", "{{.ID}}\t{{.Names}}\t{{.Image}}\t{{.Ports}}\t{{.Networks}}")

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("docker ps failed: %w", err)
	}

	var containers []RawContainerInfo
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, "\t")
		if len(parts) < 5 {
			continue
		}

		containers = append(containers, RawContainerInfo{
			ID:       parts[0],
			Name:     parts[1],
			Image:    parts[2],
			Ports:    parts[3],
			Networks: parts[4],
		})
	}

	return containers, nil
}

// ListAll returns all containers via `docker ps -a`.
func (d *DockerRuntime) ListAll(ctx context.Context) ([]RawContainerStatus, error) {
	if !d.Available() {
		return nil, fmt.Errorf("Docker not found")
	}

	cmd := exec.CommandContext(ctx, d.path, "ps", "-a",
		"--format", "{{.ID}}\t{{.Names}}\t{{.Status}}\t{{.State}}")

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("docker ps -a failed: %w", err)
	}

	var containers []RawContainerStatus
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, "\t")
		if len(parts) < 4 {
			continue
		}

		containers = append(containers, RawContainerStatus{
			ID:     parts[0],
			Name:   parts[1],
			Status: parts[2],
			State:  parts[3],
		})
	}

	return containers, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Mock Runtime for Testing
// ─────────────────────────────────────────────────────────────────────────────

// MockRuntime implements Runtime for testing.
type MockRuntime struct {
	name      string
	available bool

	// Configurable responses
	StatsResponse   []RawContainerStats
	StatsError      error
	ListResponse    []RawContainerInfo
	ListError       error
	ListAllResponse []RawContainerStatus
	ListAllError    error

	// Call tracking
	StatsCalled  int
	ListCalled   int
	ListAllCalled int
}

// NewMockRuntime creates a mock runtime for testing.
func NewMockRuntime(name string, available bool) *MockRuntime {
	return &MockRuntime{
		name:      name,
		available: available,
	}
}

func (m *MockRuntime) Name() string      { return m.name }
func (m *MockRuntime) Available() bool   { return m.available }

func (m *MockRuntime) Stats(ctx context.Context) ([]RawContainerStats, error) {
	m.StatsCalled++
	return m.StatsResponse, m.StatsError
}

func (m *MockRuntime) List(ctx context.Context) ([]RawContainerInfo, error) {
	m.ListCalled++
	return m.ListResponse, m.ListError
}

func (m *MockRuntime) ListAll(ctx context.Context) ([]RawContainerStatus, error) {
	m.ListAllCalled++
	return m.ListAllResponse, m.ListAllError
}

// ─────────────────────────────────────────────────────────────────────────────
// Runtime Detection
// ─────────────────────────────────────────────────────────────────────────────

// DetectRuntime returns the first available runtime.
func DetectRuntime() Runtime {
	docker := NewDockerRuntime()
	if docker.Available() {
		return docker
	}
	// Could add more runtimes here in the future
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Interface Compliance
// ─────────────────────────────────────────────────────────────────────────────

var (
	_ Runtime = (*DockerRuntime)(nil)
	_ Runtime = (*MockRuntime)(nil)
)
