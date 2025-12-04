package container

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSpawnRequirementsValidation(t *testing.T) {
	// Create temp project dir
	tmpDir := t.TempDir()
	projectPath := filepath.Join(tmpDir, "test-project")
	if err := os.MkdirAll(projectPath, 0755); err != nil {
		t.Fatal(err)
	}

	// Create temp env file
	envDir := filepath.Join(tmpDir, ".urp-go")
	if err := os.MkdirAll(envDir, 0755); err != nil {
		t.Fatal(err)
	}
	envFile := filepath.Join(envDir, ".env")
	if err := os.WriteFile(envFile, []byte("TEST=1"), 0644); err != nil {
		t.Fatal(err)
	}

	// Point to our temp home
	oldHome := os.Getenv("URP_HOST_HOME")
	os.Setenv("URP_HOST_HOME", tmpDir)
	defer os.Setenv("URP_HOST_HOME", oldHome)

	mgr := NewManager(nil)
	req := mgr.ValidateSpawnRequirements(projectPath)

	// Project path should be valid
	if !req.ProjectPath {
		t.Error("ProjectPath should be true for existing path")
	}

	// Env file should be valid
	if !req.EnvFile {
		t.Error("EnvFile should be true for existing .env")
	}

	// Docker socket depends on host - just verify it's set to something
	// (can't reliably test on all systems)

	// SELinux should be detected
	if req.SELinux == "" {
		t.Error("SELinux should be detected (even if 'unknown')")
	}
}

func TestSpawnRequirementsInvalid(t *testing.T) {
	mgr := NewManager(nil)
	req := mgr.ValidateSpawnRequirements("/nonexistent/path/12345")

	if req.ProjectPath {
		t.Error("ProjectPath should be false for nonexistent path")
	}

	if req.IsValid() {
		t.Error("Requirements should be invalid for nonexistent path")
	}

	if len(req.Errors) == 0 {
		t.Error("Should have at least one error")
	}
}

func TestIsSELinuxEnforcing(t *testing.T) {
	// Just verify the function doesn't panic
	_ = IsSELinuxEnforcing()
}

func TestDetectSELinux(t *testing.T) {
	mode := detectSELinux()

	// Should return one of the valid modes
	validModes := map[string]bool{
		"enforcing":  true,
		"permissive": true,
		"disabled":   true,
		"unknown":    true,
	}

	if !validModes[mode] {
		t.Errorf("detectSELinux returned invalid mode: %s", mode)
	}
}

func TestHealthCheckResultDefaults(t *testing.T) {
	result := &HealthCheckResult{}

	if result.Running {
		t.Error("Running should default to false")
	}
	if result.DockerAccess {
		t.Error("DockerAccess should default to false")
	}
}
