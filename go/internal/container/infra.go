// Package container infrastructure lifecycle commands.
package container

import (
	"fmt"
	"strings"
	"time"
)

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
