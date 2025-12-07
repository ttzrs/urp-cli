package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─────────────────────────────────────────────────────────────────────────────
// MockRuntime Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestMockRuntime_Basic(t *testing.T) {
	mock := NewMockRuntime("test-runtime", true)

	assert.Equal(t, "test-runtime", mock.Name())
	assert.True(t, mock.Available())
}

func TestMockRuntime_Unavailable(t *testing.T) {
	mock := NewMockRuntime("unavailable", false)
	assert.False(t, mock.Available())
}

func TestMockRuntime_Stats(t *testing.T) {
	mock := NewMockRuntime("docker", true)
	mock.StatsResponse = []RawContainerStats{
		{ID: "abc123", Name: "test-container", CPUPercent: "5.5%", MemUsage: "100MiB / 1GiB", NetIO: "1kB / 2kB"},
	}

	ctx := context.Background()
	stats, err := mock.Stats(ctx)

	require.NoError(t, err)
	require.Len(t, stats, 1)
	assert.Equal(t, "abc123", stats[0].ID)
	assert.Equal(t, "test-container", stats[0].Name)
	assert.Equal(t, "5.5%", stats[0].CPUPercent)
	assert.Equal(t, 1, mock.StatsCalled)
}

func TestMockRuntime_StatsError(t *testing.T) {
	mock := NewMockRuntime("docker", true)
	mock.StatsError = errors.New("connection refused")

	ctx := context.Background()
	stats, err := mock.Stats(ctx)

	require.Error(t, err)
	assert.Nil(t, stats)
	assert.Contains(t, err.Error(), "connection refused")
}

func TestMockRuntime_List(t *testing.T) {
	mock := NewMockRuntime("docker", true)
	mock.ListResponse = []RawContainerInfo{
		{ID: "def456", Name: "web", Image: "nginx:latest", Ports: "80/tcp", Networks: "bridge"},
	}

	ctx := context.Background()
	containers, err := mock.List(ctx)

	require.NoError(t, err)
	require.Len(t, containers, 1)
	assert.Equal(t, "web", containers[0].Name)
	assert.Equal(t, "nginx:latest", containers[0].Image)
	assert.Equal(t, 1, mock.ListCalled)
}

func TestMockRuntime_ListAll(t *testing.T) {
	mock := NewMockRuntime("docker", true)
	mock.ListAllResponse = []RawContainerStatus{
		{ID: "abc123", Name: "running", Status: "Up 2 hours", State: "running"},
		{ID: "def456", Name: "stopped", Status: "Exited (0) 1 hour ago", State: "exited"},
	}

	ctx := context.Background()
	containers, err := mock.ListAll(ctx)

	require.NoError(t, err)
	require.Len(t, containers, 2)
	assert.Equal(t, "running", containers[0].State)
	assert.Equal(t, "exited", containers[1].State)
	assert.Equal(t, 1, mock.ListAllCalled)
}

func TestMockRuntime_CallTracking(t *testing.T) {
	mock := NewMockRuntime("docker", true)
	ctx := context.Background()

	// Call multiple times
	mock.Stats(ctx)
	mock.Stats(ctx)
	mock.List(ctx)
	mock.ListAll(ctx)
	mock.ListAll(ctx)
	mock.ListAll(ctx)

	assert.Equal(t, 2, mock.StatsCalled)
	assert.Equal(t, 1, mock.ListCalled)
	assert.Equal(t, 3, mock.ListAllCalled)
}

// ─────────────────────────────────────────────────────────────────────────────
// DockerRuntime Tests (Unit - no Docker required)
// ─────────────────────────────────────────────────────────────────────────────

func TestDockerRuntime_Name(t *testing.T) {
	docker := &DockerRuntime{path: "/usr/bin/docker"}
	assert.Equal(t, "docker", docker.Name())
}

func TestDockerRuntime_Available_WithPath(t *testing.T) {
	docker := &DockerRuntime{path: "/usr/bin/docker"}
	assert.True(t, docker.Available())
}

func TestDockerRuntime_Available_WithoutPath(t *testing.T) {
	docker := &DockerRuntime{path: ""}
	assert.False(t, docker.Available())
}

func TestDockerRuntime_Stats_NotAvailable(t *testing.T) {
	docker := &DockerRuntime{path: ""}
	ctx := context.Background()

	stats, err := docker.Stats(ctx)

	require.Error(t, err)
	assert.Nil(t, stats)
	assert.Contains(t, err.Error(), "Docker not found")
}

func TestDockerRuntime_List_NotAvailable(t *testing.T) {
	docker := &DockerRuntime{path: ""}
	ctx := context.Background()

	containers, err := docker.List(ctx)

	require.Error(t, err)
	assert.Nil(t, containers)
	assert.Contains(t, err.Error(), "Docker not found")
}

func TestDockerRuntime_ListAll_NotAvailable(t *testing.T) {
	docker := &DockerRuntime{path: ""}
	ctx := context.Background()

	containers, err := docker.ListAll(ctx)

	require.Error(t, err)
	assert.Nil(t, containers)
	assert.Contains(t, err.Error(), "Docker not found")
}

// ─────────────────────────────────────────────────────────────────────────────
// Type Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestRawContainerStats(t *testing.T) {
	stats := RawContainerStats{
		ID:         "abc123def456",
		Name:       "my-container",
		CPUPercent: "15.5%",
		MemUsage:   "256MiB / 4GiB",
		NetIO:      "100kB / 50kB",
	}

	assert.Equal(t, "abc123def456", stats.ID)
	assert.Equal(t, "my-container", stats.Name)
	assert.Equal(t, "15.5%", stats.CPUPercent)
}

func TestRawContainerInfo(t *testing.T) {
	info := RawContainerInfo{
		ID:       "abc123",
		Name:     "web-server",
		Image:    "nginx:1.21",
		Ports:    "0.0.0.0:80->80/tcp",
		Networks: "bridge,custom-net",
	}

	assert.Equal(t, "web-server", info.Name)
	assert.Equal(t, "nginx:1.21", info.Image)
	assert.Contains(t, info.Networks, "custom-net")
}

func TestRawContainerStatus(t *testing.T) {
	status := RawContainerStatus{
		ID:     "def456",
		Name:   "db",
		Status: "Up 5 days (healthy)",
		State:  "running",
	}

	assert.Equal(t, "db", status.Name)
	assert.Equal(t, "running", status.State)
	assert.Contains(t, status.Status, "healthy")
}

// ─────────────────────────────────────────────────────────────────────────────
// Interface Compliance Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestRuntimeInterface_Docker(t *testing.T) {
	var _ Runtime = (*DockerRuntime)(nil)
}

func TestRuntimeInterface_Mock(t *testing.T) {
	var _ Runtime = (*MockRuntime)(nil)
}

// ─────────────────────────────────────────────────────────────────────────────
// DetectRuntime Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestDetectRuntime(t *testing.T) {
	// This test depends on whether Docker is installed
	runtime := DetectRuntime()

	if runtime != nil {
		// If Docker is available, should be DockerRuntime
		assert.Equal(t, "docker", runtime.Name())
		assert.True(t, runtime.Available())
	}
	// If nil, Docker is not installed - that's valid
}

// ─────────────────────────────────────────────────────────────────────────────
// Integration-like Tests (with mock, no real Docker)
// ─────────────────────────────────────────────────────────────────────────────

func TestRuntime_FullWorkflow(t *testing.T) {
	// Simulate a typical observation workflow
	mock := NewMockRuntime("docker", true)

	// Setup mock responses
	mock.StatsResponse = []RawContainerStats{
		{ID: "c1", Name: "web", CPUPercent: "10%", MemUsage: "100MiB / 500MiB", NetIO: "1MB / 500kB"},
		{ID: "c2", Name: "db", CPUPercent: "25%", MemUsage: "200MiB / 500MiB", NetIO: "500kB / 1MB"},
	}
	mock.ListResponse = []RawContainerInfo{
		{ID: "c1", Name: "web", Image: "nginx", Ports: "80/tcp", Networks: "app-net"},
		{ID: "c2", Name: "db", Image: "postgres", Ports: "5432/tcp", Networks: "app-net"},
	}
	mock.ListAllResponse = []RawContainerStatus{
		{ID: "c1", Name: "web", Status: "Up 1 hour", State: "running"},
		{ID: "c2", Name: "db", Status: "Up 1 hour", State: "running"},
		{ID: "c3", Name: "old", Status: "Exited (0) 2 days ago", State: "exited"},
	}

	ctx := context.Background()

	// Step 1: Get stats
	stats, err := mock.Stats(ctx)
	require.NoError(t, err)
	assert.Len(t, stats, 2)

	// Step 2: Get topology
	containers, err := mock.List(ctx)
	require.NoError(t, err)
	assert.Len(t, containers, 2)

	// Step 3: Check all (including stopped)
	all, err := mock.ListAll(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 3)

	// Verify call counts
	assert.Equal(t, 1, mock.StatsCalled)
	assert.Equal(t, 1, mock.ListCalled)
	assert.Equal(t, 1, mock.ListAllCalled)
}

func TestRuntime_UnavailableHandling(t *testing.T) {
	mock := NewMockRuntime("docker", false)
	ctx := context.Background()

	// When runtime is unavailable, code should check Available() first
	assert.False(t, mock.Available())

	// Calling methods still works but typically code would check Available() first
	stats, err := mock.Stats(ctx)
	assert.NoError(t, err)  // Mock doesn't return error for unavailable
	assert.Nil(t, stats)     // But returns nil
}
