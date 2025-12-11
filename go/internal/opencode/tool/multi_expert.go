// Package tool provides multi-expert execution inspired by Poetiq
// N experts run in parallel with different seeds, then vote for best solution
package tool

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/joss/urp/internal/opencode/domain"
	"github.com/joss/urp/pkg/llm"
)

// MultiExpertConfig configures multi-expert execution
// Inspired by Poetiq's solve_parallel_coding
type MultiExpertConfig struct {
	NumExperts         int     // Number of parallel experts (default: 3)
	SeedSpacing        int     // Seed spacing between experts (default: 100)
	VotingStrategy     string  // "diversity" or "majority" (default: diversity)
	MaxIterationsEach  int     // Max iterations per expert (default: 5)
	CountFailedMatches bool    // Include failures with matching outputs (default: true)
	ItersTiebreak      bool    // Use iteration count to break ties (default: false)
	LowToHighIters     bool    // Prefer fast solutions (default: true)
}

// DefaultMultiExpertConfig returns sensible defaults
func DefaultMultiExpertConfig() MultiExpertConfig {
	return MultiExpertConfig{
		NumExperts:         3,
		SeedSpacing:        100,
		VotingStrategy:     "diversity",
		MaxIterationsEach:  5,
		CountFailedMatches: true,
		ItersTiebreak:      false,
		LowToHighIters:     true,
	}
}

// ExpertResult holds the output from a single expert
type ExpertResult struct {
	ExpertID   int
	Seed       int
	Output     string
	Score      float64 // Soft score from validation
	Iteration  int     // Which iteration succeeded
	Success    bool    // Passed all validation
	Duration   time.Duration
	TokensUsed int
	Error      error
}

// VotingBucket groups results by identical outputs
type VotingBucket struct {
	Key     string          // Hash of output for grouping
	Results []ExpertResult
	Score   float64 // Best score in bucket
}

// MultiExpertExecutor runs N experts in parallel with voting
type MultiExpertExecutor struct {
	config   MultiExpertConfig
	provider llm.Provider
	registry *Registry
	workDir  string
}

// NewMultiExpertExecutor creates a multi-expert executor
func NewMultiExpertExecutor(workDir string) *MultiExpertExecutor {
	return &MultiExpertExecutor{
		config:  DefaultMultiExpertConfig(),
		workDir: workDir,
	}
}

// WithConfig sets the multi-expert configuration
func (e *MultiExpertExecutor) WithConfig(config MultiExpertConfig) *MultiExpertExecutor {
	e.config = config
	return e
}

// WithProvider sets the LLM provider
func (e *MultiExpertExecutor) WithProvider(p llm.Provider) *MultiExpertExecutor {
	e.provider = p
	return e
}

// WithRegistry sets the tool registry
func (e *MultiExpertExecutor) WithRegistry(r *Registry) *MultiExpertExecutor {
	e.registry = r
	return e
}

// Execute runs multiple experts in parallel and votes on best result
func (e *MultiExpertExecutor) Execute(
	ctx context.Context,
	prompt string,
	agentConfig domain.Agent,
	validator func(output string) (float64, bool), // Returns (score, success)
) ([]ExpertResult, error) {
	if e.provider == nil {
		return nil, fmt.Errorf("provider not configured")
	}

	// Launch experts in parallel
	results := make([]ExpertResult, e.config.NumExperts)
	var wg sync.WaitGroup

	for i := 0; i < e.config.NumExperts; i++ {
		wg.Add(1)
		go func(expertID int) {
			defer wg.Done()
			seed := expertID * e.config.SeedSpacing
			result := e.runExpert(ctx, expertID, seed, prompt, agentConfig, validator)
			results[expertID] = result
		}(i)
	}

	wg.Wait()

	// Vote on results
	ranked := e.voteOnResults(results)

	return ranked, nil
}

// runExpert runs a single expert with iterative refinement
func (e *MultiExpertExecutor) runExpert(
	ctx context.Context,
	expertID, seed int,
	prompt string,
	agentConfig domain.Agent,
	validator func(output string) (float64, bool),
) ExpertResult {
	startTime := time.Now()

	result := ExpertResult{
		ExpertID: expertID,
		Seed:     seed,
	}

	// Create session for this expert
	session := &domain.Session{
		ID:        ulid.Make().String(),
		Directory: e.workDir,
		CreatedAt: startTime,
		UpdatedAt: startTime,
	}

	// Get enabled tools
	registry := e.registry
	if registry == nil {
		registry = DefaultRegistry(e.workDir)
	}

	var enabledTools []domain.Tool
	for _, tool := range registry.All() {
		if enabled, ok := agentConfig.Tools[tool.Name]; ok && enabled {
			enabledTools = append(enabledTools, tool)
		}
	}

	// System prompt with seed injection for variation
	systemPrompt := fmt.Sprintf(`You are expert %d working on a task.
Your approach may differ from others - embrace creative solutions.
Seed: %d (use for any randomization)

Working directory: %s

Complete the task and return a clear result.`, expertID, seed, e.workDir)

	messages := []domain.Message{{
		ID:        ulid.Make().String(),
		SessionID: session.ID,
		Role:      domain.RoleUser,
		Parts:     []domain.Part{domain.TextPart{Text: prompt}},
		Timestamp: startTime,
	}}

	// Iterative refinement loop
	for iter := 0; iter < e.config.MaxIterationsEach; iter++ {
		select {
		case <-ctx.Done():
			result.Error = ctx.Err()
			result.Duration = time.Since(startTime)
			return result
		default:
		}

		req := &llm.ChatRequest{
			Model:        agentConfig.Model.ModelID,
			Messages:     messages,
			Tools:        enabledTools,
			SystemPrompt: systemPrompt,
			MaxTokens:    8192,
		}

		// Run single turn
		output, newMessages, err := e.runSingleTurn(ctx, session, req, registry)
		if err != nil {
			result.Error = err
			continue
		}

		messages = newMessages
		result.Output = output
		result.Iteration = iter + 1

		// Validate output
		if validator != nil {
			score, success := validator(output)
			result.Score = score
			result.Success = success

			if success {
				break
			}
		} else {
			result.Success = output != ""
			result.Score = 1.0
			break
		}
	}

	result.Duration = time.Since(startTime)
	return result
}

// runSingleTurn executes one LLM turn with tool handling
func (e *MultiExpertExecutor) runSingleTurn(
	ctx context.Context,
	session *domain.Session,
	req *llm.ChatRequest,
	registry *Registry,
) (string, []domain.Message, error) {
	const maxToolCalls = 5
	messages := req.Messages

	for toolRound := 0; toolRound < maxToolCalls; toolRound++ {
		req.Messages = messages

		events, err := e.provider.Chat(ctx, req)
		if err != nil {
			return "", messages, fmt.Errorf("chat: %w", err)
		}

		// Collect response
		var output string
		var toolCalls []domain.ToolCallPart

		for event := range events {
			switch event.Type {
			case domain.StreamEventText:
				output += event.Content
			case domain.StreamEventToolCall:
				if tc, ok := event.Part.(domain.ToolCallPart); ok {
					toolCalls = append(toolCalls, tc)
				}
			case domain.StreamEventError:
				return "", messages, event.Error
			}
		}

		// No tool calls - done
		if len(toolCalls) == 0 {
			return output, messages, nil
		}

		// Build assistant message
		assistantParts := []domain.Part{}
		if output != "" {
			assistantParts = append(assistantParts, domain.TextPart{Text: output})
		}
		for _, tc := range toolCalls {
			assistantParts = append(assistantParts, tc)
		}

		assistantMsg := domain.Message{
			ID:        ulid.Make().String(),
			SessionID: session.ID,
			Role:      domain.RoleAssistant,
			Parts:     assistantParts,
			Timestamp: time.Now(),
		}

		// Execute tools
		var toolResults []domain.Part
		for _, tc := range toolCalls {
			tool, ok := registry.Get(tc.Name)
			if !ok {
				toolResults = append(toolResults, domain.ToolCallPart{
					ToolID: tc.ToolID,
					Name:   tc.Name,
					Error:  fmt.Sprintf("unknown tool: %s", tc.Name),
				})
				continue
			}

			toolResult, err := tool.Execute(ctx, tc.Args)
			errStr := ""
			if err != nil {
				errStr = err.Error()
			} else if toolResult.Error != nil {
				errStr = toolResult.Error.Error()
			}

			toolResults = append(toolResults, domain.ToolCallPart{
				ToolID: tc.ToolID,
				Name:   tc.Name,
				Args:   tc.Args,
				Result: toolResult.Output,
				Error:  errStr,
			})
		}

		// Build tool result message
		toolMsg := domain.Message{
			ID:        ulid.Make().String(),
			SessionID: session.ID,
			Role:      domain.RoleUser,
			Parts:     toolResults,
			Timestamp: time.Now(),
		}

		messages = append(messages, assistantMsg, toolMsg)
	}

	return "", messages, fmt.Errorf("max tool calls reached")
}

// voteOnResults groups and ranks results using Poetiq-style voting
func (e *MultiExpertExecutor) voteOnResults(results []ExpertResult) []ExpertResult {
	// Separate passers and failures
	passBuckets := make(map[string][]ExpertResult)
	failBuckets := make(map[string][]ExpertResult)

	for _, r := range results {
		key := canonicalKey(r.Output)
		if r.Success {
			passBuckets[key] = append(passBuckets[key], r)
		} else {
			failBuckets[key] = append(failBuckets[key], r)
		}
	}

	// Optionally merge failures with matching passers
	if e.config.CountFailedMatches {
		for key, fails := range failBuckets {
			if _, exists := passBuckets[key]; exists {
				passBuckets[key] = append(passBuckets[key], fails...)
				delete(failBuckets, key)
			}
		}
	}

	// Convert to buckets for sorting
	passerGroups := make([][]ExpertResult, 0, len(passBuckets))
	for _, group := range passBuckets {
		passerGroups = append(passerGroups, group)
	}

	failureGroups := make([][]ExpertResult, 0, len(failBuckets))
	for _, group := range failBuckets {
		failureGroups = append(failureGroups, group)
	}

	// Sort within each group by score/iterations
	sortGroup := func(group []ExpertResult) {
		sort.Slice(group, func(i, j int) bool {
			if e.config.ItersTiebreak {
				if e.config.LowToHighIters {
					return group[i].Iteration < group[j].Iteration
				}
				return group[i].Iteration > group[j].Iteration
			}
			return group[i].Score > group[j].Score
		})
	}

	for _, g := range passerGroups {
		sortGroup(g)
	}
	for _, g := range failureGroups {
		sortGroup(g)
	}

	// Sort groups by vote count (size)
	sort.Slice(passerGroups, func(i, j int) bool {
		return len(passerGroups[i]) > len(passerGroups[j])
	})
	sort.Slice(failureGroups, func(i, j int) bool {
		if len(failureGroups[i]) != len(failureGroups[j]) {
			return len(failureGroups[i]) > len(failureGroups[j])
		}
		// Tie-break by best score
		return meanScore(failureGroups[i]) > meanScore(failureGroups[j])
	})

	// Build diversity-first ranking (Poetiq's key insight)
	var ranked []ExpertResult

	// One representative from each passing group (diversity)
	for _, g := range passerGroups {
		if len(g) > 0 {
			ranked = append(ranked, g[0])
		}
	}

	// One representative from each failing group (diversity)
	for _, g := range failureGroups {
		if len(g) > 0 {
			ranked = append(ranked, g[0])
		}
	}

	// Remaining passers
	for _, g := range passerGroups {
		if len(g) > 1 {
			ranked = append(ranked, g[1:]...)
		}
	}

	// Remaining failures
	for _, g := range failureGroups {
		if len(g) > 1 {
			ranked = append(ranked, g[1:]...)
		}
	}

	return ranked
}

// canonicalKey creates a hash for grouping identical outputs
func canonicalKey(output string) string {
	// Simple: use first 500 chars of output
	if len(output) > 500 {
		return output[:500]
	}
	return output
}

// meanScore calculates average score for a group
func meanScore(group []ExpertResult) float64 {
	if len(group) == 0 {
		return 0.0
	}
	total := 0.0
	for _, r := range group {
		total += r.Score
	}
	return total / float64(len(group))
}

// MultiExpertTask implements the multi-expert Task tool
type MultiExpertTask struct {
	*Task
	multiConfig MultiExpertConfig
}

// NewMultiExpertTask creates a multi-expert version of the Task tool
func NewMultiExpertTask(workDir string) *MultiExpertTask {
	return &MultiExpertTask{
		Task:        NewTask(workDir),
		multiConfig: DefaultMultiExpertConfig(),
	}
}

// WithMultiConfig sets the multi-expert configuration
func (t *MultiExpertTask) WithMultiConfig(config MultiExpertConfig) *MultiExpertTask {
	t.multiConfig = config
	return t
}

// Info returns the tool definition
func (t *MultiExpertTask) Info() domain.Tool {
	return domain.Tool{
		Name:        "multi_task",
		Description: "Launch multiple experts in parallel to solve a complex task, then vote on best solution",
		Parameters: domain.JSONSchema{
			"type": "object",
			"properties": map[string]any{
				"description": map[string]any{
					"type":        "string",
					"description": "Short (3-5 word) description of the task",
				},
				"prompt": map[string]any{
					"type":        "string",
					"description": "Detailed task for the experts to solve",
				},
				"num_experts": map[string]any{
					"type":        "integer",
					"description": "Number of parallel experts (default: 3)",
				},
				"voting": map[string]any{
					"type":        "string",
					"description": "Voting strategy: diversity or majority (default: diversity)",
					"enum":        []string{"diversity", "majority"},
				},
			},
			"required": []string{"prompt"},
		},
	}
}

// Execute runs the multi-expert task
func (t *MultiExpertTask) Execute(ctx context.Context, args map[string]any) (*Result, error) {
	description, _ := args["description"].(string)
	prompt, _ := args["prompt"].(string)

	if prompt == "" {
		return &Result{Error: fmt.Errorf("prompt is required")}, nil
	}

	// Override config from args
	config := t.multiConfig
	if ne, ok := args["num_experts"].(float64); ok {
		config.NumExperts = int(ne)
	}
	if voting, ok := args["voting"].(string); ok {
		config.VotingStrategy = voting
	}

	// Get agent config
	agentCfg := defaultSubAgents()["build"]

	// Create executor
	executor := NewMultiExpertExecutor(t.workDir).
		WithConfig(config).
		WithProvider(t.provider).
		WithRegistry(t.registry)

	// Simple validator: non-empty output is success
	validator := func(output string) (float64, bool) {
		if output == "" {
			return 0.0, false
		}
		return 1.0, true
	}

	// Execute
	results, err := executor.Execute(ctx, prompt, agentCfg, validator)
	if err != nil {
		return &Result{
			Title: fmt.Sprintf("Multi-expert failed: %s", description),
			Error: err,
		}, nil
	}

	if len(results) == 0 {
		return &Result{
			Title: fmt.Sprintf("Multi-expert failed: %s", description),
			Error: fmt.Errorf("no results from experts"),
		}, nil
	}

	// Return best result
	best := results[0]
	return &Result{
		Title:  fmt.Sprintf("Multi-expert completed: %s", description),
		Output: best.Output,
		Metadata: map[string]any{
			"num_experts":  config.NumExperts,
			"voting":       config.VotingStrategy,
			"best_score":   best.Score,
			"best_expert":  best.ExpertID,
			"iterations":   best.Iteration,
			"duration_ms":  best.Duration.Milliseconds(),
			"total_results": len(results),
		},
	}, nil
}
