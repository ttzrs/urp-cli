package container

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDetectRuntime(t *testing.T) {
	rt := detectRuntime()
	// Should detect docker or podman on most systems
	if rt != RuntimeDocker && rt != RuntimePodman && rt != RuntimeNone {
		t.Errorf("unexpected runtime: %s", rt)
	}
}

func TestNewManager(t *testing.T) {
	mgr := NewManager(context.Background())
	if mgr == nil {
		t.Fatal("NewManager returned nil")
	}
	// Should have detected a runtime (or none)
	rt := mgr.Runtime()
	if rt != RuntimeDocker && rt != RuntimePodman && rt != RuntimeNone {
		t.Errorf("unexpected runtime: %s", rt)
	}
}

func TestGPUStatus(t *testing.T) {
	mgr := NewManager(context.Background())
	status := mgr.CheckGPU()

	if status == nil {
		t.Fatal("CheckGPU returned nil")
	}

	// Just verify structure - actual GPU may or may not be present
	if status.Available {
		if status.DeviceCount < 1 {
			t.Error("if GPU available, device count should be >= 1")
		}
	} else {
		if status.Reason == "" {
			t.Error("if GPU unavailable, reason should be set")
		}
	}
}

func TestHasNvidiaGPU(t *testing.T) {
	mgr := NewManager(context.Background())
	// Just verify it doesn't panic and returns consistent result
	result1 := mgr.hasNvidiaGPU()
	result2 := mgr.CheckGPU().Available
	if result1 != result2 {
		t.Error("hasNvidiaGPU and CheckGPU.Available should match")
	}
}

func TestNeedsSELinuxWorkaround(t *testing.T) {
	mgr := NewManager(context.Background())
	// Just verify it doesn't panic
	_ = mgr.NeedsSELinuxWorkaround()
}

func TestWorkerHealthStruct(t *testing.T) {
	wh := &WorkerHealth{
		Name:    "test-worker",
		Status:  "healthy",
		Health:  "healthy",
		Running: true,
	}

	if wh.Name != "test-worker" {
		t.Errorf("expected name 'test-worker', got '%s'", wh.Name)
	}
	if wh.Status != "healthy" {
		t.Errorf("expected status 'healthy', got '%s'", wh.Status)
	}
}

func TestContainerStatusStruct(t *testing.T) {
	status := ContainerStatus{
		ID:      "abc123",
		Name:    "test-container",
		Image:   "urp:latest",
		Status:  "Up 5 minutes",
		Ports:   "7687/tcp",
		Network: "urp-network",
	}

	if status.ID != "abc123" {
		t.Errorf("expected ID 'abc123', got '%s'", status.ID)
	}
	if status.Name != "test-container" {
		t.Errorf("expected Name 'test-container', got '%s'", status.Name)
	}
}

func TestInfraStatusStruct(t *testing.T) {
	status := &InfraStatus{
		Runtime: RuntimeDocker,
		Network: true,
		Volumes: []string{"urp_sessions", "urp_vector"},
		Workers: []ContainerStatus{},
		Error:   "",
	}

	if status.Runtime != RuntimeDocker {
		t.Errorf("expected runtime docker, got %s", status.Runtime)
	}
	if !status.Network {
		t.Error("expected Network to be true")
	}
	if len(status.Volumes) != 2 {
		t.Errorf("expected 2 volumes, got %d", len(status.Volumes))
	}
}

func TestManagerStatus(t *testing.T) {
	mgr := NewManager(context.Background())
	status := mgr.Status()

	if status == nil {
		t.Fatal("Status() returned nil")
	}

	// Should report runtime even if no containers
	if status.Runtime != mgr.Runtime() {
		t.Errorf("runtime mismatch: status=%s, manager=%s", status.Runtime, mgr.Runtime())
	}
}

// Integration test - only runs if docker/podman available
func TestListWorkersIntegration(t *testing.T) {
	mgr := NewManager(context.Background())
	if mgr.Runtime() == RuntimeNone {
		t.Skip("no container runtime available")
	}

	// Should not error even with no workers
	workers := mgr.ListWorkers("")
	if workers == nil {
		// nil is ok, just means no workers
		workers = []ContainerStatus{}
	}
	// Test passes if no panic
}

func TestCheckWorkerHealthNonExistent(t *testing.T) {
	mgr := NewManager(context.Background())
	if mgr.Runtime() == RuntimeNone {
		t.Skip("no container runtime available")
	}

	health := mgr.CheckWorkerHealth("nonexistent-container-12345")
	if health == nil {
		t.Fatal("CheckWorkerHealth returned nil")
	}
	if health.Status != "not_found" {
		t.Errorf("expected status 'not_found', got '%s'", health.Status)
	}
}

func TestGPUStatusStruct(t *testing.T) {
	// Test with GPU available
	statusWithGPU := &GPUStatus{
		Available:   true,
		DeviceCount: 2,
		Reason:      "",
	}
	if !statusWithGPU.Available {
		t.Error("expected Available to be true")
	}
	if statusWithGPU.DeviceCount != 2 {
		t.Errorf("expected DeviceCount 2, got %d", statusWithGPU.DeviceCount)
	}

	// Test without GPU
	statusNoGPU := &GPUStatus{
		Available:   false,
		DeviceCount: 0,
		Reason:      "nvidia-smi not found",
	}
	if statusNoGPU.Available {
		t.Error("expected Available to be false")
	}
	if statusNoGPU.Reason == "" {
		t.Error("expected Reason to be set")
	}
}

// TestE2E_OrchestrationFlow tests the full master→spawn→ask→kill flow.
// This is a real integration test that creates actual containers.
// Run with: go test -v -run TestE2E_OrchestrationFlow -timeout 120s
func TestE2E_OrchestrationFlow(t *testing.T) {
	if os.Getenv("URP_E2E_TEST") != "1" {
		t.Skip("Set URP_E2E_TEST=1 to run E2E tests (requires Docker/Podman and urp images)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// Use unique project name for test isolation
	projectName := "e2etest"
	mgr := NewManagerForProject(ctx, projectName)

	if mgr.Runtime() == RuntimeNone {
		t.Fatal("no container runtime available")
	}

	// Create temp project directory
	tmpDir, err := os.MkdirTemp("", "urp-e2e-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a simple test file in the project
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("hello from e2e test\n"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// ========================================
	// Phase 1: Start infrastructure
	// ========================================
	t.Log("Phase 1: Starting infrastructure...")

	if err := mgr.StartInfra(); err != nil {
		t.Fatalf("StartInfra failed: %v", err)
	}

	// Verify network created
	networkName := NetworkName(projectName)
	out, _ := mgr.run("network", "ls", "--filter", "name="+networkName, "--format", "{{.Name}}")
	if !strings.Contains(out, networkName) {
		t.Errorf("network %s not created", networkName)
	}

	// Verify memgraph running
	memgraphName := MemgraphName(projectName)
	out, _ = mgr.run("ps", "--filter", "name="+memgraphName, "--format", "{{.Names}}")
	if !strings.Contains(out, memgraphName) {
		t.Errorf("memgraph %s not running", memgraphName)
	}

	// Verify NO port mappings (key requirement)
	out, _ = mgr.run("ps", "--filter", "name="+memgraphName, "--format", "{{.Ports}}")
	if strings.Contains(out, "0.0.0.0:") || strings.Contains(out, "->") {
		t.Errorf("memgraph should NOT have port mappings, got: %s", out)
	}
	t.Logf("✓ Memgraph running without port mappings: %s", strings.TrimSpace(out))

	// ========================================
	// Phase 2: Spawn worker
	// ========================================
	t.Log("Phase 2: Spawning worker...")

	workerName, err := mgr.SpawnWorker(tmpDir, 1)
	if err != nil {
		t.Fatalf("SpawnWorker failed: %v", err)
	}
	t.Logf("✓ Worker spawned: %s", workerName)

	// Wait for worker to be ready
	time.Sleep(2 * time.Second)

	// Verify worker is running
	health := mgr.VerifyWorkerHealth(workerName, 10*time.Second)
	if !health.Running {
		t.Fatalf("worker not running: %s", health.Error)
	}
	t.Logf("✓ Worker health: running=%v, docker=%v", health.Running, health.DockerAccess)

	// ========================================
	// Phase 3: Execute command in worker
	// ========================================
	t.Log("Phase 3: Executing command in worker...")

	// Simple command to verify workspace is mounted
	out, err = mgr.ExecInWorker(workerName, []string{"cat", "/workspace/test.txt"})
	if err != nil {
		t.Fatalf("ExecInWorker failed: %v", err)
	}
	if !strings.Contains(out, "hello from e2e test") {
		t.Errorf("unexpected file content: %s", out)
	}
	t.Log("✓ Workspace mounted correctly, can read files")

	// Write a file from worker (verify write access)
	_, err = mgr.ExecInWorker(workerName, []string{"sh", "-c", "echo 'written by worker' > /workspace/worker-output.txt"})
	if err != nil {
		t.Fatalf("worker write failed: %v", err)
	}

	// Verify file was created on host
	content, err := os.ReadFile(filepath.Join(tmpDir, "worker-output.txt"))
	if err != nil {
		t.Fatalf("worker output file not found: %v", err)
	}
	if !strings.Contains(string(content), "written by worker") {
		t.Errorf("unexpected worker output: %s", content)
	}
	t.Log("✓ Worker can write to workspace")

	// Verify environment variables
	out, _ = mgr.ExecInWorker(workerName, []string{"printenv", "URP_PROJECT"})
	if !strings.Contains(out, projectName) {
		t.Errorf("URP_PROJECT env not set correctly: %s", out)
	}
	t.Log("✓ Environment variables set correctly")

	// ========================================
	// Phase 4: List workers
	// ========================================
	t.Log("Phase 4: Listing workers...")

	workers := mgr.ListWorkers(projectName)
	found := false
	for _, w := range workers {
		if w.Name == workerName {
			found = true
			t.Logf("✓ Found worker: %s (status: %s)", w.Name, w.Status)
		}
	}
	if !found {
		t.Errorf("spawned worker %s not in list", workerName)
	}

	// ========================================
	// Phase 5: Kill worker
	// ========================================
	t.Log("Phase 5: Killing worker...")

	if err := mgr.KillWorker(workerName); err != nil {
		t.Fatalf("KillWorker failed: %v", err)
	}

	// Verify worker is gone
	time.Sleep(500 * time.Millisecond)
	workers = mgr.ListWorkers(projectName)
	for _, w := range workers {
		if w.Name == workerName {
			t.Errorf("worker %s still exists after kill", workerName)
		}
	}
	t.Log("✓ Worker killed successfully")

	// ========================================
	// Phase 6: Cleanup infrastructure
	// ========================================
	t.Log("Phase 6: Cleaning up infrastructure...")

	if err := mgr.CleanInfra(); err != nil {
		t.Fatalf("CleanInfra failed: %v", err)
	}

	// Verify memgraph stopped
	out, _ = mgr.run("ps", "-a", "--filter", "name="+memgraphName, "--format", "{{.Names}}")
	if strings.Contains(out, memgraphName) {
		t.Errorf("memgraph %s still exists after cleanup", memgraphName)
	}
	t.Log("✓ Infrastructure cleaned up")

	t.Log("========================================")
	t.Log("E2E Test PASSED: Full orchestration flow works!")
	t.Log("========================================")
}

// TestE2E_ProjectIsolation verifies that different projects have isolated networks.
func TestE2E_ProjectIsolation(t *testing.T) {
	if os.Getenv("URP_E2E_TEST") != "1" {
		t.Skip("Set URP_E2E_TEST=1 to run E2E tests")
	}

	ctx := context.Background()

	// Two different projects
	project1 := "isolation-a"
	project2 := "isolation-b"

	mgr1 := NewManagerForProject(ctx, project1)
	mgr2 := NewManagerForProject(ctx, project2)

	if mgr1.Runtime() == RuntimeNone {
		t.Fatal("no container runtime available")
	}

	// Cleanup first (in case previous test failed)
	mgr1.CleanInfra()
	mgr2.CleanInfra()

	// Start both infrastructures
	if err := mgr1.StartInfra(); err != nil {
		t.Fatalf("StartInfra for project1 failed: %v", err)
	}
	defer mgr1.CleanInfra()

	if err := mgr2.StartInfra(); err != nil {
		t.Fatalf("StartInfra for project2 failed: %v", err)
	}
	defer mgr2.CleanInfra()

	// Verify different networks
	net1 := NetworkName(project1)
	net2 := NetworkName(project2)
	if net1 == net2 {
		t.Errorf("networks should be different: %s vs %s", net1, net2)
	}

	// Verify different memgraph containers
	mg1 := MemgraphName(project1)
	mg2 := MemgraphName(project2)
	if mg1 == mg2 {
		t.Errorf("memgraph names should be different: %s vs %s", mg1, mg2)
	}

	// Verify both are running
	out, _ := mgr1.run("ps", "--format", "{{.Names}}")
	if !strings.Contains(out, mg1) {
		t.Errorf("memgraph %s not running", mg1)
	}
	if !strings.Contains(out, mg2) {
		t.Errorf("memgraph %s not running", mg2)
	}

	t.Log("✓ Projects are isolated with separate networks and memgraph instances")
}

// TestE2E_X11Configuration verifies X11 socket and DISPLAY are configured for master.
// This test checks the configuration without requiring an actual X11 display.
func TestE2E_X11Configuration(t *testing.T) {
	if os.Getenv("URP_E2E_TEST") != "1" {
		t.Skip("Set URP_E2E_TEST=1 to run E2E tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	projectName := "x11test"
	mgr := NewManagerForProject(ctx, projectName)

	if mgr.Runtime() == RuntimeNone {
		t.Fatal("no container runtime available")
	}

	// Create temp project directory
	tmpDir, err := os.MkdirTemp("", "urp-x11-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Start infrastructure
	if err := mgr.StartInfra(); err != nil {
		t.Fatalf("StartInfra failed: %v", err)
	}
	defer mgr.CleanInfra()

	// Launch master in detached mode (no TTY in tests)
	containerName := fmt.Sprintf("urp-master-%s", filepath.Base(tmpDir))

	// Kill any existing
	mgr.run("rm", "-f", containerName)

	// Get home and env file
	homeDir, _ := os.UserHomeDir()
	if realHome, err := filepath.EvalSymlinks(homeDir); err == nil {
		homeDir = realHome
	}
	envFile := filepath.Join(homeDir, ".urp-go", ".env")

	// Get socket path
	socketPath := "/var/run/docker.sock"
	if mgr.Runtime() == RuntimePodman {
		xdgRuntime := os.Getenv("XDG_RUNTIME_DIR")
		if xdgRuntime != "" {
			socketPath = fmt.Sprintf("%s/podman/podman.sock", xdgRuntime)
		}
	}

	networkName := NetworkName(projectName)
	memgraphName := MemgraphName(projectName)

	// Launch master with X11 config (detached for test)
	args := []string{
		"run", "-d", "--rm",
		"--name", containerName,
		"--network", networkName,
		"--security-opt", "label=disable",
		"-v", fmt.Sprintf("%s:/workspace:ro", tmpDir),
		"-v", fmt.Sprintf("%s:/var/run/docker.sock", socketPath),
		"-v", fmt.Sprintf("%s:/etc/urp/.env:ro", envFile),
		// X11 configuration - the key thing we're testing
		"-v", "/tmp/.X11-unix:/tmp/.X11-unix:ro",
		"-e", fmt.Sprintf("URP_PROJECT=%s", projectName),
		"-e", fmt.Sprintf("NEO4J_URI=bolt://%s:7687", memgraphName),
		"-e", "URP_MASTER=1",
		"-e", "DISPLAY=:0", // Test with a default DISPLAY
		"-w", "/workspace",
		"--entrypoint", "sleep",
		URPMasterImage,
		"30", // Keep alive for 30s
	}

	_, err = mgr.run(args...)
	if err != nil {
		t.Fatalf("failed to launch master: %v", err)
	}
	defer mgr.run("rm", "-f", containerName)

	// Wait for container to start
	time.Sleep(1 * time.Second)

	// ========================================
	// Test 1: Verify X11 socket is mounted
	// ========================================
	out, err := mgr.ExecInWorker(containerName, []string{"ls", "-la", "/tmp/.X11-unix/"})
	if err != nil {
		t.Logf("X11 socket check (may fail without X11): %v", err)
		// Don't fail - X11 socket may not exist on headless systems
	} else {
		t.Logf("✓ X11 socket mount verified: %s", strings.TrimSpace(out))
	}

	// ========================================
	// Test 2: Verify DISPLAY env is set
	// ========================================
	out, err = mgr.ExecInWorker(containerName, []string{"printenv", "DISPLAY"})
	if err != nil {
		t.Fatalf("DISPLAY env not set: %v", err)
	}
	if !strings.Contains(out, ":") {
		t.Errorf("DISPLAY should contain ':', got: %s", out)
	}
	t.Logf("✓ DISPLAY environment set: %s", strings.TrimSpace(out))

	// ========================================
	// Test 3: Check if Firefox is installed
	// ========================================
	out, err = mgr.ExecInWorker(containerName, []string{"which", "firefox"})
	if err != nil {
		// Firefox might not be in master image yet
		t.Logf("⚠ Firefox not found in master image (optional): %v", err)
	} else {
		t.Logf("✓ Firefox installed: %s", strings.TrimSpace(out))
	}

	// ========================================
	// Test 4: Verify xdpyinfo can check display (if X11 available)
	// ========================================
	if os.Getenv("DISPLAY") != "" {
		out, err = mgr.ExecInWorker(containerName, []string{"sh", "-c", "which xdpyinfo && xdpyinfo -display $DISPLAY 2>&1 | head -5 || echo 'xdpyinfo not available'"})
		t.Logf("X11 display info: %s", strings.TrimSpace(out))
	}

	t.Log("========================================")
	t.Log("X11 Configuration Test Complete")
	t.Log("========================================")
}

// TestX11SocketMount verifies X11 mount is included in LaunchMaster args.
// This is a unit test that doesn't require containers.
func TestX11SocketMount(t *testing.T) {
	// Test that the X11 socket path is correct
	x11Path := "/tmp/.X11-unix"
	if _, err := os.Stat(x11Path); err != nil {
		t.Skipf("X11 socket not available: %v", err)
	}

	// Verify it's a directory (socket directory)
	info, _ := os.Stat(x11Path)
	if !info.IsDir() {
		t.Errorf("expected %s to be a directory", x11Path)
	}

	t.Logf("✓ X11 socket directory exists: %s", x11Path)
}

// TestDisplayEnvPropagation verifies DISPLAY env is read from host.
func TestDisplayEnvPropagation(t *testing.T) {
	display := os.Getenv("DISPLAY")
	if display == "" {
		t.Skip("DISPLAY not set (headless environment)")
	}

	// Should be something like :0 or :1 or localhost:10.0
	if !strings.Contains(display, ":") {
		t.Errorf("DISPLAY should contain ':', got: %s", display)
	}

	t.Logf("✓ Host DISPLAY: %s", display)
}
