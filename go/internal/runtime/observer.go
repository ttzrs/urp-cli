// Package runtime provides system observability (vitals, topology, health).
// Implements DIP: Observer depends on Runtime interface, not concrete Docker.
package runtime

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/joss/urp/internal/graph"
)

// ContainerState represents container metrics (Φ energy).
type ContainerState struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Status      string  `json:"status"`
	CPUPercent  float64 `json:"cpu_percent"`
	MemoryBytes int64   `json:"memory_bytes"`
	MemoryLimit int64   `json:"memory_limit"`
	MemoryPct   float64 `json:"memory_pct"`
	NetworkRx   int64   `json:"network_rx"`
	NetworkTx   int64   `json:"network_tx"`
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
// Uses Runtime interface for DIP - can be injected with mock for testing.
type Observer struct {
	db      graph.Driver
	runtime Runtime // DIP: depends on interface, not concrete Docker
}

// ObserverOption configures Observer via functional options (DIP).
type ObserverOption func(*Observer)

// WithRuntime injects a custom runtime (for testing).
func WithRuntime(r Runtime) ObserverOption {
	return func(o *Observer) { o.runtime = r }
}

// NewObserver creates an observer, auto-detecting container runtime.
func NewObserver(db graph.Driver, opts ...ObserverOption) *Observer {
	o := &Observer{
		db:      db,
		runtime: DetectRuntime(), // Default: auto-detect
	}

	for _, opt := range opts {
		opt(o)
	}

	return o
}

// Vitals returns current container states.
func (o *Observer) Vitals(ctx context.Context) ([]ContainerState, error) {
	if o.runtime == nil || !o.runtime.Available() {
		return nil, fmt.Errorf("Docker not found. Install Docker to use URP containers.")
	}

	// Use Runtime interface (DIP)
	rawStats, err := o.runtime.Stats(ctx)
	if err != nil {
		return nil, fmt.Errorf("stats failed: %w", err)
	}

	var states []ContainerState
	for _, raw := range rawStats {
		state := ContainerState{
			ID:   truncateID(raw.ID),
			Name: raw.Name,
		}

		// Parse CPU (e.g., "1.23%")
		cpuStr := strings.TrimSuffix(raw.CPUPercent, "%")
		state.CPUPercent, _ = strconv.ParseFloat(cpuStr, 64)

		// Parse memory (e.g., "123MiB / 1GiB")
		state.MemoryBytes, state.MemoryLimit = parseMemUsage(raw.MemUsage)
		if state.MemoryLimit > 0 {
			state.MemoryPct = float64(state.MemoryBytes) / float64(state.MemoryLimit) * 100
		}

		// Parse network (e.g., "1.2kB / 3.4kB")
		state.NetworkRx, state.NetworkTx = parseNetIO(raw.NetIO)

		state.Status = "running"
		states = append(states, state)

		// Store in graph if connected
		if o.db != nil {
			o.storeVitals(ctx, state)
		}
	}

	return states, nil
}

// truncateID truncates container ID to 12 characters.
func truncateID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
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

	if o.runtime == nil || !o.runtime.Available() {
		result.Error = "no container runtime found"
		return result, nil
	}

	// Use Runtime interface (DIP)
	rawContainers, err := o.runtime.List(ctx)
	if err != nil {
		result.Error = fmt.Sprintf("list failed: %v", err)
		return result, nil
	}

	networkSet := make(map[string]bool)

	for _, raw := range rawContainers {
		info := ContainerInfo{
			ID:    truncateID(raw.ID),
			Name:  raw.Name,
			Image: raw.Image,
		}

		// Parse ports
		if raw.Ports != "" {
			info.Ports = strings.Split(raw.Ports, ", ")
		}

		// Parse networks
		if raw.Networks != "" {
			info.Networks = strings.Split(raw.Networks, ",")
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

	if o.runtime == nil || !o.runtime.Available() {
		issues = append(issues, HealthIssue{
			Container: "system",
			Type:      "NO_RUNTIME",
			Severity:  "ERROR",
			Detail:    "Docker not found. Install Docker to use URP.",
		})
		return issues, nil
	}

	// Use Runtime interface (DIP)
	rawContainers, err := o.runtime.ListAll(ctx)
	if err != nil {
		issues = append(issues, HealthIssue{
			Container: o.runtime.Name(),
			Type:      "CMD_FAILED",
			Severity:  "ERROR",
			Detail:    err.Error(),
		})
		return issues, nil
	}

	for _, raw := range rawContainers {
		name := raw.Name
		status := strings.ToLower(raw.Status)
		state := strings.ToLower(raw.State)

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

// Runtime returns detected container runtime name.
func (o *Observer) RuntimeName() string {
	if o.runtime == nil {
		return ""
	}
	return o.runtime.Name()
}

// RuntimeAvailable returns true if a container runtime is available.
func (o *Observer) RuntimeAvailable() bool {
	return o.runtime != nil && o.runtime.Available()
}
