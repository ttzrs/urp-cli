// Package config provides centralized configuration management.
// Eliminates 15+ scattered os.Getenv calls across the codebase.
package config

import (
	"os"
	"path/filepath"
	"sync"
)

// URPEnv holds all URP environment variables.
type URPEnv struct {
	// Project is the current project name (URP_PROJECT)
	Project string

	// SessionID is the current session identifier (URP_SESSION_ID)
	SessionID string

	// HostPath is the host project path for master/worker containers (URP_HOST_PATH)
	HostPath string

	// HostHome is the host home directory (URP_HOST_HOME)
	HostHome string

	// ContainerMode indicates we're running inside a container (URP_CONTAINER_MODE)
	ContainerMode bool

	// IsMaster indicates this is a master container (URP_MASTER)
	IsMaster bool

	// WorkerID identifies this worker instance (URP_WORKER_ID)
	WorkerID string

	// Model is the default LLM model (DEFAULT_MODEL)
	Model string

	// AnthropicKey is the Anthropic API key (ANTHROPIC_API_KEY)
	AnthropicKey string

	// AnthropicBaseURL overrides the Anthropic API base URL (ANTHROPIC_BASE_URL)
	AnthropicBaseURL string

	// Neo4jURI is the graph database URI (NEO4J_URI)
	Neo4jURI string

	// Neo4jUser is the graph database user (NEO4J_USER)
	Neo4jUser string

	// Neo4jPassword is the graph database password (NEO4J_PASSWORD)
	Neo4jPassword string

	// OpenAIKey is the OpenAI API key for embeddings (OPENAI_API_KEY)
	OpenAIKey string
}

var (
	env     *URPEnv
	envOnce sync.Once
)

// Env returns the singleton environment configuration.
// Thread-safe, loads once on first call.
func Env() *URPEnv {
	envOnce.Do(func() {
		env = &URPEnv{
			Project:          os.Getenv("URP_PROJECT"),
			SessionID:        os.Getenv("URP_SESSION_ID"),
			HostPath:         os.Getenv("URP_HOST_PATH"),
			HostHome:         os.Getenv("URP_HOST_HOME"),
			ContainerMode:    os.Getenv("URP_CONTAINER_MODE") == "1",
			IsMaster:         os.Getenv("URP_MASTER") == "1",
			WorkerID:         os.Getenv("URP_WORKER_ID"),
			Model:            getEnvDefault("DEFAULT_MODEL", "claude-sonnet-4-20250514"),
			AnthropicKey:     os.Getenv("ANTHROPIC_API_KEY"),
			AnthropicBaseURL: os.Getenv("ANTHROPIC_BASE_URL"),
			Neo4jURI:         getEnvDefault("NEO4J_URI", "bolt://localhost:7687"),
			Neo4jUser:        os.Getenv("NEO4J_USER"),
			Neo4jPassword:    os.Getenv("NEO4J_PASSWORD"),
			OpenAIKey:        os.Getenv("OPENAI_API_KEY"),
		}
	})
	return env
}

// ResetEnv resets the cached environment (for testing).
func ResetEnv() {
	envOnce = sync.Once{}
	env = nil
}

func getEnvDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// Paths holds standard URP directory paths.
type Paths struct {
	// Home is the URP home directory (~/.urp-go)
	Home string

	// Data is the data directory (~/.urp-go/data)
	Data string

	// Backups is the backups directory (~/.urp-go/backups)
	Backups string

	// Skills is the skills directory (~/.urp-go/skills)
	Skills string

	// Alerts is the alerts directory (~/.urp-go/alerts)
	Alerts string

	// EnvFile is the .env file path (~/.urp-go/.env)
	EnvFile string

	// Vectors is the vector store directory (~/.urp-go/vectors)
	Vectors string
}

var (
	paths     *Paths
	pathsOnce sync.Once
)

// GetPaths returns the singleton paths configuration.
func GetPaths() *Paths {
	pathsOnce.Do(func() {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		urpHome := filepath.Join(home, ".urp-go")

		paths = &Paths{
			Home:    urpHome,
			Data:    filepath.Join(urpHome, "data"),
			Backups: filepath.Join(urpHome, "backups"),
			Skills:  filepath.Join(urpHome, "skills"),
			Alerts:  filepath.Join(urpHome, "alerts"),
			EnvFile: filepath.Join(urpHome, ".env"),
			Vectors: filepath.Join(urpHome, "vectors"),
		}
	})
	return paths
}

// Path returns a path under the URP home directory.
// Equivalent to filepath.Join(~/.urp-go, parts...)
func Path(parts ...string) string {
	p := GetPaths()
	allParts := append([]string{p.Home}, parts...)
	return filepath.Join(allParts...)
}

// EnsureDir creates a directory if it doesn't exist.
func EnsureDir(path string) error {
	return os.MkdirAll(path, 0755)
}

// InContainer returns true if running inside a URP container.
func InContainer() bool {
	e := Env()
	return e.ContainerMode || e.HostPath != ""
}

// IsWorker returns true if this is a worker instance.
func IsWorker() bool {
	return Env().WorkerID != ""
}
