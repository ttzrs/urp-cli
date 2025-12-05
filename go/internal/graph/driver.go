// Package graph provides database abstraction for graph operations.
// Implements the D (Dependency Inversion) principle - high-level modules
// depend on abstractions, not concrete implementations.
package graph

import (
	"context"
)

// Record represents a single result row from a query.
type Record map[string]any

// GraphReader provides read-only graph database operations.
// Use this for query-only consumers (ISP - Interface Segregation).
type GraphReader interface {
	// Execute runs a Cypher query and returns results.
	Execute(ctx context.Context, query string, params map[string]any) ([]Record, error)
}

// GraphWriter provides write graph database operations.
// Use this for write-only consumers.
type GraphWriter interface {
	// ExecuteWrite runs a write query (CREATE, MERGE, SET, DELETE).
	ExecuteWrite(ctx context.Context, query string, params map[string]any) error
}

// Driver defines the full interface for graph database operations.
// Composes GraphReader + GraphWriter + lifecycle methods.
// Any graph DB (Memgraph, Neo4j, etc.) must implement this interface.
type Driver interface {
	GraphReader
	GraphWriter

	// Close releases database resources.
	Close() error

	// Ping checks if the database is reachable.
	Ping(ctx context.Context) error
}

// Config holds database connection configuration.
type Config struct {
	URI      string
	Username string
	Password string
	Database string
}

// DefaultConfig returns configuration from environment variables.
func DefaultConfig() Config {
	return Config{
		URI:      getEnv("NEO4J_URI", "bolt://localhost:7687"),
		Username: getEnv("NEO4J_USER", ""),
		Password: getEnv("NEO4J_PASSWORD", ""),
		Database: getEnv("NEO4J_DATABASE", "memgraph"),
	}
}

func getEnv(key, fallback string) string {
	if val, ok := lookupEnv(key); ok {
		return val
	}
	return fallback
}

// lookupEnv is a variable for testing injection.
var lookupEnv = func(key string) (string, bool) {
	// Will be replaced by os.LookupEnv in init
	return "", false
}

func init() {
	// Wire up real environment lookup
	lookupEnv = func(key string) (string, bool) {
		val := ""
		// Import os at runtime to avoid cycle
		if v, ok := envLookup(key); ok {
			return v, true
		}
		return val, false
	}
}

// envLookup is injected from main to avoid import cycle.
var envLookup = func(key string) (string, bool) { return "", false }

// SetEnvLookup allows injecting the os.LookupEnv function.
func SetEnvLookup(fn func(string) (string, bool)) {
	envLookup = fn
	lookupEnv = fn
}
