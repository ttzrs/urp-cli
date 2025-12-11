package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestDefaultSandboxConfig(t *testing.T) {
	config := DefaultSandboxConfig()

	if config.TimeoutSeconds != 5.0 {
		t.Errorf("TimeoutSeconds = %f, want 5.0", config.TimeoutSeconds)
	}
	if config.MaxMemoryMB != 256 {
		t.Errorf("MaxMemoryMB = %d, want 256", config.MaxMemoryMB)
	}
}

func TestNewSandbox(t *testing.T) {
	sandbox := NewSandbox()
	if sandbox == nil {
		t.Fatal("NewSandbox returned nil")
	}
}

func TestSandbox_WithConfig(t *testing.T) {
	config := SandboxConfig{
		TimeoutSeconds: 10.0,
		MaxMemoryMB:    512,
	}

	sandbox := NewSandbox().WithConfig(config)

	if sandbox.config.TimeoutSeconds != 10.0 {
		t.Errorf("TimeoutSeconds = %f, want 10.0", sandbox.config.TimeoutSeconds)
	}
}

func TestSandbox_Execute_SimplePython(t *testing.T) {
	sandbox := NewSandbox()

	code := `
def transform(x):
    return x * 2
`
	result, err := sandbox.Execute(context.Background(), code, 5)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got error: %s", result.Error)
	}

	var output int
	if err := json.Unmarshal(result.Output, &output); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}
	if output != 10 {
		t.Errorf("output = %d, want 10", output)
	}
}

func TestSandbox_Execute_ListTransform(t *testing.T) {
	sandbox := NewSandbox()

	// Pure Python list reversal (no numpy dependency)
	code := `
def transform(grid):
    # Reverse both rows and columns (like np.flip)
    return [row[::-1] for row in grid[::-1]]
`
	input := [][]int{{1, 2}, {3, 4}}
	result, err := sandbox.Execute(context.Background(), code, input)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got error: %s", result.Error)
	}

	var output [][]int
	if err := json.Unmarshal(result.Output, &output); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}
	// Flip reverses both axes: [[1,2],[3,4]] -> [[4,3],[2,1]]
	expected := [][]int{{4, 3}, {2, 1}}
	if output[0][0] != expected[0][0] {
		t.Errorf("output = %v, want %v", output, expected)
	}
}

func TestSandbox_Execute_Error(t *testing.T) {
	sandbox := NewSandbox()

	code := `
def transform(x):
    return x / 0  # Division by zero
`
	result, err := sandbox.Execute(context.Background(), code, 5)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected failure for division by zero")
	}
	if result.Error == "" {
		t.Error("expected error message")
	}
}

func TestSandbox_Execute_Timeout(t *testing.T) {
	sandbox := NewSandbox().WithConfig(SandboxConfig{
		TimeoutSeconds: 0.5,
	})

	code := `
import time

def transform(x):
    time.sleep(5)
    return x
`
	start := time.Now()
	result, err := sandbox.Execute(context.Background(), code, 5)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected timeout")
	}
	if result.Error != "timeout" {
		t.Errorf("error = %s, want timeout", result.Error)
	}
	// Should timeout around 0.5s, not wait full 5s
	if duration > 2*time.Second {
		t.Errorf("took %v, expected ~0.5s timeout", duration)
	}
}

func TestSandbox_Execute_SyntaxError(t *testing.T) {
	sandbox := NewSandbox()

	code := `
def transform(x)  # Missing colon
    return x
`
	result, err := sandbox.Execute(context.Background(), code, 5)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected failure for syntax error")
	}
	if !strings.Contains(result.Error, "SyntaxError") && !strings.Contains(result.Stderr, "SyntaxError") {
		t.Errorf("expected SyntaxError in error: %s / stderr: %s", result.Error, result.Stderr)
	}
}

func TestSandbox_Execute_NoTransform(t *testing.T) {
	sandbox := NewSandbox()

	code := `
# No transform function defined
x = 42
`
	result, err := sandbox.Execute(context.Background(), code, 5)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should succeed but with null result
	if !result.Success {
		t.Errorf("expected success, got error: %s", result.Error)
	}
}

func TestSandbox_wrapCode(t *testing.T) {
	sandbox := NewSandbox()

	code := `
def transform(x):
    return x * 2
`
	wrapped := sandbox.wrapCode(code)

	if !strings.Contains(wrapped, "import json") {
		t.Error("wrapped code should import json")
	}
	if !strings.Contains(wrapped, "transform(input_val)") {
		t.Error("wrapped code should call transform")
	}
	if !strings.Contains(wrapped, code) {
		t.Error("wrapped code should contain user code")
	}
}

func TestTruncateSandboxOutput(t *testing.T) {
	tests := []struct {
		input  string
		maxLen int
		want   string
	}{
		{"short", 10, "short"},
		{"longer string", 10, "longer ..."},
		{"", 5, ""},
	}

	for _, tt := range tests {
		got := truncateSandboxOutput(tt.input, tt.maxLen)
		if got != tt.want {
			t.Errorf("truncateSandboxOutput(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
		}
	}
}

func TestSandboxResult_Fields(t *testing.T) {
	result := SandboxResult{
		Success: true,
		Output:  json.RawMessage(`42`),
		Runtime: time.Second,
	}

	if !result.Success {
		t.Error("Success should be true")
	}
	if string(result.Output) != "42" {
		t.Errorf("Output = %s, want 42", result.Output)
	}
	if result.Runtime != time.Second {
		t.Errorf("Runtime = %v, want 1s", result.Runtime)
	}
}

func TestNewSandboxTool(t *testing.T) {
	tool := NewSandboxTool("/tmp/work")

	if tool == nil {
		t.Fatal("NewSandboxTool returned nil")
	}

	info := tool.Info()
	if info.Name != "sandbox" {
		t.Errorf("tool name = %s, want sandbox", info.Name)
	}
}

func TestSandboxTool_Info(t *testing.T) {
	tool := NewSandboxTool("/tmp")
	info := tool.Info()

	if info.Name != "sandbox" {
		t.Errorf("Name = %s, want sandbox", info.Name)
	}
	if info.Description == "" {
		t.Error("Description should not be empty")
	}

	params, ok := info.Parameters["properties"].(map[string]any)
	if !ok {
		t.Fatal("parameters should have properties")
	}

	requiredFields := []string{"code"}
	required, _ := info.Parameters["required"].([]string)
	for _, rf := range requiredFields {
		found := false
		for _, r := range required {
			if r == rf {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("required field %s not found", rf)
		}
	}

	// Check optional fields
	optionalFields := []string{"input", "language", "timeout"}
	for _, of := range optionalFields {
		if _, exists := params[of]; !exists {
			t.Errorf("optional field %s not found in properties", of)
		}
	}
}

func TestSandboxTool_Execute_NoCode(t *testing.T) {
	tool := NewSandboxTool("/tmp")

	result, err := tool.Execute(context.Background(), map[string]any{})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error == nil {
		t.Error("should return error when code is missing")
	}
}

func TestSandboxTool_Execute_Simple(t *testing.T) {
	tool := NewSandboxTool("/tmp")

	result, err := tool.Execute(context.Background(), map[string]any{
		"code":  "def transform(x): return x + 1",
		"input": 5,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error != nil {
		t.Errorf("unexpected result error: %v", result.Error)
	}
	if result.Output != "6" {
		t.Errorf("output = %s, want 6", result.Output)
	}
}

func TestSandboxTool_Execute_WithTimeout(t *testing.T) {
	tool := NewSandboxTool("/tmp")

	result, err := tool.Execute(context.Background(), map[string]any{
		"code":    "import time\ndef transform(x): time.sleep(5); return x",
		"input":   5,
		"timeout": 0.5,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should fail with timeout
	if result.Error == nil && result.Output == "5" {
		t.Error("expected timeout")
	}
}
