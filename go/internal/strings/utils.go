// Package strings provides common string utilities.
// utils.go consolidates 7+ duplicate truncate implementations.
package strings

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
