package tool

import (
	"context"
	"fmt"
	"strings"

	"github.com/joss/urp/internal/opencode/domain"
)

// Invalid handles malformed tool calls and provides helpful error messages
type Invalid struct {
	registry *Registry
}

// NewInvalid creates a new Invalid tool handler
func NewInvalid(registry *Registry) *Invalid {
	return &Invalid{registry: registry}
}

func (i *Invalid) Info() domain.Tool {
	return domain.Tool{
		Name:        "invalid",
		Description: "Handles malformed tool calls (internal use)",
		Parameters: domain.JSONSchema{
			"type": "object",
			"properties": map[string]any{
				"tool": map[string]any{
					"type":        "string",
					"description": "The tool that was called incorrectly",
				},
				"error": map[string]any{
					"type":        "string",
					"description": "The error message from the failed call",
				},
				"attempted_args": map[string]any{
					"type":        "object",
					"description": "The arguments that were passed",
				},
			},
			"required": []string{"tool", "error"},
		},
	}
}

func (i *Invalid) Execute(ctx context.Context, args map[string]any) (*Result, error) {
	toolName, _ := args["tool"].(string)
	errorMsg, _ := args["error"].(string)
	attemptedArgs, _ := args["attempted_args"].(map[string]any)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Tool '%s' was called incorrectly.\n\n", toolName))
	sb.WriteString(fmt.Sprintf("Error: %s\n\n", errorMsg))

	// Try to provide helpful suggestions
	if i.registry != nil {
		tool, ok := i.registry.Get(toolName)
		if ok && tool != nil {
			// Show expected parameters
			info := tool.Info()
			sb.WriteString("Expected parameters:\n")
			if props, ok := info.Parameters["properties"].(map[string]any); ok {
				for name, schema := range props {
					if schemaMap, ok := schema.(map[string]any); ok {
						typeStr, _ := schemaMap["type"].(string)
						desc, _ := schemaMap["description"].(string)
						sb.WriteString(fmt.Sprintf("  - %s (%s): %s\n", name, typeStr, desc))
					}
				}
			}
			if required, ok := info.Parameters["required"].([]string); ok {
				sb.WriteString(fmt.Sprintf("\nRequired: %s\n", strings.Join(required, ", ")))
			}
		} else {
			// Tool not found - suggest similar tools
			sb.WriteString("Available tools:\n")
			for _, t := range i.registry.All() {
				if strings.Contains(strings.ToLower(t.Name), strings.ToLower(toolName)) ||
					strings.Contains(strings.ToLower(toolName), strings.ToLower(t.Name)) {
					sb.WriteString(fmt.Sprintf("  - %s: %s\n", t.Name, t.Description))
				}
			}
		}
	}

	// Show what was attempted
	if len(attemptedArgs) > 0 {
		sb.WriteString("\nAttempted arguments:\n")
		for k, v := range attemptedArgs {
			sb.WriteString(fmt.Sprintf("  %s: %v\n", k, v))
		}
	}

	sb.WriteString("\nPlease check the tool's parameters and try again.")

	return &Result{
		Title:  fmt.Sprintf("Invalid tool call: %s", toolName),
		Output: sb.String(),
		Metadata: map[string]any{
			"tool":           toolName,
			"error":          errorMsg,
			"attempted_args": attemptedArgs,
		},
	}, nil
}

// HandleInvalidCall is a helper to create an invalid tool call result
func HandleInvalidCall(toolName, errorMsg string, args map[string]any) *Result {
	return &Result{
		Title: fmt.Sprintf("Invalid tool call: %s", toolName),
		Output: fmt.Sprintf("Tool '%s' was called incorrectly: %s\n\nPlease check the tool's parameters and try again.",
			toolName, errorMsg),
		Metadata: map[string]any{
			"tool":           toolName,
			"error":          errorMsg,
			"attempted_args": args,
		},
	}
}
