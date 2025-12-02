// Package runtime provides system observability (vitals, topology, health).
package runtime

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/joss/urp/internal/graph"
)

// ContainerState represents container metrics (Φ energy).
type ContainerState struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Status       string  `json:"status"`
	CPUPercent   float64 `json:"cpu_percent"`
	MemoryBytes  int64   `json:"memory_bytes"`
	MemoryLimit  int64   `json:"memory_limit"`
	MemoryPct    float64 `json:"memory_pct"`
	NetworkRx    int64   `json:"network_rx"`
	NetworkTx    int64   `json:"network_tx"`
}

// HealthIssue represents a container health problem (⊥ conflict).
type HealthIssue struct {
	Container string `json:"container"`
	Type      string `json:"type"`
	Severity  string `json:"severity"`
	Detail    string `json:"detail"`
}

// NetworkTopology represents container network layout (⊆ inclusion).
type NetworkTopology struct {
	Containers []ContainerInfo `json:"containers"`
	Networks   []string        `json:"networks"`
	Error      string          `json:"error,omitempty"`
}

// ContainerInfo has basic container details.
type ContainerInfo struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Image    string   `json:"image"`
	Ports    []string `json:"ports"`
	Networks []string `json:"networks"`
}

// Observer provides runtime observation capabilities.
type Observer struct {
	db      graph.Driver
	runtime string // "docker" or "podman"
}

// NewObserver creates an observer, auto-detecting container runtime.
func NewObserver(db graph.Driver) *Observer {
	runtime := detectRuntime()
	return &Observer{db: db, runtime: runtime}
}

func detectRuntime() string {
	// Check for podman first (more common on Fedora)
	if _, err := exec.LookPath("podman"); err == nil {
		return "podman"
	}
	if _, err := exec.LookPath("docker"); err == nil {
		return "docker"
	}
	return ""
}

// Vitals returns current container states.
func (o *Observer) Vitals(ctx context.Context) ([]ContainerState, error) {
	if o.runtime == "" {
		return nil, fmt.Errorf("no container runtime (docker/podman) found")
	}

	// Use `docker/podman stats --no-stream --format` for metrics
	cmd := exec.CommandContext(ctx, o.runtime, "stats", "--no-stream",
		"--format", "{{.ID}}\t{{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}\t{{.NetIO}}")

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%s stats failed: %w", o.runtime, err)
	}

	var states []ContainerState
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, "\t")
		if len(parts) < 5 {
			continue
		}

		state := ContainerState{
			ID:   parts[0][:12],
			Name: parts[1],
		}

		// Parse CPU (e.g., "1.23%")
		cpuStr := strings.TrimSuffix(parts[2], "%")
		state.CPUPercent, _ = strconv.ParseFloat(cpuStr, 64)

		// Parse memory (e.g., "123MiB / 1GiB")
		state.MemoryBytes, state.MemoryLimit = parseMemUsage(parts[3])
		if state.MemoryLimit > 0 {
			state.MemoryPct = float64(state.MemoryBytes) / float64(state.MemoryLimit) * 100
		}

		// Parse network (e.g., "1.2kB / 3.4kB")
		state.NetworkRx, state.NetworkTx = parseNetIO(parts[4])

		state.Status = "running"
		states = append(states, state)

		// Store in graph if connected
		if o.db != nil {
			o.storeVitals(ctx, state)
		}
	}

	return states, nil
}

func parseMemUsage(s string) (used, limit int64) {
	// Format: "123MiB / 1GiB" or "123.4MB / 1GB"
	parts := strings.Split(s, " / ")
	if len(parts) == 2 {
		used = parseSize(parts[0])
		limit = parseSize(parts[1])
	}
	return
}

func parseSize(s string) int64 {
	s = strings.TrimSpace(s)

	// Match number and unit
	re := regexp.MustCompile(`([\d.]+)([A-Za-z]+)`)
	matches := re.FindStringSubmatch(s)
	if len(matches) != 3 {
		return 0
	}

	val, _ := strconv.ParseFloat(matches[1], 64)
	unit := strings.ToUpper(matches[2])

	multipliers := map[string]float64{
		"B":   1,
		"KB":  1024,
		"KIB": 1024,
		"MB":  1024 * 1024,
		"MIB": 1024 * 1024,
		"GB":  1024 * 1024 * 1024,
		"GIB": 1024 * 1024 * 1024,
	}

	if mult, ok := multipliers[unit]; ok {
		return int64(val * mult)
	}
	return int64(val)
}

func parseNetIO(s string) (rx, tx int64) {
	// Format: "1.2kB / 3.4kB"
	parts := strings.Split(s, " / ")
	if len(parts) == 2 {
		rx = parseSize(parts[0])
		tx = parseSize(parts[1])
	}
	return
}

func (o *Observer) storeVitals(ctx context.Context, state ContainerState) {
	query := `
		MERGE (c:Container {id: $id})
		SET c.name = $name,
		    c.status = $status,
		    c.cpu_phi = $cpu,
		    c.mem_bytes = $mem,
		    c.mem_limit = $mem_limit,
		    c.mem_percent = $mem_pct,
		    c.net_rx = $rx,
		    c.net_tx = $tx,
		    c.last_seen = $ts
	`

	o.db.ExecuteWrite(ctx, query, map[string]any{
		"id":        state.ID,
		"name":      state.Name,
		"status":    state.Status,
		"cpu":       state.CPUPercent,
		"mem":       state.MemoryBytes,
		"mem_limit": state.MemoryLimit,
		"mem_pct":   state.MemoryPct,
		"rx":        state.NetworkRx,
		"tx":        state.NetworkTx,
		"ts":        time.Now().Unix(),
	})
}

// Topology returns container network topology.
func (o *Observer) Topology(ctx context.Context) (*NetworkTopology, error) {
	result := &NetworkTopology{
		Containers: []ContainerInfo{},
		Networks:   []string{},
	}

	if o.runtime == "" {
		result.Error = "no container runtime found"
		return result, nil
	}

	// Get container list with network info
	cmd := exec.CommandContext(ctx, o.runtime, "ps",
		"--format", "{{.ID}}\t{{.Names}}\t{{.Image}}\t{{.Ports}}\t{{.Networks}}")

	output, err := cmd.Output()
	if err != nil {
		result.Error = fmt.Sprintf("%s ps failed: %v", o.runtime, err)
		return result, nil
	}

	networkSet := make(map[string]bool)

	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, "\t")
		if len(parts) < 5 {
			continue
		}

		info := ContainerInfo{
			ID:    parts[0][:min(12, len(parts[0]))],
			Name:  parts[1],
			Image: parts[2],
		}

		// Parse ports
		if parts[3] != "" {
			info.Ports = strings.Split(parts[3], ", ")
		}

		// Parse networks
		if parts[4] != "" {
			info.Networks = strings.Split(parts[4], ",")
			for _, n := range info.Networks {
				networkSet[strings.TrimSpace(n)] = true
			}
		}

		result.Containers = append(result.Containers, info)

		// Store in graph
		if o.db != nil {
			o.storeTopology(ctx, info)
		}
	}

	for net := range networkSet {
		result.Networks = append(result.Networks, net)
	}

	return result, nil
}

func (o *Observer) storeTopology(ctx context.Context, info ContainerInfo) {
	for _, net := range info.Networks {
		query := `
			MERGE (c:Container {id: $cid})
			SET c.name = $name, c.image = $image
			MERGE (n:Network {name: $net})
			MERGE (c)-[:CONNECTED_TO]->(n)
		`

		o.db.ExecuteWrite(ctx, query, map[string]any{
			"cid":   info.ID,
			"name":  info.Name,
			"image": info.Image,
			"net":   strings.TrimSpace(net),
		})
	}
}

// Health checks for container issues.
func (o *Observer) Health(ctx context.Context) ([]HealthIssue, error) {
	var issues []HealthIssue

	if o.runtime == "" {
		issues = append(issues, HealthIssue{
			Container: "system",
			Type:      "NO_RUNTIME",
			Severity:  "WARN",
			Detail:    "No container runtime (docker/podman) found",
		})
		return issues, nil
	}

	// Get all containers including stopped ones
	cmd := exec.CommandContext(ctx, o.runtime, "ps", "-a",
		"--format", "{{.ID}}\t{{.Names}}\t{{.Status}}\t{{.State}}")

	output, err := cmd.Output()
	if err != nil {
		issues = append(issues, HealthIssue{
			Container: o.runtime,
			Type:      "CMD_FAILED",
			Severity:  "ERROR",
			Detail:    err.Error(),
		})
		return issues, nil
	}

	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, "\t")
		if len(parts) < 4 {
			continue
		}

		name := parts[1]
		status := strings.ToLower(parts[2])
		state := strings.ToLower(parts[3])

		// Check for restart loops
		if strings.Contains(status, "restarting") {
			issues = append(issues, HealthIssue{
				Container: name,
				Type:      "RESTART_LOOP",
				Severity:  "ERROR",
				Detail:    status,
			})
		}

		// Check for exited containers with error
		if state == "exited" {
			if strings.Contains(status, "exited (0)") {
				continue // Normal exit
			}
			issues = append(issues, HealthIssue{
				Container: name,
				Type:      "EXIT_ERROR",
				Severity:  "ERROR",
				Detail:    status,
			})
		}

		// Check for unhealthy
		if strings.Contains(status, "unhealthy") {
			issues = append(issues, HealthIssue{
				Container: name,
				Type:      "UNHEALTHY",
				Severity:  "ERROR",
				Detail:    status,
			})
		}
	}

	return issues, nil
}

// Runtime returns detected container runtime.
func (o *Observer) Runtime() string {
	return o.runtime
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
