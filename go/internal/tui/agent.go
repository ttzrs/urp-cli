// Package tui provides the Bubble Tea interactive agent interface.
package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/joss/urp/internal/opencode/agent"
	"github.com/joss/urp/internal/opencode/domain"
	"github.com/joss/urp/internal/opencode/graphstore"
	"github.com/joss/urp/pkg/llm"
)

// Agent-specific styles (some shared with tui.go)
var (
	agentTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("205")).
			Padding(0, 1)

	thinkingStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Italic(true)

	textStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	toolStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("33")).
			Bold(true)

	toolOutputStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("244"))

	agentErrorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196"))

	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42"))

	ultrathinkStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("201")). // magenta/pink
			Bold(true)

	agentStatusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Background(lipgloss.Color("236")).
			Padding(0, 1)

	inputBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("62")).
				Padding(0, 1)

	focusedInputStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("205")).
				Padding(0, 1)

	ultrathinkInputStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("226")). // Yellow
				Padding(0, 1)

	ultrathinkBadgeStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("0")).
				Background(lipgloss.Color("226")). // Yellow bg, black text
				Bold(true).
				Padding(0, 1)
)

// sharedState holds state that needs to be shared across model copies
// strings.Builder CANNOT be copied after use, so it must be a pointer
type sharedState struct {
	program    *tea.Program
	cancelFunc context.CancelFunc
	output     *strings.Builder
	toolCalls  *[]toolCallInfo
}

// AgentModel is the main TUI model for the interactive agent
type AgentModel struct {
	// Core state
	workDir     string
	ready       bool
	initialized bool // agent init complete
	quitting    bool
	err         error

	// Agent state
	ag          *agent.Agent
	sess        *domain.Session
	store       *graphstore.Store
	prov        llm.Provider
	agentActive bool

	// Shared state (pointer so it survives model copies)
	shared *sharedState

	// Current tool being processed
	currentTool *toolCallInfo

	// Usage tracking
	inputTokens  int
	outputTokens int
	thinkTokens  int

	// UI components
	viewport   viewport.Model
	input      textarea.Model
	spinner    spinner.Model
	filePicker *FilePicker
	inputMode  inputMode
	width      int
	height     int

	// Pending prompt from slash commands
	pendingPrompt string

	// Agent cycling
	agentRegistry *agent.Registry
	currentAgent  string

	// BrainMonitor for cognitive state visualization
	brain BrainModel

	// Debug panel for interaction visualization
	debug *DebugPanel

	// Search state
	searchQuery   string
	searchMatches []int // line numbers with matches
	searchIdx     int   // current match index
}

type toolCallInfo struct {
	name      string
	args      string
	output    string
	err       string
	collapsed bool
	done      bool
}

// Messages (prefixed to avoid conflict with tui.go)
type (
	agentStreamEventMsg domain.StreamEvent
	agentRunDoneMsg     struct{ err error }
	agentTickMsg        time.Time
)

// NewAgentModel creates a new agent TUI with pre-initialized components
func NewAgentModel(workDir string, ag *agent.Agent, store *graphstore.Store, prov llm.Provider) AgentModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	ti := textarea.New()
	ti.Placeholder = "Enter your prompt... (Enter to send)"
	ti.CharLimit = 4000
	ti.SetWidth(80)
	ti.SetHeight(3)
	ti.Focus()

	// Initialize shared state with pointers to mutable data
	toolCalls := make([]toolCallInfo, 0)
	shared := &sharedState{
		output:    &strings.Builder{},
		toolCalls: &toolCalls,
	}

	return AgentModel{
		workDir:       workDir,
		ag:            ag,
		store:         store,
		prov:          prov,
		initialized:   true,
		shared:        shared,
		spinner:       s,
		input:         ti,
		agentRegistry: agent.DefaultRegistry(),
		currentAgent:  "code",
		brain:         NewBrainModel(200000), // 200k default context
		debug:         NewDebugPanel(100),    // Keep last 100 events
	}
}

// Init initializes the TUI
func (m AgentModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.brain.Init())
}

// Update handles messages
func (m AgentModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// Handle file picker mode separately
	if m.inputMode == modeFilePicker {
		return m.updateFilePicker(msg)
	}

	// Handle search mode
	if m.inputMode == modeSearch {
		return m.updateSearch(msg)
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKeyMsg(msg)

	case tea.WindowSizeMsg:
		return m.handleWindowSize(msg)

	case agentStreamEventMsg:
		event := domain.StreamEvent(msg)
		m.handleStreamEvent(event)
		m.viewport.SetContent(m.renderOutput())
		m.viewport.GotoBottom()
		return m, nil

	case agentRunDoneMsg:
		return m.handleRunDone(msg)

	case spinner.TickMsg:
		return m.handleSpinnerTick(msg)

	// Brain state messages
	case BrainTraumaMsg, BrainRecallMsg, BrainPruneMsg, BrainWriteMsg, BrainFocusMsg, BrainIdleMsg, TokenUpdateMsg:
		var brainCmd tea.Cmd
		m.brain, brainCmd = m.brain.Update(msg)
		cmds = append(cmds, brainCmd)
	}

	// Update textarea if not running
	if !m.agentActive {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		cmds = append(cmds, cmd)
	}

	// Update viewport
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m AgentModel) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		if m.agentActive && m.shared != nil && m.shared.cancelFunc != nil {
			m.shared.cancelFunc()
			m.agentActive = false
			m.shared.output.WriteString("\n\n" + agentErrorStyle.Render("⚠ Cancelled") + "\n")
			m.viewport.SetContent(m.renderOutput())
			return m, nil
		}
		m.quitting = true
		return m, tea.Quit

	case "esc":
		if !m.agentActive {
			m.quitting = true
			return m, tea.Quit
		}

	case "ctrl+d":
		// Toggle debug panel
		if m.debug != nil {
			m.debug.Toggle()
			m.debug.SetWidth(m.width)
			m.debug.AddSystem("Debug", fmt.Sprintf("Debug mode %s", map[bool]string{true: "enabled", false: "disabled"}[m.debug.IsEnabled()]))
		}
		return m, nil

	case "ctrl+e":
		// Toggle expand/collapse all debug events
		if m.debug != nil && m.debug.IsEnabled() {
			m.debug.ToggleAll()
		}
		return m, nil

	case "ctrl+x":
		// Clear debug panel
		if m.debug != nil && m.debug.IsEnabled() {
			m.debug.Clear()
			m.debug.AddSystem("Debug", "Events cleared")
		}
		return m, nil

	case "@":
		// Trigger file picker mode
		if !m.agentActive {
			m.inputMode = modeFilePicker
			if m.filePicker == nil {
				m.filePicker = NewFilePicker(m.workDir, m.width-4, 10)
			}
			m.filePicker.LoadFiles()
			return m, nil
		}

	case "/":
		// Trigger search mode (vim-style)
		if !m.agentActive {
			m.inputMode = modeSearch
			m.searchQuery = ""
			m.searchMatches = nil
			m.searchIdx = 0
			return m, nil
		}

	case "enter":
		return m.handleEnterKey()

	case "alt+enter", "ctrl+j":
		// Alt+Enter or Ctrl+J inserts newline
		if !m.agentActive {
			m.input.SetValue(m.input.Value() + "\n")
			return m, nil
		}

	case "ctrl+l":
		// Clear output
		m.shared.output.Reset()
		*m.shared.toolCalls = []toolCallInfo{}
		m.viewport.SetContent("")

	case "ctrl+t":
		// Toggle tool call collapse
		if len(*m.shared.toolCalls) > 0 {
			for i := range *m.shared.toolCalls {
				(*m.shared.toolCalls)[i].collapsed = !(*m.shared.toolCalls)[i].collapsed
			}
			m.viewport.SetContent(m.renderOutput())
		}

	case "tab":
		// Cycle through agents when not active
		if !m.agentActive && m.agentRegistry != nil {
			m.cycleAgent()
		}

	case "up", "down", "pgup", "pgdown":
		// Viewport scrolling when agent active
		if m.agentActive {
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}

	// Vim-style navigation (always works for viewport)
	case "j":
		if m.agentActive || m.viewport.AtBottom() {
			m.viewport.LineDown(1)
		} else if !m.input.Focused() {
			m.viewport.LineDown(1)
		}
		return m, nil

	case "k":
		if m.agentActive || m.viewport.AtTop() {
			m.viewport.LineUp(1)
		} else if !m.input.Focused() {
			m.viewport.LineUp(1)
		}
		return m, nil

	case "g":
		if m.agentActive || !m.input.Focused() {
			m.viewport.GotoTop()
		}
		return m, nil

	case "G":
		if m.agentActive || !m.input.Focused() {
			m.viewport.GotoBottom()
		}
		return m, nil

	case "ctrl+u":
		if m.agentActive || !m.input.Focused() {
			m.viewport.HalfViewUp()
		}
		return m, nil

	case "ctrl+f":
		if m.agentActive || !m.input.Focused() {
			m.viewport.ViewDown()
		}
		return m, nil

	case "ctrl+b":
		if m.agentActive || !m.input.Focused() {
			m.viewport.ViewUp()
		}
		return m, nil

	case "n":
		// Next search match (vim style)
		if len(m.searchMatches) > 0 && !m.input.Focused() {
			m.searchIdx = (m.searchIdx + 1) % len(m.searchMatches)
			m.jumpToSearchMatch()
		}
		return m, nil

	case "N":
		// Previous search match (vim style)
		if len(m.searchMatches) > 0 && !m.input.Focused() {
			m.searchIdx--
			if m.searchIdx < 0 {
				m.searchIdx = len(m.searchMatches) - 1
			}
			m.jumpToSearchMatch()
		}
		return m, nil
	}

	return m, nil
}

func (m AgentModel) handleEnterKey() (tea.Model, tea.Cmd) {
	// Enter sends message (if not empty and not running)
	if !m.agentActive && strings.TrimSpace(m.input.Value()) != "" {
		prompt := m.input.Value()

		// Check for slash commands
		if isSlashCommand(prompt) {
			m.input.SetValue("")
			result := executeSlashCommand(&m, prompt)
			if result != "" {
				m.shared.output.WriteString(result + "\n")
				m.viewport.SetContent(m.renderOutput())
			}
			return m, nil
		}

		m.input.SetValue("")
		m.agentActive = true
		m.shared.output.Reset()

		// Detect ultrathink and show colored indicator
		if strings.Contains(strings.ToLower(prompt), "ultrathink") {
			m.shared.output.WriteString(ultrathinkStyle.Render("🧠 ULTRATHINK enabled (10k tokens)") + "\n")
		}

		m.shared.output.WriteString(thinkingStyle.Render("⏳ Thinking...") + "\n")
		m.viewport.SetContent(m.renderOutput())
		*m.shared.toolCalls = []toolCallInfo{}
		m.currentTool = nil
		m.inputTokens = 0
		m.outputTokens = 0
		m.thinkTokens = 0
		return m, tea.Batch(m.spinner.Tick, runAgent(m.ag, m.store, m.workDir, prompt, m.shared.program, m.shared))
	}
	// If empty, let textarea handle it (newline)
	if !m.agentActive && strings.TrimSpace(m.input.Value()) == "" {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(tea.KeyMsg{Type: tea.KeyEnter})
		return m, cmd
	}
	return m, nil
}

func (m AgentModel) handleWindowSize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = msg.Width
	m.height = msg.Height

	// Calculate viewport size (header + brain monitor + status bar + input area)
	headerHeight := 2
	brainHeight := 3 // BrainMonitor takes ~3 lines
	statusHeight := 1
	inputHeight := 5
	vpWidth := msg.Width
	vpHeight := msg.Height - headerHeight - brainHeight - statusHeight - inputHeight

	if !m.ready {
		// First time: create viewport
		m.viewport = viewport.New(vpWidth, vpHeight)
		m.viewport.SetContent(m.renderOutput())
		m.ready = true
	} else {
		// Resize: adjust dimensions and re-wrap content
		m.viewport.Width = vpWidth
		m.viewport.Height = vpHeight
		// Force re-render with new width
		m.viewport.SetContent(m.renderOutput())
	}

	// Adjust input width
	m.input.SetWidth(msg.Width - 4)

	// Update file picker width if it exists
	if m.filePicker != nil {
		m.filePicker = NewFilePicker(m.workDir, m.width-4, 10)
	}

	return m, nil
}

func (m AgentModel) handleRunDone(msg agentRunDoneMsg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	m.agentActive = false
	if msg.err != nil {
		m.shared.output.WriteString("\n" + agentErrorStyle.Render(fmt.Sprintf("Error: %v", msg.err)) + "\n")
		// Trigger brain trauma on error
		var brainCmd tea.Cmd
		m.brain, brainCmd = m.brain.Update(BrainTraumaMsg{Err: msg.err})
		cmds = append(cmds, brainCmd)
	} else {
		m.shared.output.WriteString("\n" + successStyle.Render("✓ Done") + "\n")
		// Return brain to idle
		var brainCmd tea.Cmd
		m.brain, brainCmd = m.brain.Update(BrainIdleMsg{})
		cmds = append(cmds, brainCmd)
	}
	m.viewport.SetContent(m.renderOutput())
	m.viewport.GotoBottom()

	return m, tea.Batch(cmds...)
}

func (m AgentModel) handleSpinnerTick(msg spinner.TickMsg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(msg)
	cmds = append(cmds, cmd)

	// Also update brain spinner
	var brainCmd tea.Cmd
	m.brain, brainCmd = m.brain.Update(msg)
	cmds = append(cmds, brainCmd)

	// Check for pending prompt from slash commands
	if m.pendingPrompt != "" && !m.agentActive && m.ag != nil {
		prompt := m.pendingPrompt
		m.pendingPrompt = ""
		m.agentActive = true
		m.shared.output.Reset()
		*m.shared.toolCalls = []toolCallInfo{}
		m.currentTool = nil
		m.inputTokens = 0
		m.outputTokens = 0
		m.thinkTokens = 0
		cmds = append(cmds, runAgent(m.ag, m.store, m.workDir, prompt, m.shared.program, m.shared))
	}

	return m, tea.Batch(cmds...)
}
