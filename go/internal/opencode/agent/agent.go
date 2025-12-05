package agent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/joss/urp/internal/opencode/cognitive"
	"github.com/joss/urp/internal/opencode/domain"
	"github.com/joss/urp/internal/opencode/hook"
	"github.com/joss/urp/internal/opencode/permission"
	"github.com/joss/urp/internal/opencode/tool"
	"github.com/joss/urp/pkg/llm"
)

// Agent orchestrates conversations with an LLM
// Delegates to: MessageStore, Autocorrector, PromptBuilder
type Agent struct {
	config         domain.Agent
	provider       llm.Provider
	tools          tool.ToolRegistry
	executor       *ToolExecutor
	hooks          *hook.Registry
	workDir        string
	thinkingBudget int // Extended thinking token budget (0 = disabled)

	// Delegated components (SRP)
	messages      *MessageStore
	autocorrector *Autocorrector
	promptBuilder *PromptBuilder
	cognitive     *cognitive.Engine

	// Structured logging
	logger *AgentLogger
}

// New creates an Agent with its dependencies (uses interfaces)
func New(config domain.Agent, provider llm.Provider, tools tool.ToolRegistry) *Agent {
	hooks := hook.NewRegistry()
	a := &Agent{
		config:        config,
		provider:      provider,
		tools:         tools,
		hooks:         hooks,
		messages:      NewMessageStore(),
		autocorrector: NewAutocorrector(),
		promptBuilder: NewPromptBuilder(),
		cognitive:     cognitive.NewEngine(cognitive.DefaultEngineConfig()),
		logger:        defaultLogger,
	}
	// Set custom prompt from config
	if config.Prompt != "" {
		a.promptBuilder.SetCustomPrompt(config.Prompt)
	}
	// Initialize executor with default permissions and shared hooks
	a.executor = NewToolExecutor(tools, nil).WithHooks(hooks)
	return a
}

// Cognitive returns the cognitive engine for external access
func (a *Agent) Cognitive() *cognitive.Engine {
	return a.cognitive
}

// WithCognitive sets a custom cognitive engine
func (a *Agent) WithCognitive(e *cognitive.Engine) *Agent {
	a.cognitive = e
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
	a.executor = NewToolExecutor(a.tools, perms).WithHooks(a.hooks).WithLogger(a.logger)
}

// SetThinkingBudget sets the extended thinking token budget
func (a *Agent) SetThinkingBudget(budget int) {
	a.thinkingBudget = budget
}

// SetLogger sets a custom structured logger
func (a *Agent) SetLogger(logger *AgentLogger) {
	a.logger = logger
	if a.executor != nil {
		a.executor = a.executor.WithLogger(logger)
	}
}

// Logger returns the agent's structured logger
func (a *Agent) Logger() *AgentLogger {
	return a.logger
}

// OnMessage sets the callback for message persistence
func (a *Agent) OnMessage(cb MessageCallback) {
	a.messages.OnMessage(cb)
}

// EnableAutocorrection configures the autocorrection loop
func (a *Agent) EnableAutocorrection(config AutocorrectionConfig) {
	a.autocorrector.Configure(config)
}

// Messages returns a copy of the current conversation history
func (a *Agent) Messages() []domain.Message {
	return a.messages.Messages()
}

// SetMessages replaces the conversation history (used for compaction)
func (a *Agent) SetMessages(msgs []domain.Message) {
	a.messages.SetMessages(msgs)
}

// Model returns the current model ID
func (a *Agent) Model() string {
	return a.config.Model.ModelID
}

// SetModel changes the model for subsequent requests
func (a *Agent) SetModel(modelID string) {
	a.config.Model.ModelID = modelID
}

// persistMessage delegates to MessageStore
func (a *Agent) persistMessage(ctx context.Context, msg *domain.Message) {
	a.messages.Persist(ctx, msg)
}

// Run processes a message and streams the response
func (a *Agent) Run(ctx context.Context, session *domain.Session, messages []*domain.Message, input string) (<-chan domain.StreamEvent, error) {
	// Log session start
	if a.logger != nil && len(messages) == 0 {
		a.logger.SessionStart(ctx, session.ID, a.config.Model.ModelID)
	}

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
		SystemPrompt:   a.promptBuilder.Build(session),
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
	var thinkingTokens int
	startTime := time.Now()

	for event := range providerEvents {
		switch event.Type {
		case domain.StreamEventThinking:
			// Pass thinking events through to UI
			if event.Usage != nil {
				thinkingTokens += event.Usage.OutputTokens
			}
			events <- event

		case domain.StreamEventUsage:
			// Log LLM call with usage stats
			if a.logger != nil && event.Usage != nil {
				durationMs := time.Since(startTime).Milliseconds()
				a.logger.LLMCall(ctx, a.config.Model.ModelID, durationMs,
					event.Usage.InputTokens, event.Usage.OutputTokens, thinkingTokens,
					event.Usage.CacheRead, event.Usage.CacheWrite, event.Usage.TotalCost, nil)
			}
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
				if failed, reason := a.autocorrector.DetectFailure(toolResults); failed && a.autocorrector.ShouldRetry() {
					a.autocorrector.IncrementRetry()

					// Notify cognitive engine about error and retry
					if a.cognitive != nil {
						a.cognitive.SetError(reason)
						a.cognitive.AddRetry(a.autocorrector.RetryCount(), a.autocorrector.MaxRetries())
					}

					// Emit autocorrection event for visibility
					events <- domain.StreamEvent{
						Type:    domain.StreamEventText,
						Content: fmt.Sprintf("\n\n🔄 [AUTOCORRECTION %d/%d] %s\n\n", a.autocorrector.RetryCount(), a.autocorrector.MaxRetries(), reason),
					}

					// Add correction instruction to tool message
					correctionPart := domain.TextPart{
						Text: a.autocorrector.CorrectionPrompt(),
					}
					toolMsg.Parts = append(toolMsg.Parts, correctionPart)
				} else if a.cognitive != nil && a.autocorrector.RetryCount() > 0 {
					// Task succeeded after retries - clear error state
					a.cognitive.ClearError()
				}

				// Persist tool results
				a.persistMessage(ctx, &toolMsg)

				// Continue conversation with tool results
				newMessages := append(messages, assistantMsg, toolMsg)

				req := &llm.ChatRequest{
					Model:          a.config.Model.ModelID,
					Messages:       newMessages,
					Tools:          tools,
					SystemPrompt:   a.promptBuilder.Build(session),
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


// BuiltinAgents returns the default agent configurations
func BuiltinAgents() map[string]domain.Agent {
	return map[string]domain.Agent{
		"build": {
			Name:        "build",
			Description: "Full-access agent for coding tasks",
			Mode:        domain.AgentModePrimary,
			BuiltIn:     true,
			Tools: map[string]bool{
				"bash":        true,
				"read":        true,
				"write":       true,
				"edit":        true,
				"glob":        true,
				"grep":        true,
				"ls":          true,
				"screenshot":  true,
				"diagnostics": true,
			},
			Permissions: domain.AgentPermissions{
				Edit:        domain.PermissionAllow,
				ExternalDir: domain.PermissionAllow,
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

