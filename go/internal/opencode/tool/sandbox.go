// Package tool provides secure code execution sandbox
// Inspired by Poetiq's sandbox.py with subprocess isolation
package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/joss/urp/internal/opencode/domain"
)

// SandboxConfig configures code execution sandbox
type SandboxConfig struct {
	TimeoutSeconds float64  // Execution timeout (default: 5.0)
	MaxMemoryMB    int      // Memory limit in MB (default: 256)
	AllowedImports []string // Whitelist of allowed imports (empty = all)
	WorkDir        string   // Working directory for execution
}

// DefaultSandboxConfig returns sensible defaults
func DefaultSandboxConfig() SandboxConfig {
	return SandboxConfig{
		TimeoutSeconds: 5.0,
		MaxMemoryMB:    256,
		AllowedImports: nil, // Allow all imports
	}
}

// SandboxResult holds the result of code execution
type SandboxResult struct {
	Success bool            // True if code executed without error
	Output  json.RawMessage // JSON output from code
	Error   string          // Error message if failed
	Stderr  string          // Raw stderr output
	Runtime time.Duration   // Execution time
}

// Sandbox provides isolated code execution
type Sandbox struct {
	config SandboxConfig
}

// NewSandbox creates a new sandbox with default config
func NewSandbox() *Sandbox {
	return &Sandbox{
		config: DefaultSandboxConfig(),
	}
}

// WithConfig sets the sandbox configuration
func (s *Sandbox) WithConfig(config SandboxConfig) *Sandbox {
	s.config = config
	return s
}

// Execute runs Python code in an isolated subprocess
// Input is passed via stdin as JSON, output is read from stdout as JSON
func (s *Sandbox) Execute(ctx context.Context, code string, input any) (*SandboxResult, error) {
	startTime := time.Now()

	// Create temporary directory for isolation
	tempDir, err := os.MkdirTemp("", "urp-sandbox-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Write wrapped script
	script := s.wrapCode(code)
	scriptPath := filepath.Join(tempDir, "script.py")
	if err := os.WriteFile(scriptPath, []byte(script), 0644); err != nil {
		return nil, fmt.Errorf("write script: %w", err)
	}

	// Prepare input JSON
	inputJSON, err := json.Marshal(map[string]any{"input": input})
	if err != nil {
		return nil, fmt.Errorf("marshal input: %w", err)
	}

	// Create context with timeout
	timeout := time.Duration(s.config.TimeoutSeconds * float64(time.Second))
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Execute Python
	cmd := exec.CommandContext(ctx, "python3", scriptPath)
	cmd.Dir = tempDir
	cmd.Env = []string{
		"PYTHONHASHSEED=0", // Deterministic
		"PYTHONDONTWRITEBYTECODE=1",
		"HOME=" + tempDir,  // Isolate home
		"PATH=" + os.Getenv("PATH"),
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdin = bytes.NewReader(inputJSON)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	runtime := time.Since(startTime)

	result := &SandboxResult{
		Runtime: runtime,
		Stderr:  stderr.String(),
	}

	// Check for timeout
	if ctx.Err() == context.DeadlineExceeded {
		result.Success = false
		result.Error = "timeout"
		return result, nil
	}

	// Check for execution error
	if err != nil {
		result.Success = false
		if stderr.Len() > 0 {
			result.Error = truncateSandboxOutput(stderr.String(), 1000)
		} else {
			result.Error = err.Error()
		}
		return result, nil
	}

	// Parse JSON output
	var payload struct {
		OK     bool            `json:"ok"`
		Result json.RawMessage `json:"result"`
		Error  string          `json:"error,omitempty"`
	}

	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("bad-json: %v (output: %s)", err, truncateSandboxOutput(stdout.String(), 200))
		return result, nil
	}

	result.Success = payload.OK
	result.Output = payload.Result
	if !payload.OK && payload.Error != "" {
		result.Error = payload.Error
	}

	return result, nil
}

// wrapCode wraps user code with runtime environment
// Similar to Poetiq's _build_script
func (s *Sandbox) wrapCode(code string) string {
	return fmt.Sprintf(`# URP Sandbox - Generated file
import json
import sys
import traceback

# Try to import numpy (optional)
try:
    import numpy as np
    HAS_NUMPY = True
except ImportError:
    HAS_NUMPY = False

# User code
%s

if __name__ == '__main__':
    try:
        data = json.load(sys.stdin)
        input_val = data.get('input')

        # Convert to numpy if available and it's a list
        if HAS_NUMPY and isinstance(input_val, list):
            input_val = np.array(input_val)

        # Call user's transform function if it exists
        if 'transform' in dir():
            result = transform(input_val)
            if HAS_NUMPY and hasattr(result, 'tolist'):
                result = result.tolist()
        else:
            # No transform function - return None
            result = None

        print(json.dumps({"ok": True, "result": result}))
    except Exception as e:
        print(json.dumps({
            "ok": False,
            "error": str(e),
            "traceback": traceback.format_exc()
        }))
        sys.exit(1)
`, code)
}

// ExecuteGo runs Go code in an isolated subprocess
func (s *Sandbox) ExecuteGo(ctx context.Context, code string, input any) (*SandboxResult, error) {
	startTime := time.Now()

	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "urp-sandbox-go-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Write wrapped script
	script := s.wrapGoCode(code)
	scriptPath := filepath.Join(tempDir, "main.go")
	if err := os.WriteFile(scriptPath, []byte(script), 0644); err != nil {
		return nil, fmt.Errorf("write script: %w", err)
	}

	// Prepare input JSON
	inputJSON, err := json.Marshal(map[string]any{"input": input})
	if err != nil {
		return nil, fmt.Errorf("marshal input: %w", err)
	}

	// Create context with timeout
	timeout := time.Duration(s.config.TimeoutSeconds * float64(time.Second))
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Execute Go
	cmd := exec.CommandContext(ctx, "go", "run", scriptPath)
	cmd.Dir = tempDir
	cmd.Env = append(os.Environ(),
		"GOCACHE="+filepath.Join(tempDir, ".cache"),
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdin = bytes.NewReader(inputJSON)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	runtime := time.Since(startTime)

	result := &SandboxResult{
		Runtime: runtime,
		Stderr:  stderr.String(),
	}

	if ctx.Err() == context.DeadlineExceeded {
		result.Success = false
		result.Error = "timeout"
		return result, nil
	}

	if err != nil {
		result.Success = false
		if stderr.Len() > 0 {
			result.Error = truncateSandboxOutput(stderr.String(), 1000)
		} else {
			result.Error = err.Error()
		}
		return result, nil
	}

	// Parse JSON output
	var payload struct {
		OK     bool            `json:"ok"`
		Result json.RawMessage `json:"result"`
		Error  string          `json:"error,omitempty"`
	}

	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("bad-json: %v", err)
		return result, nil
	}

	result.Success = payload.OK
	result.Output = payload.Result
	if !payload.OK && payload.Error != "" {
		result.Error = payload.Error
	}

	return result, nil
}

// wrapGoCode wraps user Go code with runtime environment
func (s *Sandbox) wrapGoCode(code string) string {
	// Check if code already has package declaration
	if strings.Contains(code, "package ") {
		return code
	}

	return fmt.Sprintf(`package main

import (
	"encoding/json"
	"fmt"
	"os"
)

%s

func main() {
	var input struct {
		Input interface{} ` + "`json:\"input\"`" + `
	}

	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		fmt.Printf("{\"ok\":false,\"error\":\"%%v\"}\n", err)
		os.Exit(1)
	}

	// Try to call user's transform function if defined
	// Note: User code should define their own main logic

	result := input.Input
	output, _ := json.Marshal(map[string]interface{}{
		"ok":     true,
		"result": result,
	})
	fmt.Println(string(output))
}
`, code)
}

// truncateSandboxOutput truncates long sandbox output
func truncateSandboxOutput(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// SandboxTool implements the sandbox tool for safe code execution
type SandboxTool struct {
	sandbox *Sandbox
	workDir string
}

// NewSandboxTool creates a new sandbox tool
func NewSandboxTool(workDir string) *SandboxTool {
	return &SandboxTool{
		sandbox: NewSandbox(),
		workDir: workDir,
	}
}

// WithConfig sets the sandbox configuration
func (t *SandboxTool) WithConfig(config SandboxConfig) *SandboxTool {
	t.sandbox.WithConfig(config)
	return t
}

// Info returns the tool definition
func (t *SandboxTool) Info() domain.Tool {
	return domain.Tool{
		Name:        "sandbox",
		Description: "Execute Python code safely in an isolated sandbox with timeout",
		Parameters: domain.JSONSchema{
			"type": "object",
			"properties": map[string]any{
				"code": map[string]any{
					"type":        "string",
					"description": "Python code to execute. Should define a transform(input) function.",
				},
				"input": map[string]any{
					"type":        "any",
					"description": "Input data to pass to the code (as JSON)",
				},
				"language": map[string]any{
					"type":        "string",
					"description": "Language: python or go (default: python)",
					"enum":        []string{"python", "go"},
				},
				"timeout": map[string]any{
					"type":        "number",
					"description": "Timeout in seconds (default: 5.0)",
				},
			},
			"required": []string{"code"},
		},
	}
}

// Execute runs code in the sandbox
func (t *SandboxTool) Execute(ctx context.Context, args map[string]any) (*Result, error) {
	code, _ := args["code"].(string)
	if code == "" {
		return &Result{Error: fmt.Errorf("code is required")}, nil
	}

	input := args["input"]
	language, _ := args["language"].(string)
	if language == "" {
		language = "python"
	}

	// Override timeout if specified
	if timeout, ok := args["timeout"].(float64); ok && timeout > 0 {
		config := t.sandbox.config
		config.TimeoutSeconds = timeout
		t.sandbox.WithConfig(config)
	}

	var result *SandboxResult
	var err error

	switch language {
	case "go":
		result, err = t.sandbox.ExecuteGo(ctx, code, input)
	default:
		result, err = t.sandbox.Execute(ctx, code, input)
	}

	if err != nil {
		return &Result{
			Title: "Sandbox execution failed",
			Error: err,
		}, nil
	}

	if !result.Success {
		return &Result{
			Title:  "Code execution failed",
			Output: result.Error,
			Metadata: map[string]any{
				"success":    false,
				"runtime_ms": result.Runtime.Milliseconds(),
				"stderr":     result.Stderr,
			},
		}, nil
	}

	// Format output
	var outputStr string
	if result.Output != nil {
		outputStr = string(result.Output)
	}

	return &Result{
		Title:  "Code executed successfully",
		Output: outputStr,
		Metadata: map[string]any{
			"success":    true,
			"runtime_ms": result.Runtime.Milliseconds(),
		},
	}, nil
}
