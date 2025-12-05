// Package tui provides a terminal user interface for URP using Bubble Tea.
package tui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/joss/urp/internal/graph"
	"github.com/joss/urp/internal/opencode/graphstore"
	"github.com/joss/urp/internal/opencode/session"
	urpstrings "github.com/joss/urp/internal/strings"
)

var db graph.Driver

// Styles
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("205")).
			MarginLeft(2)

	infoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	activeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42")).
			Bold(true)

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196"))

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			MarginTop(1)

	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(0, 1)

	statusBarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Background(lipgloss.Color("236")).
			Padding(0, 1)
)

// View represents the current view mode
type View int

const (
	ViewMain View = iota
	ViewSessions
	ViewAgent
	ViewHelp
)

// Status represents infrastructure status
type Status struct {
	GraphConnected bool
	Project        string
	EventCount     int
	Workers        int
	LastUpdate     time.Time
}

// Session represents a saved session
type Session struct {
	ID        string
	Title     string
	UpdatedAt time.Time
	Messages  int
}

// Model is the main TUI model
type Model struct {
	// State
	view        View
	status      Status
	sessions    []Session
	selectedIdx int
	err         error
	ready       bool
	quitting    bool

	// Components
	spinner   spinner.Model
	input     textinput.Model
	viewport  viewport.Model
	width     int
	height    int

	// Agent output
	agentOutput strings.Builder
	agentActive bool
}

// Message types
type statusMsg Status
type sessionsMsg []Session
type agentOutputMsg string
type agentDoneMsg struct{ err error }
type errMsg error
type tickMsg time.Time

// New creates a new TUI model
func New() Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	ti := textinput.New()
	ti.Placeholder = "Enter prompt..."
	ti.CharLimit = 500
	ti.Width = 60

	return Model{
		view:     ViewMain,
		spinner:  s,
		input:    ti,
		sessions: []Session{},
		status: Status{
			Project:    "unknown",
			LastUpdate: time.Now(),
		},
	}
}

// Init initializes the TUI
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		fetchStatus,
		fetchSessions,
		tickCmd(),
	)
}

// Update handles messages
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Global keys
		switch msg.String() {
		case "ctrl+c", "q":
			if m.view == ViewMain && !m.agentActive {
				m.quitting = true
				return m, tea.Quit
			}
			if m.view != ViewMain {
				m.view = ViewMain
				return m, nil
			}
		case "?":
			if m.view == ViewMain {
				m.view = ViewHelp
			} else {
				m.view = ViewMain
			}
			return m, nil
		case "s":
			if m.view == ViewMain && !m.agentActive {
				m.view = ViewSessions
				return m, nil
			}
		case "enter":
			if m.view == ViewMain && m.input.Focused() {
				prompt := m.input.Value()
				if prompt != "" {
					m.agentActive = true
					m.agentOutput.Reset()
					m.input.SetValue("")
					return m, startAgent(prompt)
				}
			}
			if m.view == ViewSessions && len(m.sessions) > 0 {
				// Load selected session
				m.view = ViewMain
				return m, nil
			}
		case "tab":
			if m.view == ViewMain {
				if m.input.Focused() {
					m.input.Blur()
				} else {
					m.input.Focus()
				}
			}
		case "up", "k":
			if m.view == ViewSessions && m.selectedIdx > 0 {
				m.selectedIdx--
			}
		case "down", "j":
			if m.view == ViewSessions && m.selectedIdx < len(m.sessions)-1 {
				m.selectedIdx++
			}
		case "esc":
			if m.agentActive {
				// TODO: cancel agent
			}
			m.view = ViewMain
			m.input.Blur()
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true

		// Update viewport
		headerHeight := 6
		footerHeight := 3
		m.viewport = viewport.New(msg.Width-4, msg.Height-headerHeight-footerHeight)
		m.viewport.SetContent(m.agentOutput.String())

	case statusMsg:
		m.status = Status(msg)

	case sessionsMsg:
		m.sessions = msg

	case agentOutputMsg:
		m.agentOutput.WriteString(string(msg))
		m.viewport.SetContent(m.agentOutput.String())
		m.viewport.GotoBottom()

	case agentDoneMsg:
		m.agentActive = false
		if msg.err != nil {
			m.err = msg.err
		}

	case errMsg:
		m.err = msg

	case tickMsg:
		cmds = append(cmds, fetchStatus, tickCmd())

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)
	}

	// Update text input
	if m.view == ViewMain {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		cmds = append(cmds, cmd)
	}

	// Update viewport
	if m.view == ViewMain || m.view == ViewAgent {
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

// View renders the TUI
func (m Model) View() string {
	if m.quitting {
		return "Goodbye!\n"
	}

	if !m.ready {
		return fmt.Sprintf("\n  %s Loading...", m.spinner.View())
	}

	switch m.view {
	case ViewSessions:
		return m.viewSessions()
	case ViewHelp:
		return m.viewHelp()
	default:
		return m.viewMain()
	}
}

func (m Model) viewMain() string {
	var b strings.Builder

	// Header
	header := titleStyle.Render("⚡ URP Interactive Agent")
	b.WriteString(header + "\n\n")

	// Status bar
	statusIcon := "●"
	if m.status.GraphConnected {
		statusIcon = activeStyle.Render("●")
	} else {
		statusIcon = errorStyle.Render("○")
	}

	status := fmt.Sprintf("%s %s │ Events: %d │ Workers: %d",
		statusIcon,
		m.status.Project,
		m.status.EventCount,
		m.status.Workers,
	)
	b.WriteString(infoStyle.Render(status) + "\n\n")

	// Main content area
	if m.agentActive {
		b.WriteString(fmt.Sprintf("  %s Agent running...\n\n", m.spinner.View()))
	}

	// Output viewport
	outputBox := boxStyle.Width(m.width - 4).Render(m.viewport.View())
	b.WriteString(outputBox + "\n")

	// Input
	if !m.agentActive {
		b.WriteString("\n  " + m.input.View() + "\n")
	}

	// Footer
	var helpText string
	if m.agentActive {
		helpText = "esc: cancel │ scroll: view output"
	} else {
		helpText = "enter: send │ s: sessions │ ?: help │ q: quit"
	}
	b.WriteString(helpStyle.Render("  " + helpText))

	return b.String()
}

func (m Model) viewSessions() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("📋 Sessions") + "\n\n")

	if len(m.sessions) == 0 {
		b.WriteString(infoStyle.Render("  No sessions found\n"))
	} else {
		for i, s := range m.sessions {
			cursor := "  "
			style := infoStyle
			if i == m.selectedIdx {
				cursor = "▶ "
				style = activeStyle
			}

			line := fmt.Sprintf("%s%-20s %s (%d msgs)",
				cursor,
				urpstrings.Truncate(s.Title, 20),
				s.UpdatedAt.Format("Jan 02 15:04"),
				s.Messages,
			)
			b.WriteString(style.Render(line) + "\n")
		}
	}

	b.WriteString(helpStyle.Render("\n  enter: load │ esc: back │ j/k: navigate"))

	return b.String()
}

func (m Model) viewHelp() string {
	help := `
  ⚡ URP Interactive Agent - Help

  NAVIGATION
    tab       Focus/unfocus input
    s         View sessions
    ?         Toggle help
    q         Quit

  AGENT
    enter     Send prompt to agent
    esc       Cancel running agent

  SESSIONS
    j/k       Navigate up/down
    enter     Load session
    esc       Back to main

  COMMANDS
    urp status     Show infrastructure status
    urp doctor     Run diagnostics
    urp launch     Launch container session
`
	return titleStyle.Render("Help") + "\n" + infoStyle.Render(help) + helpStyle.Render("\n  press any key to return")
}

// Commands

func fetchStatus() tea.Msg {
	connected := false
	eventCount := 0
	project := os.Getenv("URP_PROJECT")
	if project == "" {
		project = "unknown"
	}

	if db != nil {
		if err := db.Ping(context.Background()); err == nil {
			connected = true

			// Count events
			records, err := db.Execute(context.Background(),
				"MATCH (e:TerminalEvent) RETURN count(e) as count", nil)
			if err == nil && len(records) > 0 {
				if c, ok := records[0]["count"].(int64); ok {
					eventCount = int(c)
				}
			}
		}
	}

	return statusMsg{
		GraphConnected: connected,
		Project:        project,
		EventCount:     eventCount,
		Workers:        0,
		LastUpdate:     time.Now(),
	}
}

func fetchSessions() tea.Msg {
	if db == nil {
		return sessionsMsg{}
	}

	store := graphstore.New(db)
	mgr := session.NewManager(store)

	dir, _ := os.Getwd()
	sessions, err := mgr.List(context.Background(), dir, 10)
	if err != nil {
		return sessionsMsg{}
	}

	result := make([]Session, 0, len(sessions))
	for _, s := range sessions {
		msgs, _ := mgr.GetMessages(context.Background(), s.ID)
		result = append(result, Session{
			ID:        s.ID,
			Title:     s.Title,
			UpdatedAt: s.UpdatedAt,
			Messages:  len(msgs),
		})
	}

	return sessionsMsg(result)
}

func startAgent(prompt string) tea.Cmd {
	return func() tea.Msg {
		// TODO: Actually run agent
		// For now, simulate output
		return agentOutputMsg(fmt.Sprintf("Prompt: %s\n\nProcessing...\n", prompt))
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// Run starts the TUI
func Run() error {
	// Connect to graph
	graph.SetEnvLookup(os.LookupEnv)
	var err error
	db, err = graph.Connect()
	if err != nil {
		db = nil // Silent fail, TUI will show disconnected
	}
	if db != nil {
		defer db.Close()
	}

	p := tea.NewProgram(New(), tea.WithAltScreen())
	_, err = p.Run()
	return err
}
