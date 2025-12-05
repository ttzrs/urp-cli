package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// updateFilePicker handles input when in file picker mode
func (m AgentModel) updateFilePicker(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEsc:
			// Cancel file picker
			m.inputMode = modeChat
			return m, nil

		case tea.KeyEnter:
			// Select file and insert into input
			if path, ok := m.filePicker.SelectedItem(); ok {
				current := m.input.Value()
				m.input.SetValue(current + "@" + path + " ")
			}
			m.inputMode = modeChat
			return m, nil
		}
	}

	// Forward other messages to file picker
	var cmd tea.Cmd
	m.filePicker, cmd = m.filePicker.Update(msg)
	return m, cmd
}

// updateSearch handles input when in search mode
func (m AgentModel) updateSearch(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEsc:
			// Cancel search
			m.inputMode = modeChat
			m.searchQuery = ""
			return m, nil

		case tea.KeyEnter:
			// Execute search and exit search mode
			m.performSearch()
			m.inputMode = modeChat
			if len(m.searchMatches) > 0 {
				m.jumpToSearchMatch()
			}
			return m, nil

		case tea.KeyBackspace:
			if len(m.searchQuery) > 0 {
				m.searchQuery = m.searchQuery[:len(m.searchQuery)-1]
				m.performSearch() // Live search as you type
			}
			return m, nil

		default:
			// Add character to search query
			if msg.Type == tea.KeyRunes {
				m.searchQuery += string(msg.Runes)
				m.performSearch() // Live search as you type
			}
			return m, nil
		}
	}
	return m, nil
}

// performSearch finds all matching lines in the output
func (m *AgentModel) performSearch() {
	m.searchMatches = nil
	m.searchIdx = 0

	if m.searchQuery == "" || m.shared == nil {
		return
	}

	content := m.shared.output.String()
	lines := strings.Split(content, "\n")
	queryLower := strings.ToLower(m.searchQuery)

	for i, line := range lines {
		if strings.Contains(strings.ToLower(line), queryLower) {
			m.searchMatches = append(m.searchMatches, i)
		}
	}
}

// jumpToSearchMatch scrolls viewport to current match
func (m *AgentModel) jumpToSearchMatch() {
	if len(m.searchMatches) == 0 {
		return
	}

	lineNum := m.searchMatches[m.searchIdx]
	// Scroll to the line (approximately)
	m.viewport.SetYOffset(lineNum)
}

// cycleAgent cycles through available agents
func (m *AgentModel) cycleAgent() {
	if m.agentRegistry == nil {
		return
	}

	agents := m.agentRegistry.Names()
	if len(agents) == 0 {
		return
	}

	// Find current index
	currentIdx := 0
	for i, name := range agents {
		if name == m.currentAgent {
			currentIdx = i
			break
		}
	}

	// Cycle to next
	nextIdx := (currentIdx + 1) % len(agents)
	m.currentAgent = agents[nextIdx]
}

// queuePrompt sets a pending prompt to be executed on next tick
func (m *AgentModel) queuePrompt(prompt string) {
	m.pendingPrompt = prompt
}
