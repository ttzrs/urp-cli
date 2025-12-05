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
