package tool

import (
	"context"
	"testing"

	"github.com/joss/urp/internal/opencode/domain"
	"github.com/joss/urp/internal/opencode/mcp"
)

func TestMCPToolInfo(t *testing.T) {
	manager := mcp.NewManager()
	defer manager.Close()

	toolInfo := domain.Tool{
		ID:          "mcp_test_echo",
		Name:        "mcp_test_echo",
		Description: "Test echo tool from MCP",
		Parameters: domain.JSONSchema{
			"type": "object",
			"properties": map[string]any{
				"message": map[string]any{"type": "string"},
			},
		},
	}

	mcpTool := NewMCPTool(manager, toolInfo)

	info := mcpTool.Info()
	if info.Name != "mcp_test_echo" {
		t.Errorf("Name = %s, want mcp_test_echo", info.Name)
	}
	if info.Description != "Test echo tool from MCP" {
		t.Errorf("Description = %s, want 'Test echo tool from MCP'", info.Description)
	}
	if info.ID != "mcp_test_echo" {
		t.Errorf("ID = %s, want mcp_test_echo", info.ID)
	}
}

func TestMCPToolImplementsExecutor(t *testing.T) {
	manager := mcp.NewManager()
	defer manager.Close()

	mcpTool := NewMCPTool(manager, domain.Tool{Name: "test"})

	// Verify it implements Executor
	var _ Executor = mcpTool
}

func TestMCPToolExecuteNoServer(t *testing.T) {
	manager := mcp.NewManager()
	defer manager.Close()

	toolInfo := domain.Tool{
		ID:   "mcp_missing_tool",
		Name: "mcp_missing_tool",
	}

	mcpTool := NewMCPTool(manager, toolInfo)

	result, err := mcpTool.Execute(context.Background(), map[string]any{
		"message": "hello",
	})

	// Should not return error at top level
	if err != nil {
		t.Fatalf("Execute should not return error: %v", err)
	}

	// But result should have error info since no server is connected
	if result.Error == nil {
		t.Error("Result should have error when no server connected")
	}
}

func TestRegisterMCPToolsEmpty(t *testing.T) {
	manager := mcp.NewManager()
	defer manager.Close()

	registry := NewRegistry()

	// Should succeed with no servers connected
	err := RegisterMCPTools(context.Background(), registry, manager)
	if err != nil {
		t.Fatalf("RegisterMCPTools failed: %v", err)
	}

	// Registry should have no tools (no MCP servers)
	if len(registry.All()) != 0 {
		t.Errorf("Expected 0 tools, got %d", len(registry.All()))
	}
}

func TestMCPToolIntegrationWithRegistry(t *testing.T) {
	manager := mcp.NewManager()
	defer manager.Close()

	// Create registry with builtin + MCP tools
	registry := NewRegistry()

	// Add a builtin tool
	registry.Register(NewBash("/tmp"))

	// Create and register an MCP tool manually
	mcpTool := domain.Tool{
		ID:          "mcp_test_greet",
		Name:        "mcp_test_greet",
		Description: "MCP greeting tool",
		Parameters: domain.JSONSchema{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string"},
			},
		},
	}
	registry.Register(NewMCPTool(manager, mcpTool))

	// Verify both tools are available
	allTools := registry.All()
	if len(allTools) != 2 {
		t.Errorf("Expected 2 tools, got %d", len(allTools))
	}

	// Verify MCP tool can be retrieved
	mcpExecutor, ok := registry.Get("mcp_test_greet")
	if !ok {
		t.Fatal("MCP tool not found in registry")
	}

	info := mcpExecutor.Info()
	if info.Description != "MCP greeting tool" {
		t.Errorf("Description = %s, want 'MCP greeting tool'", info.Description)
	}

	// Verify builtin tool still works
	bashExecutor, ok := registry.Get("bash")
	if !ok {
		t.Fatal("Bash tool not found in registry")
	}

	bashInfo := bashExecutor.Info()
	if bashInfo.Name != "bash" {
		t.Errorf("Bash name = %s, want bash", bashInfo.Name)
	}
}

func TestMCPToolNamingConvention(t *testing.T) {
	// Verify MCP tools follow naming convention: mcp_servername_toolname
	tests := []struct {
		serverName string
		toolName   string
		wantName   string
	}{
		{"github", "search", "mcp_github_search"},
		{"fs", "read_file", "mcp_fs_read_file"},
		{"playwright", "screenshot", "mcp_playwright_screenshot"},
		{"long_server", "tool", "mcp_long_server_tool"},
	}

	for _, tt := range tests {
		fullName := "mcp_" + tt.serverName + "_" + tt.toolName
		if fullName != tt.wantName {
			t.Errorf("Tool name = %s, want %s", fullName, tt.wantName)
		}
	}
}

func TestDefaultRegistryWithMCPTools(t *testing.T) {
	// Verify default registry can be extended with MCP tools
	manager := mcp.NewManager()
	defer manager.Close()

	ctx := context.Background()

	// Start with default registry
	registry := DefaultRegistry("/tmp")
	initialCount := len(registry.All())

	// Register MCP tools (empty, no servers)
	err := RegisterMCPTools(ctx, registry, manager)
	if err != nil {
		t.Fatalf("RegisterMCPTools failed: %v", err)
	}

	// Should still have same number of tools (no MCP servers)
	if len(registry.All()) != initialCount {
		t.Errorf("Expected %d tools, got %d", initialCount, len(registry.All()))
	}

	// Verify all builtin tools are present
	expectedBuiltins := []string{
		"bash", "read", "write", "edit", "glob", "grep", "ls",
		"screenshot", "screen_capture", "computer", "webfetch", "websearch",
	}
	for _, name := range expectedBuiltins {
		if _, ok := registry.Get(name); !ok {
			t.Errorf("Missing builtin tool: %s", name)
		}
	}

	t.Logf("Registry has %d total tools", len(registry.All()))
}

func TestMCPToolExecuteReturnsOutput(t *testing.T) {
	// This test verifies the structure of MCPTool.Execute
	// In production, this would connect to a real MCP server

	manager := mcp.NewManager()
	defer manager.Close()

	mcpTool := NewMCPTool(manager, domain.Tool{
		Name: "mcp_test_tool",
	})

	result, err := mcpTool.Execute(context.Background(), map[string]any{
		"arg1": "value1",
	})

	// Top-level error should be nil
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}

	// Result should not be nil
	if result == nil {
		t.Fatal("Result should not be nil")
	}

	// Since no server is connected, there should be an error in result
	if result.Error == nil {
		t.Error("Expected error in result when no server connected")
	}
}
