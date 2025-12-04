package tool

import (
	"context"

	"github.com/joss/urp/internal/opencode/domain"
	"github.com/joss/urp/internal/opencode/mcp"
)

// MCPTool wraps an MCP tool to implement Executor interface
type MCPTool struct {
	manager  *mcp.Manager
	toolInfo domain.Tool
}

func NewMCPTool(manager *mcp.Manager, tool domain.Tool) *MCPTool {
	return &MCPTool{
		manager:  manager,
		toolInfo: tool,
	}
}

func (m *MCPTool) Info() domain.Tool {
	return m.toolInfo
}

func (m *MCPTool) Execute(ctx context.Context, args map[string]any) (*Result, error) {
	output, err := m.manager.CallTool(ctx, m.toolInfo.Name, args)
	if err != nil {
		return &Result{
			Output: err.Error(),
			Error:  err,
		}, nil
	}

	return &Result{
		Title:  m.toolInfo.Name,
		Output: output,
	}, nil
}

var _ Executor = (*MCPTool)(nil)

// RegisterMCPTools registers all tools from MCP servers into the registry
func RegisterMCPTools(ctx context.Context, registry *Registry, manager *mcp.Manager) error {
	tools, err := manager.GetTools(ctx)
	if err != nil {
		return err
	}

	for _, tool := range tools {
		registry.Register(NewMCPTool(manager, tool))
	}

	return nil
}
