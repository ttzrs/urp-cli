package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEnv(t *testing.T) {
	// Reset env for clean test
	ResetEnv()

	// Set test environment
	os.Setenv("URP_PROJECT", "test-project")
	os.Setenv("URP_SESSION_ID", "sess-123")
	os.Setenv("URP_CONTAINER_MODE", "1")
	os.Setenv("URP_MASTER", "1")
	os.Setenv("NEO4J_URI", "bolt://testhost:7687")
	defer func() {
		os.Unsetenv("URP_PROJECT")
		os.Unsetenv("URP_SESSION_ID")
		os.Unsetenv("URP_CONTAINER_MODE")
		os.Unsetenv("URP_MASTER")
		os.Unsetenv("NEO4J_URI")
		ResetEnv()
	}()

	env := Env()

	assert.Equal(t, "test-project", env.Project)
	assert.Equal(t, "sess-123", env.SessionID)
	assert.True(t, env.ContainerMode)
	assert.True(t, env.IsMaster)
	assert.Equal(t, "bolt://testhost:7687", env.Neo4jURI)
}

func TestEnvDefaults(t *testing.T) {
	ResetEnv()

	// Clear environment
	os.Unsetenv("DEFAULT_MODEL")
	os.Unsetenv("NEO4J_URI")
	defer ResetEnv()

	env := Env()

	// Check defaults
	assert.Equal(t, "claude-sonnet-4-20250514", env.Model)
	assert.Equal(t, "bolt://localhost:7687", env.Neo4jURI)
}

func TestEnvSingleton(t *testing.T) {
	ResetEnv()
	defer ResetEnv()

	env1 := Env()
	env2 := Env()

	// Should return same instance
	assert.Same(t, env1, env2)
}

func TestResetEnv(t *testing.T) {
	os.Setenv("URP_PROJECT", "first")
	env1 := Env()
	assert.Equal(t, "first", env1.Project)

	os.Setenv("URP_PROJECT", "second")
	ResetEnv()

	env2 := Env()
	assert.Equal(t, "second", env2.Project)

	// Cleanup
	os.Unsetenv("URP_PROJECT")
	ResetEnv()
}

func TestGetEnvDefault(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		envVal   string
		fallback string
		want     string
	}{
		{"env set", "TEST_KEY", "value", "default", "value"},
		{"env empty", "TEST_KEY", "", "default", "default"},
		{"env not set", "TEST_KEY_NOTSET", "", "fallback", "fallback"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envVal != "" {
				os.Setenv(tt.key, tt.envVal)
				defer os.Unsetenv(tt.key)
			}
			got := getEnvDefault(tt.key, tt.fallback)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGetPaths(t *testing.T) {
	paths := GetPaths()

	assert.NotEmpty(t, paths.Home)
	assert.Contains(t, paths.Home, ".urp-go")
	assert.Equal(t, filepath.Join(paths.Home, "data"), paths.Data)
	assert.Equal(t, filepath.Join(paths.Home, "backups"), paths.Backups)
	assert.Equal(t, filepath.Join(paths.Home, "skills"), paths.Skills)
	assert.Equal(t, filepath.Join(paths.Home, "alerts"), paths.Alerts)
	assert.Equal(t, filepath.Join(paths.Home, "vectors"), paths.Vectors)
	assert.Equal(t, filepath.Join(paths.Home, ".env"), paths.EnvFile)
}

func TestPath(t *testing.T) {
	result := Path("subdir", "file.txt")

	assert.Contains(t, result, ".urp-go")
	assert.Contains(t, result, "subdir")
	assert.Contains(t, result, "file.txt")
}

func TestEnsureDir(t *testing.T) {
	// Create temp directory
	tempDir := filepath.Join(os.TempDir(), "urp-test-ensure")
	defer os.RemoveAll(tempDir)

	// Ensure it doesn't exist
	os.RemoveAll(tempDir)

	// Create it
	err := EnsureDir(tempDir)
	assert.NoError(t, err)

	// Verify it exists
	info, err := os.Stat(tempDir)
	assert.NoError(t, err)
	assert.True(t, info.IsDir())

	// Running again should be idempotent
	err = EnsureDir(tempDir)
	assert.NoError(t, err)
}

func TestInContainer(t *testing.T) {
	ResetEnv()
	defer ResetEnv()

	// Not in container by default
	assert.False(t, InContainer())

	// Container mode
	os.Setenv("URP_CONTAINER_MODE", "1")
	ResetEnv()
	assert.True(t, InContainer())
	os.Unsetenv("URP_CONTAINER_MODE")

	// Host path set
	ResetEnv()
	os.Setenv("URP_HOST_PATH", "/host/path")
	ResetEnv()
	assert.True(t, InContainer())
	os.Unsetenv("URP_HOST_PATH")
}

func TestIsWorker(t *testing.T) {
	ResetEnv()
	defer ResetEnv()

	// Not a worker by default
	assert.False(t, IsWorker())

	// With worker ID
	os.Setenv("URP_WORKER_ID", "worker-1")
	ResetEnv()
	assert.True(t, IsWorker())
	os.Unsetenv("URP_WORKER_ID")
}
