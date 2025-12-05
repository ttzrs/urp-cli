// Package render provides output formatting for CLI commands.
// Separates presentation from business logic (SRP compliance).
package render

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// Writer wraps an io.Writer with formatting utilities.
// Use this for direct-to-stdout writing without string building.
type Writer struct {
	out io.Writer
}

// NewWriter creates a Writer that writes to the given io.Writer.
func NewWriter(w io.Writer) *Writer {
	return &Writer{out: w}
}

// Stdout returns a Writer that writes to os.Stdout.
func Stdout() *Writer {
	return NewWriter(os.Stdout)
}

// Stderr returns a Writer that writes to os.Stderr.
func Stderr() *Writer {
	return NewWriter(os.Stderr)
}

// Print writes formatted text.
func (w *Writer) Print(format string, args ...any) {
	fmt.Fprintf(w.out, format, args...)
}

// Println writes formatted text with newline.
func (w *Writer) Println(format string, args ...any) {
	fmt.Fprintf(w.out, format+"\n", args...)
}

// Line writes a blank line.
func (w *Writer) Line() {
	fmt.Fprintln(w.out)
}

// Header writes a header line.
func (w *Writer) Header(title string, args ...any) {
	if len(args) > 0 {
		title = fmt.Sprintf(title, args...)
	}
	fmt.Fprintln(w.out, strings.ToUpper(title))
	fmt.Fprintln(w.out)
}

// Section writes a section header.
func (w *Writer) Section(title string) {
	fmt.Fprintln(w.out)
	fmt.Fprintln(w.out, strings.ToUpper(title)+":")
}

// Item writes an indented item line.
func (w *Writer) Item(format string, args ...any) {
	fmt.Fprintf(w.out, "  "+format+"\n", args...)
}

// SubItem writes a double-indented sub-item.
func (w *Writer) SubItem(format string, args ...any) {
	fmt.Fprintf(w.out, "    "+format+"\n", args...)
}

// Nested writes a nested item with tree connector.
func (w *Writer) Nested(format string, args ...any) {
	fmt.Fprintf(w.out, "    └─ "+format+"\n", args...)
}

// Empty writes an empty state message.
func (w *Writer) Empty(msg string) {
	fmt.Fprintln(w.out, msg)
}

// StatusIcon returns icon for status string.
func StatusIcon(status string) string {
	switch status {
	case "success":
		return "✓"
	case "error":
		return "✗"
	case "warning":
		return "!"
	case "timeout":
		return "⏱"
	default:
		return "•"
	}
}

// LevelIcon returns icon for anomaly level.
func LevelIcon(level string) string {
	switch level {
	case "low":
		return "○"
	case "medium":
		return "◐"
	case "high":
		return "●"
	case "critical":
		return "◉"
	default:
		return "•"
	}
}

// BoolIcon returns icon for boolean.
func BoolIcon(b bool) string {
	if b {
		return "✓"
	}
	return "✗"
}

// Truncate shortens a string to max length.
func Truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}
