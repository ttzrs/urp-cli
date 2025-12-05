// Package tui provides the Bubble Tea interactive agent interface.
package tui

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/oklog/ulid/v2"

	"github.com/joss/urp/internal/graph"
	"github.com/joss/urp/internal/opencode/agent"
	"github.com/joss/urp/internal/opencode/domain"
	"github.com/joss/urp/internal/opencode/graphstore"
	"github.com/joss/urp/internal/opencode/provider"
	"github.com/joss/urp/internal/opencode/tool"
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
)

// sharedState holds state that needs to be shared across model copies
type sharedState struct {
	program    *tea.Program
	cancelFunc context.CancelFunc
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

	// Output buffer
	output      strings.Builder
	toolCalls   []toolCallInfo
	currentTool *toolCallInfo

	// Usage tracking
	inputTokens  int
	outputTokens int
	thinkTokens  int

	// UI components
	viewport viewport.Model
	input    textarea.Model
	spinner  spinner.Model
	width    int
	height   int
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

	return AgentModel{
		workDir:     workDir,
		ag:          ag,
		store:       store,
		prov:        prov,
		initialized: true, // Already initialized
		shared:      &sharedState{}, // Will be set by RunAgent
		spinner:     s,
		input:       ti,
		toolCalls:   []toolCallInfo{},
	}
}

// Init initializes the TUI
func (m AgentModel) Init() tea.Cmd {
	return m.spinner.Tick
}

// Update handles messages
func (m AgentModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			if m.agentActive && m.shared != nil && m.shared.cancelFunc != nil {
				m.shared.cancelFunc()
				m.agentActive = false
				m.output.WriteString("\n\n" + agentErrorStyle.Render("⚠ Cancelled") + "\n")
				m.viewport.SetContent(m.renderOutput())
				return m, nil
			}
			m.quitting = true
			return m, tea.Quit

		case "ctrl+d", "esc":
			if !m.agentActive {
				m.quitting = true
				return m, tea.Quit
			}

		case "enter":
			// Enter sends message (if not empty and not running)
			if !m.agentActive && strings.TrimSpace(m.input.Value()) != "" {
				prompt := m.input.Value()
				m.input.SetValue("")
				m.agentActive = true
				m.output.Reset()
				m.output.WriteString(thinkingStyle.Render("⏳ Thinking...") + "\n")
				m.viewport.SetContent(m.renderOutput())
				m.toolCalls = []toolCallInfo{}
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
			m.output.Reset()
			m.toolCalls = []toolCallInfo{}
			m.viewport.SetContent("")

		case "ctrl+t":
			// Toggle tool call collapse
			if len(m.toolCalls) > 0 {
				for i := range m.toolCalls {
					m.toolCalls[i].collapsed = !m.toolCalls[i].collapsed
				}
				m.viewport.SetContent(m.renderOutput())
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
		m.ready = true

		// Calculate viewport size
		headerHeight := 2
		statusHeight := 1
		inputHeight := 5
		m.viewport = viewport.New(msg.Width, msg.Height-headerHeight-statusHeight-inputHeight)
		m.viewport.SetContent(m.renderOutput())

		// Adjust input width
		m.input.SetWidth(msg.Width - 4)

	case agentStreamEventMsg:
		event := domain.StreamEvent(msg)
		m.handleStreamEvent(event)
		m.viewport.SetContent(m.renderOutput())
		m.viewport.GotoBottom()
		return m, nil

	case agentRunDoneMsg:
		m.agentActive = false
		if msg.err != nil {
			m.output.WriteString("\n" + agentErrorStyle.Render(fmt.Sprintf("Error: %v", msg.err)) + "\n")
		} else {
			m.output.WriteString("\n" + successStyle.Render("✓ Done") + "\n")
		}
		m.viewport.SetContent(m.renderOutput())
		m.viewport.GotoBottom()

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)
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
		m.output.WriteString(thinkingStyle.Render(event.Content))
		if event.Usage != nil {
			m.thinkTokens += event.Usage.OutputTokens
		}

	case domain.StreamEventText:
		m.output.WriteString(textStyle.Render(event.Content))
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
			m.toolCalls = append(m.toolCalls, info)
			m.currentTool = &m.toolCalls[len(m.toolCalls)-1]
			m.output.WriteString("\n" + toolStyle.Render(fmt.Sprintf("▶ %s", tc.Name)) + "\n")
		}

	case domain.StreamEventToolDone:
		if tc, ok := event.Part.(domain.ToolCallPart); ok {
			if m.currentTool != nil {
				m.currentTool.output = truncateOutput(tc.Result)
				m.currentTool.err = tc.Error
				m.currentTool.done = true

				if tc.Error != "" {
					m.output.WriteString(agentErrorStyle.Render(fmt.Sprintf("  ✗ %s\n", tc.Error)))
				} else {
					m.output.WriteString(successStyle.Render("  ✓\n"))
				}
			}
		}

	case domain.StreamEventError:
		m.output.WriteString(agentErrorStyle.Render(fmt.Sprintf("\nError: %v\n", event.Error)))

	case domain.StreamEventUsage:
		if event.Usage != nil {
			m.inputTokens = event.Usage.InputTokens
			m.outputTokens = event.Usage.OutputTokens
		}
	}
}

func (m AgentModel) renderOutput() string {
	return m.output.String()
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

	// Header
	header := agentTitleStyle.Render("⚡ URP Agent") + "  " +
		lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render(m.workDir)
	b.WriteString(header + "\n\n")

	// Main viewport
	b.WriteString(m.viewport.View())
	b.WriteString("\n")

	// Status bar
	status := m.renderStatus()
	b.WriteString(status + "\n")

	// Input area
	if m.agentActive {
		b.WriteString(fmt.Sprintf("  %s Running...", m.spinner.View()))
	} else {
		inputBox := inputBorderStyle.Width(m.width - 4).Render(m.input.View())
		if m.input.Focused() {
			inputBox = focusedInputStyle.Width(m.width - 4).Render(m.input.View())
		}
		b.WriteString(inputBox)
	}

	return b.String()
}

func (m AgentModel) renderStatus() string {
	var parts []string

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
	if len(m.toolCalls) > 0 {
		parts = append(parts, fmt.Sprintf("Tools:%d", len(m.toolCalls)))
	}

	// Help
	if m.agentActive {
		parts = append(parts, "Ctrl+C: cancel │ ↑↓: scroll")
	} else {
		parts = append(parts, "Enter: send │ Alt+Enter: newline │ Esc: quit │ Ctrl+T: tools")
	}

	return agentStatusStyle.Width(m.width).Render(strings.Join(parts, " │ "))
}

// Commands

// warmupConnection pre-establishes connection to API endpoint
func warmupConnection(prov llm.Provider) {
	// Get base URL from provider if possible
	baseURL := os.Getenv("OPENAI_BASE_URL")
	if baseURL == "" {
		baseURL = os.Getenv("ANTHROPIC_BASE_URL")
	}
	if baseURL == "" {
		return
	}

	// Just do a HEAD request to establish TCP/TLS
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "HEAD", baseURL, nil)
	if err != nil {
		return
	}

	resp, err := http.DefaultClient.Do(req)
	if err == nil && resp != nil {
		resp.Body.Close()
	}
}

func runAgent(ag *agent.Agent, store *graphstore.Store, workDir string, prompt string, program *tea.Program, shared *sharedState) tea.Cmd {
	return func() tea.Msg {
		// Check if agent was initialized
		if ag == nil {
			return agentRunDoneMsg{err: fmt.Errorf("agent not initialized - check API key")}
		}

		ctx, cancel := context.WithCancel(context.Background())
		if shared != nil {
			shared.cancelFunc = cancel
		}

		// Create new session for this prompt
		now := time.Now()
		sess := &domain.Session{
			ID:        ulid.Make().String(),
			ProjectID: filepath.Base(workDir),
			Directory: workDir,
			Title:     "interactive",
			CreatedAt: now,
			UpdatedAt: now,
		}

		// Persist session if store available
		if store != nil {
			store.CreateSession(ctx, sess)
			ag.OnMessage(func(ctx context.Context, msg *domain.Message) error {
				return store.CreateMessage(ctx, msg)
			})
		}

		// Run agent - this is the only slow part now
		events, err := ag.Run(ctx, sess, nil, prompt)
		if err != nil {
			return agentRunDoneMsg{err: err}
		}

		// Stream events - send each to program
		for event := range events {
			if program != nil {
				program.Send(agentStreamEventMsg(event))
			}
		}

		return agentRunDoneMsg{err: nil}
	}
}

func initProvider() (llm.Provider, error) {
	// Load .env from ~/.urp-go/.env if not already set
	loadEnvFile()

	// Try OpenAI-compatible first
	if apiKey := os.Getenv("OPENAI_API_KEY"); apiKey != "" {
		baseURL := os.Getenv("OPENAI_BASE_URL")
		return provider.NewOpenAI(apiKey, baseURL), nil
	}

	// Try Anthropic
	if apiKey := os.Getenv("ANTHROPIC_API_KEY"); apiKey != "" {
		baseURL := os.Getenv("ANTHROPIC_BASE_URL")
		return provider.NewAnthropic(apiKey, baseURL), nil
	}

	// Try Anthropic via proxy token
	if authToken := os.Getenv("ANTHROPIC_AUTH_TOKEN"); authToken != "" {
		baseURL := os.Getenv("ANTHROPIC_BASE_URL")
		return provider.NewAnthropic(authToken, baseURL), nil
	}

	return nil, fmt.Errorf("no API key found. Set ANTHROPIC_API_KEY or OPENAI_API_KEY")
}

// loadEnvFile loads environment variables from ~/.urp-go/.env
func loadEnvFile() {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}

	envPath := filepath.Join(home, ".urp-go", ".env")
	data, err := os.ReadFile(envPath)
	if err != nil {
		return
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		// Don't override existing env vars
		if os.Getenv(key) == "" {
			os.Setenv(key, value)
		}
	}
}

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

// RunAgent starts the interactive agent TUI
func RunAgent(workDir string) error {
	// Initialize everything BEFORE starting the TUI
	loadEnvFile()

	// Connect to graph
	graph.SetEnvLookup(os.LookupEnv)
	var store *graphstore.Store
	gdb, err := graph.Connect()
	if err == nil && gdb != nil {
		store = graphstore.New(gdb)
	}

	// Initialize provider
	prov, err := initProvider()
	if err != nil {
		return fmt.Errorf("provider init failed: %w", err)
	}

	// Create agent
	tools := tool.DefaultRegistry(workDir)
	agentConfig := agent.BuiltinAgents()["build"]

	defaultModel := "claude-sonnet-4-5-20250929"
	if model := os.Getenv("URP_MODEL"); model != "" {
		defaultModel = model
	}
	agentConfig.Model = &domain.ModelConfig{ModelID: defaultModel}

	ag := agent.New(agentConfig, prov, tools)
	ag.SetWorkDir(workDir)
	ag.SetThinkingBudget(4000)

	// Create model with shared state
	model := NewAgentModel(workDir, ag, store, prov)

	// Create program
	p := tea.NewProgram(
		model,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	// Store program reference in shared state (survives model copies)
	model.shared.program = p

	_, err = p.Run()
	return err
}

// RunAgentDebug runs the agent with static stdout output (for debugging)
func RunAgentDebug(workDir string) error {
	ctx := context.Background()

	// Load env
	loadEnvFile()

	fmt.Println("Loading environment...")

	// Connect to graph
	graph.SetEnvLookup(os.LookupEnv)
	gdb, err := graph.Connect()
	if err != nil {
		fmt.Printf("⚠ Memgraph: %v\n", err)
		gdb = nil
	} else {
		fmt.Println("✓ Memgraph connected")
	}

	var store *graphstore.Store
	if gdb != nil {
		store = graphstore.New(gdb)
	}

	// Initialize provider
	fmt.Println("Initializing provider...")
	prov, err := initProvider()
	if err != nil {
		return fmt.Errorf("provider init failed: %w", err)
	}
	fmt.Println("✓ Provider initialized")

	// Create agent
	fmt.Println("Creating agent...")
	tools := tool.DefaultRegistry(workDir)
	agentConfig := agent.BuiltinAgents()["build"]

	defaultModel := "claude-sonnet-4-5-20250929"
	if model := os.Getenv("URP_MODEL"); model != "" {
		defaultModel = model
	}
	fmt.Printf("Model: %s\n", defaultModel)
	agentConfig.Model = &domain.ModelConfig{ModelID: defaultModel}

	ag := agent.New(agentConfig, prov, tools)
	ag.SetWorkDir(workDir)
	ag.SetThinkingBudget(4000)
	fmt.Println("✓ Agent created")

	// Create session
	now := time.Now()
	sess := &domain.Session{
		ID:        ulid.Make().String(),
		ProjectID: filepath.Base(workDir),
		Directory: workDir,
		Title:     "debug",
		CreatedAt: now,
		UpdatedAt: now,
	}

	if store != nil {
		store.CreateSession(ctx, sess)
		ag.OnMessage(func(ctx context.Context, msg *domain.Message) error {
			return store.CreateMessage(ctx, msg)
		})
	}

	fmt.Println("\n✓ Ready. Enter prompt (empty to quit):")
	fmt.Print("> ")

	// Read prompt from stdin
	reader := bufio.NewReader(os.Stdin)
	prompt, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		fmt.Println("Empty prompt, exiting.")
		return nil
	}

	fmt.Printf("\nRunning agent with prompt: %s\n", prompt)
	fmt.Println("---")

	// Run agent
	events, err := ag.Run(ctx, sess, nil, prompt)
	if err != nil {
		return fmt.Errorf("agent run failed: %w", err)
	}

	// Stream events to stdout
	for event := range events {
		switch event.Type {
		case domain.StreamEventThinking:
			fmt.Printf("\033[2m%s\033[0m", event.Content) // dim
		case domain.StreamEventText:
			fmt.Print(event.Content)
		case domain.StreamEventToolCall:
			if tc, ok := event.Part.(domain.ToolCallPart); ok {
				fmt.Printf("\n[tool: %s]\n", tc.Name)
			}
		case domain.StreamEventToolDone:
			if tc, ok := event.Part.(domain.ToolCallPart); ok {
				if tc.Error != "" {
					fmt.Printf("[error: %s]\n", tc.Error)
				} else {
					fmt.Println("[done]")
				}
			}
		case domain.StreamEventError:
			fmt.Printf("\n[ERROR: %v]\n", event.Error)
		case domain.StreamEventDone:
			fmt.Println("\n---\n✓ Done")
		}
	}

	return nil
}
