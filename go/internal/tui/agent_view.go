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

	// BrainMonitor - cognitive state + token progress bar
	b.WriteString(m.brain.View() + "\n\n")

	// Calculate available height for content
	headerHeight := 2
	brainHeight := 3
	statusHeight := 1
	inputHeight := 5
	debugHeight := 0

	// Debug panel (if enabled, takes bottom portion)
	if m.debug != nil && m.debug.IsEnabled() {
		debugHeight = 12 // Fixed height for debug panel
	}

	// Main viewport (reduced if debug is on)
	vpHeight := m.height - headerHeight - brainHeight - statusHeight - inputHeight - debugHeight
	if vpHeight < 5 {
		vpHeight = 5
	}
	// Temporarily adjust viewport height
	oldHeight := m.viewport.Height
	if m.viewport.Height != vpHeight {
		// Note: we can't modify viewport here as View() is immutable
		// The resize happens in Update on WindowSizeMsg
	}
	_ = oldHeight // suppress unused warning

	b.WriteString(m.viewport.View())
	b.WriteString("\n")

	// Debug panel (between viewport and status)
	if m.debug != nil && m.debug.IsEnabled() {
		b.WriteString(m.debug.View(debugHeight) + "\n")
	}

	// Status bar
	status := m.renderStatus()
	b.WriteString(status + "\n")

	// Input area, file picker, or search
	b.WriteString(m.renderInputArea())

	return b.String()
}

func (m AgentModel) renderInputArea() string {
	var b strings.Builder

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
		b.WriteString(thinkingStyle.Render("  Type to search │ Enter: confirm │ Esc: cancel │ n/N: next/prev match"))
	} else if m.agentActive {
		b.WriteString(fmt.Sprintf("  %s Running...", m.spinner.View()))
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

	// Token usage
	if m.inputTokens > 0 || m.outputTokens > 0 {
		tokens := fmt.Sprintf("In:%d Out:%d", m.inputTokens, m.outputTokens)
		if m.thinkTokens > 0 {
			tokens += fmt.Sprintf(" Think:%d", m.thinkTokens)
		}
		parts = append(parts, tokens)
	}

	// Tool calls count
	if len(*m.shared.toolCalls) > 0 {
		parts = append(parts, fmt.Sprintf("Tools:%d", len(*m.shared.toolCalls)))
	}

	// Help
	if m.agentActive {
		parts = append(parts, "Ctrl+C: cancel │ j/k: scroll │ g/G: top/bottom │ Ctrl+D: debug")
	} else {
		parts = append(parts, "Enter: send │ @: files │ /: search │ j/k: scroll │ Esc: quit")
	}

	return agentStatusStyle.Width(m.width).Render(strings.Join(parts, " │ "))
}

func (m AgentModel) renderOutput() string {
	content := m.shared.output.String()
	// Wrap text to viewport width for responsive display
	if m.width > 4 {
		content = urpstrings.WordWrap(content, m.width-4)
	}
	return content
}

// Helpers - delegate to urpstrings

func truncateArgsMap(args map[string]any) string {
	return urpstrings.TruncateMap(args, 100)
}

func truncateOutput(s string) string {
	return urpstrings.Truncate(s, 500)
}
