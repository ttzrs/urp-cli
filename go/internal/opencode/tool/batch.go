package tool

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/joss/urp/internal/opencode/domain"
)

// Batch executes multiple tool operations
type Batch struct {
	registry *Registry
}

// NewBatch creates a new Batch tool
func NewBatch(registry *Registry) *Batch {
	return &Batch{registry: registry}
}

func (b *Batch) Info() domain.Tool {
	return domain.Tool{
		Name:        "batch",
		Description: "Execute multiple tool operations (sequentially or in parallel)",
		Parameters: domain.JSONSchema{
			"type": "object",
			"properties": map[string]any{
				"operations": map[string]any{
					"type":        "array",
					"description": "Array of operations: [{tool, args}, ...]",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"tool": map[string]any{"type": "string"},
							"args": map[string]any{"type": "object"},
						},
					},
				},
				"parallel": map[string]any{
					"type":        "boolean",
					"description": "Run operations in parallel (default: false)",
				},
			},
			"required": []string{"operations"},
		},
	}
}

// BatchOp represents a single batch operation
type BatchOp struct {
	Tool string         `json:"tool"`
	Args map[string]any `json:"args"`
}

// BatchResult holds the result of a batch operation
type BatchResult struct {
	Tool   string `json:"tool"`
	Output string `json:"output"`
	Error  string `json:"error,omitempty"`
}

func (b *Batch) Execute(ctx context.Context, args map[string]any) (*Result, error) {
	opsRaw, _ := args["operations"].([]any)
	parallel, _ := args["parallel"].(bool)

	if len(opsRaw) == 0 {
		return &Result{Error: fmt.Errorf("at least one operation is required")}, nil
	}

	// Parse operations
	var ops []BatchOp
	for i, raw := range opsRaw {
		op, ok := raw.(map[string]any)
		if !ok {
			return &Result{Error: fmt.Errorf("operation %d: invalid format", i)}, nil
		}

		toolName, _ := op["tool"].(string)
		if toolName == "" {
			return &Result{Error: fmt.Errorf("operation %d: tool name required", i)}, nil
		}

		toolArgs, _ := op["args"].(map[string]any)
		if toolArgs == nil {
			toolArgs = make(map[string]any)
		}

		ops = append(ops, BatchOp{Tool: toolName, Args: toolArgs})
	}

	// Execute operations
	var results []BatchResult

	if parallel {
		results = b.executeParallel(ctx, ops)
	} else {
		results = b.executeSequential(ctx, ops)
	}

	// Format output
	var sb strings.Builder
	successCount := 0
	errorCount := 0

	for i, r := range results {
		if r.Error != "" {
			errorCount++
			sb.WriteString(fmt.Sprintf("%d. [ERROR] %s: %s\n", i+1, r.Tool, r.Error))
		} else {
			successCount++
			sb.WriteString(fmt.Sprintf("%d. [OK] %s\n", i+1, r.Tool))
			if r.Output != "" {
				// Indent output
				lines := strings.Split(r.Output, "\n")
				for _, line := range lines {
					if line != "" {
						sb.WriteString(fmt.Sprintf("   %s\n", line))
					}
				}
			}
		}
	}

	mode := "sequential"
	if parallel {
		mode = "parallel"
	}

	return &Result{
		Title: fmt.Sprintf("Batch: %d/%d succeeded (%s)", successCount, len(ops), mode),
		Output: sb.String(),
		Metadata: map[string]any{
			"total":    len(ops),
			"success":  successCount,
			"errors":   errorCount,
			"parallel": parallel,
			"results":  results,
		},
	}, nil
}

func (b *Batch) executeSequential(ctx context.Context, ops []BatchOp) []BatchResult {
	results := make([]BatchResult, len(ops))

	for i, op := range ops {
		results[i] = b.executeSingle(ctx, op)
	}

	return results
}

func (b *Batch) executeParallel(ctx context.Context, ops []BatchOp) []BatchResult {
	results := make([]BatchResult, len(ops))
	var wg sync.WaitGroup

	for i, op := range ops {
		wg.Add(1)
		go func(idx int, operation BatchOp) {
			defer wg.Done()
			results[idx] = b.executeSingle(ctx, operation)
		}(i, op)
	}

	wg.Wait()
	return results
}

func (b *Batch) executeSingle(ctx context.Context, op BatchOp) BatchResult {
	result := BatchResult{Tool: op.Tool}

	// Find tool
	tool, ok := b.registry.Get(op.Tool)
	if !ok || tool == nil {
		result.Error = fmt.Sprintf("tool not found: %s", op.Tool)
		return result
	}

	// Execute
	toolResult, err := tool.Execute(ctx, op.Args)
	if err != nil {
		result.Error = err.Error()
		return result
	}

	if toolResult.Error != nil {
		result.Error = toolResult.Error.Error()
		return result
	}

	result.Output = toolResult.Output
	return result
}
