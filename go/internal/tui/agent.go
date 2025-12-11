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
	la          *agent.LearningAgent // Optional learning wrapper
	sess        *domain.Session
	store       *graphstore.Store
	prov        llm.Provider
	agentActive bool

	// Shared state (pointer so it survives model copies)
	shared *sharedState

	// Current tool being processed
	currentTool *toolCallInfo

	// Usage tracking (per-request)
	inputTokens  int
	outputTokens int
	thinkTokens  int

	// Session totals (accumulated)
	sessionInput  int
	sessionOutput int
	sessionThink  int

	// UI components
	viewport      viewport.Model
	toolsViewport viewport.Model // Separate viewport for tools
	input         textarea.Model
	spinner       spinner.Model
	filePicker    *FilePicker
	inputMode     inputMode
	width         int
	height        int
	mouseY        int // Track mouse Y position

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

	// Quit confirmation (double Ctrl+C within 3s)
	lastCtrlC time.Time
}

type toolCallInfo struct {
	name      string
	args      string
	output    string
	err       string
	collapsed bool
	done      bool
	// LLM-specific fields
	isLLMCall bool
	model     string
	prompt    string // The text/prompt sent to LLM
}

// Messages (prefixed to avoid conflict with tui.go)
type (
	agentStreamEventMsg domain.StreamEvent
	agentRunDoneMsg     struct{ err error }
	agentTickMsg        time.Time
)

// NewAgentModel creates a new agent TUI with pre-initialized components
func NewAgentModel(workDir string, ag *agent.Agent, la *agent.LearningAgent, store *graphstore.Store, prov llm.Provider) AgentModel {
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
		la:            la,
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

	case tea.MouseMsg:
		// Track mouse position
		m.mouseY = msg.Y
		
		// Calculate tools area position (after header, brain, and before main viewport)
		headerHeight := 2
		brainHeight := 3
		toolsStartY := headerHeight + brainHeight
		
		// Tools area is 1/3 of total height
		toolsHeight := m.height / 3
		toolsEndY := toolsStartY + toolsHeight
		
		// Handle mouse wheel scrolling based on position
		var cmd tea.Cmd
		if msg.Y >= toolsStartY && msg.Y < toolsEndY && len(*m.shared.toolCalls) > 0 {
			// Mouse is over tools area - scroll the tools viewport
			m.toolsViewport, cmd = m.toolsViewport.Update(msg)
		} else {
			// Mouse is over main output area - scroll the main viewport
			m.viewport, cmd = m.viewport.Update(msg)
		}
		return m, cmd

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
	// Handle help mode - any key closes it
	if m.inputMode == modeHelp {
		m.inputMode = modeChat
		return m, nil
	}

	switch msg.String() {
	case "ctrl+c":
		// If agent is active, cancel it first
		if m.agentActive && m.shared != nil && m.shared.cancelFunc != nil {
			m.shared.cancelFunc()
			m.agentActive = false
			m.shared.output.WriteString("\n\n" + agentErrorStyle.Render("⚠ Cancelled") + "\n")
			m.viewport.SetContent(m.renderOutput())
			m.lastCtrlC = time.Time{} // Reset quit timer
			return m, nil
		}
		// Double Ctrl+C within 3 seconds to quit
		now := time.Now()
		if !m.lastCtrlC.IsZero() && now.Sub(m.lastCtrlC) < 3*time.Second {
			m.quitting = true
			return m, tea.Quit
		}
		m.lastCtrlC = now
		m.shared.output.WriteString("\n" + agentStatusStyle.Render("Press Ctrl+C again to quit") + "\n")
		m.viewport.SetContent(m.renderOutput())
		m.viewport.GotoBottom()
		return m, nil

	case "esc":
		// Esc also requires double-tap
		now := time.Now()
		if !m.agentActive {
			if !m.lastCtrlC.IsZero() && now.Sub(m.lastCtrlC) < 3*time.Second {
				m.quitting = true
				return m, tea.Quit
			}
			m.lastCtrlC = now
			m.shared.output.WriteString("\n" + agentStatusStyle.Render("Press Esc again to quit") + "\n")
			m.viewport.SetContent(m.renderOutput())
			m.viewport.GotoBottom()
			return m, nil
		}

	case "q":
		// Block "q" from quitting - let it go to textarea
		if !m.agentActive && m.input.Focused() {
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}
		return m, nil // Ignore when agent active

	case "ctrl+h":
		// Show help overlay
		m.inputMode = modeHelp
		return m, nil

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

	case "ctrl+a":
		// Trigger file picker mode (Ctrl+A for Attach)
		if !m.agentActive {
			m.inputMode = modeFilePicker
			if m.filePicker == nil {
				m.filePicker = NewFilePicker(m.workDir, m.width-4, 10)
			}
			m.filePicker.LoadFiles()
			return m, nil
		}

	case "ctrl+s":
		// Trigger search mode (Ctrl+S for Search)
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

	case "ctrl+n":
		// Cycle through agents (Ctrl+N for Next agent)
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

	case "ctrl+u":
		m.viewport.HalfViewUp()
		return m, nil

	case "ctrl+f":
		m.viewport.ViewDown()
		return m, nil

	case "ctrl+b":
		m.viewport.ViewUp()
		return m, nil

	case "ctrl+g":
		// Go to top (Ctrl+G for Go)
		m.viewport.GotoTop()
		return m, nil

	case "ctrl+o":
		// Go to bottom (Ctrl+O for bOttom)
		m.viewport.GotoBottom()
		return m, nil

	case "ctrl+p":
		// Next search match
		if len(m.searchMatches) > 0 {
			m.searchIdx = (m.searchIdx + 1) % len(m.searchMatches)
			m.jumpToSearchMatch()
			return m, nil
		}

	case "ctrl+r":
		// Previous search match (Ctrl+R for Reverse)
		if len(m.searchMatches) > 0 {
			m.searchIdx--
			if m.searchIdx < 0 {
				m.searchIdx = len(m.searchMatches) - 1
			}
			m.jumpToSearchMatch()
			return m, nil
		}
	}

	// DEFAULT: pass unhandled keys to textarea when not agent active
	if !m.agentActive && m.input.Focused() {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
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
		return m, tea.Batch(m.spinner.Tick, runAgent(m.ag, m.la, m.store, m.workDir, prompt, m.shared.program, m.shared))
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

	// Calculate viewport sizes with fixed 1/3 height for tools area
	headerHeight := 2
	brainHeight := 3
	statusHeight := 1
	inputHeight := 5
	debugPanelHeight := 0

	// Debug panel height (if enabled)
	if m.debug != nil && m.debug.IsEnabled() {
		debugPanelHeight = 12 // Fixed height for debug panel
	}

	// Tools area is FIXED at 1/3 of total height
	toolsHeight := msg.Height / 3

	// Main viewport gets the remaining space
	vpWidth := msg.Width
	vpHeight := msg.Height - headerHeight - brainHeight - statusHeight - inputHeight - debugPanelHeight - toolsHeight
	if vpHeight < 5 {
		vpHeight = 5 // Minimum height
	}

	if !m.ready {
		// First time: create viewports
		m.viewport = viewport.New(vpWidth, vpHeight)
		// Disable default key bindings (q, etc) - we handle navigation ourselves
		m.viewport.KeyMap = viewport.KeyMap{} // Empty keymap disables all default bindings
		m.viewport.SetContent(m.renderOutput())
		
		// Create tools viewport with fixed 1/3 height
		m.toolsViewport = viewport.New(vpWidth, toolsHeight)
		m.toolsViewport.KeyMap = viewport.KeyMap{} // Disable default keys
		m.toolsViewport.MouseWheelEnabled = true    // Enable mouse wheel scrolling
		m.toolsViewport.SetContent(m.renderToolCallsSummary())
		
		m.ready = true
	} else {
		// Resize: adjust dimensions and re-wrap content
		m.viewport.Width = vpWidth
		m.viewport.Height = vpHeight
		// Force re-render with new width
		m.viewport.SetContent(m.renderOutput())
		
		// Also resize tools viewport
		m.toolsViewport.Width = vpWidth
		m.toolsViewport.Height = toolsHeight
		m.toolsViewport.SetContent(m.renderToolCallsSummary())
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

	// Accumulate session totals before resetting per-request counts
	m.sessionInput += m.inputTokens
	m.sessionOutput += m.outputTokens
	m.sessionThink += m.thinkTokens

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
		cmds = append(cmds, runAgent(m.ag, m.la, m.store, m.workDir, prompt, m.shared.program, m.shared))
	}

	return m, tea.Batch(cmds...)
}
