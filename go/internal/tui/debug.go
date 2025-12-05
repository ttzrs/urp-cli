// Package tui provides the debug panel for interaction visualization
package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// DebugEventType represents the type of debug event
type DebugEventType int

const (
	DebugEventAPI DebugEventType = iota
	DebugEventTool
	DebugEventPermission
	DebugEventStream
	DebugEventError
	DebugEventThinking
	DebugEventSystem
)

// DebugEvent represents a single debug event
type DebugEvent struct {
	Type      DebugEventType
	Timestamp time.Time
	Title     string
	Content   string
	Metadata  map[string]string
	Collapsed bool
	Duration  time.Duration
}

// DebugPanel manages debug event visualization
type DebugPanel struct {
	events    []DebugEvent
	maxEvents int
	enabled   bool
	width     int
	scroll    int
}

// Debug panel styles
var (
	debugHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("212")).
				Background(lipgloss.Color("235")).
				Padding(0, 1)

	debugAPIStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("39")). // Blue
			Bold(true)

	debugToolStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("214")). // Orange
			Bold(true)

	debugPermStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("226")). // Yellow
			Bold(true)

	debugStreamStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("247")). // Gray
				Italic(true)

	debugErrorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")). // Red
			Bold(true)

	debugThinkStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("201")). // Magenta
			Italic(true)

	debugSystemStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("42")). // Green
				Bold(true)

	debugContentStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("250")).
				PaddingLeft(2)

	debugMetaStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Italic(true).
			PaddingLeft(2)

	debugBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("238"))

	debugCollapsedIcon = "▶"
	debugExpandedIcon  = "▼"
)

// NewDebugPanel creates a new debug panel
func NewDebugPanel(maxEvents int) *DebugPanel {
	return &DebugPanel{
		events:    make([]DebugEvent, 0, maxEvents),
		maxEvents: maxEvents,
		enabled:   false,
		width:     80,
	}
}

// Toggle enables/disables debug mode
func (d *DebugPanel) Toggle() {
	d.enabled = !d.enabled
}

// IsEnabled returns if debug mode is active
func (d *DebugPanel) IsEnabled() bool {
	return d.enabled
}

// SetWidth sets the panel width
func (d *DebugPanel) SetWidth(w int) {
	d.width = w
}

// AddEvent adds a new debug event
func (d *DebugPanel) AddEvent(evt DebugEvent) {
	evt.Timestamp = time.Now()
	evt.Collapsed = true // Start collapsed

	d.events = append(d.events, evt)

	// Trim old events
	if len(d.events) > d.maxEvents {
		d.events = d.events[len(d.events)-d.maxEvents:]
	}
}

// AddAPI logs an API call
func (d *DebugPanel) AddAPI(method, endpoint string, duration time.Duration, tokens int) {
	d.AddEvent(DebugEvent{
		Type:     DebugEventAPI,
		Title:    fmt.Sprintf("API: %s %s", method, endpoint),
		Duration: duration,
		Metadata: map[string]string{
			"duration": duration.String(),
			"tokens":   fmt.Sprintf("%d", tokens),
		},
	})
}

// AddTool logs a tool call
func (d *DebugPanel) AddTool(name string, args map[string]any, output string, err string, duration time.Duration) {
	// Format args nicely
	var argsStr strings.Builder
	for k, v := range args {
		val := fmt.Sprintf("%v", v)
		if len(val) > 100 {
			val = val[:97] + "..."
		}
		argsStr.WriteString(fmt.Sprintf("  %s: %s\n", k, val))
	}

	content := fmt.Sprintf("Args:\n%s", argsStr.String())
	if output != "" {
		outPreview := output
		if len(outPreview) > 500 {
			outPreview = outPreview[:497] + "..."
		}
		content += fmt.Sprintf("\nOutput:\n  %s", strings.ReplaceAll(outPreview, "\n", "\n  "))
	}
	if err != "" {
		content += fmt.Sprintf("\nError: %s", err)
	}

	d.AddEvent(DebugEvent{
		Type:     DebugEventTool,
		Title:    fmt.Sprintf("Tool: %s", name),
		Content:  content,
		Duration: duration,
		Metadata: map[string]string{
			"duration": duration.String(),
		},
	})
}

// AddPermission logs a permission request
func (d *DebugPanel) AddPermission(tool, command, path, result string) {
	content := ""
	if command != "" {
		content += fmt.Sprintf("Command: %s\n", command)
	}
	if path != "" {
		content += fmt.Sprintf("Path: %s\n", path)
	}
	content += fmt.Sprintf("Result: %s", result)

	d.AddEvent(DebugEvent{
		Type:    DebugEventPermission,
		Title:   fmt.Sprintf("Permission: %s", tool),
		Content: content,
	})
}

// AddStream logs a stream event
func (d *DebugPanel) AddStream(eventType, preview string) {
	d.AddEvent(DebugEvent{
		Type:    DebugEventStream,
		Title:   fmt.Sprintf("Stream: %s", eventType),
		Content: preview,
	})
}

// AddError logs an error
func (d *DebugPanel) AddError(source, message string) {
	d.AddEvent(DebugEvent{
		Type:    DebugEventError,
		Title:   fmt.Sprintf("Error: %s", source),
		Content: message,
	})
}

// AddThinking logs thinking/reasoning
func (d *DebugPanel) AddThinking(preview string, tokens int) {
	d.AddEvent(DebugEvent{
		Type:    DebugEventThinking,
		Title:   fmt.Sprintf("Thinking (%d tokens)", tokens),
		Content: preview,
	})
}

// AddSystem logs system events
func (d *DebugPanel) AddSystem(event, detail string) {
	d.AddEvent(DebugEvent{
		Type:    DebugEventSystem,
		Title:   fmt.Sprintf("System: %s", event),
		Content: detail,
	})
}

// ToggleEvent toggles collapse state of an event
func (d *DebugPanel) ToggleEvent(index int) {
	if index >= 0 && index < len(d.events) {
		d.events[index].Collapsed = !d.events[index].Collapsed
	}
}

// ToggleAll collapses or expands all events
func (d *DebugPanel) ToggleAll() {
	// Check if any are expanded
	anyExpanded := false
	for _, e := range d.events {
		if !e.Collapsed {
			anyExpanded = true
			break
		}
	}

	// Toggle all to opposite
	for i := range d.events {
		d.events[i].Collapsed = anyExpanded
	}
}

// ScrollUp scrolls the panel up
func (d *DebugPanel) ScrollUp() {
	if d.scroll > 0 {
		d.scroll--
	}
}

// ScrollDown scrolls the panel down
func (d *DebugPanel) ScrollDown() {
	maxScroll := len(d.events) - 1
	if maxScroll < 0 {
		maxScroll = 0
	}
	if d.scroll < maxScroll {
		d.scroll++
	}
}

// Clear clears all events
func (d *DebugPanel) Clear() {
	d.events = d.events[:0]
	d.scroll = 0
}

// View renders the debug panel
func (d *DebugPanel) View(height int) string {
	if !d.enabled {
		return ""
	}

	var b strings.Builder

	// Header
	header := debugHeaderStyle.Width(d.width).Render(
		fmt.Sprintf("🔍 DEBUG [%d events] │ Ctrl+D: toggle │ Ctrl+E: expand/collapse │ ↑↓: scroll │ Ctrl+X: clear",
			len(d.events)))
	b.WriteString(header + "\n")

	if len(d.events) == 0 {
		b.WriteString(debugMetaStyle.Render("  No events yet..."))
		return debugBorderStyle.Width(d.width).Render(b.String())
	}

	// Calculate visible events
	visibleLines := height - 3 // Account for header and border
	if visibleLines < 1 {
		visibleLines = 1
	}

	// Render events from scroll position
	linesRendered := 0
	for i := d.scroll; i < len(d.events) && linesRendered < visibleLines; i++ {
		evt := d.events[i]
		eventView := d.renderEvent(evt, i)
		eventLines := strings.Count(eventView, "\n") + 1

		if linesRendered+eventLines > visibleLines && linesRendered > 0 {
			break
		}

		b.WriteString(eventView)
		linesRendered += eventLines
	}

	return debugBorderStyle.Width(d.width).Render(b.String())
}

func (d *DebugPanel) renderEvent(evt DebugEvent, index int) string {
	var b strings.Builder

	// Icon and style based on type
	icon := debugCollapsedIcon
	if !evt.Collapsed {
		icon = debugExpandedIcon
	}

	var titleStyle lipgloss.Style
	switch evt.Type {
	case DebugEventAPI:
		titleStyle = debugAPIStyle
	case DebugEventTool:
		titleStyle = debugToolStyle
	case DebugEventPermission:
		titleStyle = debugPermStyle
	case DebugEventStream:
		titleStyle = debugStreamStyle
	case DebugEventError:
		titleStyle = debugErrorStyle
	case DebugEventThinking:
		titleStyle = debugThinkStyle
	case DebugEventSystem:
		titleStyle = debugSystemStyle
	default:
		titleStyle = debugStreamStyle
	}

	// Timestamp
	ts := evt.Timestamp.Format("15:04:05.000")

	// Title line
	title := fmt.Sprintf("%s [%s] %s", icon, ts, titleStyle.Render(evt.Title))
	if evt.Duration > 0 {
		title += debugMetaStyle.Render(fmt.Sprintf(" (%s)", evt.Duration))
	}
	b.WriteString(title + "\n")

	// Content (if expanded)
	if !evt.Collapsed && evt.Content != "" {
		// Wrap content to fit width
		content := evt.Content
		if len(content) > 1000 {
			content = content[:997] + "..."
		}
		lines := strings.Split(content, "\n")
		for _, line := range lines {
			// Truncate long lines
			if len(line) > d.width-6 {
				line = line[:d.width-9] + "..."
			}
			b.WriteString(debugContentStyle.Render(line) + "\n")
		}
	}

	// Metadata (if expanded)
	if !evt.Collapsed && len(evt.Metadata) > 0 {
		var meta []string
		for k, v := range evt.Metadata {
			meta = append(meta, fmt.Sprintf("%s=%s", k, v))
		}
		b.WriteString(debugMetaStyle.Render(strings.Join(meta, " │ ")) + "\n")
	}

	return b.String()
}

// Stats returns debug statistics
func (d *DebugPanel) Stats() map[string]int {
	stats := map[string]int{
		"total":      len(d.events),
		"api":        0,
		"tool":       0,
		"permission": 0,
		"error":      0,
		"thinking":   0,
	}

	for _, evt := range d.events {
		switch evt.Type {
		case DebugEventAPI:
			stats["api"]++
		case DebugEventTool:
			stats["tool"]++
		case DebugEventPermission:
			stats["permission"]++
		case DebugEventError:
			stats["error"]++
		case DebugEventThinking:
			stats["thinking"]++
		}
	}

	return stats
}
