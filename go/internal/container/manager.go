// Package container manages Docker infrastructure for URP.
//
// # Architecture
//
// URP uses Docker containers for isolation:
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
//
// # Requirements
//
// Docker must be installed and running. Podman is not supported.
package container

import (
	"bufio"
	"context"
	"fmt"
	osexec "os/exec"
	"strings"

	urpexec "github.com/joss/urp/internal/exec"
)

const (
	MemgraphImage   = "memgraph/memgraph-platform:latest"
	URPImage        = "urp:latest"
	URPWorkerImage  = "urp:worker"
	URPMasterImage  = "urp:master"
	URPBrowserImage = "urp:browser" // Chrome + go-rod for browser automation
	NeMoImage       = "nvcr.io/nvidia/nemo:24.07"
	URPConfigDir    = "~/.urp-go"
	URPEnvFile      = "~/.urp-go/.env"
)

// NetworkName returns the URP network name.
// All URP containers share a single network for simplicity.
func NetworkName(project string) string {
	return "urp-network"
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
	RuntimeDocker Runtime = "docker"
	RuntimeNone   Runtime = ""
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
	Runtime  Runtime
	Network  bool
	Memgraph *ContainerStatus
	Volumes  []string
	Workers  []ContainerStatus
	Error    string
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
	// Docker is the only supported runtime
	if _, err := osexec.LookPath("docker"); err == nil {
		return RuntimeDocker
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
		status.Error = "Docker not found. Install Docker to use URP containers."
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
