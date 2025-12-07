package command

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─────────────────────────────────────────────────────────────────────────────
// Mock Agent for Testing
// ─────────────────────────────────────────────────────────────────────────────

type MockAgent struct {
	model      string
	runCalled  int
	lastPrompt string
	runError   error
}

func NewMockAgent() *MockAgent {
	return &MockAgent{model: "claude-3-sonnet"}
}

func (m *MockAgent) Run(ctx context.Context, prompt string) error {
	m.runCalled++
	m.lastPrompt = prompt
	return m.runError
}

func (m *MockAgent) Model() string {
	return m.model
}

func (m *MockAgent) SetModel(modelID string) {
	m.model = modelID
}

// Verify MockAgent implements AgentInterface
var _ AgentInterface = (*MockAgent)(nil)

// ─────────────────────────────────────────────────────────────────────────────
// Parse Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestParse(t *testing.T) {
	tests := []struct {
		input        string
		expectedName string
		expectedArgs string
		expectedOk   bool
	}{
		{"/help", "help", "", true},
		{"/model claude-3", "model", "claude-3", true},
		{"/review path/to/file", "review", "path/to/file", true},
		{"/INIT", "init", "", true}, // Case insensitive
		{"not a command", "", "", false},
		{"help", "", "", false},
		{"/", "", "", true}, // Edge case: just slash
		{"/cmd arg1 arg2 arg3", "cmd", "arg1 arg2 arg3", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			name, args, ok := Parse(tt.input)
			assert.Equal(t, tt.expectedOk, ok)
			if ok {
				assert.Equal(t, tt.expectedName, name)
				assert.Equal(t, tt.expectedArgs, args)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Registry Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	require.NotNil(t, r)
	assert.Empty(t, r.List())
}

func TestRegistry_Register(t *testing.T) {
	r := NewRegistry()
	cmd := &ClearCommand{}

	r.Register(cmd)

	retrieved, ok := r.Get("clear")
	assert.True(t, ok)
	assert.Equal(t, cmd, retrieved)
}

func TestRegistry_Get_NotFound(t *testing.T) {
	r := NewRegistry()

	_, ok := r.Get("nonexistent")
	assert.False(t, ok)
}

func TestRegistry_List(t *testing.T) {
	r := NewRegistry()
	r.Register(&ClearCommand{})
	r.Register(&InitCommand{})

	list := r.List()
	assert.Len(t, list, 2)
}

func TestDefaultRegistry(t *testing.T) {
	r := DefaultRegistry()

	// Should have all built-in commands
	expectedCmds := []string{"help", "init", "review", "compact", "model", "clear"}
	for _, name := range expectedCmds {
		t.Run("has_"+name, func(t *testing.T) {
			cmd, ok := r.Get(name)
			assert.True(t, ok, "missing command: %s", name)
			assert.NotEmpty(t, cmd.Name())
			assert.NotEmpty(t, cmd.Description())
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Command Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestHelpCommand(t *testing.T) {
	r := NewRegistry()
	r.Register(&ClearCommand{})
	r.Register(&InitCommand{})

	cmd := &HelpCommand{registry: r}

	assert.Equal(t, "help", cmd.Name())
	assert.NotEmpty(t, cmd.Description())

	// Execute should not error
	err := cmd.Execute(context.Background(), "", &Session{})
	assert.NoError(t, err)
}

func TestInitCommand(t *testing.T) {
	cmd := &InitCommand{}

	assert.Equal(t, "init", cmd.Name())
	assert.NotEmpty(t, cmd.Description())

	// With nil agent
	err := cmd.Execute(context.Background(), "", &Session{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "agent not available")

	// With mock agent
	agent := NewMockAgent()
	sess := &Session{Agent: agent}

	err = cmd.Execute(context.Background(), "", sess)
	assert.NoError(t, err)
	assert.Equal(t, 1, agent.runCalled)
	assert.Contains(t, agent.lastPrompt, "CLAUDE.md")
}

func TestReviewCommand(t *testing.T) {
	cmd := &ReviewCommand{}

	assert.Equal(t, "review", cmd.Name())
	assert.NotEmpty(t, cmd.Description())

	// With nil agent
	err := cmd.Execute(context.Background(), "", &Session{})
	assert.Error(t, err)

	// With mock agent - default target
	agent := NewMockAgent()
	sess := &Session{Agent: agent}

	err = cmd.Execute(context.Background(), "", sess)
	assert.NoError(t, err)
	assert.Contains(t, agent.lastPrompt, "uncommitted changes")

	// With specific target
	err = cmd.Execute(context.Background(), "src/main.go", sess)
	assert.NoError(t, err)
	assert.Contains(t, agent.lastPrompt, "src/main.go")
}

func TestCompactCommand(t *testing.T) {
	cmd := &CompactCommand{}

	assert.Equal(t, "compact", cmd.Name())
	assert.NotEmpty(t, cmd.Description())

	// With nil agent
	err := cmd.Execute(context.Background(), "", &Session{})
	assert.Error(t, err)

	// With mock agent
	agent := NewMockAgent()
	sess := &Session{Agent: agent}

	err = cmd.Execute(context.Background(), "", sess)
	assert.NoError(t, err)
	assert.Contains(t, agent.lastPrompt, "Summarize")
}

func TestModelCommand(t *testing.T) {
	cmd := &ModelCommand{}

	assert.Equal(t, "model", cmd.Name())
	assert.NotEmpty(t, cmd.Description())

	// With nil agent
	err := cmd.Execute(context.Background(), "", &Session{})
	assert.Error(t, err)

	// Show current model (no args)
	agent := NewMockAgent()
	sess := &Session{Agent: agent}

	err = cmd.Execute(context.Background(), "", sess)
	assert.NoError(t, err)

	// Change model
	err = cmd.Execute(context.Background(), "claude-opus-4", sess)
	assert.NoError(t, err)
	assert.Equal(t, "claude-opus-4", agent.Model())
}

func TestClearCommand(t *testing.T) {
	cmd := &ClearCommand{}

	assert.Equal(t, "clear", cmd.Name())
	assert.NotEmpty(t, cmd.Description())

	// Execute should not error
	err := cmd.Execute(context.Background(), "", &Session{})
	assert.NoError(t, err)
}

// ─────────────────────────────────────────────────────────────────────────────
// Session Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestSession(t *testing.T) {
	agent := NewMockAgent()
	sess := &Session{
		ID:        "sess-123",
		Directory: "/test/project",
		Agent:     agent,
	}

	assert.Equal(t, "sess-123", sess.ID)
	assert.Equal(t, "/test/project", sess.Directory)
	assert.NotNil(t, sess.Agent)
}
