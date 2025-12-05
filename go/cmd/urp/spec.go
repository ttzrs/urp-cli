package main

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/spf13/cobra"
	"github.com/joss/urp/internal/graph"
	"github.com/joss/urp/internal/opencode/agent"
	"github.com/joss/urp/internal/opencode/domain"
	"github.com/joss/urp/internal/opencode/graphstore"
	"github.com/joss/urp/internal/opencode/provider"
	"github.com/joss/urp/internal/opencode/tool"
	"github.com/joss/urp/internal/specs"
	"github.com/joss/urp/pkg/llm"
)

func specCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "spec",
		Short: "Spec-Driven Development commands",
		Long:  "Manage specifications using the GitHub Spec-Kit methodology",
	}

	// spec init <name>
	initCmd := &cobra.Command{
		Use:   "init <name>",
		Short: "Initialize a new spec-driven project",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			cwd, _ := os.Getwd()
			engine := specs.NewEngine(cwd)
			
			if err := engine.InitProject(context.Background(), args[0]); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			
			fmt.Printf("✓ Initialized spec project: %s\n", args[0])
			fmt.Println("  - .specify/memory/constitution.md created")
			fmt.Println("  - specs/ directory created")
		},
	}

	// spec list
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List available specifications",
		Run: func(cmd *cobra.Command, args []string) {
			cwd, _ := os.Getwd()
			engine := specs.NewEngine(cwd)
			
			list, err := engine.ListSpecs(context.Background())
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			
			if len(list) == 0 {
				fmt.Println("No specs found")
				return
			}
			
			fmt.Println("SPECIFICATIONS:")
			for _, s := range list {
				fmt.Printf("  - %s\n", s)
			}
		},
	}

	// spec run <spec-name>
	runCmd := &cobra.Command{
		Use:   "run <spec-name>",
		Short: "Run a spec using the OpenCode orchestrator",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			specName := args[0]
			cwd, _ := os.Getwd()
			ctx := context.Background()

			fmt.Printf("🚀 Starting OpenCode Orchestrator for spec: %s\n", specName)

			// 1. Connect to Memgraph
			graph.SetEnvLookup(os.LookupEnv)
			db, err := graph.Connect()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: Memgraph not available, using volatile session: %v\n", err)
				db = nil
			}
			if db != nil {
				defer db.Close()
			}

			var store *graphstore.Store
			if db != nil {
				store = graphstore.New(db)
			}

			// 2. Initialize provider with fallback support
			p, err := initProviderWithFallback()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			// Initialize tools with current directory
			tools := tool.DefaultRegistry(cwd)

			// Get build agent configuration
			agentConfig := agent.BuiltinAgents()["build"]

			// Set default model
			defaultModel := "claude-sonnet-4-5-20250929"
			if model := os.Getenv("URP_MODEL"); model != "" {
				defaultModel = model
			}
			agentConfig.Model = &domain.ModelConfig{
				ModelID: defaultModel,
			}

			// Create agent
			ag := agent.New(agentConfig, p, tools)
			ag.SetWorkDir(cwd)
			ag.SetThinkingBudget(4000) // Enable thinking
			ag.EnableAutocorrection(agent.DefaultAutocorrection()) // Enable retry on test failures

			// 3. Create session with persistence
			now := time.Now()
			sess := &domain.Session{
				ID:        ulid.Make().String(),
				ProjectID: specName,
				Directory: cwd,
				Title:     "spec-run: " + specName,
				CreatedAt: now,
				UpdatedAt: now,
			}

			// Persist session if store available
			if store != nil {
				if err := store.CreateSession(ctx, sess); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to persist session: %v\n", err)
				} else {
					fmt.Printf("📝 Session persisted: %s\n", sess.ID)
				}

				// Wire up message persistence
				ag.OnMessage(func(ctx context.Context, msg *domain.Message) error {
					return store.CreateMessage(ctx, msg)
				})
			}

			// 4. Parse spec with frontmatter + graph enrichment
			engine := specs.NewEngine(cwd)
			if db != nil {
				engine.WithDB(db)
			}

			spec, err := engine.ParseSpec(ctx, specName)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not parse spec: %v\n", err)
				spec = nil
			}

			constitution, _ := engine.ReadConstitution(ctx)

			// 5. Build structured prompt with parsed spec
			var prompt string
			if spec != nil {
				// Show spec metadata
				if spec.Title != "" {
					fmt.Printf("📋 Title: %s\n", spec.Title)
				}
				if spec.Status != "" {
					fmt.Printf("📊 Status: %s\n", spec.Status)
				}
				if len(spec.Requirements) > 0 {
					fmt.Printf("✅ Requirements: %d (%.0f%% complete)\n", len(spec.Requirements), spec.Progress())
				}
				if len(spec.Context) > 0 {
					fmt.Printf("📁 Context files: %d\n", len(spec.Context))
				}

				// Build structured prompt
				prompt, _ = engine.BuildPrompt(ctx, spec, constitution)
			} else {
				// Fallback to simple prompt
				prompt = fmt.Sprintf(`
I need you to implement the feature described in the specification '%s'.

Please follow these steps:
1. Read the specification files in specs/%s/
2. Create a plan of action
3. Execute the plan to implement the feature
4. Verify the implementation with tests: go test ./...

Start by reading specs/%s/spec.md
`, specName, specName, specName)
			}

			// 6. Run agent loop
			events, err := ag.Run(ctx, sess, nil, prompt)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error starting agent: %v\n", err)
				os.Exit(1)
			}

			// Stream output
			for event := range events {
				switch event.Type {
				case domain.StreamEventThinking:
					fmt.Printf("\033[2m%s\033[0m", event.Content)
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
						}
					}
				case domain.StreamEventError:
					fmt.Fprintf(os.Stderr, "\nError: %v\n", event.Error)
				case domain.StreamEventDone:
					fmt.Println("\n✓ Mission accomplished")
				}
			}
		},
	}

	// spec show <name> - preview parsed spec
	showCmd := &cobra.Command{
		Use:   "show <spec-name>",
		Short: "Show parsed spec with frontmatter",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			specName := args[0]
			cwd, _ := os.Getwd()
			ctx := context.Background()

			engine := specs.NewEngine(cwd)

			// Try to connect to graph for enrichment
			graph.SetEnvLookup(os.LookupEnv)
			if db, err := graph.Connect(); err == nil {
				defer db.Close()
				engine.WithDB(db)
			}

			spec, err := engine.ParseSpec(ctx, specName)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			// Print metadata
			fmt.Println("═══════════════════════════════════════")
			fmt.Printf("SPECIFICATION: %s\n", specName)
			fmt.Println("═══════════════════════════════════════")

			if spec.ID != "" {
				fmt.Printf("ID:     %s\n", spec.ID)
			}
			if spec.Title != "" {
				fmt.Printf("Title:  %s\n", spec.Title)
			}
			if spec.Status != "" {
				fmt.Printf("Status: %s\n", spec.Status)
			}
			if spec.Owner != "" {
				fmt.Printf("Owner:  %s\n", spec.Owner)
			}
			if spec.Type != "" {
				fmt.Printf("Type:   %s\n", spec.Type)
			}

			// Context files
			if len(spec.Context) > 0 {
				fmt.Println("\nContext Files:")
				for _, f := range spec.Context {
					fmt.Printf("  - %s\n", f)
				}
			}

			// Requirements
			if len(spec.Requirements) > 0 {
				fmt.Printf("\nRequirements (%d):\n", len(spec.Requirements))
				for _, r := range spec.Requirements {
					check := "[ ]"
					if r.Complete {
						check = "[x]"
					}
					fmt.Printf("  %s %s\n", check, r.Text)
				}
				fmt.Printf("\nProgress: %.0f%%\n", spec.Progress())
			}

			// Plan
			if len(spec.Plan) > 0 {
				fmt.Println("\nPlan:")
				for i, step := range spec.Plan {
					fmt.Printf("  %d. %s\n", i+1, step)
				}
			}

			// Enriched context
			enriched, _ := engine.EnrichContext(ctx, spec, 2)
			if len(enriched) > len(spec.Context) {
				fmt.Println("\nEnriched Context (from graph):")
				for _, f := range enriched {
					fmt.Printf("  - %s\n", f)
				}
			}

			fmt.Println("\n═══════════════════════════════════════")
		},
	}

	cmd.AddCommand(initCmd, listCmd, runCmd, showCmd)
	return cmd
}

// Backup provider configuration (loaded from comments in .env or hardcoded)
type backupConfig struct {
	APIKey  string
	BaseURL string
	Model   string
}

// initProviderWithFallback tries primary provider, prompts for backup on failure
func initProviderWithFallback() (llm.Provider, error) {
	// Try primary OpenAI-compatible provider first
	apiKey := os.Getenv("OPENAI_API_KEY")
	baseURL := os.Getenv("OPENAI_BASE_URL")

	if apiKey != "" && baseURL != "" {
		// Test connection to primary
		if testProviderConnection(baseURL) {
			fmt.Println("🔌 Using primary provider:", baseURL)
			return provider.NewOpenAI(apiKey, baseURL), nil
		}

		// Primary failed, check for backup
		fmt.Printf("⚠️  Primary provider unavailable: %s\n", baseURL)
		backup := loadBackupConfig()

		if backup != nil {
			if askUserConfirmation(fmt.Sprintf("Switch to backup provider (%s)?", backup.BaseURL)) {
				// Apply backup config
				os.Setenv("OPENAI_API_KEY", backup.APIKey)
				os.Setenv("OPENAI_BASE_URL", backup.BaseURL)
				if backup.Model != "" {
					os.Setenv("URP_MODEL", backup.Model)
				}
				fmt.Println("🔌 Using backup provider:", backup.BaseURL)
				return provider.NewOpenAI(backup.APIKey, backup.BaseURL), nil
			}
			return nil, fmt.Errorf("user declined backup provider")
		}
	}

	// Try Anthropic direct
	if apiKey := os.Getenv("ANTHROPIC_API_KEY"); apiKey != "" {
		baseURL := os.Getenv("ANTHROPIC_BASE_URL")
		fmt.Println("🔌 Using Anthropic provider")
		return provider.NewAnthropic(apiKey, baseURL), nil
	}

	// Try Anthropic via proxy token
	if authToken := os.Getenv("ANTHROPIC_AUTH_TOKEN"); authToken != "" {
		baseURL := os.Getenv("ANTHROPIC_BASE_URL")
		fmt.Println("🔌 Using Anthropic provider (via proxy)")
		return provider.NewAnthropic(authToken, baseURL), nil
	}

	// Fallback: try OpenAI without testing
	if apiKey != "" {
		fmt.Println("🔌 Using OpenAI provider")
		return provider.NewOpenAI(apiKey, baseURL), nil
	}

	return nil, fmt.Errorf("no API key found. Set ANTHROPIC_API_KEY or OPENAI_API_KEY")
}

// testProviderConnection checks if the provider endpoint is reachable
func testProviderConnection(baseURL string) bool {
	// Normalize URL - test /models endpoint
	testURL := strings.TrimSuffix(baseURL, "/")
	if strings.HasSuffix(testURL, "/v1") {
		testURL = testURL + "/models"
	} else if !strings.Contains(testURL, "/models") {
		testURL = testURL + "/v1/models"
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(testURL)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// loadBackupConfig reads backup configuration from environment variables.
// Set URP_BACKUP_API_KEY, URP_BACKUP_BASE_URL, and URP_BACKUP_MODEL.
func loadBackupConfig() *backupConfig {
	apiKey := os.Getenv("URP_BACKUP_API_KEY")
	if apiKey == "" {
		return nil // No backup configured
	}
	baseURL := os.Getenv("URP_BACKUP_BASE_URL")
	if baseURL == "" {
		baseURL = "http://100.105.212.98:8317/v1"
	}
	model := os.Getenv("URP_BACKUP_MODEL")
	if model == "" {
		model = "claude-sonnet-4-20250514"
	}
	return &backupConfig{
		APIKey:  apiKey,
		BaseURL: baseURL,
		Model:   model,
	}
}

// askUserConfirmation prompts user for yes/no confirmation
func askUserConfirmation(prompt string) bool {
	fmt.Printf("%s [y/N]: ", prompt)
	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	response = strings.TrimSpace(strings.ToLower(response))
	return response == "y" || response == "yes" || response == "s" || response == "si"
}
