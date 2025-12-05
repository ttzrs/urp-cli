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

	tea "github.com/charmbracelet/bubbletea"
	"github.com/oklog/ulid/v2"

	"github.com/joss/urp/internal/graph"
	"github.com/joss/urp/internal/opencode/agent"
	"github.com/joss/urp/internal/opencode/domain"
	"github.com/joss/urp/internal/opencode/graphstore"
	"github.com/joss/urp/internal/opencode/provider"
	"github.com/joss/urp/internal/opencode/tool"
	"github.com/joss/urp/pkg/llm"
)

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

		// Check for "ultrathink" keyword to enable extended thinking for this call
		actualPrompt := prompt
		if strings.Contains(strings.ToLower(prompt), "ultrathink") {
			ag.SetThinkingBudget(10000)
			actualPrompt = strings.ReplaceAll(prompt, "ultrathink", "")
			actualPrompt = strings.ReplaceAll(actualPrompt, "ULTRATHINK", "")
			actualPrompt = strings.ReplaceAll(actualPrompt, "Ultrathink", "")
			actualPrompt = strings.TrimSpace(actualPrompt)
		} else {
			ag.SetThinkingBudget(0) // Reset to no thinking
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

		// Run agent
		events, err := ag.Run(ctx, sess, nil, actualPrompt)
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
	// ThinkingBudget disabled by default for speed. Set URP_THINKING=4000 to enable.
	if tb := os.Getenv("URP_THINKING"); tb != "" {
		var budget int
		if _, err := fmt.Sscanf(tb, "%d", &budget); err == nil && budget > 0 {
			ag.SetThinkingBudget(budget)
		}
	}

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
	// Read prompt from stdin
	fmt.Println("\n✓ Ready. Enter prompt (empty to quit):")
	fmt.Print("> ")

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

	return RunAgentWithPrompt(workDir, prompt)
}

// RunAgentWithPrompt runs the agent with a given prompt (non-interactive, for containers)
func RunAgentWithPrompt(workDir, prompt string) error {
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
	// ThinkingBudget disabled by default for speed. Set URP_THINKING=4000 to enable.
	if tb := os.Getenv("URP_THINKING"); tb != "" {
		var budget int
		if _, err := fmt.Sscanf(tb, "%d", &budget); err == nil && budget > 0 {
			ag.SetThinkingBudget(budget)
			fmt.Printf("Thinking budget: %d\n", budget)
		}
	}
	fmt.Println("✓ Agent created")

	// Create session
	now := time.Now()
	sess := &domain.Session{
		ID:        ulid.Make().String(),
		ProjectID: filepath.Base(workDir),
		Directory: workDir,
		Title:     "worker-task",
		CreatedAt: now,
		UpdatedAt: now,
	}

	if store != nil {
		store.CreateSession(ctx, sess)
		ag.OnMessage(func(ctx context.Context, msg *domain.Message) error {
			return store.CreateMessage(ctx, msg)
		})
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
		case domain.StreamEventPermissionAsk:
			// Auto-approve in non-interactive mode (workers run with full permissions)
			if event.PermissionResp != nil {
				event.PermissionResp <- true
			}
		case domain.StreamEventError:
			fmt.Printf("\n[ERROR: %v]\n", event.Error)
		case domain.StreamEventDone:
			fmt.Println("\n---\n✓ Done")
		}
	}

	return nil
}
