package tui

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─────────────────────────────────────────────────────────────────────────────
// Commands Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestIsSlashCommand(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"/help", true},
		{"/clear", true},
		{"  /tokens", true},
		{"help", false},
		{"", false},
		{"hello /world", false},
		{"/ space", true}, // Edge case: starts with /
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := isSlashCommand(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTruncateForSummary(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "short string",
			input:    "hello world",
			expected: "hello world",
		},
		{
			name:     "exactly 200 chars",
			input:    string(make([]byte, 200)),
			expected: string(make([]byte, 200)),
		},
		{
			name:     "over 200 chars",
			input:    string(make([]byte, 250)),
			expected: string(make([]byte, 197)) + "...",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "whitespace trimmed",
			input:    "  hello  ",
			expected: "hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateForSummary(tt.input)
			if len(tt.input) > 200 {
				assert.Len(t, result, 200)
				assert.True(t, len(result) <= 200)
			} else {
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestBuiltinCommands(t *testing.T) {
	cmds := builtinCommands()

	// Verify required commands exist
	requiredCmds := []string{"help", "clear", "compact", "tokens", "model", "init", "review"}
	for _, name := range requiredCmds {
		t.Run("has_"+name, func(t *testing.T) {
			cmd, ok := cmds[name]
			require.True(t, ok, "missing command: %s", name)
			assert.NotEmpty(t, cmd.Name)
			assert.NotEmpty(t, cmd.Description)
			assert.NotNil(t, cmd.Handler)
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Brain Monitor Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestNewBrainModel(t *testing.T) {
	brain := NewBrainModel(100000)

	assert.Equal(t, StateIdle, brain.State)
	assert.Equal(t, 100000, brain.MaxTokens)
	assert.Equal(t, 0, brain.UsedTokens)
	assert.False(t, brain.ContextExpanded)
	assert.NotEmpty(t, brain.Message)
}

func TestBrainModel_TokenUsagePercent(t *testing.T) {
	tests := []struct {
		name      string
		used      int
		max       int
		expected  float64
	}{
		{"zero", 0, 100000, 0},
		{"half", 50000, 100000, 50},
		{"full", 100000, 100000, 100},
		{"over", 150000, 100000, 150},
		{"max_zero", 50000, 0, 0}, // Edge case: division by zero
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			brain := BrainModel{
				UsedTokens: tt.used,
				MaxTokens:  tt.max,
			}
			result := brain.TokenUsagePercent()
			assert.InDelta(t, tt.expected, result, 0.01)
		})
	}
}

func TestBrainModel_IsExpanded(t *testing.T) {
	brain := NewBrainModel(100000)
	assert.False(t, brain.IsExpanded())

	brain.ContextExpanded = true
	assert.True(t, brain.IsExpanded())
}

func TestBrainModel_StateColor(t *testing.T) {
	tests := []struct {
		state    CognitiveState
		notEmpty bool
	}{
		{StateIdle, true},
		{StateFocus, true},
		{StateTrauma, true},
		{StateRecall, true},
		{StatePruning, true},
		{StateWrite, true},
	}

	for _, tt := range tests {
		t.Run("state_color", func(t *testing.T) {
			brain := BrainModel{State: tt.state}
			color := brain.StateColor()
			assert.NotEmpty(t, string(color))
		})
	}
}

func TestRepeat(t *testing.T) {
	tests := []struct {
		s        string
		count    int
		expected string
	}{
		{"█", 3, "███"},
		{"ab", 2, "abab"},
		{"x", 0, ""},
		{"x", -1, ""},
		{"", 5, ""},
	}

	for _, tt := range tests {
		t.Run(tt.s, func(t *testing.T) {
			result := repeat(tt.s, tt.count)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Debug Panel Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestNewDebugPanel(t *testing.T) {
	panel := NewDebugPanel(50)

	assert.NotNil(t, panel)
	assert.Equal(t, 50, panel.maxEvents)
	assert.False(t, panel.enabled)
	assert.Equal(t, 80, panel.width)
	assert.Empty(t, panel.events)
}

func TestDebugPanel_Toggle(t *testing.T) {
	panel := NewDebugPanel(50)
	assert.False(t, panel.IsEnabled())

	panel.Toggle()
	assert.True(t, panel.IsEnabled())

	panel.Toggle()
	assert.False(t, panel.IsEnabled())
}

func TestDebugPanel_SetWidth(t *testing.T) {
	panel := NewDebugPanel(50)
	panel.SetWidth(120)
	assert.Equal(t, 120, panel.width)
}

func TestDebugPanel_AddEvent(t *testing.T) {
	panel := NewDebugPanel(5)

	panel.AddEvent(DebugEvent{
		Type:    DebugEventAPI,
		Title:   "Test API",
		Content: "test content",
	})

	assert.Len(t, panel.events, 1)
	assert.True(t, panel.events[0].Collapsed) // Starts collapsed
	assert.NotZero(t, panel.events[0].Timestamp)
}

func TestDebugPanel_AddEvent_MaxLimit(t *testing.T) {
	panel := NewDebugPanel(3)

	// Add 5 events
	for i := 0; i < 5; i++ {
		panel.AddEvent(DebugEvent{
			Type:  DebugEventAPI,
			Title: "Test",
		})
	}

	// Should only keep last 3
	assert.Len(t, panel.events, 3)
}

func TestDebugPanel_AddAPI(t *testing.T) {
	panel := NewDebugPanel(10)
	panel.AddAPI("POST", "/v1/messages", 150*time.Millisecond, 1000)

	require.Len(t, panel.events, 1)
	assert.Equal(t, DebugEventAPI, panel.events[0].Type)
	assert.Contains(t, panel.events[0].Title, "POST")
	assert.Contains(t, panel.events[0].Title, "/v1/messages")
	assert.Equal(t, 150*time.Millisecond, panel.events[0].Duration)
}

func TestDebugPanel_AddTool(t *testing.T) {
	panel := NewDebugPanel(10)
	panel.AddTool("bash", map[string]any{"command": "ls"}, "file.txt", "", 50*time.Millisecond)

	require.Len(t, panel.events, 1)
	assert.Equal(t, DebugEventTool, panel.events[0].Type)
	assert.Contains(t, panel.events[0].Title, "bash")
	assert.Contains(t, panel.events[0].Content, "command")
}

func TestDebugPanel_AddError(t *testing.T) {
	panel := NewDebugPanel(10)
	panel.AddError("API", "connection refused")

	require.Len(t, panel.events, 1)
	assert.Equal(t, DebugEventError, panel.events[0].Type)
	assert.Contains(t, panel.events[0].Title, "API")
	assert.Equal(t, "connection refused", panel.events[0].Content)
}

func TestDebugPanel_AddThinking(t *testing.T) {
	panel := NewDebugPanel(10)
	panel.AddThinking("Processing request...", 500)

	require.Len(t, panel.events, 1)
	assert.Equal(t, DebugEventThinking, panel.events[0].Type)
	assert.Contains(t, panel.events[0].Title, "500 tokens")
}

func TestDebugPanel_AddSystem(t *testing.T) {
	panel := NewDebugPanel(10)
	panel.AddSystem("startup", "initialized successfully")

	require.Len(t, panel.events, 1)
	assert.Equal(t, DebugEventSystem, panel.events[0].Type)
	assert.Contains(t, panel.events[0].Title, "startup")
}

func TestDebugPanel_ToggleEvent(t *testing.T) {
	panel := NewDebugPanel(10)
	panel.AddEvent(DebugEvent{Type: DebugEventAPI, Title: "Test"})

	// Initially collapsed
	assert.True(t, panel.events[0].Collapsed)

	// Toggle to expand
	panel.ToggleEvent(0)
	assert.False(t, panel.events[0].Collapsed)

	// Toggle to collapse
	panel.ToggleEvent(0)
	assert.True(t, panel.events[0].Collapsed)

	// Invalid index - should not panic
	panel.ToggleEvent(-1)
	panel.ToggleEvent(100)
}

func TestDebugPanel_ToggleAll(t *testing.T) {
	panel := NewDebugPanel(10)
	panel.AddEvent(DebugEvent{Type: DebugEventAPI, Title: "Test1"})
	panel.AddEvent(DebugEvent{Type: DebugEventTool, Title: "Test2"})
	panel.AddEvent(DebugEvent{Type: DebugEventError, Title: "Test3"})

	// All start collapsed
	for _, evt := range panel.events {
		assert.True(t, evt.Collapsed)
	}

	// Toggle all - should expand
	panel.ToggleAll()
	for _, evt := range panel.events {
		assert.False(t, evt.Collapsed)
	}

	// Toggle all again - should collapse
	panel.ToggleAll()
	for _, evt := range panel.events {
		assert.True(t, evt.Collapsed)
	}
}

func TestDebugPanel_Scroll(t *testing.T) {
	panel := NewDebugPanel(10)
	for i := 0; i < 5; i++ {
		panel.AddEvent(DebugEvent{Type: DebugEventAPI, Title: "Test"})
	}

	assert.Equal(t, 0, panel.scroll)

	panel.ScrollDown()
	assert.Equal(t, 1, panel.scroll)

	panel.ScrollDown()
	panel.ScrollDown()
	assert.Equal(t, 3, panel.scroll)

	// Should not go beyond max
	panel.ScrollDown()
	panel.ScrollDown()
	panel.ScrollDown()
	assert.Equal(t, 4, panel.scroll) // Max is len-1 = 4

	panel.ScrollUp()
	assert.Equal(t, 3, panel.scroll)

	// Should not go below 0
	panel.scroll = 0
	panel.ScrollUp()
	assert.Equal(t, 0, panel.scroll)
}

func TestDebugPanel_Clear(t *testing.T) {
	panel := NewDebugPanel(10)
	panel.AddEvent(DebugEvent{Type: DebugEventAPI, Title: "Test"})
	panel.AddEvent(DebugEvent{Type: DebugEventAPI, Title: "Test2"})
	panel.scroll = 5

	panel.Clear()

	assert.Empty(t, panel.events)
	assert.Equal(t, 0, panel.scroll)
}

func TestDebugPanel_Stats(t *testing.T) {
	panel := NewDebugPanel(50)

	panel.AddAPI("POST", "/test", 0, 0)
	panel.AddAPI("GET", "/test", 0, 0)
	panel.AddTool("bash", nil, "", "", 0)
	panel.AddError("test", "error")
	panel.AddThinking("test", 100)
	panel.AddSystem("test", "detail")

	stats := panel.Stats()

	assert.Equal(t, 6, stats["total"])
	assert.Equal(t, 2, stats["api"])
	assert.Equal(t, 1, stats["tool"])
	assert.Equal(t, 1, stats["error"])
	assert.Equal(t, 1, stats["thinking"])
}

func TestDebugPanel_View_Disabled(t *testing.T) {
	panel := NewDebugPanel(10)
	panel.AddEvent(DebugEvent{Type: DebugEventAPI, Title: "Test"})

	// When disabled, should return empty
	view := panel.View(20)
	assert.Empty(t, view)
}

func TestDebugPanel_View_Enabled(t *testing.T) {
	panel := NewDebugPanel(10)
	panel.Toggle() // Enable
	panel.AddAPI("POST", "/v1/messages", 100*time.Millisecond, 500)

	view := panel.View(20)
	assert.NotEmpty(t, view)
	assert.Contains(t, view, "DEBUG")
	assert.Contains(t, view, "POST")
}

// ─────────────────────────────────────────────────────────────────────────────
// Model Tests (Basic)
// ─────────────────────────────────────────────────────────────────────────────

func TestNew(t *testing.T) {
	model := New()

	assert.Equal(t, ViewMain, model.view)
	assert.NotNil(t, model.spinner)
	assert.NotNil(t, model.input)
	assert.Empty(t, model.sessions)
	assert.False(t, model.quitting)
	assert.False(t, model.ready)
}

func TestStatus(t *testing.T) {
	status := Status{
		GraphConnected: true,
		Project:        "test-project",
		EventCount:     100,
		Workers:        2,
		LastUpdate:     time.Now(),
	}

	assert.True(t, status.GraphConnected)
	assert.Equal(t, "test-project", status.Project)
	assert.Equal(t, 100, status.EventCount)
	assert.Equal(t, 2, status.Workers)
}

func TestSession(t *testing.T) {
	session := Session{
		ID:        "sess-123",
		Title:     "Test Session",
		UpdatedAt: time.Now(),
		Messages:  10,
	}

	assert.Equal(t, "sess-123", session.ID)
	assert.Equal(t, "Test Session", session.Title)
	assert.Equal(t, 10, session.Messages)
}

// ─────────────────────────────────────────────────────────────────────────────
// View Constants Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestViewConstants(t *testing.T) {
	assert.Equal(t, View(0), ViewMain)
	assert.Equal(t, View(1), ViewSessions)
	assert.Equal(t, View(2), ViewAgent)
	assert.Equal(t, View(3), ViewHelp)
}

func TestCognitiveStateConstants(t *testing.T) {
	assert.Equal(t, CognitiveState(0), StateIdle)
	assert.Equal(t, CognitiveState(1), StateFocus)
	assert.Equal(t, CognitiveState(2), StateTrauma)
	assert.Equal(t, CognitiveState(3), StateRecall)
	assert.Equal(t, CognitiveState(4), StatePruning)
	assert.Equal(t, CognitiveState(5), StateWrite)
}

func TestDebugEventTypeConstants(t *testing.T) {
	assert.Equal(t, DebugEventType(0), DebugEventAPI)
	assert.Equal(t, DebugEventType(1), DebugEventTool)
	assert.Equal(t, DebugEventType(2), DebugEventPermission)
	assert.Equal(t, DebugEventType(3), DebugEventStream)
	assert.Equal(t, DebugEventType(4), DebugEventError)
	assert.Equal(t, DebugEventType(5), DebugEventThinking)
	assert.Equal(t, DebugEventType(6), DebugEventSystem)
}
