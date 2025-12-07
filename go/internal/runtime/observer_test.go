package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─────────────────────────────────────────────────────────────────────────────
// Observer Tests with Mock Runtime (DIP in action)
// ─────────────────────────────────────────────────────────────────────────────

func TestNewObserver_WithMockRuntime(t *testing.T) {
	mock := NewMockRuntime("docker", true)

	// Use WithRuntime option (DIP)
	obs := NewObserver(nil, WithRuntime(mock))

	require.NotNil(t, obs)
	assert.Equal(t, "docker", obs.RuntimeName())
	assert.True(t, obs.RuntimeAvailable())
}

func TestObserver_NoRuntime(t *testing.T) {
	// Observer with unavailable runtime
	mock := NewMockRuntime("docker", false)
	obs := NewObserver(nil, WithRuntime(mock))

	assert.False(t, obs.RuntimeAvailable())
}

func TestObserver_NilRuntime(t *testing.T) {
	obs := NewObserver(nil, WithRuntime(nil))

	assert.False(t, obs.RuntimeAvailable())
	assert.Equal(t, "", obs.RuntimeName())
}

// ─────────────────────────────────────────────────────────────────────────────
// Vitals Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestObserver_Vitals_Success(t *testing.T) {
	mock := NewMockRuntime("docker", true)
	mock.StatsResponse = []RawContainerStats{
		{
			ID:         "abc123def456gh",
			Name:       "web-server",
			CPUPercent: "25.5%",
			MemUsage:   "256MiB / 1GiB",
			NetIO:      "1.5MB / 500kB",
		},
		{
			ID:         "xyz789",
			Name:       "database",
			CPUPercent: "10.2%",
			MemUsage:   "512MiB / 2GiB",
			NetIO:      "100kB / 50kB",
		},
	}

	obs := NewObserver(nil, WithRuntime(mock))
	ctx := context.Background()

	states, err := obs.Vitals(ctx)

	require.NoError(t, err)
	require.Len(t, states, 2)

	// Check first container
	assert.Equal(t, "abc123def456", states[0].ID) // Truncated to 12 chars
	assert.Equal(t, "web-server", states[0].Name)
	assert.InDelta(t, 25.5, states[0].CPUPercent, 0.1)
	assert.Equal(t, "running", states[0].Status)

	// Check second container
	assert.Equal(t, "xyz789", states[1].ID) // Less than 12 chars, not truncated
	assert.Equal(t, "database", states[1].Name)
	assert.InDelta(t, 10.2, states[1].CPUPercent, 0.1)
}

func TestObserver_Vitals_NoRuntime(t *testing.T) {
	mock := NewMockRuntime("docker", false)
	obs := NewObserver(nil, WithRuntime(mock))
	ctx := context.Background()

	states, err := obs.Vitals(ctx)

	require.Error(t, err)
	assert.Nil(t, states)
	assert.Contains(t, err.Error(), "Docker not found")
}

func TestObserver_Vitals_Error(t *testing.T) {
	mock := NewMockRuntime("docker", true)
	mock.StatsError = errors.New("connection refused")

	obs := NewObserver(nil, WithRuntime(mock))
	ctx := context.Background()

	states, err := obs.Vitals(ctx)

	require.Error(t, err)
	assert.Nil(t, states)
	assert.Contains(t, err.Error(), "connection refused")
}

// ─────────────────────────────────────────────────────────────────────────────
// Topology Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestObserver_Topology_Success(t *testing.T) {
	mock := NewMockRuntime("docker", true)
	mock.ListResponse = []RawContainerInfo{
		{
			ID:       "abc123",
			Name:     "web",
			Image:    "nginx:latest",
			Ports:    "80/tcp, 443/tcp",
			Networks: "bridge,app-network",
		},
		{
			ID:       "def456",
			Name:     "db",
			Image:    "postgres:14",
			Ports:    "5432/tcp",
			Networks: "app-network",
		},
	}

	obs := NewObserver(nil, WithRuntime(mock))
	ctx := context.Background()

	topo, err := obs.Topology(ctx)

	require.NoError(t, err)
	require.NotNil(t, topo)
	assert.Empty(t, topo.Error)

	// Check containers
	require.Len(t, topo.Containers, 2)
	assert.Equal(t, "web", topo.Containers[0].Name)
	assert.Equal(t, "nginx:latest", topo.Containers[0].Image)
	assert.Contains(t, topo.Containers[0].Ports, "80/tcp")

	// Check networks
	assert.Contains(t, topo.Networks, "bridge")
	assert.Contains(t, topo.Networks, "app-network")
}

func TestObserver_Topology_NoRuntime(t *testing.T) {
	mock := NewMockRuntime("docker", false)
	obs := NewObserver(nil, WithRuntime(mock))
	ctx := context.Background()

	topo, err := obs.Topology(ctx)

	require.NoError(t, err) // Topology returns error in result, not as error
	assert.Equal(t, "no container runtime found", topo.Error)
}

func TestObserver_Topology_Error(t *testing.T) {
	mock := NewMockRuntime("docker", true)
	mock.ListError = errors.New("docker daemon not responding")

	obs := NewObserver(nil, WithRuntime(mock))
	ctx := context.Background()

	topo, err := obs.Topology(ctx)

	require.NoError(t, err)
	assert.Contains(t, topo.Error, "docker daemon not responding")
}

// ─────────────────────────────────────────────────────────────────────────────
// Health Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestObserver_Health_AllHealthy(t *testing.T) {
	mock := NewMockRuntime("docker", true)
	mock.ListAllResponse = []RawContainerStatus{
		{ID: "c1", Name: "web", Status: "Up 2 hours", State: "running"},
		{ID: "c2", Name: "db", Status: "Up 2 hours (healthy)", State: "running"},
	}

	obs := NewObserver(nil, WithRuntime(mock))
	ctx := context.Background()

	issues, err := obs.Health(ctx)

	require.NoError(t, err)
	assert.Empty(t, issues) // No issues for healthy containers
}

func TestObserver_Health_Unhealthy(t *testing.T) {
	mock := NewMockRuntime("docker", true)
	mock.ListAllResponse = []RawContainerStatus{
		{ID: "c1", Name: "unhealthy-app", Status: "Up 10 minutes (unhealthy)", State: "running"},
	}

	obs := NewObserver(nil, WithRuntime(mock))
	ctx := context.Background()

	issues, err := obs.Health(ctx)

	require.NoError(t, err)
	require.Len(t, issues, 1)
	assert.Equal(t, "unhealthy-app", issues[0].Container)
	assert.Equal(t, "UNHEALTHY", issues[0].Type)
	assert.Equal(t, "ERROR", issues[0].Severity)
}

func TestObserver_Health_RestartLoop(t *testing.T) {
	mock := NewMockRuntime("docker", true)
	mock.ListAllResponse = []RawContainerStatus{
		{ID: "c1", Name: "crashing-app", Status: "Restarting (1) 5 seconds ago", State: "restarting"},
	}

	obs := NewObserver(nil, WithRuntime(mock))
	ctx := context.Background()

	issues, err := obs.Health(ctx)

	require.NoError(t, err)
	require.Len(t, issues, 1)
	assert.Equal(t, "crashing-app", issues[0].Container)
	assert.Equal(t, "RESTART_LOOP", issues[0].Type)
}

func TestObserver_Health_ExitError(t *testing.T) {
	mock := NewMockRuntime("docker", true)
	mock.ListAllResponse = []RawContainerStatus{
		{ID: "c1", Name: "failed-app", Status: "Exited (1) 10 minutes ago", State: "exited"},
		{ID: "c2", Name: "normal-exit", Status: "Exited (0) 1 hour ago", State: "exited"}, // Normal exit, should not be an issue
	}

	obs := NewObserver(nil, WithRuntime(mock))
	ctx := context.Background()

	issues, err := obs.Health(ctx)

	require.NoError(t, err)
	require.Len(t, issues, 1) // Only the error exit
	assert.Equal(t, "failed-app", issues[0].Container)
	assert.Equal(t, "EXIT_ERROR", issues[0].Type)
}

func TestObserver_Health_NoRuntime(t *testing.T) {
	mock := NewMockRuntime("docker", false)
	obs := NewObserver(nil, WithRuntime(mock))
	ctx := context.Background()

	issues, err := obs.Health(ctx)

	require.NoError(t, err)
	require.Len(t, issues, 1)
	assert.Equal(t, "system", issues[0].Container)
	assert.Equal(t, "NO_RUNTIME", issues[0].Type)
}

func TestObserver_Health_Error(t *testing.T) {
	mock := NewMockRuntime("docker", true)
	mock.ListAllError = errors.New("permission denied")

	obs := NewObserver(nil, WithRuntime(mock))
	ctx := context.Background()

	issues, err := obs.Health(ctx)

	require.NoError(t, err)
	require.Len(t, issues, 1)
	assert.Equal(t, "CMD_FAILED", issues[0].Type)
	assert.Contains(t, issues[0].Detail, "permission denied")
}

func TestObserver_Health_MultipleIssues(t *testing.T) {
	mock := NewMockRuntime("docker", true)
	mock.ListAllResponse = []RawContainerStatus{
		{ID: "c1", Name: "unhealthy", Status: "Up 1 hour (unhealthy)", State: "running"},
		{ID: "c2", Name: "restarting", Status: "Restarting (5) 10s ago", State: "restarting"},
		{ID: "c3", Name: "crashed", Status: "Exited (137) 5 min ago", State: "exited"},
		{ID: "c4", Name: "healthy", Status: "Up 2 hours (healthy)", State: "running"},
	}

	obs := NewObserver(nil, WithRuntime(mock))
	ctx := context.Background()

	issues, err := obs.Health(ctx)

	require.NoError(t, err)
	assert.Len(t, issues, 3) // 3 problematic containers, 1 healthy
}

// ─────────────────────────────────────────────────────────────────────────────
// Helper Function Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestTruncateID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"abc123def456gh", "abc123def456"},   // Longer than 12, truncate
		{"abc123", "abc123"},                  // Shorter than 12, keep as is
		{"abc123def456", "abc123def456"},     // Exactly 12
		{"", ""},                              // Empty
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := truncateID(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseMemUsage(t *testing.T) {
	tests := []struct {
		input     string
		wantUsed  int64
		wantLimit int64
	}{
		{"256MiB / 1GiB", 256 * 1024 * 1024, 1024 * 1024 * 1024},
		{"512MB / 2GB", 512 * 1024 * 1024, 2 * 1024 * 1024 * 1024},
		{"100KiB / 500KiB", 100 * 1024, 500 * 1024},
		{"invalid", 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			used, limit := parseMemUsage(tt.input)
			assert.Equal(t, tt.wantUsed, used)
			assert.Equal(t, tt.wantLimit, limit)
		})
	}
}

func TestParseNetIO(t *testing.T) {
	tests := []struct {
		input  string
		wantRx int64
		wantTx int64
	}{
		{"1.5MB / 500kB", 1572864, 512000}, // 1.5*1024*1024, 500*1024
		{"100B / 200B", 100, 200},
		{"invalid", 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			rx, tx := parseNetIO(tt.input)
			assert.Equal(t, tt.wantRx, rx)
			assert.Equal(t, tt.wantTx, tx)
		})
	}
}

func TestParseSize(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"256MiB", 256 * 1024 * 1024},
		{"1GiB", 1024 * 1024 * 1024},
		{"512MB", 512 * 1024 * 1024},
		{"100KB", 100 * 1024},
		{"50B", 50},
		{"1.5GB", 1610612736}, // 1.5 * 1024^3
		{"invalid", 0},
		{"", 0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parseSize(tt.input)
			assert.Equal(t, tt.want, result)
		})
	}
}
