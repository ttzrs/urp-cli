package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"

	"github.com/joss/urp/internal/opencode/domain"
)

const (
	ConfigFileName = "opencode.json"
	ConfigDirName  = ".opencode"
)

// Config holds the application configuration
type Config struct {
	Provider       string                   `json:"provider"`
	Model          string                   `json:"model"`
	Agent          string                   `json:"agent"`
	ThinkingBudget int                      `json:"thinkingBudget,omitempty"` // Extended thinking token budget (0 = disabled)
	APIKeys        map[string]string        `json:"apiKeys,omitempty"`
	BaseURLs       map[string]string        `json:"baseURLs,omitempty"`
	MCPServers     []domain.MCPServer       `json:"mcpServers,omitempty"`
	Agents         map[string]domain.Agent  `json:"agents,omitempty"`
}

// Default returns the default configuration
func Default() *Config {
	return &Config{
		Provider: "anthropic",
		Model:    "claude-sonnet-4-20250514",
		Agent:    "build",
		APIKeys:  make(map[string]string),
	}
}

// Load loads configuration from the filesystem
func Load(workDir string) (*Config, error) {
	cfg := Default()

	// Load from global config
	globalPath := GlobalConfigPath()
	if data, err := os.ReadFile(globalPath); err == nil {
		json.Unmarshal(data, cfg)
	}

	// Load from project config (overrides global)
	projectPath := findProjectConfig(workDir)
	if projectPath != "" {
		if data, err := os.ReadFile(projectPath); err == nil {
			json.Unmarshal(data, cfg)
		}
	}

	// Override from environment
	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		cfg.APIKeys["anthropic"] = key
	}
	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		cfg.APIKeys["openai"] = key
	}
	if provider := os.Getenv("OPENCODE_PROVIDER"); provider != "" {
		cfg.Provider = provider
	}
	if model := os.Getenv("OPENCODE_MODEL"); model != "" {
		cfg.Model = model
	}
	if budget := os.Getenv("OPENCODE_THINKING_BUDGET"); budget != "" {
		if b, err := strconv.Atoi(budget); err == nil {
			cfg.ThinkingBudget = b
		}
	}

	return cfg, nil
}

// Save saves the configuration to disk
func (c *Config) Save() error {
	path := GlobalConfigPath()
	dir := filepath.Dir(path)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// GlobalConfigPath returns the path to the global config file
func GlobalConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ConfigDirName, ConfigFileName)
}

// DataDir returns the path to the data directory
func DataDir() string {
	// Check XDG_DATA_HOME first
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "opencode")
	}

	home, _ := os.UserHomeDir()
	return filepath.Join(home, ConfigDirName, "data")
}

func findProjectConfig(dir string) string {
	for {
		// Check for opencode.json
		configPath := filepath.Join(dir, ConfigFileName)
		if _, err := os.Stat(configPath); err == nil {
			return configPath
		}

		// Check for .opencode/opencode.json
		configPath = filepath.Join(dir, ConfigDirName, ConfigFileName)
		if _, err := os.Stat(configPath); err == nil {
			return configPath
		}

		// Move up
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return ""
}
