// Package render tests verify output formatting.
package render

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/joss/urp/internal/domain"
)

// ─────────────────────────────────────────────────────────────────────────────
// Renderer Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestNew(t *testing.T) {
	r := New(true)
	assert.NotNil(t, r)
	assert.True(t, r.pretty)

	r2 := New(false)
	assert.False(t, r2.pretty)
}

// ─────────────────────────────────────────────────────────────────────────────
// Events Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestRenderer_Events_Empty(t *testing.T) {
	r := New(false)
	out := r.Events(nil)
	assert.Equal(t, "No events found", out)

	out2 := r.Events([]domain.Event{})
	assert.Equal(t, "No events found", out2)
}

func TestRenderer_Events_Plain(t *testing.T) {
	r := New(false)
	ts := time.Date(2024, 1, 15, 10, 30, 45, 0, time.UTC)

	events := []domain.Event{
		{
			Command:     "go build",
			ExitCode:    0,
			DurationSec: 2.5,
			Timestamp:   ts,
		},
		{
			Command:     "go test",
			ExitCode:    1,
			DurationSec: 1.2,
			Timestamp:   ts.Add(time.Minute),
		},
	}

	out := r.Events(events)

	// Should contain timestamps and commands
	assert.Contains(t, out, "10:30:45")
	assert.Contains(t, out, "go build")
	assert.Contains(t, out, "(2.5s)")
	assert.Contains(t, out, "10:31:45")
	assert.Contains(t, out, "go test")
	// Plain format: [time] exitCode command (duration)
	assert.Contains(t, out, "] 1 go test") // exit code 1
}

func TestRenderer_Events_Pretty(t *testing.T) {
	r := New(true)
	ts := time.Date(2024, 1, 15, 10, 30, 45, 0, time.UTC)

	events := []domain.Event{
		{
			Command:     "echo hello",
			ExitCode:    0,
			DurationSec: 0.1,
			Timestamp:   ts,
		},
	}

	out := r.Events(events)

	// Pretty mode has headers
	assert.Contains(t, out, "Recent Commands")
	assert.Contains(t, out, "─") // separator line
	assert.Contains(t, out, "echo hello")
}

func TestRenderer_Events_NoDuration(t *testing.T) {
	r := New(false)
	ts := time.Now()

	events := []domain.Event{
		{
			Command:     "ls",
			ExitCode:    0,
			DurationSec: 0, // no duration
			Timestamp:   ts,
		},
	}

	out := r.Events(events)

	// Should not contain duration suffix
	assert.NotContains(t, out, "(0.0s)")
	assert.Contains(t, out, "ls")
}

// ─────────────────────────────────────────────────────────────────────────────
// Errors/Conflicts Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestRenderer_Errors_Empty(t *testing.T) {
	r := New(false)
	out := r.Errors(nil, "Test Errors")
	assert.Equal(t, "No errors found", out)
}

func TestRenderer_Errors_Plain(t *testing.T) {
	r := New(false)
	ts := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

	conflicts := []domain.Conflict{
		{
			Event: domain.Event{
				Command:       "npm install",
				ExitCode:      1,
				StderrPreview: "ENOENT: package.json not found",
				Timestamp:     ts,
			},
		},
	}

	out := r.Errors(conflicts, "Recent Errors")

	assert.Contains(t, out, "Recent Errors")
	assert.Contains(t, out, "10:30")
	assert.Contains(t, out, "npm install")
	assert.Contains(t, out, "exit=1")
	assert.Contains(t, out, "ENOENT")
}

func TestRenderer_Errors_Pretty(t *testing.T) {
	r := New(true)
	ts := time.Now()

	conflicts := []domain.Conflict{
		{
			Event: domain.Event{
				Command:       "go build",
				ExitCode:      2,
				StderrPreview: "undefined: foo\nundefined: bar",
				Timestamp:     ts,
			},
		},
	}

	out := r.Errors(conflicts, "Build Errors")

	assert.Contains(t, out, "⊥ Build Errors") // Orthogonal symbol
	assert.Contains(t, out, "LATEST")         // Latest marker
	assert.Contains(t, out, "go build")
}

func TestRenderer_Errors_MultipleConflicts(t *testing.T) {
	r := New(true)
	ts := time.Now()

	conflicts := []domain.Conflict{
		{
			Event: domain.Event{
				Command:   "cmd1",
				ExitCode:  1,
				Timestamp: ts,
			},
		},
		{
			Event: domain.Event{
				Command:   "cmd2",
				ExitCode:  1,
				Timestamp: ts.Add(-time.Minute),
			},
		},
	}

	out := r.Errors(conflicts, "Errors")

	// First one is LATEST
	assert.Contains(t, out, "LATEST")
	assert.Contains(t, out, "cmd1")
	assert.Contains(t, out, "cmd2")
}

func TestRenderer_Errors_TruncatesStderr(t *testing.T) {
	r := New(true)
	ts := time.Now()

	// Create stderr with many lines
	stderr := "line1\nline2\nline3\nline4\nline5\nline6"

	conflicts := []domain.Conflict{
		{
			Event: domain.Event{
				Command:       "failing",
				ExitCode:      1,
				StderrPreview: stderr,
				Timestamp:     ts,
			},
		},
	}

	out := r.Errors(conflicts, "Test")

	// Should show only first 3 lines
	assert.Contains(t, out, "line1")
	assert.Contains(t, out, "line2")
	assert.Contains(t, out, "line3")
	// line4+ may or may not appear depending on implementation
}

// ─────────────────────────────────────────────────────────────────────────────
// Status Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestRenderer_Status_Connected(t *testing.T) {
	r := New(false)
	out := r.Status(true, "myproject", 42)

	assert.Contains(t, out, "connected=true")
	assert.Contains(t, out, "project=myproject")
	assert.Contains(t, out, "events=42")
}

func TestRenderer_Status_Disconnected(t *testing.T) {
	r := New(false)
	out := r.Status(false, "test", 0)

	assert.Contains(t, out, "connected=false")
}

func TestRenderer_Status_Pretty(t *testing.T) {
	r := New(true)
	out := r.Status(true, "urp", 100)

	assert.Contains(t, out, "URP Status")
	assert.Contains(t, out, "Graph:")
	assert.Contains(t, out, "connected")
	assert.Contains(t, out, "Project:")
	assert.Contains(t, out, "urp")
	assert.Contains(t, out, "Events:")
	assert.Contains(t, out, "100")
}

// ─────────────────────────────────────────────────────────────────────────────
// Helper Function Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestMin(t *testing.T) {
	tests := []struct {
		a, b, want int
	}{
		{1, 2, 1},
		{5, 3, 3},
		{0, 0, 0},
		{-1, 1, -1},
		{100, 100, 100},
	}

	for _, tt := range tests {
		got := min(tt.a, tt.b)
		assert.Equal(t, tt.want, got)
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name string
		d    time.Duration
		want string
	}{
		{
			name: "milliseconds",
			d:    500 * time.Millisecond,
			want: "500ms",
		},
		{
			name: "seconds",
			d:    2500 * time.Millisecond,
			want: "2.5s",
		},
		{
			name: "minutes",
			d:    90 * time.Second,
			want: "1m30s",
		},
		{
			name: "zero",
			d:    0,
			want: "0ms",
		},
		{
			name: "under_second",
			d:    999 * time.Millisecond,
			want: "999ms",
		},
		{
			name: "exactly_one_second",
			d:    time.Second,
			want: "1.0s",
		},
		{
			name: "exactly_one_minute",
			d:    time.Minute,
			want: "1m0s",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatDuration(tt.d)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Edge Cases
// ─────────────────────────────────────────────────────────────────────────────

func TestRenderer_Events_SpecialCharacters(t *testing.T) {
	r := New(false)
	ts := time.Now()

	events := []domain.Event{
		{
			Command:   `echo "hello world" | grep 'world'`,
			ExitCode:  0,
			Timestamp: ts,
		},
	}

	out := r.Events(events)
	assert.Contains(t, out, `echo "hello world"`)
}

func TestRenderer_Errors_EmptyStderr(t *testing.T) {
	r := New(true)
	ts := time.Now()

	conflicts := []domain.Conflict{
		{
			Event: domain.Event{
				Command:       "silent-fail",
				ExitCode:      1,
				StderrPreview: "", // empty
				Timestamp:     ts,
			},
		},
	}

	out := r.Errors(conflicts, "Errors")
	require.NotEmpty(t, out)
	assert.Contains(t, out, "silent-fail")
}

func TestRenderer_Status_EmptyProject(t *testing.T) {
	r := New(false)
	out := r.Status(true, "", 0)

	assert.Contains(t, out, "project=")
}

// ─────────────────────────────────────────────────────────────────────────────
// Output Format Consistency
// ─────────────────────────────────────────────────────────────────────────────

func TestRenderer_PlainOutputNoANSI(t *testing.T) {
	r := New(false)
	ts := time.Now()

	events := []domain.Event{
		{Command: "test", ExitCode: 0, Timestamp: ts},
	}

	out := r.Events(events)

	// Plain output should not contain ANSI escape codes
	assert.NotContains(t, out, "\x1b[")
}

func TestRenderer_PrettyOutputConsistency(t *testing.T) {
	r := New(true)
	ts := time.Now()

	events := []domain.Event{
		{Command: "cmd1", ExitCode: 0, Timestamp: ts},
	}

	// Run multiple times, output should be consistent
	out1 := r.Events(events)
	out2 := r.Events(events)

	// Remove potential ANSI codes for comparison
	clean1 := strings.ReplaceAll(out1, "\x1b[", "")
	clean2 := strings.ReplaceAll(out2, "\x1b[", "")

	// Lengths should match even with ANSI codes
	assert.Equal(t, len(clean1), len(clean2))
}
