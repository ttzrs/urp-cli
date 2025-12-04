package container

import (
	"context"
	"testing"
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
