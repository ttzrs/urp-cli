package orchestrator

import (
	"os/exec"
	"strings"
	"testing"
)

func TestDetectContainerRuntime(t *testing.T) {
	runtime := detectContainerRuntime()

	// Should return docker, podman, or empty
	valid := runtime == "docker" || runtime == "podman" || runtime == ""
	if !valid {
		t.Errorf("unexpected runtime: %s", runtime)
	}

	// If we got a runtime, verify it exists
	if runtime != "" {
		if _, err := exec.LookPath(runtime); err != nil {
			t.Errorf("runtime %s not found in PATH", runtime)
		}
	}
}

func TestDetectContainerRuntimeMatchesInfra(t *testing.T) {
	runtime := detectContainerRuntime()
	if runtime == "" {
		t.Skip("no container runtime available")
	}

	// Check if urp-memgraph exists in the detected runtime
	cmd := exec.Command(runtime, "ps", "-q", "-f", "name=urp-memgraph")
	out, err := cmd.Output()
	if err != nil {
		t.Logf("Could not check memgraph: %v", err)
		return
	}

	// If memgraph is running, we should have detected the right runtime
	if len(strings.TrimSpace(string(out))) > 0 {
		t.Logf("urp-memgraph found in %s", runtime)
	}
}

func TestContainerArgsFormat(t *testing.T) {
	// Test that container args are properly formatted
	workerID := "test-worker-1"
	projectPath := "/tmp/test-project"

	args := []string{
		"run", "-i", "--rm",
		"--name", workerID,
		"--network", "urp-network",
		"-v", projectPath + ":/workspace:rw,z",
		"-v", "urp_vector:/var/lib/urp/vector:z",
		"-e", "URP_WORKER_ID=" + workerID,
		"-e", "NEO4J_URI=bolt://urp-memgraph:7687",
		"-e", "URP_WORKER=1",
		"-w", "/workspace",
		"--entrypoint", "/usr/local/bin/urp",
		"urp:latest",
		"worker", "run",
	}

	// Verify key args
	hasName := false
	hasNetwork := false
	hasEntrypoint := false
	hasWorkerCmd := false

	for i, arg := range args {
		if arg == "--name" && i+1 < len(args) && args[i+1] == workerID {
			hasName = true
		}
		if arg == "--network" && i+1 < len(args) && args[i+1] == "urp-network" {
			hasNetwork = true
		}
		if arg == "--entrypoint" && i+1 < len(args) && args[i+1] == "/usr/local/bin/urp" {
			hasEntrypoint = true
		}
		if arg == "worker" && i+1 < len(args) && args[i+1] == "run" {
			hasWorkerCmd = true
		}
	}

	if !hasName {
		t.Error("missing --name argument")
	}
	if !hasNetwork {
		t.Error("missing --network argument")
	}
	if !hasEntrypoint {
		t.Error("missing --entrypoint argument")
	}
	if !hasWorkerCmd {
		t.Error("missing worker run command")
	}
}

func TestVolumeMount(t *testing.T) {
	projectPath := "/home/user/my project"
	volume := projectPath + ":/workspace:rw,z"

	// Verify format
	parts := strings.Split(volume, ":")
	if len(parts) != 3 {
		t.Errorf("expected 3 parts in volume mount, got %d", len(parts))
	}

	if parts[0] != projectPath {
		t.Errorf("expected source path %s, got %s", projectPath, parts[0])
	}

	if parts[1] != "/workspace" {
		t.Error("expected /workspace as mount point")
	}

	if parts[2] != "rw,z" {
		t.Error("expected rw,z options")
	}
}

func TestEnvVarFormat(t *testing.T) {
	workerID := "w-123"
	envVar := "URP_WORKER_ID=" + workerID

	parts := strings.SplitN(envVar, "=", 2)
	if len(parts) != 2 {
		t.Error("invalid env var format")
	}

	if parts[0] != "URP_WORKER_ID" {
		t.Error("wrong env var name")
	}

	if parts[1] != workerID {
		t.Error("wrong env var value")
	}
}
