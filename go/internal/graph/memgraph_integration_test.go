//go:build integration

// Package graph integration tests require Memgraph running.
// Run with: go test -tags=integration ./internal/graph/...
package graph

import (
	"context"
	"testing"
	"time"
)

func TestMemgraphConnection(t *testing.T) {
	db, err := Connect()
	if err != nil {
		t.Fatalf("Failed to connect to Memgraph: %v\n\nStart Memgraph with: docker compose up -d memgraph", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.Ping(ctx); err != nil {
		t.Fatalf("Memgraph ping failed: %v", err)
	}
}

func TestMemgraphQuery(t *testing.T) {
	db, err := Connect()
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	records, err := db.Execute(ctx, "RETURN 1 as n", nil)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(records) != 1 {
		t.Errorf("Expected 1 record, got %d", len(records))
	}
}

func TestMemgraphWrite(t *testing.T) {
	db, err := Connect()
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	// Create a test node
	err = db.ExecuteWrite(ctx, "CREATE (t:TestNode {name: 'integration-test', ts: $ts})", map[string]any{
		"ts": time.Now().Unix(),
	})
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Verify it was created
	records, err := db.Execute(ctx, "MATCH (t:TestNode {name: 'integration-test'}) RETURN t.ts as ts", nil)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if len(records) == 0 {
		t.Error("Test node was not created")
	}

	// Cleanup
	_ = db.ExecuteWrite(ctx, "MATCH (t:TestNode {name: 'integration-test'}) DELETE t", nil)
}

func TestConnectWithRetry(t *testing.T) {
	db, err := ConnectWithRetry(3)
	if err != nil {
		t.Fatalf("ConnectWithRetry failed: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.Ping(ctx); err != nil {
		t.Fatalf("Ping after ConnectWithRetry failed: %v", err)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.URI == "" {
		t.Error("DefaultConfig URI should not be empty")
	}

	// Default should be bolt://localhost:7687
	if cfg.URI != "bolt://localhost:7687" {
		t.Logf("URI: %s (may be overridden by NEO4J_URI env)", cfg.URI)
	}
}
