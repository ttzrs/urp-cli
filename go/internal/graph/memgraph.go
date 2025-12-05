// Package graph provides Memgraph implementation.
package graph

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// Memgraph implements Driver for Memgraph database.
type Memgraph struct {
	driver neo4j.DriverWithContext
	config Config
}

// NewMemgraph creates a new Memgraph driver.
func NewMemgraph(cfg Config) (*Memgraph, error) {
	var auth neo4j.AuthToken
	if cfg.Username != "" {
		auth = neo4j.BasicAuth(cfg.Username, cfg.Password, "")
	} else {
		auth = neo4j.NoAuth()
	}

	driver, err := neo4j.NewDriverWithContext(cfg.URI, auth)
	if err != nil {
		return nil, fmt.Errorf("failed to create driver: %w", err)
	}

	return &Memgraph{
		driver: driver,
		config: cfg,
	}, nil
}

// Execute runs a read query and returns results.
func (m *Memgraph) Execute(ctx context.Context, query string, params map[string]any) ([]Record, error) {
	session := m.driver.NewSession(ctx, neo4j.SessionConfig{
		AccessMode: neo4j.AccessModeRead,
	})
	defer session.Close(ctx)

	result, err := session.Run(ctx, query, params)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}

	var records []Record
	for result.Next(ctx) {
		rec := result.Record()
		record := make(Record)
		for _, key := range rec.Keys {
			val, _ := rec.Get(key)
			record[key] = val
		}
		records = append(records, record)
	}

	if err := result.Err(); err != nil {
		return nil, fmt.Errorf("result iteration failed: %w", err)
	}

	return records, nil
}

// ExecuteWrite runs a write query.
func (m *Memgraph) ExecuteWrite(ctx context.Context, query string, params map[string]any) error {
	session := m.driver.NewSession(ctx, neo4j.SessionConfig{
		AccessMode: neo4j.AccessModeWrite,
	})
	defer session.Close(ctx)

	_, err := session.Run(ctx, query, params)
	if err != nil {
		return fmt.Errorf("write query failed: %w", err)
	}

	return nil
}

// Close releases the database driver.
func (m *Memgraph) Close() error {
	return m.driver.Close(context.Background())
}

// Ping checks database connectivity.
func (m *Memgraph) Ping(ctx context.Context) error {
	return m.driver.VerifyConnectivity(ctx)
}

// MustConnect creates a Memgraph driver or panics.
func MustConnect(cfg Config) *Memgraph {
	mg, err := NewMemgraph(cfg)
	if err != nil {
		panic(fmt.Sprintf("failed to connect to Memgraph: %v", err))
	}
	return mg
}

// Connect creates a Memgraph driver with default config.
func Connect() (*Memgraph, error) {
	return NewMemgraph(DefaultConfig())
}

// ConnectWithRetry tries to connect with exponential backoff.
// Returns nil if all retries fail (graceful degradation).
func ConnectWithRetry(maxRetries int) *Memgraph {
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		mg, err := Connect()
		if err == nil {
			// Verify connectivity
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			if pingErr := mg.Ping(ctx); pingErr == nil {
				cancel()
				return mg
			}
			cancel()
			mg.Close()
			lastErr = err
		} else {
			lastErr = err
		}
		// Exponential backoff: 100ms, 200ms, 400ms...
		time.Sleep(time.Duration(100<<i) * time.Millisecond)
	}
	if lastErr != nil {
		// Log but don't fail - graceful degradation
		fmt.Fprintf(os.Stderr, "⚠ Memgraph unavailable: %v (continuing without graph)\n", lastErr)
	}
	return nil
}

// IsConnectionError checks if an error is a connection-related error.
func IsConnectionError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "connection reset") ||
		strings.Contains(errStr, "no such host") ||
		strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "EOF")
}
