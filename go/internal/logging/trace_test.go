package logging

import (
	"context"
	"testing"
)

func TestNewRequestID(t *testing.T) {
	id1 := NewRequestID()
	id2 := NewRequestID()

	if len(id1) != 16 {
		t.Errorf("expected 16 char ID, got %d: %s", len(id1), id1)
	}

	if id1 == id2 {
		t.Error("request IDs should be unique")
	}
}

func TestWithRequestID(t *testing.T) {
	ctx := context.Background()

	// Test with provided ID
	ctx1 := WithRequestID(ctx, "test-id-123")
	if got := GetRequestID(ctx1); got != "test-id-123" {
		t.Errorf("expected 'test-id-123', got '%s'", got)
	}

	// Test with auto-generated ID
	ctx2 := WithRequestID(ctx, "")
	id := GetRequestID(ctx2)
	if len(id) != 16 {
		t.Errorf("expected 16 char auto-generated ID, got %d: %s", len(id), id)
	}
}

func TestGetRequestIDEmpty(t *testing.T) {
	ctx := context.Background()
	if got := GetRequestID(ctx); got != "" {
		t.Errorf("expected empty string for context without ID, got '%s'", got)
	}
}

func TestRequestIDFromContext(t *testing.T) {
	ctx := WithRequestID(context.Background(), "alias-test")
	if got := RequestIDFromContext(ctx); got != "alias-test" {
		t.Errorf("expected 'alias-test', got '%s'", got)
	}
}

func TestRequestIDUniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		id := NewRequestID()
		if seen[id] {
			t.Errorf("duplicate ID generated: %s", id)
		}
		seen[id] = true
	}
}
