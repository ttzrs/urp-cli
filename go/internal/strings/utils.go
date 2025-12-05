// Package strings provides common string utilities.
// utils.go consolidates 7+ duplicate truncate implementations.
package strings

import (
	"fmt"
	"os"
	"strings"
)

// GetCwd returns the current working directory, or "unknown" on error.
// Consolidates 2 duplicate getCwd() implementations.
func GetCwd() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "unknown"
	}
	return cwd
}

// TruncateMap formats a map[string]any as "key=value, ..." with max length.
// Used for tool argument display.
func TruncateMap(args map[string]any, maxLen int) string {
	if args == nil {
		return ""
	}
	var parts []string
	for k, v := range args {
		parts = append(parts, fmt.Sprintf("%s=%v", k, v))
	}
	s := strings.Join(parts, ", ")
	if len(s) > maxLen {
		return s[:maxLen-3] + "..."
	}
	return s
}

// Truncate shortens a string to n characters with ellipsis.
// If n < 4, uses n = 4 to ensure room for "...".
// Eliminates 7 duplicate truncate functions across packages.
func Truncate(s string, n int) string {
	if n < 4 {
		n = 4
	}
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}

// TruncateNoEllipsis shortens a string to n characters without ellipsis.
// Used where exact length limits are required.
func TruncateNoEllipsis(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// TruncateRunes truncates by rune count, not byte count.
// Safer for unicode strings.
func TruncateRunes(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	if n < 4 {
		n = 4
	}
	return string(runes[:n-3]) + "..."
}

// WordWrap wraps text to a maximum width, breaking on word boundaries.
// Preserves existing newlines and handles ANSI escape codes.
func WordWrap(s string, width int) string {
	if width <= 0 {
		return s
	}

	var result strings.Builder
	lines := strings.Split(s, "\n")

	for i, line := range lines {
		if i > 0 {
			result.WriteString("\n")
		}

		// Handle empty lines
		if line == "" {
			continue
		}

		// Calculate visible length (excluding ANSI codes)
		visibleLen := visibleLength(line)
		if visibleLen <= width {
			result.WriteString(line)
			continue
		}

		// Wrap long lines
		result.WriteString(wrapLine(line, width))
	}

	return result.String()
}

// wrapLine wraps a single line to width, preserving ANSI codes
func wrapLine(line string, width int) string {
	if width <= 0 {
		return line
	}

	var result strings.Builder
	words := strings.Fields(line)
	currentLen := 0
	lineStart := true

	for _, word := range words {
		wordLen := visibleLength(word)

		// If word alone is longer than width, just add it
		if wordLen > width {
			if !lineStart {
				result.WriteString("\n")
			}
			result.WriteString(word)
			result.WriteString("\n")
			currentLen = 0
			lineStart = true
			continue
		}

		// Check if word fits on current line
		spaceNeeded := wordLen
		if !lineStart {
			spaceNeeded++ // for the space before
		}

		if currentLen+spaceNeeded > width {
			// Start new line
			result.WriteString("\n")
			result.WriteString(word)
			currentLen = wordLen
			lineStart = false
		} else {
			// Add to current line
			if !lineStart {
				result.WriteString(" ")
				currentLen++
			}
			result.WriteString(word)
			currentLen += wordLen
			lineStart = false
		}
	}

	return result.String()
}

// visibleLength calculates string length excluding ANSI escape codes
func visibleLength(s string) int {
	inEscape := false
	count := 0
	for _, r := range s {
		if r == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEscape = false
			}
			continue
		}
		count++
	}
	return count
}
