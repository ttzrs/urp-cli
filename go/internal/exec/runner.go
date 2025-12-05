// Package exec provides a testable command execution abstraction.
// Eliminates 80+ direct exec.Command calls, enabling DI and mocking.
package exec

import (
	"bytes"
	"context"
	"io"
	"os"
	osexec "os/exec"
)

// Runner defines the interface for executing external commands.
// Inject this instead of calling exec.Command directly.
type Runner interface {
	// Run executes a command and returns combined stdout/stderr.
	Run(ctx context.Context, name string, args ...string) ([]byte, error)

	// RunInDir executes a command in a specific directory.
	RunInDir(ctx context.Context, dir, name string, args ...string) ([]byte, error)

	// RunWithStdin executes a command with stdin input.
	RunWithStdin(ctx context.Context, stdin io.Reader, name string, args ...string) ([]byte, error)

	// RunSeparate executes and returns stdout and stderr separately.
	RunSeparate(ctx context.Context, name string, args ...string) (stdout, stderr []byte, err error)

	// Start begins a command without waiting for completion.
	Start(ctx context.Context, name string, args ...string) (*osexec.Cmd, error)
}

// OSRunner implements Runner using os/exec.
type OSRunner struct {
	// Env overrides environment variables (nil = inherit from parent)
	Env []string
}

// NewOSRunner creates a new OS-based command runner.
func NewOSRunner() *OSRunner {
	return &OSRunner{}
}

// Run executes a command and returns combined output.
func (r *OSRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := osexec.CommandContext(ctx, name, args...)
	if r.Env != nil {
		cmd.Env = r.Env
	}
	return cmd.CombinedOutput()
}

// RunInDir executes a command in a specific directory.
func (r *OSRunner) RunInDir(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	cmd := osexec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	if r.Env != nil {
		cmd.Env = r.Env
	}
	return cmd.CombinedOutput()
}

// RunWithStdin executes a command with stdin input.
func (r *OSRunner) RunWithStdin(ctx context.Context, stdin io.Reader, name string, args ...string) ([]byte, error) {
	cmd := osexec.CommandContext(ctx, name, args...)
	cmd.Stdin = stdin
	if r.Env != nil {
		cmd.Env = r.Env
	}
	return cmd.CombinedOutput()
}

// RunSeparate executes and returns stdout and stderr separately.
func (r *OSRunner) RunSeparate(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	cmd := osexec.CommandContext(ctx, name, args...)
	if r.Env != nil {
		cmd.Env = r.Env
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

// Start begins a command without waiting.
func (r *OSRunner) Start(ctx context.Context, name string, args ...string) (*osexec.Cmd, error) {
	cmd := osexec.CommandContext(ctx, name, args...)
	if r.Env != nil {
		cmd.Env = r.Env
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Start()
	return cmd, err
}

// MockRunner implements Runner for testing.
type MockRunner struct {
	// Calls records all command invocations
	Calls []MockCall

	// Responses maps "name args..." to response
	Responses map[string]MockResponse
}

// MockCall records a single command invocation.
type MockCall struct {
	Name string
	Args []string
	Dir  string
}

// MockResponse defines the response for a mocked command.
type MockResponse struct {
	Stdout []byte
	Stderr []byte
	Err    error
}

// NewMockRunner creates a new mock runner.
func NewMockRunner() *MockRunner {
	return &MockRunner{
		Responses: make(map[string]MockResponse),
	}
}

// AddResponse sets the response for a command pattern.
func (m *MockRunner) AddResponse(name string, resp MockResponse) {
	m.Responses[name] = resp
}

func (m *MockRunner) record(name string, args []string, dir string) {
	m.Calls = append(m.Calls, MockCall{Name: name, Args: args, Dir: dir})
}

func (m *MockRunner) getResponse(name string) MockResponse {
	if resp, ok := m.Responses[name]; ok {
		return resp
	}
	return MockResponse{}
}

func (m *MockRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	m.record(name, args, "")
	resp := m.getResponse(name)
	out := append(resp.Stdout, resp.Stderr...)
	return out, resp.Err
}

func (m *MockRunner) RunInDir(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	m.record(name, args, dir)
	resp := m.getResponse(name)
	out := append(resp.Stdout, resp.Stderr...)
	return out, resp.Err
}

func (m *MockRunner) RunWithStdin(ctx context.Context, stdin io.Reader, name string, args ...string) ([]byte, error) {
	m.record(name, args, "")
	resp := m.getResponse(name)
	out := append(resp.Stdout, resp.Stderr...)
	return out, resp.Err
}

func (m *MockRunner) RunSeparate(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	m.record(name, args, "")
	resp := m.getResponse(name)
	return resp.Stdout, resp.Stderr, resp.Err
}

func (m *MockRunner) Start(ctx context.Context, name string, args ...string) (*osexec.Cmd, error) {
	m.record(name, args, "")
	resp := m.getResponse(name)
	// Return a dummy cmd that does nothing
	cmd := osexec.Command("true")
	return cmd, resp.Err
}

// Default is the default runner used by helper functions.
var Default Runner = NewOSRunner()

// Run executes using the default runner.
func Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return Default.Run(ctx, name, args...)
}

// RunInDir executes in directory using the default runner.
func RunInDir(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	return Default.RunInDir(ctx, dir, name, args...)
}
