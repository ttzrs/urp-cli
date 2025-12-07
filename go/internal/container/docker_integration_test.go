//go:build integration

// Package container integration tests require Docker and urp-network.
// Run with: go test -tags=integration ./internal/container/...
package container

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestDockerRequired(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Fatal("Docker is required but not installed")
	}

	cmd := exec.Command("docker", "info")
	if err := cmd.Run(); err != nil {
		t.Fatal("Docker is installed but not running. Start Docker daemon first.")
	}
}

func TestURPNetworkExists(t *testing.T) {
	cmd := exec.Command("docker", "network", "inspect", "urp-network")
	if err := cmd.Run(); err != nil {
		t.Fatal("urp-network not found. Run: docker compose up -d memgraph")
	}
}

func TestMemgraphRunning(t *testing.T) {
	cmd := exec.Command("docker", "ps", "--filter", "name=urp-memgraph", "--filter", "status=running", "-q")
	out, err := cmd.Output()
	if err != nil || strings.TrimSpace(string(out)) == "" {
		t.Fatal("urp-memgraph not running. Run: docker compose up -d memgraph")
	}
}

func TestMemgraphConnectivity(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Try to connect via bolt
	cmd := exec.CommandContext(ctx, "docker", "exec", "urp-memgraph", "bash", "-c", "echo 'RETURN 1;' | mgconsole")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Memgraph not responding: %v (output: %s)", err, out)
	}
}

func TestURPImagesExist(t *testing.T) {
	images := []string{"urp:latest", "urp:master", "urp:worker"}
	var missing []string

	for _, img := range images {
		cmd := exec.Command("docker", "image", "inspect", img)
		if err := cmd.Run(); err != nil {
			missing = append(missing, img)
		}
	}

	if len(missing) > 0 {
		t.Errorf("Images not found: %v. Run: make docker-all", missing)
	}
}

func TestDockerSocketAccessible(t *testing.T) {
	cmd := exec.Command("docker", "ps", "-q")
	if err := cmd.Run(); err != nil {
		t.Fatalf("Cannot access Docker socket: %v", err)
	}
}

func TestNetworkName(t *testing.T) {
	// After unification, NetworkName should always return "urp-network"
	name := NetworkName("any-project")
	if name != "urp-network" {
		t.Errorf("NetworkName should always return 'urp-network', got: %s", name)
	}

	name = NetworkName("")
	if name != "urp-network" {
		t.Errorf("NetworkName with empty project should return 'urp-network', got: %s", name)
	}
}

func TestManagerDockerOnly(t *testing.T) {
	mgr := NewManager(context.Background())

	// After removing Podman support, only Docker or None should be returned
	rt := mgr.Runtime()
	if rt != RuntimeDocker && rt != RuntimeNone {
		t.Errorf("Runtime should be docker or none, got: %s", rt)
	}
}
