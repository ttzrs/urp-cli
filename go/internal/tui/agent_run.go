package tui

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/oklog/ulid/v2"

	"github.com/joss/urp/internal/bootstrap"
	"github.com/joss/urp/internal/opencode/agent"
	"github.com/joss/urp/internal/opencode/domain"
	"github.com/joss/urp/internal/opencode/graphstore"
)

// runAgent is the Tea command to execute the agent logic
func runAgent(ag *agent.Agent, la *agent.LearningAgent, store *graphstore.Store, workDir string, prompt string, program *tea.Program, shared *sharedState) tea.Cmd {
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
		defer cancel()
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

		// Persist session if store available (async to not block)
		if store != nil {
			go store.CreateSession(ctx, sess)
			ag.OnMessage(func(ctx context.Context, msg *domain.Message) error {
				go store.CreateMessage(ctx, msg) // Async persistence
				return nil
			})
		}

		// Start Learning Task
		if la != nil {
			la.StartTask(sess.ID, actualPrompt)
		}

		// Run agent
		events, err := ag.Run(ctx, sess, nil, actualPrompt)
		
		// Finish Learning Task
		if la != nil {
			// Determine success based on error (partial heuristic)
			la.PostTask(ctx, err == nil)
		}

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

// RunAgent starts the interactive agent TUI
func RunAgent(workDir string) error {
	ctx := context.Background()
	
	// Bootstrap the application
	app, err := bootstrap.Initialize(ctx, bootstrap.InitializeOptions{
		WorkDir: workDir,
	})
	if err != nil {
		return err
	}
	defer app.Close()

	// Create model with shared state
	model := NewAgentModel(workDir, app.Agent, app.LearningAgent, app.Store, app.Provider)

	// Create program
	p := tea.NewProgram(
		model,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	// Store program reference
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

	fmt.Println("Bootstrapping URP...")
	
	// Bootstrap the application
	app, err := bootstrap.Initialize(ctx, bootstrap.InitializeOptions{
		WorkDir: workDir,
	})
	if err != nil {
		return fmt.Errorf("bootstrap failed: %w", err)
	}
	defer app.Close()
	
	fmt.Println("✓ Application initialized")

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

	if app.Store != nil {
		app.Store.CreateSession(ctx, sess)
		app.Agent.OnMessage(func(ctx context.Context, msg *domain.Message) error {
			return app.Store.CreateMessage(ctx, msg)
		})
	}

	fmt.Printf("\nRunning agent with prompt: %s\n", prompt)
	fmt.Println("---")

	// Start Learning
	if app.LearningAgent != nil {
		app.LearningAgent.StartTask(sess.ID, prompt)
	}

	// Run agent
	events, err := app.Agent.Run(ctx, sess, nil, prompt)
	
	// Finish Learning
	if app.LearningAgent != nil {
		errLearn := app.LearningAgent.PostTask(ctx, err == nil)
		if errLearn != nil {
			fmt.Printf("\n[Learning Error: %v]\n", errLearn)
		} else {
			fmt.Println("\n[Cycle Learned]")
		}
	}

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
			// Auto-approve in non-interactive mode
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