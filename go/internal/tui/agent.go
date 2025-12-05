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

// queuePrompt sets a pending prompt to be executed on next tick
func (m *AgentModel) queuePrompt(prompt string) {
	m.pendingPrompt = prompt
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

// Update handles messages
func (m AgentModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// Handle file picker mode separately
	if m.inputMode == modeFilePicker {
		return m.updateFilePicker(msg)
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
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

		case "enter":
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
				m.input, cmd = m.input.Update(msg)
				return m, cmd
			}

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
		}

	case tea.WindowSizeMsg:
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

	case agentStreamEventMsg:
		event := domain.StreamEvent(msg)
		m.handleStreamEvent(event)
		m.viewport.SetContent(m.renderOutput())
		m.viewport.GotoBottom()
		return m, nil

	case agentRunDoneMsg:
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

	case spinner.TickMsg:
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

func (m *AgentModel) handleStreamEvent(event domain.StreamEvent) {
	switch event.Type {
	case domain.StreamEventThinking:
		m.shared.output.WriteString(thinkingStyle.Render(event.Content))
		if event.Usage != nil {
			m.thinkTokens += event.Usage.OutputTokens
			// Debug: Log thinking tokens
			if m.debug != nil && m.debug.IsEnabled() {
				preview := event.Content
				if len(preview) > 100 {
					preview = preview[:97] + "..."
				}
				m.debug.AddThinking(preview, event.Usage.OutputTokens)
			}
		}
		// Brain: Focus state when thinking
		m.brain, _ = m.brain.Update(BrainFocusMsg{Task: "Thinking..."})

	case domain.StreamEventText:
		m.shared.output.WriteString(textStyle.Render(event.Content))
		if event.Usage != nil {
			m.outputTokens += event.Usage.OutputTokens
		}

	case domain.StreamEventToolCall:
		if tc, ok := event.Part.(domain.ToolCallPart); ok {
			info := toolCallInfo{
				name:      tc.Name,
				args:      truncateArgsMap(tc.Args),
				collapsed: true,
			}
			*m.shared.toolCalls = append(*m.shared.toolCalls, info)
			m.currentTool = &(*m.shared.toolCalls)[len(*m.shared.toolCalls)-1]
			m.shared.output.WriteString("\n" + toolStyle.Render(fmt.Sprintf("▶ %s", tc.Name)) + "\n")

			// Debug: Log tool call start
			if m.debug != nil && m.debug.IsEnabled() {
				m.debug.AddEvent(DebugEvent{
					Type:    DebugEventTool,
					Title:   fmt.Sprintf("Tool Start: %s", tc.Name),
					Content: truncateArgsMap(tc.Args),
				})
			}

			// Brain: Show tool activity
			switch tc.Name {
			case "write", "edit", "multi_edit", "patch":
				path := getToolPath(tc.Args)
				m.brain, _ = m.brain.Update(BrainWriteMsg{Path: path})
			case "grep", "glob", "read":
				m.brain, _ = m.brain.Update(BrainRecallMsg{Context: tc.Name})
			default:
				m.brain, _ = m.brain.Update(BrainFocusMsg{Task: tc.Name})
			}
		}

	case domain.StreamEventToolDone:
		if tc, ok := event.Part.(domain.ToolCallPart); ok {
			if m.currentTool != nil {
				m.currentTool.output = truncateOutput(tc.Result)
				m.currentTool.err = tc.Error
				m.currentTool.done = true

				// Debug: Log tool completion
				if m.debug != nil && m.debug.IsEnabled() {
					m.debug.AddTool(tc.Name, tc.Args, tc.Result, tc.Error, tc.Duration)
				}

				if tc.Error != "" {
					m.shared.output.WriteString(agentErrorStyle.Render(fmt.Sprintf("  ✗ %s\n", tc.Error)))
					// Brain: Trauma on tool error
					m.brain, _ = m.brain.Update(BrainTraumaMsg{Err: fmt.Errorf("%s", tc.Error)})
				} else {
					m.shared.output.WriteString(successStyle.Render("  ✓\n"))
				}
			}
		}

	case domain.StreamEventError:
		m.shared.output.WriteString(agentErrorStyle.Render(fmt.Sprintf("\nError: %v\n", event.Error)))
		// Debug: Log error
		if m.debug != nil && m.debug.IsEnabled() {
			m.debug.AddError("Stream", event.Error.Error())
		}
		// Brain: Trauma on error
		m.brain, _ = m.brain.Update(BrainTraumaMsg{Err: event.Error})

	case domain.StreamEventUsage:
		if event.Usage != nil {
			m.inputTokens = event.Usage.InputTokens
			m.outputTokens = event.Usage.OutputTokens

			// Debug: Log LLM usage (this is critical!)
			if m.debug != nil && m.debug.IsEnabled() {
				model := "unknown"
				if m.ag != nil {
					model = m.ag.Model()
				}
				m.debug.AddEvent(DebugEvent{
					Type:  DebugEventAPI,
					Title: fmt.Sprintf("LLM Call: %s", model),
					Content: fmt.Sprintf("Input: %d tokens\nOutput: %d tokens\nCache Read: %d\nCache Write: %d\nCost: $%.4f",
						event.Usage.InputTokens,
						event.Usage.OutputTokens,
						event.Usage.CacheRead,
						event.Usage.CacheWrite,
						event.Usage.TotalCost),
					Metadata: map[string]string{
						"model":        model,
						"input_tokens": fmt.Sprintf("%d", event.Usage.InputTokens),
						"output_tokens": fmt.Sprintf("%d", event.Usage.OutputTokens),
						"total_cost":   fmt.Sprintf("$%.4f", event.Usage.TotalCost),
					},
				})
			}

			// Brain: Update token usage for progress bar
			totalTokens := m.inputTokens + m.outputTokens + m.thinkTokens
			m.brain, _ = m.brain.Update(TokenUpdateMsg{Current: totalTokens, Max: m.brain.MaxTokens})
		}

	case domain.StreamEventPermissionAsk:
		// Debug: Log permission request
		if m.debug != nil && m.debug.IsEnabled() && event.PermissionReq != nil {
			m.debug.AddPermission(
				event.PermissionReq.Tool,
				event.PermissionReq.Command,
				event.PermissionReq.Path,
				"asking...",
			)
		}
	}
}

// getToolPath extracts path from tool args
func getToolPath(args map[string]any) string {
	if p, ok := args["path"].(string); ok {
		return p
	}
	if p, ok := args["file_path"].(string); ok {
		return p
	}
	return ""
}

func (m AgentModel) renderOutput() string {
	return m.shared.output.String()
}

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

	// Input area or file picker
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
		parts = append(parts, "Ctrl+C: cancel │ ↑↓: scroll │ Ctrl+D: debug")
	} else {
		parts = append(parts, "Enter: send │ @: files │ Tab: agent │ Ctrl+D: debug │ Esc: quit")
	}

	return agentStatusStyle.Width(m.width).Render(strings.Join(parts, " │ "))
}

// Helpers

func truncateArgsMap(args map[string]any) string {
	if args == nil {
		return ""
	}
	var parts []string
	for k, v := range args {
		parts = append(parts, fmt.Sprintf("%s=%v", k, v))
	}
	s := strings.Join(parts, ", ")
	if len(s) > 100 {
		return s[:97] + "..."
	}
	return s
}

func truncateOutput(s string) string {
	if len(s) > 500 {
		return s[:497] + "..."
	}
	return s
}
