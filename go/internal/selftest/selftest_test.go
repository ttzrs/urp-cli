package selftest

import (
	"strings"
	"testing"
)

func TestCheck(t *testing.T) {
	env := Check()

	// Should have a runtime detected (docker or podman) or error
	if env.Runtime == "" {
		t.Error("Runtime should not be empty")
	}

	// ImagesExist map should be initialized
	if env.ImagesExist == nil {
		t.Error("ImagesExist map should be initialized")
	}
}

func TestEnvironmentSummary(t *testing.T) {
	env := &Environment{
		HasTTY:         false,
		Runtime:        "docker",
		RuntimeVersion: "Docker version 24.0.0",
		MemgraphUp:     true,
		NetworkExists:  true,
		ImagesExist: map[string]bool{
			"urp:latest": true,
			"urp:master": false,
		},
		Warnings: []string{"Image urp:master not found"},
	}

	summary := env.Summary()

	// Check key elements are present
	if !strings.Contains(summary, "URP ENVIRONMENT CHECK") {
		t.Error("Summary should have header")
	}
	if !strings.Contains(summary, "Docker version 24.0.0") {
		t.Error("Summary should show runtime version")
	}
	if !strings.Contains(summary, "detached mode") {
		t.Error("Summary should mention detached mode when no TTY")
	}
	if !strings.Contains(summary, "urp:master not found") {
		t.Error("Summary should show warnings")
	}
}

func TestQuickCheck(t *testing.T) {
	tests := []struct {
		name     string
		env      *Environment
		contains string
	}{
		{
			name: "healthy detached",
			env: &Environment{
				Runtime:       "docker",
				HasTTY:        false,
				MemgraphUp:    true,
				NetworkExists: true,
				ImagesExist:   map[string]bool{},
			},
			contains: "runtime:docker mode:detached infra:up",
		},
		{
			name: "healthy interactive",
			env: &Environment{
				Runtime:       "docker",
				HasTTY:        true,
				MemgraphUp:    true,
				NetworkExists: true,
				ImagesExist:   map[string]bool{},
			},
			contains: "mode:interactive",
		},
		{
			name: "unhealthy no runtime",
			env: &Environment{
				Runtime:     "none",
				Errors:      []string{"No container runtime"},
				ImagesExist: map[string]bool{},
			},
			contains: "unhealthy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.env.QuickCheck()
			if !strings.Contains(result, tt.contains) {
				t.Errorf("QuickCheck() = %q, want to contain %q", result, tt.contains)
			}
		})
	}
}

func TestIsHealthy(t *testing.T) {
	tests := []struct {
		name    string
		env     *Environment
		healthy bool
	}{
		{
			name:    "healthy with docker",
			env:     &Environment{Runtime: "docker", ImagesExist: map[string]bool{}},
			healthy: true,
		},
		{
			name:    "healthy with podman",
			env:     &Environment{Runtime: "podman", ImagesExist: map[string]bool{}},
			healthy: true,
		},
		{
			name:    "unhealthy no runtime",
			env:     &Environment{Runtime: "none", ImagesExist: map[string]bool{}},
			healthy: false,
		},
		{
			name:    "unhealthy with errors",
			env:     &Environment{Runtime: "docker", Errors: []string{"something failed"}, ImagesExist: map[string]bool{}},
			healthy: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.env.IsHealthy(); got != tt.healthy {
				t.Errorf("IsHealthy() = %v, want %v", got, tt.healthy)
			}
		})
	}
}

func TestCanLaunchMaster(t *testing.T) {
	env := &Environment{
		Runtime: "docker",
		ImagesExist: map[string]bool{
			"urp:master": true,
		},
	}

	if !env.CanLaunchMaster() {
		t.Error("Should be able to launch master when image exists")
	}

	env.ImagesExist["urp:master"] = false
	if env.CanLaunchMaster() {
		t.Error("Should not be able to launch master when image missing")
	}
}

func TestCanSpawnWorker(t *testing.T) {
	env := &Environment{
		Runtime: "docker",
		ImagesExist: map[string]bool{
			"urp:worker": true,
		},
	}

	if !env.CanSpawnWorker() {
		t.Error("Should be able to spawn worker when image exists")
	}

	env.ImagesExist["urp:worker"] = false
	if env.CanSpawnWorker() {
		t.Error("Should not be able to spawn worker when image missing")
	}
}

func TestCanLaunchStandalone(t *testing.T) {
	env := &Environment{
		Runtime: "docker",
		ImagesExist: map[string]bool{
			"urp:latest": true,
		},
	}

	if !env.CanLaunchStandalone() {
		t.Error("Should be able to launch standalone when image exists")
	}

	env.ImagesExist["urp:latest"] = false
	if env.CanLaunchStandalone() {
		t.Error("Should not be able to launch standalone when image missing")
	}
}
