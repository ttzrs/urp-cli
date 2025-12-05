package strings

import (
	"testing"
)

func TestWordWrap(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		width    int
		expected string
	}{
		{
			name:     "short line no wrap",
			input:    "hello world",
			width:    80,
			expected: "hello world",
		},
		{
			name:     "wrap at width",
			input:    "hello world test",
			width:    10,
			expected: "hello\nworld test",
		},
		{
			name:     "preserves newlines",
			input:    "line1\nline2",
			width:    80,
			expected: "line1\nline2",
		},
		{
			name:     "empty string",
			input:    "",
			width:    80,
			expected: "",
		},
		{
			name:     "width zero returns input",
			input:    "test",
			width:    0,
			expected: "test",
		},
		{
			name:     "long word exceeds width",
			input:    "superlongword short",
			width:    5,
			expected: "superlongword\nshort",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := WordWrap(tt.input, tt.width)
			if result != tt.expected {
				t.Errorf("WordWrap(%q, %d) = %q, want %q", tt.input, tt.width, result, tt.expected)
			}
		})
	}
}

func TestVisibleLength(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{
			name:     "plain text",
			input:    "hello",
			expected: 5,
		},
		{
			name:     "with ANSI color",
			input:    "\x1b[31mred\x1b[0m",
			expected: 3,
		},
		{
			name:     "empty",
			input:    "",
			expected: 0,
		},
		{
			name:     "only ANSI",
			input:    "\x1b[31m\x1b[0m",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := visibleLength(tt.input)
			if result != tt.expected {
				t.Errorf("visibleLength(%q) = %d, want %d", tt.input, result, tt.expected)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		n        int
		expected string
	}{
		{
			name:     "no truncation needed",
			input:    "hello",
			n:        10,
			expected: "hello",
		},
		{
			name:     "truncation with ellipsis",
			input:    "hello world",
			n:        8,
			expected: "hello...",
		},
		{
			name:     "min length enforced",
			input:    "hello",
			n:        2,
			expected: "h...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Truncate(tt.input, tt.n)
			if result != tt.expected {
				t.Errorf("Truncate(%q, %d) = %q, want %q", tt.input, tt.n, result, tt.expected)
			}
		})
	}
}
