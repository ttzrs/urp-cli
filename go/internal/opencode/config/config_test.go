package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─────────────────────────────────────────────────────────────────────────────
// Default Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestDefault(t *testing.T) {
	cfg := Default()

	require.NotNil(t, cfg)
	assert.Equal(t, "anthropic", cfg.Provider)
	assert.Equal(t, "claude-sonnet-4-20250514", cfg.Model)
	assert.Equal(t, "build", cfg.Agent)
	assert.NotNil(t, cfg.APIKeys)
}

// ─────────────────────────────────────────────────────────────────────────────
// Load Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestLoad_DefaultsWhenNoConfig(t *testing.T) {
	// Use a temp directory with no config files
	tmpDir, err := os.MkdirTemp("", "config_test_*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	cfg, err := Load(tmpDir)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Should have defaults
	assert.Equal(t, "anthropic", cfg.Provider)
	assert.Equal(t, "claude-sonnet-4-20250514", cfg.Model)
}

func TestLoad_ProjectConfig(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "config_test_*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create project config
	projectCfg := &Config{
		Provider: "openai",
		Model:    "gpt-4",
		Agent:    "custom",
	}
	cfgData, _ := json.Marshal(projectCfg)
	err = os.WriteFile(filepath.Join(tmpDir, ConfigFileName), cfgData, 0644)
	require.NoError(t, err)

	cfg, err := Load(tmpDir)
	require.NoError(t, err)

	assert.Equal(t, "openai", cfg.Provider)
	assert.Equal(t, "gpt-4", cfg.Model)
	assert.Equal(t, "custom", cfg.Agent)
}

func TestLoad_ProjectConfigInSubdir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "config_test_*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create .opencode/opencode.json
	opencodeDir := filepath.Join(tmpDir, ConfigDirName)
	err = os.MkdirAll(opencodeDir, 0755)
	require.NoError(t, err)

	projectCfg := &Config{
		Provider: "custom-provider",
		Model:    "custom-model",
	}
	cfgData, _ := json.Marshal(projectCfg)
	err = os.WriteFile(filepath.Join(opencodeDir, ConfigFileName), cfgData, 0644)
	require.NoError(t, err)

	cfg, err := Load(tmpDir)
	require.NoError(t, err)

	assert.Equal(t, "custom-provider", cfg.Provider)
	assert.Equal(t, "custom-model", cfg.Model)
}

func TestLoad_EnvOverrides(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "config_test_*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Set env vars
	os.Setenv("ANTHROPIC_API_KEY", "test-anthropic-key")
	os.Setenv("OPENAI_API_KEY", "test-openai-key")
	os.Setenv("OPENCODE_PROVIDER", "env-provider")
	os.Setenv("OPENCODE_MODEL", "env-model")
	os.Setenv("OPENCODE_THINKING_BUDGET", "10000")
	defer func() {
		os.Unsetenv("ANTHROPIC_API_KEY")
		os.Unsetenv("OPENAI_API_KEY")
		os.Unsetenv("OPENCODE_PROVIDER")
		os.Unsetenv("OPENCODE_MODEL")
		os.Unsetenv("OPENCODE_THINKING_BUDGET")
	}()

	cfg, err := Load(tmpDir)
	require.NoError(t, err)

	assert.Equal(t, "test-anthropic-key", cfg.APIKeys["anthropic"])
	assert.Equal(t, "test-openai-key", cfg.APIKeys["openai"])
	assert.Equal(t, "env-provider", cfg.Provider)
	assert.Equal(t, "env-model", cfg.Model)
	assert.Equal(t, 10000, cfg.ThinkingBudget)
}

func TestLoad_EnvOverridesConfig(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "config_test_*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create config with one value
	projectCfg := &Config{
		Provider: "file-provider",
		Model:    "file-model",
	}
	cfgData, _ := json.Marshal(projectCfg)
	err = os.WriteFile(filepath.Join(tmpDir, ConfigFileName), cfgData, 0644)
	require.NoError(t, err)

	// Set env var to override
	os.Setenv("OPENCODE_PROVIDER", "env-override")
	defer os.Unsetenv("OPENCODE_PROVIDER")

	cfg, err := Load(tmpDir)
	require.NoError(t, err)

	// Env should override file
	assert.Equal(t, "env-override", cfg.Provider)
	// File value should still be there
	assert.Equal(t, "file-model", cfg.Model)
}

// ─────────────────────────────────────────────────────────────────────────────
// Save Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestConfig_Save(t *testing.T) {
	// Use temp dir for home
	tmpDir, err := os.MkdirTemp("", "config_test_*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Override home for test
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	cfg := &Config{
		Provider:       "test-provider",
		Model:          "test-model",
		Agent:          "test-agent",
		ThinkingBudget: 5000,
		APIKeys:        map[string]string{"test": "key"},
	}

	err = cfg.Save()
	require.NoError(t, err)

	// Verify file exists
	savedPath := filepath.Join(tmpDir, ConfigDirName, ConfigFileName)
	_, err = os.Stat(savedPath)
	assert.NoError(t, err)

	// Verify content
	data, err := os.ReadFile(savedPath)
	require.NoError(t, err)

	var loaded Config
	err = json.Unmarshal(data, &loaded)
	require.NoError(t, err)

	assert.Equal(t, "test-provider", loaded.Provider)
	assert.Equal(t, "test-model", loaded.Model)
	assert.Equal(t, 5000, loaded.ThinkingBudget)
}

// ─────────────────────────────────────────────────────────────────────────────
// Path Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestGlobalConfigPath(t *testing.T) {
	path := GlobalConfigPath()
	assert.NotEmpty(t, path)
	assert.Contains(t, path, ConfigDirName)
	assert.Contains(t, path, ConfigFileName)
}

func TestDataDir(t *testing.T) {
	// Without XDG_DATA_HOME
	os.Unsetenv("XDG_DATA_HOME")
	dir := DataDir()
	assert.NotEmpty(t, dir)
	assert.Contains(t, dir, ConfigDirName)

	// With XDG_DATA_HOME
	os.Setenv("XDG_DATA_HOME", "/custom/data")
	defer os.Unsetenv("XDG_DATA_HOME")

	dir = DataDir()
	assert.Equal(t, "/custom/data/opencode", dir)
}

// ─────────────────────────────────────────────────────────────────────────────
// findProjectConfig Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestFindProjectConfig_InCurrentDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "config_test_*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create config in current dir
	cfgPath := filepath.Join(tmpDir, ConfigFileName)
	os.WriteFile(cfgPath, []byte("{}"), 0644)

	found := findProjectConfig(tmpDir)
	assert.Equal(t, cfgPath, found)
}

func TestFindProjectConfig_InParentDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "config_test_*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create config in parent
	cfgPath := filepath.Join(tmpDir, ConfigFileName)
	os.WriteFile(cfgPath, []byte("{}"), 0644)

	// Create subdirectory
	subDir := filepath.Join(tmpDir, "sub", "dir")
	os.MkdirAll(subDir, 0755)

	found := findProjectConfig(subDir)
	assert.Equal(t, cfgPath, found)
}

func TestFindProjectConfig_NotFound(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "config_test_*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	found := findProjectConfig(tmpDir)
	assert.Empty(t, found)
}

// ─────────────────────────────────────────────────────────────────────────────
// Config Struct Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestConfig_JSON(t *testing.T) {
	cfg := &Config{
		Provider:       "anthropic",
		Model:          "claude-3",
		Agent:          "build",
		ThinkingBudget: 10000,
		APIKeys:        map[string]string{"anthropic": "key123"},
		BaseURLs:       map[string]string{"anthropic": "https://api.example.com"},
	}

	data, err := json.Marshal(cfg)
	require.NoError(t, err)

	var parsed Config
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)

	assert.Equal(t, cfg.Provider, parsed.Provider)
	assert.Equal(t, cfg.Model, parsed.Model)
	assert.Equal(t, cfg.ThinkingBudget, parsed.ThinkingBudget)
	assert.Equal(t, "key123", parsed.APIKeys["anthropic"])
}

func TestConfig_JSONOmitEmpty(t *testing.T) {
	cfg := &Config{
		Provider: "anthropic",
		Model:    "claude-3",
	}

	data, err := json.Marshal(cfg)
	require.NoError(t, err)

	// ThinkingBudget should be omitted when 0
	assert.NotContains(t, string(data), "thinkingBudget")
}
