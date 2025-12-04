package mcp

import (
	"encoding/json"
	"testing"
)

func TestRPCErrorImplementsError(t *testing.T) {
	err := &RPCError{
		Code:    -32600,
		Message: "Invalid Request",
	}

	msg := err.Error()
	if msg != "RPC error -32600: Invalid Request" {
		t.Errorf("Error() = %q, want %q", msg, "RPC error -32600: Invalid Request")
	}
}

func TestRequestMarshal(t *testing.T) {
	req := &Request{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/list",
		Params:  map[string]any{"key": "value"},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if parsed["jsonrpc"] != "2.0" {
		t.Errorf("jsonrpc = %v, want 2.0", parsed["jsonrpc"])
	}
	if parsed["method"] != "tools/list" {
		t.Errorf("method = %v, want tools/list", parsed["method"])
	}
}

func TestResponseUnmarshal(t *testing.T) {
	tests := []struct {
		name      string
		json      string
		wantID    int64
		wantError bool
	}{
		{
			name:      "success response",
			json:      `{"jsonrpc":"2.0","id":1,"result":{"tools":[]}}`,
			wantID:    1,
			wantError: false,
		},
		{
			name:      "error response",
			json:      `{"jsonrpc":"2.0","id":2,"error":{"code":-32600,"message":"Invalid"}}`,
			wantID:    2,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var resp Response
			if err := json.Unmarshal([]byte(tt.json), &resp); err != nil {
				t.Fatalf("Unmarshal error: %v", err)
			}

			if resp.ID != tt.wantID {
				t.Errorf("ID = %d, want %d", resp.ID, tt.wantID)
			}
			if (resp.Error != nil) != tt.wantError {
				t.Errorf("hasError = %v, want %v", resp.Error != nil, tt.wantError)
			}
		})
	}
}

func TestManagerCreation(t *testing.T) {
	m := NewManager()
	if m == nil {
		t.Fatal("NewManager returned nil")
	}
	if m.clients == nil {
		t.Error("clients map should be initialized")
	}
}

func TestManagerToolParsing(t *testing.T) {
	// Test that tool name parsing works correctly
	tests := []struct {
		toolName   string
		serverName string
		wantMatch  bool
	}{
		{"mcp_github_search", "github", true},
		{"mcp_fs_read", "fs", true},
		{"mcp_long_server_name_tool", "long_server_name", true},
		{"not_mcp_tool", "github", false},
		{"mcp__empty", "", true},
	}

	for _, tt := range tests {
		prefix := "mcp_" + tt.serverName + "_"
		hasPrefix := len(tt.toolName) > len(prefix) && tt.toolName[:len(prefix)] == prefix

		if hasPrefix != tt.wantMatch {
			t.Errorf("tool %q prefix match for server %q = %v, want %v",
				tt.toolName, tt.serverName, hasPrefix, tt.wantMatch)
		}
	}
}

func TestManagerCloseEmpty(t *testing.T) {
	m := NewManager()

	// Should not panic on empty manager
	m.Close()

	if len(m.clients) != 0 {
		t.Error("clients should be empty after close")
	}
}
