// Package exec tests verify command execution and mocking.
package exec

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOSRunner_Run tests basic command execution.
func TestOSRunner_Run(t *testing.T) {
	runner := NewOSRunner()
	ctx := context.Background()

	// Test successful command
	out, err := runner.Run(ctx, "echo", "hello")
	require.NoError(t, err)
	assert.Equal(t, "hello\n", string(out))
}

// TestOSRunner_RunInDir tests execution in specific directory.
func TestOSRunner_RunInDir(t *testing.T) {
	runner := NewOSRunner()
	ctx := context.Background()

	// Run pwd in /tmp
	out, err := runner.RunInDir(ctx, "/tmp", "pwd")
	require.NoError(t, err)
	assert.Contains(t, string(out), "tmp")
}

// TestOSRunner_RunWithStdin tests stdin input.
func TestOSRunner_RunWithStdin(t *testing.T) {
	runner := NewOSRunner()
	ctx := context.Background()

	input := strings.NewReader("hello world")
	out, err := runner.RunWithStdin(ctx, input, "cat")
	require.NoError(t, err)
	assert.Equal(t, "hello world", string(out))
}

// TestOSRunner_RunSeparate tests stdout/stderr separation.
func TestOSRunner_RunSeparate(t *testing.T) {
	runner := NewOSRunner()
	ctx := context.Background()

	// Command that writes to stdout
	stdout, stderr, err := runner.RunSeparate(ctx, "echo", "stdout")
	require.NoError(t, err)
	assert.Equal(t, "stdout\n", string(stdout))
	assert.Empty(t, stderr)

	// Command that writes to stderr (using sh -c)
	stdout, stderr, err = runner.RunSeparate(ctx, "sh", "-c", "echo stderr >&2")
	require.NoError(t, err)
	assert.Empty(t, stdout)
	assert.Equal(t, "stderr\n", string(stderr))
}

// TestOSRunner_RunError tests command failure.
func TestOSRunner_RunError(t *testing.T) {
	runner := NewOSRunner()
	ctx := context.Background()

	_, err := runner.Run(ctx, "false") // always exits with 1
	require.Error(t, err)
}

// TestOSRunner_RunNotFound tests missing command.
func TestOSRunner_RunNotFound(t *testing.T) {
	runner := NewOSRunner()
	ctx := context.Background()

	_, err := runner.Run(ctx, "nonexistent-command-xyz")
	require.Error(t, err)
}

// TestOSRunner_ContextCancellation tests context timeout.
func TestOSRunner_ContextCancellation(t *testing.T) {
	runner := NewOSRunner()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// sleep longer than timeout
	_, err := runner.Run(ctx, "sleep", "5")
	require.Error(t, err)
	assert.True(t, errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "killed"))
}

// TestOSRunner_WithEnv tests environment variable injection.
func TestOSRunner_WithEnv(t *testing.T) {
	runner := &OSRunner{
		Env: []string{"TEST_VAR=hello_world"},
	}
	ctx := context.Background()

	out, err := runner.Run(ctx, "sh", "-c", "echo $TEST_VAR")
	require.NoError(t, err)
	assert.Equal(t, "hello_world\n", string(out))
}

// TestOSRunner_Start tests background execution.
func TestOSRunner_Start(t *testing.T) {
	runner := NewOSRunner()
	ctx := context.Background()

	cmd, err := runner.Start(ctx, "sleep", "0.1")
	require.NoError(t, err)
	require.NotNil(t, cmd)

	// Wait for completion
	err = cmd.Wait()
	require.NoError(t, err)
}

// ─────────────────────────────────────────────────────────────────────────────
// MockRunner Tests
// ─────────────────────────────────────────────────────────────────────────────

// TestMockRunner_Basic tests mock response.
func TestMockRunner_Basic(t *testing.T) {
	mock := NewMockRunner()
	mock.AddResponse("git", MockResponse{
		Stdout: []byte("On branch main\n"),
		Stderr: nil,
		Err:    nil,
	})

	ctx := context.Background()
	out, err := mock.Run(ctx, "git", "status")

	require.NoError(t, err)
	assert.Equal(t, "On branch main\n", string(out))

	// Verify call was recorded
	require.Len(t, mock.Calls, 1)
	assert.Equal(t, "git", mock.Calls[0].Name)
	assert.Equal(t, []string{"status"}, mock.Calls[0].Args)
}

// TestMockRunner_Error tests error simulation.
func TestMockRunner_Error(t *testing.T) {
	mock := NewMockRunner()
	expectedErr := errors.New("command failed")
	mock.AddResponse("failing-cmd", MockResponse{
		Stderr: []byte("error output"),
		Err:    expectedErr,
	})

	ctx := context.Background()
	out, err := mock.Run(ctx, "failing-cmd", "arg1")

	require.ErrorIs(t, err, expectedErr)
	assert.Contains(t, string(out), "error output")
}

// TestMockRunner_RunInDir tests directory tracking.
func TestMockRunner_RunInDir(t *testing.T) {
	mock := NewMockRunner()
	mock.AddResponse("ls", MockResponse{
		Stdout: []byte("file1\nfile2\n"),
	})

	ctx := context.Background()
	_, _ = mock.RunInDir(ctx, "/some/dir", "ls", "-la")

	require.Len(t, mock.Calls, 1)
	assert.Equal(t, "/some/dir", mock.Calls[0].Dir)
}

// TestMockRunner_RunSeparate tests stdout/stderr separation in mock.
func TestMockRunner_RunSeparate(t *testing.T) {
	mock := NewMockRunner()
	mock.AddResponse("cmd", MockResponse{
		Stdout: []byte("stdout content"),
		Stderr: []byte("stderr content"),
	})

	ctx := context.Background()
	stdout, stderr, err := mock.RunSeparate(ctx, "cmd")

	require.NoError(t, err)
	assert.Equal(t, "stdout content", string(stdout))
	assert.Equal(t, "stderr content", string(stderr))
}

// TestMockRunner_MultipleCalls tests call recording.
func TestMockRunner_MultipleCalls(t *testing.T) {
	mock := NewMockRunner()
	ctx := context.Background()

	_, _ = mock.Run(ctx, "cmd1", "a", "b")
	_, _ = mock.Run(ctx, "cmd2", "x")
	_, _ = mock.RunInDir(ctx, "/dir", "cmd3")

	require.Len(t, mock.Calls, 3)
	assert.Equal(t, "cmd1", mock.Calls[0].Name)
	assert.Equal(t, "cmd2", mock.Calls[1].Name)
	assert.Equal(t, "cmd3", mock.Calls[2].Name)
	assert.Equal(t, "/dir", mock.Calls[2].Dir)
}

// TestMockRunner_DefaultResponse tests missing response returns empty.
func TestMockRunner_DefaultResponse(t *testing.T) {
	mock := NewMockRunner()
	ctx := context.Background()

	out, err := mock.Run(ctx, "unknown-cmd")

	require.NoError(t, err)
	assert.Empty(t, out)
}

// TestMockRunner_Start tests start without error.
func TestMockRunner_Start(t *testing.T) {
	mock := NewMockRunner()
	ctx := context.Background()

	cmd, err := mock.Start(ctx, "background-cmd")

	require.NoError(t, err)
	require.NotNil(t, cmd)
	require.Len(t, mock.Calls, 1)
}

// ─────────────────────────────────────────────────────────────────────────────
// Default Runner Tests
// ─────────────────────────────────────────────────────────────────────────────

// TestDefaultRunner tests the package-level functions.
func TestDefaultRunner(t *testing.T) {
	ctx := context.Background()

	// Use default runner (OSRunner)
	out, err := Run(ctx, "echo", "test")
	require.NoError(t, err)
	assert.Equal(t, "test\n", string(out))
}

// TestDefaultRunner_RunInDir tests package-level RunInDir.
func TestDefaultRunner_RunInDir(t *testing.T) {
	ctx := context.Background()

	out, err := RunInDir(ctx, "/tmp", "pwd")
	require.NoError(t, err)
	assert.Contains(t, string(out), "tmp")
}

// TestDefaultRunner_Override tests replacing default runner.
func TestDefaultRunner_Override(t *testing.T) {
	// Save original
	original := Default
	defer func() { Default = original }()

	// Replace with mock
	mock := NewMockRunner()
	mock.AddResponse("custom", MockResponse{
		Stdout: []byte("mocked output"),
	})
	Default = mock

	ctx := context.Background()
	out, err := Run(ctx, "custom")

	require.NoError(t, err)
	assert.Equal(t, "mocked output", string(out))
}

// ─────────────────────────────────────────────────────────────────────────────
// Interface Compliance Tests
// ─────────────────────────────────────────────────────────────────────────────

// TestRunnerInterface verifies both implementations satisfy Runner.
func TestRunnerInterface(t *testing.T) {
	var _ Runner = (*OSRunner)(nil)
	var _ Runner = (*MockRunner)(nil)
}

// ─────────────────────────────────────────────────────────────────────────────
// Table-Driven Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestOSRunner_Commands(t *testing.T) {
	runner := NewOSRunner()
	ctx := context.Background()

	tests := []struct {
		name    string
		cmd     string
		args    []string
		wantErr bool
		check   func(t *testing.T, out []byte)
	}{
		{
			name: "echo",
			cmd:  "echo",
			args: []string{"hello"},
			check: func(t *testing.T, out []byte) {
				assert.Equal(t, "hello\n", string(out))
			},
		},
		{
			name: "true",
			cmd:  "true",
			args: nil,
			check: func(t *testing.T, out []byte) {
				assert.Empty(t, out)
			},
		},
		{
			name:    "false",
			cmd:     "false",
			args:    nil,
			wantErr: true,
		},
		{
			name: "multi args",
			cmd:  "echo",
			args: []string{"a", "b", "c"},
			check: func(t *testing.T, out []byte) {
				assert.Equal(t, "a b c\n", string(out))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := runner.Run(ctx, tt.cmd, tt.args...)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			if tt.check != nil {
				tt.check(t, out)
			}
		})
	}
}
