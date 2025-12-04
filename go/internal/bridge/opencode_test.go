package bridge

import (
	"encoding/json"
	"testing"
	"time"
)

func TestSessionStruct(t *testing.T) {
	sess := Session{
		ID:        "sess-123",
		ProjectID: "proj-abc",
		Directory: "/home/user/project",
		Title:     "Test Session",
		Version:   "1.0.0",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Summary: &Summary{
			Additions: 10,
			Deletions: 5,
			Files:     []string{"main.go", "test.go"},
		},
	}

	if sess.ID != "sess-123" {
		t.Errorf("expected sess-123, got %s", sess.ID)
	}
	if sess.Summary.Additions != 10 {
		t.Errorf("expected 10 additions, got %d", sess.Summary.Additions)
	}
}

func TestMessageStruct(t *testing.T) {
	msg := Message{
		ID:        "msg-001",
		SessionID: "sess-123",
		Role:      "user",
		Parts: []Part{
			{Type: "text", Text: "Hello world"},
		},
		Timestamp: time.Now(),
	}

	if msg.Role != "user" {
		t.Errorf("expected user, got %s", msg.Role)
	}
	if len(msg.Parts) != 1 {
		t.Errorf("expected 1 part, got %d", len(msg.Parts))
	}
	if msg.Parts[0].Type != "text" {
		t.Errorf("expected text, got %s", msg.Parts[0].Type)
	}
}

func TestPartTypes(t *testing.T) {
	tests := []struct {
		name string
		part Part
	}{
		{
			name: "text part",
			part: Part{Type: "text", Text: "Hello"},
		},
		{
			name: "reasoning part",
			part: Part{Type: "reasoning", Text: "Thinking..."},
		},
		{
			name: "tool_call part",
			part: Part{
				Type:   "tool_call",
				ToolID: "tool-1",
				Name:   "bash",
				Args:   map[string]any{"command": "ls"},
				Result: "file1\nfile2",
			},
		},
		{
			name: "file part",
			part: Part{
				Type:     "file",
				Path:     "/path/to/file.go",
				Content:  "package main",
				Language: "go",
			},
		},
		{
			name: "image part",
			part: Part{
				Type:   "image",
				Base64: "iVBORw0KGgo...",
				Media:  "image/png",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify JSON roundtrip
			data, err := json.Marshal(tt.part)
			if err != nil {
				t.Fatalf("failed to marshal: %v", err)
			}

			var decoded Part
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("failed to unmarshal: %v", err)
			}

			if decoded.Type != tt.part.Type {
				t.Errorf("type mismatch: got %s, want %s", decoded.Type, tt.part.Type)
			}
		})
	}
}

func TestUsageStruct(t *testing.T) {
	usage := Usage{
		InputTokens:  1000,
		OutputTokens: 500,
		CacheRead:    800,
		CacheWrite:   200,
		InputCost:    0.003,
		OutputCost:   0.0075,
		TotalCost:    0.0105,
	}

	if usage.InputTokens != 1000 {
		t.Errorf("expected 1000 input tokens, got %d", usage.InputTokens)
	}
	if usage.TotalCost != 0.0105 {
		t.Errorf("expected 0.0105 total cost, got %f", usage.TotalCost)
	}
}

func TestSessionUsageStruct(t *testing.T) {
	su := SessionUsage{
		SessionID:    "sess-123",
		ProviderID:   "anthropic",
		ModelID:      "claude-sonnet-4",
		MessageCount: 10,
		ToolCalls:    5,
		Usage: Usage{
			InputTokens:  5000,
			OutputTokens: 2500,
		},
		UpdatedAt: time.Now(),
	}

	if su.ProviderID != "anthropic" {
		t.Errorf("expected anthropic, got %s", su.ProviderID)
	}
	if su.MessageCount != 10 {
		t.Errorf("expected 10 messages, got %d", su.MessageCount)
	}
}

func TestRecordHelpers(t *testing.T) {
	record := map[string]any{
		"name":  "test",
		"count": int64(42),
		"price": float64(19.99),
		"other": int(100),
	}

	if getString(record, "name") != "test" {
		t.Error("getString failed")
	}
	if getString(record, "missing") != "" {
		t.Error("getString should return empty for missing")
	}

	if getInt64(record, "count") != 42 {
		t.Error("getInt64 failed for int64")
	}
	if getInt64(record, "other") != 100 {
		t.Error("getInt64 failed for int")
	}
	if getInt64(record, "price") != 19 {
		t.Error("getInt64 failed for float64")
	}
	if getInt64(record, "missing") != 0 {
		t.Error("getInt64 should return 0 for missing")
	}
}

func TestMessagePartsJSON(t *testing.T) {
	msg := Message{
		ID:        "msg-001",
		SessionID: "sess-123",
		Role:      "assistant",
		Parts: []Part{
			{Type: "text", Text: "Let me help you with that."},
			{Type: "tool_call", ToolID: "t1", Name: "read", Args: map[string]any{"path": "/file.go"}},
		},
	}

	// Marshal
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	// Unmarshal
	var decoded Message
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if len(decoded.Parts) != 2 {
		t.Errorf("expected 2 parts, got %d", len(decoded.Parts))
	}
	if decoded.Parts[0].Type != "text" {
		t.Errorf("first part should be text")
	}
	if decoded.Parts[1].Name != "read" {
		t.Errorf("second part should be read tool")
	}
}
