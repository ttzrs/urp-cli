package bootstrap

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/joss/urp/internal/compiler"
	"github.com/joss/urp/internal/config"
	"github.com/joss/urp/internal/gate"
	"github.com/joss/urp/internal/graph"
	"github.com/joss/urp/internal/opencode/agent"
	"github.com/joss/urp/internal/opencode/domain"
	"github.com/joss/urp/internal/opencode/graphstore"
	"github.com/joss/urp/internal/opencode/model"
	"github.com/joss/urp/internal/opencode/provider"
	"github.com/joss/urp/internal/opencode/tool"
	"github.com/joss/urp/internal/orchestrator"
	"github.com/joss/urp/pkg/llm"
)

// App holds the wired application components.
type App struct {
	Agent         *agent.Agent
	LearningAgent *agent.LearningAgent
	Store         *graphstore.Store
	Provider      llm.Provider
	GraphDB       graph.Driver
	WorkDir       string
	Orchestrator  *orchestrator.Orchestrator
}

// InitializeOptions configures the bootstrap process.
type InitializeOptions struct {
	WorkDir        string
	ThinkingBudget int
}

// Initialize sets up the entire application stack.
func Initialize(ctx context.Context, opts InitializeOptions) (*App, error) {
	// 1. Load Environment
	loadEnvFile()

	// 2. Connect to Graph (Memgraph)
	// REQUIRED: We need graph for state and memory
	graph.SetEnvLookup(os.LookupEnv)
	gdb, err := graph.Connect()
	if err != nil {
		return nil, fmt.Errorf("memgraph required: %w (run: docker compose up -d memgraph)", err)
	}

	// 3. Setup Stores & Ingestion
	store := graphstore.New(gdb)
	tool.SetGraphDB(gdb)

	// 4. Initialize LLM Provider (Master Agent)
	masterConfig := config.GetMasterModelConfig()

	// Check both URP_MASTER_MODEL_ID and URP_DEFAULT_MASTER_MODEL to debug override issue
	actualMasterModel := os.Getenv("URP_MASTER_MODEL_ID")
	actualDefaultModel := os.Getenv("URP_DEFAULT_MASTER_MODEL")
	fmt.Printf("[DEBUG] Initialize: URP_MASTER_MODEL_ID='%s', URP_DEFAULT_MASTER_MODEL='%s'\n", actualMasterModel, actualDefaultModel)
	fmt.Printf("[DEBUG] Initialize: masterModelID='%s', fallbacks='%v'\n", masterConfig.ModelID, masterConfig.Fallbacks)

	// FIX #3: Build provider options with proper precedence
	// Priority: Proxy > DeepSeek > OpenAI > Anthropic
	// (Last option wins in current factory implementation)
	// We pass them in reverse priority so proxy overrides others
	providerOpts := []provider.ConfigOption{}

	// Lowest priority: Anthropic
	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		providerOpts = append(providerOpts, provider.WithAPIKey(key))
	}
	if url := os.Getenv("ANTHROPIC_BASE_URL"); url != "" {
		providerOpts = append(providerOpts, provider.WithBaseURL(url))
	}

	// OpenAI/OpenRouter (medium priority)
	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		providerOpts = append(providerOpts, provider.WithAPIKey(key))
	}
	if url := os.Getenv("OPENAI_BASE_URL"); url != "" {
		providerOpts = append(providerOpts, provider.WithBaseURL(url))
	}

	// DeepSeek (higher priority)
	if key := os.Getenv("DEEPSEEK_API_KEY"); key != "" {
		providerOpts = append(providerOpts, provider.WithAPIKey(key))
	}
	if url := os.Getenv("DEEPSEEK_BASE_URL"); url != "" {
		providerOpts = append(providerOpts, provider.WithBaseURL(url))
	}

	// Proxy (HIGHEST priority - overrides everything if set)
	if key := os.Getenv("PROXY_API_KEY"); key != "" {
		providerOpts = append(providerOpts, provider.WithAPIKey(key))
		fmt.Printf("[DEBUG] Bootstrap: Using PROXY_API_KEY for authentication\n")
	}
	if key := os.Getenv("UNIFIED_API_KEY"); key != "" {
		providerOpts = append(providerOpts, provider.WithAPIKey(key))
		fmt.Printf("[DEBUG] Bootstrap: Using UNIFIED_API_KEY for authentication\n")
	}
	if url := os.Getenv("PROXY_BASE_URL"); url != "" {
		providerOpts = append(providerOpts, provider.WithBaseURL(url))
		fmt.Printf("[DEBUG] Bootstrap: Using PROXY_BASE_URL endpoint\n")
	}
	if url := os.Getenv("UNIFIED_BASE_URL"); url != "" {
		providerOpts = append(providerOpts, provider.WithBaseURL(url))
		fmt.Printf("[DEBUG] Bootstrap: Using UNIFIED_BASE_URL endpoint\n")
	}

	// Use the new fallback system
	prov, resolvedMasterModelID, err := config.GetModelWithFallback(
		masterConfig.ModelID,
		masterConfig.Fallbacks,
		providerOpts...,
	)
	if err != nil {
		gdb.Close()
		return nil, fmt.Errorf("provider init failed: %w", err)
	}
	go warmupConnection(prov)

	fmt.Printf("[DEBUG] Initialize: Provider initialized with resolved model ID '%s' (Provider: %s)\n", resolvedMasterModelID, prov.Name())

	// 5. Initialize Context Compiler (V2 Architecture)
	var ctxCompiler *compiler.ContextCompiler
	var embedStore *agent.EmbeddingStore
	
	// Setup Vector Store (LanceDB)
	vectorPath := config.GetPaths().Vectors
	if err := os.MkdirAll(vectorPath, 0755); err == nil {
		es, err := agent.NewEmbeddingStore(vectorPath)
		if err == nil {
			embedStore = es
		}
	}

	if gdb != nil {
		compStore := compiler.NewStore(gdb)
		// Create Gate client - it will use env vars for configuration
		// If URP_GATE_MODEL_ID is not set, it defaults to gpt-4o-mini
		// The Gate is optional - if no API key is configured, it returns empty results
		gateClient := gate.NewOpenAIClient("")
		if gateClient.IsConfigured() {
			fmt.Printf("[DEBUG] Gate configured: model=%s, url=%s\n", gateClient.Model(), gateClient.BaseURL)
		} else {
			fmt.Printf("[DEBUG] Gate not configured (no API key), context filtering disabled\n")
		}
		ctxCompiler = compiler.NewContextCompiler(compStore, gateClient)

		// Connect Retrieval (End of Cycle)
		if embedStore != nil {
			ctxCompiler.SetStrategyRetriever(&retrievalAdapter{store: embedStore})
		}
	}

	// 6. Create Agent & Tools
	tools := tool.DefaultRegistry(opts.WorkDir)
	
	// Setup Orchestrator if not in container (Master Mode)
	// This enables Master-Worker isolation for dangerous tools
	var orch *orchestrator.Orchestrator
	if !config.InContainer() {
		orch = orchestrator.New()
		workerID := "urp-worker-" + filepath.Base(opts.WorkDir)
		
		// Spawn worker in background
		go func() {
			// We mount the workdir to /workspace in the worker
			if err := orch.SpawnWorkerContainer(context.Background(), workerID, opts.WorkDir); err != nil {
				// Log error? For now silent, tools will fail/fallback if worker dies
				// fmt.Fprintf(os.Stderr, "Worker spawn failed: %v\n", err)
			}
		}()
		
		// Configure tools to use remote executor
		remoteExec := NewRemoteExecutor(orch, workerID)
		
		if t, ok := tools.Get("bash"); ok {
			if bash, ok := t.(*tool.Bash); ok {
				bash.SetExecutor(remoteExec)
			}
		}
		if t, ok := tools.Get("sandbox"); ok {
			if sandbox, ok := t.(*tool.SandboxTool); ok {
				sandbox.SetExecutor(remoteExec)
			}
		}
	}

	agentConfig := agent.BuiltinAgents()["build"]

	// Set the resolved master model ID
	agentConfig.Model = &domain.ModelConfig{ModelID: resolvedMasterModelID}
	// Debugging: write the resolved model ID to a file in the container
	os.WriteFile("/tmp/agent_model.log", []byte(resolvedMasterModelID), 0644)

	// DeepSeek Provider (Optional)
	var deepseekProv llm.Provider
	if ds := provider.NewDeepSeekProvider(); ds.IsConfigured() {
		deepseekProv = ds
	}

	// Construct Base Agent
	ag := agent.New(agentConfig, prov, tools, 
		agent.WithDeepSeekProvider(deepseekProv),
		agent.WithContextCompiler(ctxCompiler),
	)
	ag.SetWorkDir(opts.WorkDir)
	if opts.ThinkingBudget > 0 {
		ag.SetThinkingBudget(opts.ThinkingBudget)
	} else if tb := os.Getenv("URP_THINKING"); tb != "" {
		var budget int
		if _, err := fmt.Sscanf(tb, "%d", &budget); err == nil && budget > 0 {
			ag.SetThinkingBudget(budget)
		}
	}

	// 7. Wrap with Learning Agent
	stratStore := agent.NewGraphStrategyStore(gdb)
	la, _ := agent.NewLearningAgent(ag, stratStore, vectorPath)

	return &App{
		Agent:         ag,
		LearningAgent: la,
		Store:         store,
		Provider:      prov,
		GraphDB:       gdb,
		WorkDir:       opts.WorkDir,
		Orchestrator:  orch,
	}, nil
}

// Close releases resources.
func (app *App) Close() {
	if app.Orchestrator != nil {
		app.Orchestrator.Shutdown()
	}
	if app.GraphDB != nil {
		app.GraphDB.Close()
	}
	if app.LearningAgent != nil {
		app.LearningAgent.Close()
	}
}

// --- Internal Helpers ---


type retrievalAdapter struct {
	store *agent.EmbeddingStore
}

func (r *retrievalAdapter) GetStrategyHint(ctx context.Context, goal string) (string, error) {
	env := "go" // TODO: Detect environment
	similar, err := r.store.FindSimilarSuccessful(ctx, goal, env, 3)
	if err != nil {
		return "", err
	}
	if len(similar) == 0 {
		return "", nil
	}

	var sb strings.Builder
	sb.WriteString("Similar successful tasks:\n")
	for _, t := range similar {
		sb.WriteString(fmt.Sprintf("- %s (Strategy: %s, Similarity: %.2f)\n", t.Objective, t.StrategyUsed, t.Similarity))
	}
	
	stratName, confidence := agent.GetStrategyFromSimilar(similar)
	if stratName != "" {
		sb.WriteString(fmt.Sprintf("\nRecommended Strategy: %s (Confidence: %.2f)\n", stratName, confidence))
	}
	
	return sb.String(), nil
}

func loadEnvFile() {
	envPath := config.GetPaths().EnvFile
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
		if os.Getenv(key) == "" {
			os.Setenv(key, value)
		}
	}
}

// initProvider initializes the LLM provider based on a primary and fallback model ID.
func initProvider(masterModelID, fallbackModelID string) (llm.Provider, string, error) {
	fmt.Printf("[DEBUG] initProvider: Entering function with masterModelID='%s', fallbackModelID='%s'\n", masterModelID, fallbackModelID)
	fmt.Printf("[DEBUG] initProvider: Attempting primary model '%s'\n", masterModelID)

	// SPECIAL CASE: If master model is zai-glm-4.6, create unified provider directly with proxy settings
	if masterModelID == "zai-glm-4.6" {
		// Get proxy configuration directly from environment
		apiKey := getProxyAPIKey()
		baseURL := getProxyBaseURL()

		if apiKey != "" && baseURL != "" {
			fmt.Printf("[DEBUG] initProvider: Detected zai-glm-4.6, creating unified provider with proxy: %s\n", baseURL)
			unifiedProvider := provider.NewUnifiedProvider(apiKey, baseURL, model.DefaultModelRegistry)

			// Test the provider by attempting to validate it can handle the model
			// At minimum, we know the provider created successfully
			fmt.Printf("[DEBUG] initProvider: Successfully created unified provider for zai-glm-4.6\n")
			return unifiedProvider, masterModelID, nil
		} else {
			fmt.Println("[DEBUG] initProvider: Warning: zai-glm-4.6 specified but proxy credentials not configured")
		}
	}

	// For other models or if proxy not available, use original logic
	var prov llm.Provider
	var resolvedModelID string
	var err error

	// Try to create provider for the primary model using original logic
	prov, resolvedModelID, err = provider.Default.CreateForModel(masterModelID,
		provider.WithAPIKey(os.Getenv("ANTHROPIC_API_KEY")),
		provider.WithAPIKey(os.Getenv("OPENAI_API_KEY")), // OpenRouter uses OpenAI API key
		provider.WithAPIKey(os.Getenv("DEEPSEEK_API_KEY")),
		provider.WithAPIKey(os.Getenv("UNIFIED_API_KEY")), // Proxy API key
		provider.WithAPIKey(os.Getenv("PROXY_API_KEY")), // Fallback proxy key
		provider.WithBaseURL(os.Getenv("ANTHROPIC_BASE_URL")),
		provider.WithBaseURL(os.Getenv("OPENAI_BASE_URL")),
		provider.WithBaseURL(os.Getenv("DEEPSEEK_BASE_URL")),
		provider.WithBaseURL(os.Getenv("UNIFIED_BASE_URL")), // Proxy base URL
		provider.WithBaseURL(os.Getenv("PROXY_BASE_URL")), // Fallback proxy URL
	)

	if err != nil || prov == nil {
		fmt.Printf("[DEBUG] initProvider: Warning: Failed to initialize primary URP_MASTER_MODEL '%s': %v\n", masterModelID, err)
		// Attempt fallback
		fmt.Printf("[DEBUG] initProvider: Attempting to use fallback model: '%s'\n", fallbackModelID)

		// Special handling for fallback if it's also zai-glm-4.6
		if fallbackModelID == "zai-glm-4.6" {
			// Create unified provider for fallback zai-glm-4.6
			apiKey := getProxyAPIKey()
			baseURL := getProxyBaseURL()

			if apiKey != "" && baseURL != "" {
				fmt.Printf("[DEBUG] initProvider: Detected fallback zai-glm-4.6, creating unified provider with proxy: %s\n", baseURL)
				unifiedProvider := provider.NewUnifiedProvider(apiKey, baseURL, model.DefaultModelRegistry)

				fmt.Printf("[DEBUG] initProvider: Successfully created unified provider for fallback zai-glm-4.6\n")
				return unifiedProvider, fallbackModelID, nil
			} else {
				fmt.Println("[DEBUG] initProvider: Warning: fallback zai-glm-4.6 but proxy credentials not configured")
			}
		}

		// Original fallback logic if unified provider not applicable
		fallbackProv, resolvedFallbackModelID, fallbackErr := provider.Default.CreateForModel(fallbackModelID,
			provider.WithAPIKey(os.Getenv("ANTHROPIC_API_KEY")),
			provider.WithAPIKey(os.Getenv("OPENAI_API_KEY")),
			provider.WithAPIKey(os.Getenv("DEEPSEEK_API_KEY")),
			provider.WithAPIKey(os.Getenv("UNIFIED_API_KEY")), // Proxy API key
			provider.WithAPIKey(os.Getenv("PROXY_API_KEY")), // Fallback proxy key
			provider.WithBaseURL(os.Getenv("ANTHROPIC_BASE_URL")),
			provider.WithBaseURL(os.Getenv("OPENAI_BASE_URL")),
			provider.WithBaseURL(os.Getenv("DEEPSEEK_BASE_URL")),
			provider.WithBaseURL(os.Getenv("UNIFIED_BASE_URL")), // Proxy base URL
			provider.WithBaseURL(os.Getenv("PROXY_BASE_URL")), // Fallback proxy URL
		)

		if fallbackErr != nil || fallbackProv == nil {
			return nil, "", fmt.Errorf("failed to initialize primary or fallback LLM provider: %v. Fallback error: %v", err, fallbackErr)
		}
		fmt.Printf("[DEBUG] initProvider: Successfully initialized fallback model: %s (Provider: %s)\n", resolvedFallbackModelID, fallbackProv.Name())
		return fallbackProv, resolvedFallbackModelID, nil
	}
	fmt.Printf("[DEBUG] initProvider: Successfully initialized primary URP_MASTER_MODEL: %s (Provider: %s)\n", resolvedModelID, prov.Name())
	return prov, resolvedModelID, nil
}

// getProxyAPIKey gets the proxy API key from environment
func getProxyAPIKey() string {
	apiKey := os.Getenv("UNIFIED_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("PROXY_API_KEY")
	}
	if apiKey == "" {
		apiKey = "sk-urp-proxy-key"  // Default if none provided
	}
	return apiKey
}

// getProxyBaseURL gets the proxy base URL from environment
func getProxyBaseURL() string {
	baseURL := os.Getenv("UNIFIED_BASE_URL")
	if baseURL == "" {
		baseURL = os.Getenv("PROXY_BASE_URL")
	}
	if baseURL == "" {
		baseURL = "http://tizz.win:8317/v1"  // Default proxy URL
	}
	return baseURL
}

func createUnifiedProviderForProxy() (llm.Provider, error) {
	fmt.Println("[DEBUG] Creating unified provider for proxy...")
	// Get proxy configuration from environment
	apiKey := os.Getenv("UNIFIED_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("PROXY_API_KEY")
	}
	if apiKey == "" {
		apiKey = "sk-urp-proxy-key" // Default if none provided
	}

	baseURL := os.Getenv("UNIFIED_BASE_URL")
	if baseURL == "" {
		baseURL = os.Getenv("PROXY_BASE_URL")
	}
	if baseURL == "" {
		baseURL = "http://tizz.win:8317/v1" // Default proxy URL
	}

	fmt.Printf("[DEBUG] Creating unified provider with API key: %s and Base URL: %s\n", apiKey, baseURL)

	// Create unified provider with proxy settings
	// Use the model registry from internal/opencode/model package
	prov := provider.NewUnifiedProvider(apiKey, baseURL, model.DefaultModelRegistry)
	return prov, nil
}

func warmupConnection(prov llm.Provider) {
	baseURL := os.Getenv("OPENAI_BASE_URL")
	if baseURL == "" {
		baseURL = os.Getenv("ANTHROPIC_BASE_URL")
	}
	if baseURL == "" {
		return
	}
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