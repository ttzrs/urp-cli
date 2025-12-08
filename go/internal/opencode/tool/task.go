package tool

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/joss/urp/internal/opencode/domain"
	"github.com/joss/urp/pkg/llm"
)

// SubAgentExecutor runs a subagent with isolated context
type SubAgentExecutor interface {
	Run(ctx context.Context, cfg SubAgentConfig) (*SubAgentResult, error)
}

// SubAgentConfig configures a subagent execution
type SubAgentConfig struct {
	Type        string        // "explore", "plan", "build"
	Prompt      string        // Task description
	Model       string        // Override model (optional)
	WorkDir     string        // Working directory
	Timeout     time.Duration // Execution timeout
	ParentID    string        // Parent session ID for linking
}

// SubAgentResult holds the output of a subagent execution
type SubAgentResult struct {
	TaskID   string
	Output   string
	Messages []domain.Message
	Duration time.Duration
	Error    error
}

// Task implements the Task tool for spawning subagents
type Task struct {
	workDir     string
	provider    llm.Provider
	registry    *Registry
	agentConfig map[string]domain.Agent
}

// NewTask creates a new Task tool
func NewTask(workDir string) *Task {
	return &Task{
		workDir:     workDir,
		agentConfig: defaultSubAgents(),
	}
}

// WithProvider sets the LLM provider for subagent execution
func (t *Task) WithProvider(p llm.Provider) *Task {
	t.provider = p
	return t
}

// WithRegistry sets the tool registry for subagents
func (t *Task) WithRegistry(r *Registry) *Task {
	t.registry = r
	return t
}

func (t *Task) Info() domain.Tool {
	return domain.Tool{
		Name:        "task",
		Description: "Launch a subagent to handle a complex task autonomously",
		Parameters: domain.JSONSchema{
			"type": "object",
			"properties": map[string]any{
				"description": map[string]any{
					"type":        "string",
					"description": "Short (3-5 word) description of the task",
				},
				"prompt": map[string]any{
					"type":        "string",
					"description": "Detailed task for the subagent to perform",
				},
				"subagent_type": map[string]any{
					"type":        "string",
					"description": "Type of subagent: explore, plan, or build",
					"enum":        []string{"explore", "plan", "build"},
				},
				"model": map[string]any{
					"type":        "string",
					"description": "Model to use (optional, inherits from parent)",
				},
			},
			"required": []string{"prompt", "subagent_type"},
		},
	}
}

func (t *Task) Execute(ctx context.Context, args map[string]any) (*Result, error) {
	description, _ := args["description"].(string)
	prompt, _ := args["prompt"].(string)
	subagentType, _ := args["subagent_type"].(string)
	model, _ := args["model"].(string)

	if prompt == "" {
		return &Result{Error: fmt.Errorf("prompt is required")}, nil
	}

	if subagentType == "" {
		subagentType = "explore"
	}

	// Get agent config
	agentCfg, ok := t.agentConfig[subagentType]
	if !ok {
		return &Result{
			Error: fmt.Errorf("unknown subagent type: %s (available: explore, plan, build)", subagentType),
		}, nil
	}

	if model != "" {
		agentCfg.Model.ModelID = model
	}

	// Check if we can run (need provider)
	if t.provider == nil {
		return &Result{
			Output: fmt.Sprintf("[Task: %s]\nSubagent type: %s\nPrompt: %s\n\n(Provider not configured - task logged but not executed)",
				description, subagentType, prompt),
		}, nil
	}

	// Create task ID
	taskID := ulid.Make().String()

	// Run subagent
	result, err := t.runSubAgent(ctx, taskID, agentCfg, prompt)
	if err != nil {
		return &Result{
			Title: fmt.Sprintf("Task failed: %s", description),
			Error: err,
		}, nil
	}

	return &Result{
		Title:  fmt.Sprintf("Task completed: %s", description),
		Output: result.Output,
		Metadata: map[string]any{
			"task_id":       taskID,
			"subagent_type": subagentType,
			"duration_ms":   result.Duration.Milliseconds(),
			"message_count": len(result.Messages),
		},
	}, nil
}

func (t *Task) runSubAgent(ctx context.Context, taskID string, cfg domain.Agent, prompt string) (*SubAgentResult, error) {
	start := time.Now()

	// Create a minimal session for the subagent
	session := &domain.Session{
		ID:        taskID,
		Directory: t.workDir,
		CreatedAt: start,
		UpdatedAt: start,
	}

	// Get tools for this agent type
	registry := t.registry
	if registry == nil {
		registry = DefaultRegistry(t.workDir)
	}

	// Build enabled tools list
	var enabledTools []domain.Tool
	for _, tool := range registry.All() {
		if enabled, ok := cfg.Tools[tool.Name]; ok && enabled {
			enabledTools = append(enabledTools, tool)
		}
	}

	// Create system prompt with task context injection point
	systemPrompt := fmt.Sprintf(`You are a %s subagent. Your task is to complete the following request autonomously.

Working directory: %s

Guidelines:
- Complete the task efficiently
- Return a clear summary of what you found or did
- Do not ask questions - make reasonable decisions
- FOCUS: Only do what was asked. No extra features/docs/tests.

Agent type: %s
Description: %s

<task-context>
OBJECTIVE: %s
PHASE: executing
RULES: Complete this specific task. Don't expand scope.
</task-context>`, cfg.Name, t.workDir, cfg.Name, cfg.Description, truncatePrompt(prompt, 150))

	messages := []domain.Message{{
		ID:        ulid.Make().String(),
		SessionID: taskID,
		Role:      domain.RoleUser,
		Parts:     []domain.Part{domain.TextPart{Text: prompt}},
		Timestamp: start,
	}}

	req := &llm.ChatRequest{
		Model:        cfg.Model.ModelID,
		Messages:     messages,
		Tools:        enabledTools,
		SystemPrompt: systemPrompt,
		MaxTokens:    8192,
	}

	// Run the subagent with tool loop
	result, err := t.runSubAgentLoop(ctx, session, req, registry, cfg, start)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// runSubAgentLoop executes the subagent with tool call handling
func (t *Task) runSubAgentLoop(
	ctx context.Context,
	session *domain.Session,
	req *llm.ChatRequest,
	registry *Registry,
	cfg domain.Agent,
	start time.Time,
) (*SubAgentResult, error) {
	const maxTurns = 10
	allMessages := req.Messages

	for turn := 0; turn < maxTurns; turn++ {
		events, err := t.provider.Chat(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("subagent chat: %w", err)
		}

		// Collect response
		var output strings.Builder
		var toolCalls []domain.ToolCallPart

		for event := range events {
			switch event.Type {
			case domain.StreamEventText:
				output.WriteString(event.Content)
			case domain.StreamEventToolCall:
				if tc, ok := event.Part.(domain.ToolCallPart); ok {
					toolCalls = append(toolCalls, tc)
				}
			case domain.StreamEventError:
				return nil, event.Error
			}
		}

		// If no tool calls, we're done
		if len(toolCalls) == 0 {
			return &SubAgentResult{
				TaskID:   session.ID,
				Output:   output.String(),
				Messages: allMessages,
				Duration: time.Since(start),
			}, nil
		}

		// Build assistant message with tool calls
		assistantParts := []domain.Part{}
		if output.Len() > 0 {
			assistantParts = append(assistantParts, domain.TextPart{Text: output.String()})
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

			result, err := tool.Execute(ctx, tc.Args)
			if err != nil {
				toolResults = append(toolResults, domain.ToolCallPart{
					ToolID: tc.ToolID,
					Name:   tc.Name,
					Error:  err.Error(),
				})
				continue
			}

			errStr := ""
			if result.Error != nil {
				errStr = result.Error.Error()
			}
			toolResults = append(toolResults, domain.ToolCallPart{
				ToolID: tc.ToolID,
				Name:   tc.Name,
				Args:   tc.Args,
				Result: result.Output,
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

		// Update messages for next turn
		allMessages = append(allMessages, assistantMsg, toolMsg)
		req.Messages = allMessages
	}

	// Max turns reached
	return &SubAgentResult{
		TaskID:   session.ID,
		Output:   "[SubAgent reached max turns]",
		Messages: allMessages,
		Duration: time.Since(start),
	}, nil
}

// truncatePrompt truncates a prompt for display in task context
func truncatePrompt(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

// defaultSubAgents returns built-in subagent configurations
func defaultSubAgents() map[string]domain.Agent {
	return map[string]domain.Agent{
		"explore": {
			Name:        "explore",
			Description: "Fast exploration of codebase structure and content",
			Mode:        domain.AgentModeSubagent,
			Model:       &domain.ModelConfig{ModelID: "claude-sonnet-4-20250514"},
			Tools: map[string]bool{
				"read": true,
				"glob": true,
				"grep": true,
				"ls":   true,
			},
			Permissions: domain.AgentPermissions{
				Edit: domain.PermissionDeny,
			},
		},
		"plan": {
			Name:        "plan",
			Description: "Analyze requirements and create implementation plans",
			Mode:        domain.AgentModeSubagent,
			Model:       &domain.ModelConfig{ModelID: "claude-sonnet-4-20250514"},
			Tools: map[string]bool{
				"read": true,
				"glob": true,
				"grep": true,
				"ls":   true,
			},
			Permissions: domain.AgentPermissions{
				Edit: domain.PermissionDeny,
			},
		},
		"build": {
			Name:        "build",
			Description: "Execute coding tasks with full tool access",
			Mode:        domain.AgentModeSubagent,
			Model:       &domain.ModelConfig{ModelID: "claude-sonnet-4-20250514"},
			Tools: map[string]bool{
				"bash":  true,
				"read":  true,
				"write": true,
				"edit":  true,
				"glob":  true,
				"grep":  true,
				"ls":    true,
			},
			Permissions: domain.AgentPermissions{
				Edit:        domain.PermissionAllow,
				ExternalDir: domain.PermissionAllow,
				Bash: map[string]domain.Permission{
					"*": domain.PermissionAllow,
				},
			},
		},
	}
}
