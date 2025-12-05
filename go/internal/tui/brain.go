// Package tui provides the BrainMonitor component for cognitive state visualization
package tui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// CognitiveState represents the current state of the AI agent's cognition
type CognitiveState int

const (
	StateIdle    CognitiveState = iota // Default waiting state
	StateFocus                         // Thinking/Generating code
	StateTrauma                        // Error detected (Pain)
	StateRecall                        // Searching memory (Graph/Vector)
	StatePruning                       // Forgetting noise (Compaction)
	StateWrite                         // Writing to disk
)

// BrainMonitor styles
var (
	brainStyleIdle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#555555"))

	brainStyleFocus = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")).
			Bold(true)

	brainStyleTrauma = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FF0000")).
				Bold(true)

	brainStyleRecall = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#00FFFF")).
				Italic(true)

	brainStylePruning = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#00FF00"))

	brainStyleWrite = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFF00")).
			Bold(true)

	brainContainerStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#444444")).
				Padding(0, 1)

	// Progress bar colors
	barColorSafe   = lipgloss.Color("#00FF00") // Green
	barColorWarn   = lipgloss.Color("#FFFF00") // Yellow
	barColorDanger = lipgloss.Color("#FF0000") // Red
	barColorEmpty  = lipgloss.Color("#333333") // Dark gray
)

// BrainModel is the TUI component for cognitive state visualization
type BrainModel struct {
	State        CognitiveState
	Spinner      spinner.Model
	Message      string
	LastActivity time.Time

	// Token metrics
	UsedTokens int
	MaxTokens  int

	// Context window tracking
	ContextExpanded bool // True if context expanded due to error
}

// Messages for brain state updates
type (
	BrainTraumaMsg struct{ Err error }
	BrainRecallMsg struct{ Context string }
	BrainPruneMsg  struct{ TokensFreed int }
	BrainWriteMsg  struct{ Path string }
	BrainIdleMsg   struct{}
	BrainFocusMsg  struct{ Task string }
	TokenUpdateMsg struct {
		Current int
		Max     int
	}
)

// NewBrainModel creates a new brain monitor component
func NewBrainModel(maxTokens int) BrainModel {
	s := spinner.New()
	s.Spinner = spinner.Pulse
	s.Style = brainStyleIdle
	return BrainModel{
		State:     StateIdle,
		Spinner:   s,
		Message:   "SYSTEM READY via URP Protocol...",
		MaxTokens: maxTokens,
	}
}

// Init initializes the brain spinner
func (m BrainModel) Init() tea.Cmd {
	return m.Spinner.Tick
}

// Update handles brain state changes
func (m BrainModel) Update(msg tea.Msg) (BrainModel, tea.Cmd) {
	var cmd tea.Cmd
	m.Spinner, cmd = m.Spinner.Update(msg)

	switch msg := msg.(type) {
	case BrainTraumaMsg:
		m.State = StateTrauma
		errStr := "unknown error"
		if msg.Err != nil {
			errStr = msg.Err.Error()
			if len(errStr) > 50 {
				errStr = errStr[:47] + "..."
			}
		}
		m.Message = fmt.Sprintf("⚡ TRAUMA: %s", errStr)
		m.Spinner.Style = brainStyleTrauma
		m.ContextExpanded = true // Elastic context: expand on error
		// Return to idle after 4 seconds
		return m, tea.Batch(cmd, tea.Tick(4*time.Second, func(t time.Time) tea.Msg {
			return BrainIdleMsg{}
		}))

	case BrainRecallMsg:
		m.State = StateRecall
		ctx := msg.Context
		if len(ctx) > 40 {
			ctx = ctx[:37] + "..."
		}
		m.Message = fmt.Sprintf("🧠 RECALL: %s", ctx)
		m.Spinner.Style = brainStyleRecall
		return m, cmd

	case BrainPruneMsg:
		m.State = StatePruning
		m.Message = fmt.Sprintf("🧹 GC: Freed %d tokens", msg.TokensFreed)
		m.Spinner.Style = brainStylePruning
		m.ContextExpanded = false // Elastic context: contract after cleanup
		return m, tea.Batch(cmd, tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
			return BrainIdleMsg{}
		}))

	case BrainWriteMsg:
		m.State = StateWrite
		path := msg.Path
		if len(path) > 40 {
			path = "..." + path[len(path)-37:]
		}
		m.Message = fmt.Sprintf("✍️ WRITE: %s", path)
		m.Spinner.Style = brainStyleWrite
		return m, cmd

	case BrainFocusMsg:
		m.State = StateFocus
		task := msg.Task
		if len(task) > 40 {
			task = task[:37] + "..."
		}
		m.Message = fmt.Sprintf("🔍 FOCUS: %s", task)
		m.Spinner.Style = brainStyleFocus
		return m, cmd

	case BrainIdleMsg:
		m.State = StateIdle
		m.Message = "👁️ MONITORING..."
		m.Spinner.Style = brainStyleIdle
		return m, cmd

	case TokenUpdateMsg:
		m.UsedTokens = msg.Current
		m.MaxTokens = msg.Max
		// Auto-trigger pruning warning at 90%
		if m.MaxTokens > 0 && float64(m.UsedTokens)/float64(m.MaxTokens) > 0.9 {
			m.State = StatePruning
			m.Message = "⚠️ CRITICAL MEMORY: AUTO-COMPACTING..."
			m.Spinner.Style = brainStylePruning
		}
		return m, nil
	}

	return m, cmd
}

// View renders the brain monitor with token progress bar
func (m BrainModel) View() string {
	var labelStyle lipgloss.Style

	switch m.State {
	case StateTrauma:
		labelStyle = brainStyleTrauma
	case StateRecall:
		labelStyle = brainStyleRecall
	case StatePruning:
		labelStyle = brainStylePruning
	case StateWrite:
		labelStyle = brainStyleWrite
	case StateFocus:
		labelStyle = brainStyleFocus
	default:
		labelStyle = brainStyleIdle
	}

	// Brain block: [spinner] [message]
	brainBlock := fmt.Sprintf("%s %s", m.Spinner.View(), labelStyle.Render(m.Message))

	// Token bar: [||||....] 50k
	tokenBar := m.renderProgressBar(20)

	// Left: brain state, Right: token usage
	leftView := brainContainerStyle.Render(brainBlock)
	rightView := brainContainerStyle.Render("CTX: " + tokenBar)

	return lipgloss.JoinHorizontal(lipgloss.Center, leftView, "  ", rightView)
}

// renderProgressBar creates a visual progress bar for token usage
func (m BrainModel) renderProgressBar(width int) string {
	if m.MaxTokens == 0 {
		m.MaxTokens = 200000 // Default to 200k context
	}

	pct := float64(m.UsedTokens) / float64(m.MaxTokens)
	if pct > 1 {
		pct = 1
	}

	// Determine color based on usage
	var barColor lipgloss.Color
	switch {
	case pct < 0.5:
		barColor = barColorSafe
	case pct < 0.8:
		barColor = barColorWarn
	default:
		barColor = barColorDanger
	}

	// Calculate filled/empty segments
	fullSize := int(pct * float64(width))
	if fullSize > width {
		fullSize = width
	}
	emptySize := width - fullSize

	// Build bar: █ (full), ░ (empty)
	fullBar := lipgloss.NewStyle().Foreground(barColor).Render(repeat("█", fullSize))
	emptyBar := lipgloss.NewStyle().Foreground(barColorEmpty).Render(repeat("░", emptySize))

	// Label: "14.5k / 200k"
	label := lipgloss.NewStyle().
		Foreground(barColor).
		Bold(true).
		Render(fmt.Sprintf(" %dk/%dk", m.UsedTokens/1000, m.MaxTokens/1000))

	return fullBar + emptyBar + label
}

// repeat repeats a string n times
func repeat(s string, count int) string {
	if count <= 0 {
		return ""
	}
	result := ""
	for i := 0; i < count; i++ {
		result += s
	}
	return result
}

// StateColor returns the color for the current state (for external use)
func (m BrainModel) StateColor() lipgloss.Color {
	switch m.State {
	case StateTrauma:
		return lipgloss.Color("#FF0000")
	case StateRecall:
		return lipgloss.Color("#00FFFF")
	case StatePruning:
		return lipgloss.Color("#00FF00")
	case StateWrite:
		return lipgloss.Color("#FFFF00")
	case StateFocus:
		return lipgloss.Color("#FFFFFF")
	default:
		return lipgloss.Color("#555555")
	}
}

// IsExpanded returns true if context is in expanded mode (error recovery)
func (m BrainModel) IsExpanded() bool {
	return m.ContextExpanded
}

// TokenUsagePercent returns the current token usage as a percentage
func (m BrainModel) TokenUsagePercent() float64 {
	if m.MaxTokens == 0 {
		return 0
	}
	return float64(m.UsedTokens) / float64(m.MaxTokens) * 100
}
