package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/joss/urp/internal/opencode/domain"
	"github.com/joss/urp/internal/opencode/hook"
	"github.com/joss/urp/internal/opencode/permission"
	"github.com/joss/urp/internal/opencode/tool"
	"github.com/joss/urp/pkg/llm"
)

// MessageCallback is called when a message should be persisted
type MessageCallback func(ctx context.Context, msg *domain.Message) error

// AutocorrectionConfig defines behavior for automatic retry on test failures
type AutocorrectionConfig struct {
	Enabled    bool     // Enable autocorrection loop
	MaxRetries int      // Maximum retry attempts (default: 3)
	Patterns   []string // Patterns that trigger retry (e.g., "FAIL", "error:", "panic:")
}

// DefaultAutocorrection returns sensible defaults for autocorrection
func DefaultAutocorrection() AutocorrectionConfig {
	return AutocorrectionConfig{
		Enabled:    true,
		MaxRetries: 3,
		Patterns:   []string{"FAIL", "--- FAIL:", "panic:", "error:", "Error:", "failed"},
	}
}

// Agent orchestrates conversations with an LLM
type Agent struct {
	config          domain.Agent
	provider        llm.Provider
	tools           tool.ToolRegistry
	executor        *ToolExecutor
	hooks           *hook.Registry
	workDir         string
	thinkingBudget  int                  // Extended thinking token budget (0 = disabled)
	onMessage       MessageCallback      // Called to persist messages
	autocorrection  AutocorrectionConfig // Autocorrection settings
	retryCount      int                  // Current retry counter
}

// New creates an Agent with its dependencies (uses interfaces)
func New(config domain.Agent, provider llm.Provider, tools tool.ToolRegistry) *Agent {
	hooks := hook.NewRegistry()
	a := &Agent{
		config:   config,
		provider: provider,
		tools:    tools,
		hooks:    hooks,
	}
	// Initialize executor with default permissions and shared hooks
	a.executor = NewToolExecutor(tools, nil).WithHooks(hooks)
	return a
}

// Hooks returns the hook registry for external registration
func (a *Agent) Hooks() *hook.Registry {
	return a.hooks
}

// WithHooks sets a custom hook registry
func (a *Agent) WithHooks(hooks *hook.Registry) *Agent {
	a.hooks = hooks
	a.executor = a.executor.WithHooks(hooks)
	return a
}

// SetWorkDir sets up the permission manager with the work directory
func (a *Agent) SetWorkDir(workDir string) {
	a.workDir = workDir
	perms := permission.NewManager(a.config.Permissions, workDir)
	a.executor = NewToolExecutor(a.tools, perms).WithHooks(a.hooks)
}

// SetThinkingBudget sets the extended thinking token budget
func (a *Agent) SetThinkingBudget(budget int) {
	a.thinkingBudget = budget
}

// OnMessage sets the callback for message persistence
func (a *Agent) OnMessage(cb MessageCallback) {
	a.onMessage = cb
}

// EnableAutocorrection configures the autocorrection loop
func (a *Agent) EnableAutocorrection(config AutocorrectionConfig) {
	a.autocorrection = config
}

// persistMessage calls the callback if set
func (a *Agent) persistMessage(ctx context.Context, msg *domain.Message) {
	if a.onMessage != nil {
		a.onMessage(ctx, msg)
	}
}

// Run processes a message and streams the response
func (a *Agent) Run(ctx context.Context, session *domain.Session, messages []*domain.Message, input string) (<-chan domain.StreamEvent, error) {
	// Run session start hook (only if this is the first message)
	if len(messages) == 0 && a.hooks != nil {
		hctx := &hook.Context{
			Type:      hook.HookSessionStart,
			SessionID: session.ID,
		}
		result := a.hooks.Run(ctx, hctx)
		if !result.Continue {
			return nil, result.Error
		}
	}

	// Create user message
	userMsg := &domain.Message{
		ID:        ulid.Make().String(),
		SessionID: session.ID,
		Role:      domain.RoleUser,
		Parts:     []domain.Part{domain.TextPart{Text: input}},
		Timestamp: time.Now(),
	}

	// Run pre-message hook
	if a.hooks != nil {
		hctx := &hook.Context{
			Type:      hook.HookPreMessage,
			SessionID: session.ID,
			Message:   userMsg,
		}
		result := a.hooks.Run(ctx, hctx)
		if !result.Continue {
			return nil, result.Error
		}
	}

	// Persist user message
	a.persistMessage(ctx, userMsg)

	// Build conversation
	allMessages := make([]domain.Message, 0, len(messages)+1)
	for _, m := range messages {
		allMessages = append(allMessages, *m)
	}
	allMessages = append(allMessages, *userMsg)

	// Get enabled tools
	var enabledTools []domain.Tool
	for _, t := range a.tools.All() {
		if enabled, ok := a.config.Tools[t.Name]; ok && enabled {
			enabledTools = append(enabledTools, t)
		}
	}

	// Build request
	req := &llm.ChatRequest{
		Model:          a.config.Model.ModelID,
		Messages:       allMessages,
		Tools:          enabledTools,
		SystemPrompt:   a.buildSystemPrompt(session),
		MaxTokens:      16384,
		ThinkingBudget: a.thinkingBudget,
	}

	if a.provider == nil {
		return nil, fmt.Errorf("provider is nil")
	}

	// Start streaming
	providerEvents, err := a.provider.Chat(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("chat: %w", err)
	}

	// Process events and handle tool calls
	events := make(chan domain.StreamEvent, 100)
	go a.processEvents(ctx, session, allMessages, enabledTools, providerEvents, events)

	return events, nil
}

func (a *Agent) processEvents(
	ctx context.Context,
	session *domain.Session,
	messages []domain.Message,
	tools []domain.Tool,
	providerEvents <-chan domain.StreamEvent,
	events chan<- domain.StreamEvent,
) {
	a.processEventsLoop(ctx, session, messages, tools, providerEvents, events)
	close(events)
}

func (a *Agent) processEventsLoop(
	ctx context.Context,
	session *domain.Session,
	messages []domain.Message,
	tools []domain.Tool,
	providerEvents <-chan domain.StreamEvent,
	events chan<- domain.StreamEvent,
) {
	var pendingToolCalls []domain.ToolCallPart
	var textBuffer string

	for event := range providerEvents {
		switch event.Type {
		case domain.StreamEventThinking:
			// Pass thinking events through to UI
			events <- event

		case domain.StreamEventText:
			textBuffer += event.Content
			events <- event

		case domain.StreamEventToolCall:
			if tc, ok := event.Part.(domain.ToolCallPart); ok {
				pendingToolCalls = append(pendingToolCalls, tc)
				events <- event
			}

		case domain.StreamEventDone:
			// Execute pending tool calls
			if len(pendingToolCalls) > 0 {
				// Build assistant message with tool calls
				assistantParts := []domain.Part{}
				if textBuffer != "" {
					assistantParts = append(assistantParts, domain.TextPart{Text: textBuffer})
				}
				for _, tc := range pendingToolCalls {
					assistantParts = append(assistantParts, tc)
				}

				assistantMsg := domain.Message{
					ID:        ulid.Make().String(),
					SessionID: session.ID,
					Role:      domain.RoleAssistant,
					Parts:     assistantParts,
					Timestamp: time.Now(),
				}

				// Persist assistant message
				a.persistMessage(ctx, &assistantMsg)

				// Execute tools in parallel and collect results
				toolResults := a.executeToolsParallel(ctx, pendingToolCalls, assistantMsg.Timestamp, events)

				// Build tool result message (as user role for next turn)
				toolMsg := domain.Message{
					ID:        ulid.Make().String(),
					SessionID: session.ID,
					Role:      domain.RoleUser,
					Parts:     toolResults,
					Timestamp: time.Now(),
				}

				// Check for failures and trigger autocorrection
				if failed, reason := a.detectFailure(toolResults); failed && a.shouldRetry() {
					a.retryCount++

					// Emit autocorrection event for visibility
					events <- domain.StreamEvent{
						Type:    domain.StreamEventText,
						Content: fmt.Sprintf("\n\n🔄 [AUTOCORRECTION %d/%d] %s\n\n", a.retryCount, a.autocorrection.MaxRetries, reason),
					}

					// Add correction instruction to tool message
					correctionPart := domain.TextPart{
						Text: fmt.Sprintf(`
⚠️ FAILURE DETECTED - AUTOCORRECTION ATTEMPT %d/%d

The previous command failed. Please:
1. Analyze the error output above
2. Identify the root cause
3. Fix the issue
4. Run the tests again to verify

Do not give up - fix the error and retry.
`, a.retryCount, a.autocorrection.MaxRetries),
					}
					toolMsg.Parts = append(toolMsg.Parts, correctionPart)
				}

				// Persist tool results
				a.persistMessage(ctx, &toolMsg)

				// Continue conversation with tool results
				newMessages := append(messages, assistantMsg, toolMsg)

				req := &llm.ChatRequest{
					Model:          a.config.Model.ModelID,
					Messages:       newMessages,
					Tools:          tools,
					SystemPrompt:   a.buildSystemPrompt(session),
					MaxTokens:      16384,
					ThinkingBudget: a.thinkingBudget,
				}

				// Continue loop for tool results
				nextEvents, err := a.provider.Chat(ctx, req)
				if err != nil {
					events <- domain.StreamEvent{
						Type:  domain.StreamEventError,
						Error: err,
					}
					return
				}

				// Process next round (recursive but doesn't close channel)
				a.processEventsLoop(ctx, session, newMessages, tools, nextEvents, events)
				return
			}

			// No tool calls - persist final assistant message if there's text
			if textBuffer != "" {
				finalMsg := domain.Message{
					ID:        ulid.Make().String(),
					SessionID: session.ID,
					Role:      domain.RoleAssistant,
					Parts:     []domain.Part{domain.TextPart{Text: textBuffer}},
					Timestamp: time.Now(),
				}
				a.persistMessage(ctx, &finalMsg)
			}

			events <- event
		}
	}
}

// executeToolsParallel executes multiple tool calls with conflict awareness
// Tools that modify the same resource are serialized, others run in parallel
// Results are returned in the same order as the input tool calls
// Also collects any images from tool results for vision support
func (a *Agent) executeToolsParallel(
	ctx context.Context,
	toolCalls []domain.ToolCallPart,
	startTime time.Time,
	events chan<- domain.StreamEvent,
) []domain.Part {
	if len(toolCalls) == 0 {
		return nil
	}

	// Single tool - no need for parallelization overhead
	if len(toolCalls) == 1 {
		result := a.executor.Execute(ctx, toolCalls[0], startTime, events)
		parts := []domain.Part{result.Part}
		// Add images as separate parts for vision
		for _, img := range result.Images {
			parts = append(parts, img)
		}
		return parts
	}

	// Group tools by conflict key (file path or "serial" for tools that must serialize)
	groups := a.groupByConflict(toolCalls)

	results := make([]ExecuteResult, len(toolCalls))
	var wg sync.WaitGroup

	// Execute each conflict group - within a group, tools run sequentially
	// Different groups run in parallel
	for _, group := range groups {
		wg.Add(1)
		go func(g conflictGroup) {
			defer wg.Done()
			for _, item := range g.items {
				result := a.executor.Execute(ctx, item.tc, startTime, events)
				results[item.idx] = result
			}
		}(group)
	}

	wg.Wait()

	// Build parts array: tool results followed by their images
	var parts []domain.Part
	for _, r := range results {
		parts = append(parts, r.Part)
		for _, img := range r.Images {
			parts = append(parts, img)
		}
	}
	return parts
}

type indexedToolCall struct {
	idx int
	tc  domain.ToolCallPart
}

type conflictGroup struct {
	key   string
	items []indexedToolCall
}

// groupByConflict groups tool calls that would conflict if run in parallel
func (a *Agent) groupByConflict(toolCalls []domain.ToolCallPart) []conflictGroup {
	keyMap := make(map[string]*conflictGroup)
	var order []string // preserve first-seen order

	for i, tc := range toolCalls {
		key := a.getConflictKey(tc)

		if g, exists := keyMap[key]; exists {
			g.items = append(g.items, indexedToolCall{idx: i, tc: tc})
		} else {
			keyMap[key] = &conflictGroup{
				key:   key,
				items: []indexedToolCall{{idx: i, tc: tc}},
			}
			order = append(order, key)
		}
	}

	// Convert to slice in original order
	groups := make([]conflictGroup, 0, len(keyMap))
	for _, key := range order {
		groups = append(groups, *keyMap[key])
	}
	return groups
}

// getConflictKey returns a key for grouping conflicting tools
// Tools with the same key will be serialized
func (a *Agent) getConflictKey(tc domain.ToolCallPart) string {
	// File-modifying tools: group by file path
	switch tc.Name {
	case "edit", "write":
		if path := getPath(tc.Args); path != "" {
			return "file:" + path
		}
	case "bash":
		// Bash commands could have side effects - serialize all bash
		return "bash"
	}

	// Read-only tools can run fully parallel - unique key per call
	return fmt.Sprintf("parallel:%s:%s", tc.Name, tc.ToolID)
}

// detectFailure checks if any tool result contains failure patterns
func (a *Agent) detectFailure(parts []domain.Part) (bool, string) {
	if !a.autocorrection.Enabled {
		return false, ""
	}

	for _, part := range parts {
		tc, ok := part.(domain.ToolCallPart)
		if !ok {
			continue
		}

		// Check both result and error
		output := tc.Result
		if tc.Error != "" {
			output += "\n" + tc.Error
		}

		for _, pattern := range a.autocorrection.Patterns {
			if strings.Contains(output, pattern) {
				// Extract a snippet around the failure
				idx := strings.Index(output, pattern)
				start := idx - 100
				if start < 0 {
					start = 0
				}
				end := idx + 200
				if end > len(output) {
					end = len(output)
				}
				snippet := output[start:end]
				return true, fmt.Sprintf("Detected '%s' in output: ...%s...", pattern, snippet)
			}
		}
	}
	return false, ""
}

// shouldRetry checks if we should trigger autocorrection
func (a *Agent) shouldRetry() bool {
	if !a.autocorrection.Enabled {
		return false
	}
	return a.retryCount < a.autocorrection.MaxRetries
}

func (a *Agent) buildSystemPrompt(session *domain.Session) string {
	basePrompt := `You are an AI coding assistant. You help users with software engineering tasks.

Working directory: %s

Guidelines:
- Be concise and direct
- Use tools to accomplish tasks
- Read files before editing them
- Prefer editing existing files over creating new ones
- Don't create documentation unless explicitly asked
`
	prompt := fmt.Sprintf(basePrompt, session.Directory)

	if a.config.Prompt != "" {
		prompt += "\n" + a.config.Prompt
	}

	return prompt
}

// BuiltinAgents returns the default agent configurations
func BuiltinAgents() map[string]domain.Agent {
	return map[string]domain.Agent{
		"build": {
			Name:        "build",
			Description: "Full-access agent for coding tasks",
			Mode:        domain.AgentModePrimary,
			BuiltIn:     true,
			Tools: map[string]bool{
				"bash":       true,
				"read":       true,
				"write":      true,
				"edit":       true,
				"glob":       true,
				"grep":       true,
				"ls":         true,
				"screenshot": true,
			},
			Permissions: domain.AgentPermissions{
				Edit: domain.PermissionAllow,
				Bash: map[string]domain.Permission{
					"*": domain.PermissionAllow,
				},
			},
		},
		"plan": {
			Name:        "plan",
			Description: "Read-only agent for exploration and planning",
			Mode:        domain.AgentModePrimary,
			BuiltIn:     true,
			Tools: map[string]bool{
				"bash": true,
				"read": true,
				"glob": true,
				"grep": true,
				"ls":   true,
			},
			Permissions: domain.AgentPermissions{
				Edit: domain.PermissionDeny,
				Bash: map[string]domain.Permission{
					"git *":  domain.PermissionAllow,
					"ls *":   domain.PermissionAllow,
					"cat *":  domain.PermissionAllow,
					"head *": domain.PermissionAllow,
					"*":      domain.PermissionAsk,
				},
			},
		},
		"explore": {
			Name:        "explore",
			Description: "Fast subagent for codebase exploration",
			Mode:        domain.AgentModeSubagent,
			BuiltIn:     true,
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
	}
}

