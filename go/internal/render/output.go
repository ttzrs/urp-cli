// Package render provides output formatting for terminal and LLM consumption.
package render

import (
	"fmt"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/joss/urp/internal/domain"
)

// Renderer handles output formatting.
type Renderer struct {
	pretty bool
	noColor bool
}

// New creates a new renderer.
func New(pretty bool) *Renderer {
	return &Renderer{pretty: pretty}
}

// Events formats a list of events.
func (r *Renderer) Events(events []domain.Event) string {
	if len(events) == 0 {
		return "No events found"
	}

	var sb strings.Builder

	if r.pretty {
		sb.WriteString(color.CyanString("Recent Commands\n"))
		sb.WriteString(strings.Repeat("─", 60) + "\n")
	}

	for _, e := range events {
		r.formatEvent(&sb, e)
	}

	return sb.String()
}

func (r *Renderer) formatEvent(sb *strings.Builder, e domain.Event) {
	// Time
	timeStr := e.Timestamp.Format("15:04:05")

	// Status indicator
	status := color.GreenString("✓")
	if e.ExitCode != 0 {
		status = color.RedString("✗")
	}

	// Duration
	durStr := ""
	if e.DurationSec > 0 {
		durStr = fmt.Sprintf(" (%.1fs)", e.DurationSec)
	}

	if r.pretty {
		fmt.Fprintf(sb, "%s %s %s%s\n", status, color.HiBlackString(timeStr), e.Command, durStr)
	} else {
		fmt.Fprintf(sb, "[%s] %d %s%s\n", timeStr, e.ExitCode, e.Command, durStr)
	}
}

// Errors formats a list of conflicts as a causal trace.
func (r *Renderer) Errors(conflicts []domain.Conflict, title string) string {
	if len(conflicts) == 0 {
		return "No errors found"
	}

	var sb strings.Builder

	if r.pretty {
		sb.WriteString(color.RedString("⊥ " + title + "\n"))
		sb.WriteString(strings.Repeat("─", 60) + "\n\n")
	} else {
		sb.WriteString(title + "\n")
	}

	for i, c := range conflicts {
		r.formatConflict(&sb, c, i == 0)
	}

	return sb.String()
}

func (r *Renderer) formatConflict(sb *strings.Builder, c domain.Conflict, isLatest bool) {
	timeStr := c.Timestamp.Format("15:04")

	marker := "○"
	if isLatest {
		marker = color.RedString("✗")
	}

	if r.pretty {
		if isLatest {
			fmt.Fprintf(sb, "[%s] LATEST: %s\n", marker, color.YellowString(c.Command))
		} else {
			fmt.Fprintf(sb, "[%s] %s: %s\n", marker, color.HiBlackString(timeStr), c.Command)
		}

		if c.StderrPreview != "" {
			lines := strings.Split(c.StderrPreview, "\n")
			for _, line := range lines[:min(3, len(lines))] {
				fmt.Fprintf(sb, "    %s\n", color.RedString(line))
			}
		}
		sb.WriteString("\n")
	} else {
		fmt.Fprintf(sb, "[%s] exit=%d %s\n", timeStr, c.ExitCode, c.Command)
		if c.StderrPreview != "" {
			fmt.Fprintf(sb, "    Error: %s\n", c.StderrPreview)
		}
	}
}

// Status formats the URP status.
func (r *Renderer) Status(connected bool, project string, eventCount int) string {
	var sb strings.Builder

	if r.pretty {
		sb.WriteString(color.CyanString("URP Status\n"))
		sb.WriteString(strings.Repeat("─", 40) + "\n")

		if connected {
			fmt.Fprintf(&sb, "  Graph:   %s\n", color.GreenString("connected"))
		} else {
			fmt.Fprintf(&sb, "  Graph:   %s\n", color.RedString("disconnected"))
		}

		fmt.Fprintf(&sb, "  Project: %s\n", project)
		fmt.Fprintf(&sb, "  Events:  %d\n", eventCount)
	} else {
		fmt.Fprintf(&sb, "connected=%v project=%s events=%d\n", connected, project, eventCount)
	}

	return sb.String()
}

// min helper for Go < 1.21
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// FormatDuration formats a duration in human-readable form.
func FormatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
}
