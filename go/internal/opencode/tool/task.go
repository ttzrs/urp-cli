package tool

import (
	"context"
	"fmt"
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
	_ = &domain.Session{
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

	// Create request
	systemPrompt := fmt.Sprintf(`You are a %s subagent. Your task is to complete the following request autonomously.

Working directory: %s

Guidelines:
- Complete the task efficiently
- Return a clear summary of what you found or did
- Do not ask questions - make reasonable decisions

Agent type: %s
Description: %s`, cfg.Name, t.workDir, cfg.Name, cfg.Description)

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

	// Run the subagent loop (simplified - single turn for now)
	events, err := t.provider.Chat(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("subagent chat: %w", err)
	}

	// Collect response
	var output string
	for event := range events {
		switch event.Type {
		case domain.StreamEventText:
			output += event.Content
		case domain.StreamEventError:
			return nil, event.Error
		}
	}

	return &SubAgentResult{
		TaskID:   taskID,
		Output:   output,
		Messages: messages,
		Duration: time.Since(start),
	}, nil
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
