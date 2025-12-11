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
	"github.com/joss/urp/internal/ingest"
	"github.com/joss/urp/internal/opencode/agent"
	"github.com/joss/urp/internal/opencode/domain"
	"github.com/joss/urp/internal/opencode/graphstore"
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
	
	// Auto-ingest in background (non-blocking)
	go autoIngest(gdb, opts.WorkDir)

	// 4. Initialize LLM Provider
	prov, err := initProvider()
	if err != nil {
		gdb.Close()
		return nil, fmt.Errorf("provider init failed: %w", err)
	}
	go warmupConnection(prov)

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
		gateClient := gate.NewOpenAIClient("MODEL_GATE")
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

	// Config overrides
	defaultModel := "claude-sonnet-4-5-20250929"
	if model := os.Getenv("URP_MODEL"); model != "" {
		defaultModel = model
	}
	agentConfig.Model = &domain.ModelConfig{ModelID: defaultModel}

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

func initProvider() (llm.Provider, error) {
	if apiKey := os.Getenv("OPENAI_API_KEY"); apiKey != "" {
		baseURL := os.Getenv("OPENAI_BASE_URL")
		return provider.NewOpenAI(apiKey, baseURL), nil
	}
	if apiKey := os.Getenv("ANTHROPIC_API_KEY"); apiKey != "" {
		baseURL := os.Getenv("ANTHROPIC_BASE_URL")
		return provider.NewAnthropic(apiKey, baseURL), nil
	}
	if authToken := os.Getenv("ANTHROPIC_AUTH_TOKEN"); authToken != "" {
		baseURL := os.Getenv("ANTHROPIC_BASE_URL")
		return provider.NewAnthropic(authToken, baseURL), nil
	}
	return nil, fmt.Errorf("no API key found")
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

func autoIngest(gdb graph.Driver, workDir string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if _, err := os.Stat(workDir); err != nil {
		return
	}
	// Check if already ingested
	projectName := filepath.Base(workDir)
	query := `MATCH (f:File) WHERE f.path STARTS WITH $prefix RETURN count(f) as count`
	records, err := gdb.Execute(ctx, query, map[string]any{"prefix": projectName})
	if err == nil && len(records) > 0 {
		if count, ok := records[0]["count"].(int64); ok && count > 0 {
			return 
		}
	}
	ingester := ingest.NewIngester(gdb)
	ingester.Ingest(ctx, workDir)
	
	gitDir := filepath.Join(workDir, ".git")
	if _, err := os.Stat(gitDir); err == nil {
		gitLoader := ingest.NewGitLoader(gdb, workDir)
		gitLoader.LoadHistory(ctx, 500)
	}
}