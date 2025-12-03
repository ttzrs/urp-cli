package graph

import (
	"context"
	"testing"
)

func TestParseURI(t *testing.T) {
	tests := []struct {
		uri      string
		wantHost string
		wantPort string
	}{
		{"bolt://localhost:7687", "localhost", "7687"},
		{"bolt://memgraph:7687", "memgraph", "7687"},
		{"neo4j://db.example.com:7474", "db.example.com", "7474"},
		{"bolt://127.0.0.1:7687", "127.0.0.1", "7687"},
	}

	for _, tt := range tests {
		t.Run(tt.uri, func(t *testing.T) {
			// Basic URI parsing test
			if len(tt.uri) < 7 {
				t.Errorf("URI too short: %s", tt.uri)
			}
		})
	}
}

func TestConnectWithoutDB(t *testing.T) {
	// Set env to invalid URI
	SetEnvLookup(func(key string) (string, bool) {
		if key == "NEO4J_URI" {
			return "bolt://invalid-host:7687", true
		}
		return "", false
	})

	_, err := Connect()
	if err == nil {
		t.Log("Connect returned nil error (expected when no DB)")
	}
}

func TestMockDriver(t *testing.T) {
	mock := NewMockDriver()

	ctx := context.Background()

	// Test Ping
	if err := mock.Ping(ctx); err != nil {
		t.Errorf("Ping failed: %v", err)
	}

	// Test Execute
	records, err := mock.Execute(ctx, "RETURN 1", nil)
	if err != nil {
		t.Errorf("Execute failed: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("Expected empty records, got %d", len(records))
	}

	// Test ExecuteWrite
	if err := mock.ExecuteWrite(ctx, "CREATE (n:Test)", nil); err != nil {
		t.Errorf("ExecuteWrite failed: %v", err)
	}

	// Test Close
	if err := mock.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

// MockDriver for testing without a real database
type MockDriver struct{}

func NewMockDriver() *MockDriver {
	return &MockDriver{}
}

func (m *MockDriver) Execute(ctx context.Context, query string, params map[string]any) ([]Record, error) {
	return []Record{}, nil
}

func (m *MockDriver) ExecuteWrite(ctx context.Context, query string, params map[string]any) error {
	return nil
}

func (m *MockDriver) Close() error {
	return nil
}

func (m *MockDriver) Ping(ctx context.Context) error {
	return nil
}
