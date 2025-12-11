package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	urpstrings "github.com/joss/urp/internal/strings"
)

// View renders the TUI
func (m AgentModel) View() string {
	if m.quitting {
		return "Goodbye!\n"
	}

	if !m.ready {
		return fmt.Sprintf("\n  %s Initializing...", m.spinner.View())
	}

	var b strings.Builder

	// Header with BrainMonitor
	header := agentTitleStyle.Render("⚡ URP Agent") + "  " +
		lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render(m.workDir)
	if m.debug != nil && m.debug.IsEnabled() {
		header += "  " + lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true).Render("[DEBUG]")
	}
	b.WriteString(header + "\n")

	// BrainMonitor - cognitive state + token progress bar (CTX bars)
	b.WriteString(m.brain.View() + "\n\n")

	// Tool calls area - FIXED 1/3 height with scrollable viewport
	if len(*m.shared.toolCalls) > 0 {
		// Update tools viewport content
		m.toolsViewport.SetContent(m.renderToolCallsSummary())
		b.WriteString(m.toolsViewport.View() + "\n")
	}

	// Debug panel (if enabled, shown above output)
	if m.debug != nil && m.debug.IsEnabled() {
		b.WriteString(m.debug.View(12) + "\n")
	}

	// Main viewport (output area)
	b.WriteString(m.viewport.View())
	b.WriteString("\n")

	// Status bar
	status := m.renderStatus()
	b.WriteString(status + "\n")

	// Input area, file picker, or search
	b.WriteString(m.renderInputArea())

	return b.String()
}

func (m AgentModel) renderInputArea() string {
	var b strings.Builder

	if m.inputMode == modeHelp {
		// Show help overlay with all keyboard shortcuts
		b.WriteString(m.renderHelpOverlay())
		return b.String()
	}

	if m.inputMode == modeFilePicker && m.filePicker != nil {
		// Show file picker overlay
		pickerStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("205")).
			Padding(0, 1).
			Width(m.width - 4)
		b.WriteString(pickerStyle.Render(m.filePicker.View()))
		b.WriteString("\n")
		b.WriteString(thinkingStyle.Render("  ↑↓: navigate │ Enter: select │ Esc: cancel"))
	} else if m.inputMode == modeSearch {
		// Show search input
		searchStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("226")). // Yellow for search
			Padding(0, 1).
			Width(m.width - 4)
		matchInfo := ""
		if m.searchQuery != "" {
			if len(m.searchMatches) > 0 {
				matchInfo = fmt.Sprintf(" [%d/%d]", m.searchIdx+1, len(m.searchMatches))
			} else {
				matchInfo = " [no matches]"
			}
		}
		searchContent := fmt.Sprintf("/ %s%s", m.searchQuery, matchInfo)
		b.WriteString(searchStyle.Render(searchContent))
		b.WriteString("\n")
		b.WriteString(thinkingStyle.Render("  Type to search │ Enter: confirm │ Esc: cancel │ Ctrl+P/R: next/prev"))
	} else if m.agentActive {
		// Show more context during agent activity
		activity := "Processing..."
		if len(*m.shared.toolCalls) > 0 {
			lastTool := (*m.shared.toolCalls)[len(*m.shared.toolCalls)-1]
			if !lastTool.done {
				activity = fmt.Sprintf("Running: %s", lastTool.name)
			}
		}
		b.WriteString(fmt.Sprintf("  %s %s (Ctrl+C to cancel)", m.spinner.View(), activity))
	} else {
		// Check if ultrathink is typed - show yellow indicator
		hasUltrathink := strings.Contains(strings.ToLower(m.input.Value()), "ultrathink")

		var inputBox string
		if hasUltrathink {
			// Yellow border + badge when ultrathink detected
			inputBox = ultrathinkInputStyle.Width(m.width - 4).Render(m.input.View())
			b.WriteString(ultrathinkBadgeStyle.Render("🧠 ULTRATHINK") + " ")
		} else if m.input.Focused() {
			inputBox = focusedInputStyle.Width(m.width - 4).Render(m.input.View())
		} else {
			inputBox = inputBorderStyle.Width(m.width - 4).Render(m.input.View())
		}
		b.WriteString(inputBox)
	}

	return b.String()
}

func (m AgentModel) renderStatus() string {
	var parts []string

	// Current agent
	if m.currentAgent != "" {
		parts = append(parts, toolStyle.Render("▸ "+m.currentAgent))
	}

	// Connection status
	if m.store != nil {
		parts = append(parts, successStyle.Render("●")+" Graph")
	} else {
		parts = append(parts, agentErrorStyle.Render("○")+" Graph")
	}

	// Token usage - current request
	if m.inputTokens > 0 || m.outputTokens > 0 {
		tokens := fmt.Sprintf("In:%d Out:%d", m.inputTokens, m.outputTokens)
		if m.thinkTokens > 0 {
			tokens += fmt.Sprintf(" Th:%d", m.thinkTokens)
		}
		parts = append(parts, tokens)
	}

	// Session total tokens
	sessionTotal := m.sessionInput + m.sessionOutput + m.sessionThink
	if sessionTotal > 0 {
		parts = append(parts, fmt.Sprintf("Session:%dk", sessionTotal/1000))
	}

	// Tool calls count
	if len(*m.shared.toolCalls) > 0 {
		parts = append(parts, fmt.Sprintf("Tools:%d", len(*m.shared.toolCalls)))
	}

	// Help
	if m.agentActive {
		parts = append(parts, "Ctrl+C: cancel │ ↑↓: scroll │ Ctrl+H: help")
	} else {
		parts = append(parts, "Enter: send │ Ctrl+A: files │ Ctrl+H: help")
	}

	return agentStatusStyle.Width(m.width).Render(strings.Join(parts, " │ "))
}

func (m AgentModel) renderOutput() string {
	var b strings.Builder

	// Render text output only
	textContent := m.shared.output.String()
	b.WriteString(textContent)

	content := b.String()
	if m.width > 4 {
		content = urpstrings.WordWrap(content, m.width-4)
	}
	return content
}

// renderToolCallsSummary renders the tool calls summary section for the top
func (m AgentModel) renderToolCallsSummary() string {
	var b strings.Builder

	// Count completed/running tools
	completed := 0
	running := 0
	for _, tc := range *m.shared.toolCalls {
		if tc.done {
			completed++
		} else {
			running++
		}
	}

	// Tool summary header
	summary := fmt.Sprintf("─── Tools: %d completed", completed)
	if running > 0 {
		summary += fmt.Sprintf(", %d running", running)
	}
	summary += " (Ctrl+T to expand/collapse) ───"
	b.WriteString(thinkingStyle.Render(summary) + "\n")

	// Render each tool call
	for i, tc := range *m.shared.toolCalls {
		b.WriteString(m.renderToolCall(i, tc))
	}

	content := b.String()
	if m.width > 4 {
		content = urpstrings.WordWrap(content, m.width-4)
	}
	return content
}

// renderToolCall renders a single tool call with expand/collapse
func (m AgentModel) renderToolCall(index int, tc toolCallInfo) string {
	var b strings.Builder

	// Icon based on collapse state
	icon := "▶"
	if !tc.collapsed {
		icon = "▼"
	}

	// Status indicator
	var statusIcon string
	var statusStyle lipgloss.Style
	if !tc.done {
		statusIcon = "⏳"
		statusStyle = thinkingStyle
	} else if tc.err != "" {
		statusIcon = "✗"
		statusStyle = agentErrorStyle
	} else {
		statusIcon = "✓"
		statusStyle = successStyle
	}

	// Header line: ▶ tool_name ✓ [model for LLM calls]
	header := fmt.Sprintf("%s %s %s", icon, toolStyle.Render(tc.name), statusStyle.Render(statusIcon))
	if tc.isLLMCall && tc.model != "" {
		header += " " + lipgloss.NewStyle().Foreground(lipgloss.Color("99")).Render("("+tc.model+")")
	}
	b.WriteString(header + "\n")

	// Expanded content
	if !tc.collapsed {
		// For LLM calls, show prompt info
		if tc.isLLMCall {
			if tc.prompt != "" {
				b.WriteString(toolOutputStyle.Render("  "+tc.prompt) + "\n")
			}
		} else {
			// For regular tool calls, show Args
			if tc.args != "" {
				b.WriteString(toolOutputStyle.Render("  Args: "+tc.args) + "\n")
			}
		}

		// Output (truncated for display)
		if tc.output != "" {
			out := tc.output
			if len(out) > 500 {
				out = out[:497] + "..."
			}
			lines := strings.Split(out, "\n")
			if !tc.isLLMCall {
				b.WriteString(toolOutputStyle.Render("  Output:") + "\n")
			}
			for _, line := range lines {
				if len(line) > m.width-8 {
					line = line[:m.width-11] + "..."
				}
				b.WriteString(toolOutputStyle.Render("    "+line) + "\n")
			}
		}

		// Error
		if tc.err != "" {
			b.WriteString(agentErrorStyle.Render("  Error: "+tc.err) + "\n")
		}
	}

	return b.String()
}

// Helpers - delegate to urpstrings

func truncateArgsMap(args map[string]any) string {
	return urpstrings.TruncateMap(args, 100)
}

func truncateOutput(s string) string {
	return urpstrings.Truncate(s, 500)
}

// renderHelpOverlay renders the keyboard shortcuts help panel
func (m AgentModel) renderHelpOverlay() string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("205")).
		Align(lipgloss.Center)

	keyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("33")).
		Bold(true)

	descStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252"))

	sectionStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("226")).
		Bold(true)

	shortcuts := []struct {
		section string
		items   [][2]string
	}{
		{
			section: "Input",
			items: [][2]string{
				{"Enter", "Send message"},
				{"Ctrl+J", "Insert newline"},
				{"Ctrl+A", "Attach file"},
				{"Ctrl+N", "Cycle agent"},
				{"Esc", "Quit"},
			},
		},
		{
			section: "Navigation",
			items: [][2]string{
				{"Ctrl+S", "Search in output"},
				{"Ctrl+P", "Next match"},
				{"Ctrl+R", "Previous match"},
				{"Ctrl+G", "Go to top"},
				{"Ctrl+O", "Go to bottom"},
			},
		},
		{
			section: "View",
			items: [][2]string{
				{"Ctrl+U", "Half page up"},
				{"Ctrl+F", "Page down"},
				{"Ctrl+B", "Page up"},
				{"Ctrl+L", "Clear output"},
				{"Ctrl+T", "Toggle tools"},
			},
		},
		{
			section: "Debug",
			items: [][2]string{
				{"Ctrl+D", "Toggle debug panel"},
				{"Ctrl+E", "Expand/collapse all"},
				{"Ctrl+X", "Clear debug"},
			},
		},
		{
			section: "Agent Running",
			items: [][2]string{
				{"Ctrl+C", "Cancel"},
				{"Up/Down", "Scroll output"},
			},
		},
	}

	var content strings.Builder
	content.WriteString(titleStyle.Width(m.width - 8).Render("Keyboard Shortcuts") + "\n\n")

	for _, section := range shortcuts {
		content.WriteString(sectionStyle.Render(section.section) + "\n")
		for _, item := range section.items {
			key := keyStyle.Width(12).Render(item[0])
			desc := descStyle.Render(item[1])
			content.WriteString(fmt.Sprintf("  %s %s\n", key, desc))
		}
		content.WriteString("\n")
	}

	content.WriteString(thinkingStyle.Render("Press any key to close"))

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("205")).
		Padding(1, 2).
		Width(m.width - 4)

	return boxStyle.Render(content.String())
}
