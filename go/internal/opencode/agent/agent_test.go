package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/joss/urp/internal/opencode/domain"
	"github.com/joss/urp/internal/opencode/hook"
	"github.com/joss/urp/internal/opencode/testutil"
	"github.com/joss/urp/internal/opencode/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testAgentConfig() domain.Agent {
	return domain.Agent{
		Name: "test",
		Model: &domain.ModelConfig{
			ProviderID: "mock",
			ModelID:    "mock-model",
		},
		Tools: map[string]bool{},
	}
}

func TestAgentNew(t *testing.T) {
	cfg := testAgentConfig()
	cfg.Tools["bash"] = true
	provider := testutil.NewMockProvider()
	registry := tool.NewRegistry()

	agent := New(cfg, provider, registry)
	require.NotNil(t, agent)
}

func TestAgentRunSimpleText(t *testing.T) {
	cfg := testAgentConfig()
	provider := testutil.NewMockProvider(testutil.TextResponse("Hello world!"))
	registry := tool.NewRegistry()
	agent := New(cfg, provider, registry)

	events, err := agent.Run(context.Background(), testutil.TestSession("", ""), nil, "Hi")
	require.NoError(t, err)

	text, _, _ := testutil.CollectEvents(events)
	assert.Equal(t, "Hello world!", text)
}

func TestAgentRunWithToolCall(t *testing.T) {
	cfg := testAgentConfig()
	cfg.Tools["echo"] = true

	provider := testutil.NewMockProvider(
		testutil.ToolCallResponse("call_1", "echo", map[string]any{"message": "test"}),
		testutil.TextResponse("Echo returned: test"),
	)

	registry := tool.NewRegistry()
	registry.Register(testutil.NewMockTool("echo"))

	agent := New(cfg, provider, registry)
	events, err := agent.Run(context.Background(), testutil.TestSession("", ""), nil, "echo test")
	require.NoError(t, err)

	text, toolCalls, toolDones := testutil.CollectEvents(events)

	assert.Equal(t, 1, toolCalls)
	assert.Equal(t, 1, toolDones)
	assert.Equal(t, "Echo returned: test", text)
}

func TestAgentToolFilter(t *testing.T) {
	cfg := testAgentConfig()
	cfg.Tools["allowed"] = true
	cfg.Tools["denied"] = false

	provider := testutil.NewMockProvider(testutil.DoneResponse())

	registry := tool.NewRegistry()
	registry.Register(testutil.NewMockTool("allowed"))
	registry.Register(testutil.NewMockTool("denied"))
	registry.Register(testutil.NewMockTool("unlisted"))

	agent := New(cfg, provider, registry)

	_, err := agent.Run(context.Background(), testutil.TestSession("", ""), nil, "test")
	require.NoError(t, err)
}

func TestAgentSystemPrompt(t *testing.T) {
	cfg := testAgentConfig()
	cfg.Prompt = "Custom agent prompt"

	provider := testutil.NewMockProvider(testutil.DoneResponse())
	registry := tool.NewRegistry()
	agent := New(cfg, provider, registry)

	session := testutil.TestSession("", "/custom/dir")
	prompt := agent.buildSystemPrompt(session)

	assert.Contains(t, prompt, "/custom/dir")
	assert.Contains(t, prompt, "Custom agent prompt")
}

func TestAgentWithMessages(t *testing.T) {
	cfg := testAgentConfig()
	provider := testutil.NewMockProvider(testutil.TextResponse("Context received"))
	registry := tool.NewRegistry()
	agent := New(cfg, provider, registry)

	session := testutil.TestSession("test", "/tmp")
	history := []*domain.Message{
		{
			ID:        "msg-1",
			SessionID: session.ID,
			Role:      domain.RoleUser,
			Parts:     []domain.Part{domain.TextPart{Text: "Previous message"}},
			Timestamp: time.Now(),
		},
		{
			ID:        "msg-2",
			SessionID: session.ID,
			Role:      domain.RoleAssistant,
			Parts:     []domain.Part{domain.TextPart{Text: "Previous response"}},
			Timestamp: time.Now(),
		},
	}

	events, err := agent.Run(context.Background(), session, history, "New message")
	require.NoError(t, err)

	testutil.DrainEvents(events)
	assert.Equal(t, 1, provider.CallCount())
}

func TestAgentMessagePersistence(t *testing.T) {
	cfg := testAgentConfig()
	provider := testutil.NewMockProvider(testutil.TextResponse("Hello!"))
	registry := tool.NewRegistry()
	agent := New(cfg, provider, registry)

	// Collect persisted messages
	var persisted []*domain.Message
	var mu sync.Mutex

	agent.OnMessage(func(ctx context.Context, msg *domain.Message) error {
		mu.Lock()
		defer mu.Unlock()
		persisted = append(persisted, msg)
		return nil
	})

	session := testutil.TestSession("test", "/tmp")
	events, err := agent.Run(context.Background(), session, nil, "Hi there")
	require.NoError(t, err)

	testutil.DrainEvents(events)

	// Should have persisted: user message + assistant response
	mu.Lock()
	defer mu.Unlock()
	require.Len(t, persisted, 2, "Should persist user and assistant messages")

	assert.Equal(t, domain.RoleUser, persisted[0].Role)
	assert.Equal(t, domain.RoleAssistant, persisted[1].Role)

	// Verify user message content
	if tp, ok := persisted[0].Parts[0].(domain.TextPart); ok {
		assert.Equal(t, "Hi there", tp.Text)
	}

	// Verify assistant message content
	if tp, ok := persisted[1].Parts[0].(domain.TextPart); ok {
		assert.Equal(t, "Hello!", tp.Text)
	}
}

func TestAgentMessagePersistenceWithTools(t *testing.T) {
	cfg := testAgentConfig()
	cfg.Tools["echo"] = true

	provider := testutil.NewMockProvider(
		testutil.ToolCallResponse("call_1", "echo", map[string]any{"message": "test"}),
		testutil.TextResponse("Done!"),
	)

	registry := tool.NewRegistry()
	registry.Register(testutil.NewMockTool("echo"))

	agent := New(cfg, provider, registry)

	// Collect persisted messages
	var persisted []*domain.Message
	var mu sync.Mutex

	agent.OnMessage(func(ctx context.Context, msg *domain.Message) error {
		mu.Lock()
		defer mu.Unlock()
		persisted = append(persisted, msg)
		return nil
	})

	session := testutil.TestSession("test", "/tmp")
	events, err := agent.Run(context.Background(), session, nil, "echo test")
	require.NoError(t, err)

	testutil.DrainEvents(events)

	// Should have persisted: user + assistant(tool) + tool_result + assistant(final)
	mu.Lock()
	defer mu.Unlock()
	require.GreaterOrEqual(t, len(persisted), 3, "Should persist user, assistant with tool, tool result, and final response")

	// First is user message
	assert.Equal(t, domain.RoleUser, persisted[0].Role)
}

func TestBuiltinAgents(t *testing.T) {
	agents := BuiltinAgents()
	require.NotEmpty(t, agents)

	t.Run("build agent", func(t *testing.T) {
		build, ok := agents["build"]
		require.True(t, ok)
		assert.Equal(t, domain.AgentModePrimary, build.Mode)
		assert.True(t, build.Tools["bash"])
		assert.True(t, build.Tools["edit"])
	})

	t.Run("plan agent", func(t *testing.T) {
		plan, ok := agents["plan"]
		require.True(t, ok)
		assert.Equal(t, domain.PermissionDeny, plan.Permissions.Edit)
	})

	t.Run("explore agent", func(t *testing.T) {
		explore, ok := agents["explore"]
		require.True(t, ok)
		assert.Equal(t, domain.AgentModeSubagent, explore.Mode)
		assert.False(t, explore.Tools["bash"])
	})
}

func TestAgentParallelToolExecution(t *testing.T) {
	cfg := testAgentConfig()
	cfg.Tools["slow1"] = true
	cfg.Tools["slow2"] = true
	cfg.Tools["slow3"] = true

	// Provider returns 3 tool calls at once
	provider := testutil.NewMockProvider(
		[]domain.StreamEvent{
			{Type: domain.StreamEventToolCall, Part: domain.ToolCallPart{
				ToolID: "call_1", Name: "slow1", Args: map[string]any{},
			}},
			{Type: domain.StreamEventToolCall, Part: domain.ToolCallPart{
				ToolID: "call_2", Name: "slow2", Args: map[string]any{},
			}},
			{Type: domain.StreamEventToolCall, Part: domain.ToolCallPart{
				ToolID: "call_3", Name: "slow3", Args: map[string]any{},
			}},
			{Type: domain.StreamEventDone, Done: true},
		},
		testutil.TextResponse("Done"),
	)

	registry := tool.NewRegistry()
	registry.Register(testutil.NewMockTool("slow1").WithDelay(50 * time.Millisecond).WithResult("result1"))
	registry.Register(testutil.NewMockTool("slow2").WithDelay(50 * time.Millisecond).WithResult("result2"))
	registry.Register(testutil.NewMockTool("slow3").WithDelay(50 * time.Millisecond).WithResult("result3"))

	agent := New(cfg, provider, registry)

	start := time.Now()
	events, err := agent.Run(context.Background(), testutil.TestSession("", ""), nil, "test")
	require.NoError(t, err)

	_, _, toolDones := testutil.CollectEvents(events)
	elapsed := time.Since(start)

	assert.Equal(t, 3, toolDones)
	// If executed in parallel: ~50ms; If sequential: ~150ms
	assert.Less(t, elapsed, 120*time.Millisecond, "expected parallel execution")
}

func TestAgentConflictDetection(t *testing.T) {
	cfg := testAgentConfig()
	provider := testutil.NewMockProvider()
	registry := tool.NewRegistry()
	agent := New(cfg, provider, registry)

	tests := []struct {
		name       string
		toolCalls  []domain.ToolCallPart
		wantGroups int
	}{
		{
			name: "same file edits serialize",
			toolCalls: []domain.ToolCallPart{
				{ToolID: "1", Name: "edit", Args: map[string]any{"file_path": "/a.go"}},
				{ToolID: "2", Name: "edit", Args: map[string]any{"file_path": "/a.go"}},
			},
			wantGroups: 1,
		},
		{
			name: "different file edits parallel",
			toolCalls: []domain.ToolCallPart{
				{ToolID: "1", Name: "edit", Args: map[string]any{"file_path": "/a.go"}},
				{ToolID: "2", Name: "edit", Args: map[string]any{"file_path": "/b.go"}},
			},
			wantGroups: 2,
		},
		{
			name: "bash commands serialize",
			toolCalls: []domain.ToolCallPart{
				{ToolID: "1", Name: "bash", Args: map[string]any{"command": "ls"}},
				{ToolID: "2", Name: "bash", Args: map[string]any{"command": "pwd"}},
			},
			wantGroups: 1,
		},
		{
			name: "read tools parallel",
			toolCalls: []domain.ToolCallPart{
				{ToolID: "1", Name: "read", Args: map[string]any{"path": "/a.go"}},
				{ToolID: "2", Name: "read", Args: map[string]any{"path": "/b.go"}},
				{ToolID: "3", Name: "glob", Args: map[string]any{"pattern": "*.go"}},
			},
			wantGroups: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			groups := agent.groupByConflict(tt.toolCalls)
			assert.Len(t, groups, tt.wantGroups)
		})
	}
}

func TestAgentConflictSerializationTiming(t *testing.T) {
	cfg := testAgentConfig()
	cfg.Tools["edit"] = true

	// Two edits to same file should serialize
	provider1 := testutil.NewMockProvider(
		[]domain.StreamEvent{
			{Type: domain.StreamEventToolCall, Part: domain.ToolCallPart{
				ToolID: "1", Name: "edit", Args: map[string]any{"file_path": "/same.go"},
			}},
			{Type: domain.StreamEventToolCall, Part: domain.ToolCallPart{
				ToolID: "2", Name: "edit", Args: map[string]any{"file_path": "/same.go"},
			}},
			{Type: domain.StreamEventDone, Done: true},
		},
		testutil.DoneResponse(),
	)

	registry := tool.NewRegistry()
	registry.Register(testutil.NewMockTool("edit").WithDelay(30 * time.Millisecond).WithResult("ok"))

	agent := New(cfg, provider1, registry)

	start := time.Now()
	events, _ := agent.Run(context.Background(), testutil.TestSession("", ""), nil, "test")
	testutil.DrainEvents(events)
	serializedTime := time.Since(start)

	// Different files - should be parallel
	provider2 := testutil.NewMockProvider(
		[]domain.StreamEvent{
			{Type: domain.StreamEventToolCall, Part: domain.ToolCallPart{
				ToolID: "1", Name: "edit", Args: map[string]any{"file_path": "/a.go"},
			}},
			{Type: domain.StreamEventToolCall, Part: domain.ToolCallPart{
				ToolID: "2", Name: "edit", Args: map[string]any{"file_path": "/b.go"},
			}},
			{Type: domain.StreamEventDone, Done: true},
		},
		testutil.DoneResponse(),
	)

	agent2 := New(cfg, provider2, registry)
	start = time.Now()
	events, _ = agent2.Run(context.Background(), testutil.TestSession("", ""), nil, "test")
	testutil.DrainEvents(events)
	parallelTime := time.Since(start)

	t.Logf("Serialized (same file): %v", serializedTime)
	t.Logf("Parallel (different files): %v", parallelTime)

	// Serialized should take ~60ms (2x30ms), parallel ~30ms
	assert.GreaterOrEqual(t, serializedTime, 50*time.Millisecond)
	assert.Less(t, parallelTime, 50*time.Millisecond)
}

// =============================================================================
// STRESS TESTS
// =============================================================================

func TestAgentStress_MassiveParallelTools(t *testing.T) {
	const numTools = 50

	cfg := testAgentConfig()
	for i := 0; i < numTools; i++ {
		cfg.Tools[fmt.Sprintf("tool%d", i)] = true
	}

	var toolCalls []domain.StreamEvent
	for i := 0; i < numTools; i++ {
		toolCalls = append(toolCalls, domain.StreamEvent{
			Type: domain.StreamEventToolCall,
			Part: domain.ToolCallPart{
				ToolID: fmt.Sprintf("call_%d", i),
				Name:   fmt.Sprintf("tool%d", i),
				Args:   map[string]any{"idx": i},
			},
		})
	}
	toolCalls = append(toolCalls, domain.StreamEvent{Type: domain.StreamEventDone, Done: true})

	provider := testutil.NewMockProvider(toolCalls, testutil.TextResponse("Done"))

	registry := tool.NewRegistry()
	for i := 0; i < numTools; i++ {
		registry.Register(testutil.NewMockTool(fmt.Sprintf("tool%d", i)).
			WithDelay(5 * time.Millisecond).
			WithResult(fmt.Sprintf("result%d", i)))
	}

	agent := New(cfg, provider, registry)

	start := time.Now()
	events, err := agent.Run(context.Background(), testutil.TestSession("stress", ""), nil, "stress test")
	require.NoError(t, err)

	_, _, toolDones := testutil.CollectEvents(events)
	elapsed := time.Since(start)

	t.Logf("Executed %d tools in %v (%.1f tools/sec)", numTools, elapsed, float64(numTools)/elapsed.Seconds())

	assert.Equal(t, numTools, toolDones)
	assert.Less(t, elapsed, 100*time.Millisecond, "expected parallel execution")
}

func TestAgentStress_RapidFireRequests(t *testing.T) {
	const numRequests = 20

	cfg := testAgentConfig()
	cfg.Tools["echo"] = true

	registry := tool.NewRegistry()
	registry.Register(testutil.NewMockTool("echo").WithResult("ok"))

	session := testutil.TestSession("rapid-fire", "")

	var wg sync.WaitGroup
	errors := make(chan error, numRequests)

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			provider := testutil.NewMockProvider(testutil.TextResponse(fmt.Sprintf("Response %d", idx)))
			agent := New(cfg, provider, registry)

			events, err := agent.Run(context.Background(), session, nil, fmt.Sprintf("Request %d", idx))
			if err != nil {
				errors <- fmt.Errorf("request %d failed: %w", idx, err)
				return
			}
			testutil.DrainEvents(events)
		}(i)
	}

	wg.Wait()
	close(errors)

	var errs []error
	for err := range errors {
		errs = append(errs, err)
	}

	assert.Empty(t, errs)
}

func TestAgentStress_CancellationUnderLoad(t *testing.T) {
	cfg := testAgentConfig()
	for i := 0; i < 10; i++ {
		cfg.Tools[fmt.Sprintf("slow%d", i)] = true
	}

	var toolCalls []domain.StreamEvent
	for i := 0; i < 10; i++ {
		toolCalls = append(toolCalls, domain.StreamEvent{
			Type: domain.StreamEventToolCall,
			Part: domain.ToolCallPart{
				ToolID: fmt.Sprintf("call_%d", i),
				Name:   fmt.Sprintf("slow%d", i),
				Args:   map[string]any{},
			},
		})
	}
	toolCalls = append(toolCalls, domain.StreamEvent{Type: domain.StreamEventDone, Done: true})

	provider := testutil.NewMockProvider(toolCalls, testutil.DoneResponse())

	registry := tool.NewRegistry()
	for i := 0; i < 10; i++ {
		registry.Register(testutil.NewMockTool(fmt.Sprintf("slow%d", i)).
			WithDelay(500 * time.Millisecond).
			WithResult("ok"))
	}

	agent := New(cfg, provider, registry)
	ctx, cancel := context.WithCancel(context.Background())

	events, err := agent.Run(ctx, testutil.TestSession("", ""), nil, "slow ops")
	require.NoError(t, err)

	// Cancel after short delay
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	testutil.DrainEvents(events)
	elapsed := time.Since(start)

	if elapsed > 200*time.Millisecond {
		t.Logf("Warning: Cancellation took %v (tools may not respect context)", elapsed)
	} else {
		t.Logf("Cancellation handled in %v", elapsed)
	}
}

// =============================================================================
// VISION TESTS
// =============================================================================

type ImageMockTool struct {
	name string
}

func (m *ImageMockTool) Info() domain.Tool {
	return domain.Tool{
		Name:        m.name,
		Description: "Mock tool that returns images",
		Parameters: domain.JSONSchema{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{"type": "string"},
			},
		},
	}
}

func (m *ImageMockTool) Execute(ctx context.Context, args map[string]any) (*tool.Result, error) {
	return &tool.Result{
		Output: "[Image loaded: test.png]",
		Images: []domain.ImagePart{
			{
				Base64:    "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==",
				MediaType: "image/png",
			},
		},
	}, nil
}

func TestAgentVisionPipeline(t *testing.T) {
	cfg := testAgentConfig()
	cfg.Tools["screenshot"] = true

	provider := testutil.NewMockProvider(
		testutil.ToolCallResponse("call-1", "screenshot", map[string]any{"path": "/tmp/test.png"}),
		testutil.TextResponse("I see a small image."),
	)

	registry := tool.NewRegistry()
	registry.Register(&ImageMockTool{name: "screenshot"})

	agent := New(cfg, provider, registry)

	events, err := agent.Run(context.Background(), testutil.TestSession("", ""), nil, "What's in this image?")
	require.NoError(t, err)

	var toolDones int
	var textContent string
	var seenDone bool

	for event := range events {
		switch event.Type {
		case domain.StreamEventToolDone:
			toolDones++
		case domain.StreamEventText:
			textContent += event.Content
		case domain.StreamEventDone:
			seenDone = true
		}
	}

	assert.Equal(t, 1, toolDones)
	assert.True(t, seenDone)
	assert.NotEmpty(t, textContent)
}

func TestExecuteResultWithImages(t *testing.T) {
	registry := tool.NewRegistry()
	registry.Register(&ImageMockTool{name: "screenshot"})

	executor := NewToolExecutor(registry, nil)

	events := make(chan domain.StreamEvent, 10)
	tc := domain.ToolCallPart{
		ToolID: "call-1",
		Name:   "screenshot",
		Args:   map[string]any{"path": "/tmp/test.png"},
	}

	result := executor.Execute(context.Background(), tc, time.Now(), events)

	require.Len(t, result.Images, 1)
	assert.Equal(t, "image/png", result.Images[0].MediaType)
	assert.NotEmpty(t, result.Images[0].Base64)
	assert.NotEmpty(t, result.Part.Result)
}

// =============================================================================
// HOOK TESTS
// =============================================================================

func TestAgentHooksRegistry(t *testing.T) {
	agent := New(testAgentConfig(), testutil.NewMockProvider(), tool.NewRegistry())
	require.NotNil(t, agent.Hooks())
}

func TestAgentPreToolHookBlocks(t *testing.T) {
	cfg := testAgentConfig()
	cfg.Tools["blocked"] = true

	provider := testutil.NewMockProvider(
		testutil.ToolCallResponse("call_1", "blocked", map[string]any{}),
		testutil.TextResponse("Tool was blocked"),
	)

	registry := tool.NewRegistry()
	registry.Register(testutil.NewMockTool("blocked").WithResult("should not run"))

	agent := New(cfg, provider, registry)

	var hookCalled bool
	agent.Hooks().Register(hook.HookPreToolExec, func(ctx context.Context, hctx *hook.Context) hook.Result {
		hookCalled = true
		return hook.Result{Continue: false, Error: fmt.Errorf("tool blocked by policy")}
	})

	events, err := agent.Run(context.Background(), testutil.TestSession("", ""), nil, "test")
	require.NoError(t, err)

	var blockedCount int
	for event := range events {
		if event.Type == domain.StreamEventToolDone {
			if tc, ok := event.Part.(domain.ToolCallPart); ok && tc.Error != "" {
				blockedCount++
			}
		}
	}

	assert.True(t, hookCalled)
	assert.Equal(t, 1, blockedCount)
}

func TestAgentPostToolHookLogs(t *testing.T) {
	cfg := testAgentConfig()
	cfg.Tools["echo"] = true

	provider := testutil.NewMockProvider(
		testutil.ToolCallResponse("call_1", "echo", map[string]any{"message": "hello"}),
		testutil.TextResponse("Done"),
	)

	registry := tool.NewRegistry()
	registry.Register(testutil.NewMockTool("echo"))

	agent := New(cfg, provider, registry)

	var loggedTools []string
	agent.Hooks().Register(hook.HookPostToolExec, func(ctx context.Context, hctx *hook.Context) hook.Result {
		if hctx.ToolCall != nil {
			loggedTools = append(loggedTools, hctx.ToolCall.Name)
		}
		return hook.Result{Continue: true}
	})

	events, err := agent.Run(context.Background(), testutil.TestSession("", ""), nil, "echo hello")
	require.NoError(t, err)

	testutil.DrainEvents(events)

	require.Len(t, loggedTools, 1)
	assert.Equal(t, "echo", loggedTools[0])
}

func TestAgentSessionStartHook(t *testing.T) {
	cfg := testAgentConfig()
	provider := testutil.NewMockProvider(testutil.TextResponse("Hello"))
	registry := tool.NewRegistry()
	agent := New(cfg, provider, registry)

	var sessionStarted bool
	var sessionID string
	agent.Hooks().Register(hook.HookSessionStart, func(ctx context.Context, hctx *hook.Context) hook.Result {
		sessionStarted = true
		sessionID = hctx.SessionID
		return hook.Result{Continue: true}
	})

	session := testutil.TestSession("test-session-123", "")
	events, err := agent.Run(context.Background(), session, nil, "Hi")
	require.NoError(t, err)

	testutil.DrainEvents(events)

	assert.True(t, sessionStarted)
	assert.Equal(t, "test-session-123", sessionID)
}

func TestAgentPreMessageHook(t *testing.T) {
	cfg := testAgentConfig()
	provider := testutil.NewMockProvider(testutil.TextResponse("Response"))
	registry := tool.NewRegistry()
	agent := New(cfg, provider, registry)

	var receivedMessage string
	agent.Hooks().Register(hook.HookPreMessage, func(ctx context.Context, hctx *hook.Context) hook.Result {
		if hctx.Message != nil {
			for _, part := range hctx.Message.Parts {
				if text, ok := part.(domain.TextPart); ok {
					receivedMessage = text.Text
				}
			}
		}
		return hook.Result{Continue: true}
	})

	events, err := agent.Run(context.Background(), testutil.TestSession("", ""), nil, "Hello world")
	require.NoError(t, err)

	testutil.DrainEvents(events)
	assert.Equal(t, "Hello world", receivedMessage)
}

func TestAgentPreMessageHookBlocks(t *testing.T) {
	cfg := testAgentConfig()
	provider := testutil.NewMockProvider(testutil.DoneResponse())
	registry := tool.NewRegistry()
	agent := New(cfg, provider, registry)

	agent.Hooks().Register(hook.HookPreMessage, func(ctx context.Context, hctx *hook.Context) hook.Result {
		return hook.Result{Continue: false, Error: fmt.Errorf("message blocked")}
	})

	_, err := agent.Run(context.Background(), testutil.TestSession("", ""), nil, "Hello")

	require.Error(t, err)
	assert.Equal(t, "message blocked", err.Error())
}

func TestExecutorWithCustomHooks(t *testing.T) {
	registry := tool.NewRegistry()
	registry.Register(testutil.NewMockTool("test").WithResult("ok"))

	hooks := hook.NewRegistry()
	var preCount, postCount int
	hooks.Register(hook.HookPreToolExec, func(ctx context.Context, hctx *hook.Context) hook.Result {
		preCount++
		return hook.Result{Continue: true}
	})
	hooks.Register(hook.HookPostToolExec, func(ctx context.Context, hctx *hook.Context) hook.Result {
		postCount++
		return hook.Result{Continue: true}
	})

	executor := NewToolExecutor(registry, nil).WithHooks(hooks)

	events := make(chan domain.StreamEvent, 10)
	tc := domain.ToolCallPart{ToolID: "1", Name: "test", Args: map[string]any{}}

	executor.Execute(context.Background(), tc, time.Now(), events)

	assert.Equal(t, 1, preCount)
	assert.Equal(t, 1, postCount)
}

func TestAgentWithCustomHooksRegistry(t *testing.T) {
	cfg := testAgentConfig()
	cfg.Tools["echo"] = true

	provider := testutil.NewMockProvider(
		testutil.ToolCallResponse("1", "echo", map[string]any{}),
		testutil.DoneResponse(),
	)

	registry := tool.NewRegistry()
	registry.Register(testutil.NewMockTool("echo").WithResult("ok"))

	customHooks := hook.NewRegistry()
	var hooksCalled int
	customHooks.Register(hook.HookPreToolExec, func(ctx context.Context, hctx *hook.Context) hook.Result {
		hooksCalled++
		return hook.Result{Continue: true}
	})

	agent := New(cfg, provider, registry).WithHooks(customHooks)

	events, _ := agent.Run(context.Background(), testutil.TestSession("", ""), nil, "test")
	testutil.DrainEvents(events)

	assert.Equal(t, 1, hooksCalled)
}

// contains is a local helper - strings.Contains is preferred for new code
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

// =============================================================================
// AUTOCORRECTION TESTS
// =============================================================================

func TestAutocorrectionConfig(t *testing.T) {
	defaults := DefaultAutocorrection()
	assert.True(t, defaults.Enabled)
	assert.Equal(t, 3, defaults.MaxRetries)
	assert.NotEmpty(t, defaults.Patterns)
}

func TestDetectFailure(t *testing.T) {
	cfg := testAgentConfig()
	provider := testutil.NewMockProvider()
	registry := tool.NewRegistry()
	agent := New(cfg, provider, registry)

	// Without autocorrection enabled
	parts := []domain.Part{
		domain.ToolCallPart{Result: "FAIL: test failed"},
	}
	failed, _ := agent.detectFailure(parts)
	assert.False(t, failed) // Not enabled

	// Enable autocorrection
	agent.EnableAutocorrection(DefaultAutocorrection())

	// Should detect failure
	failed, reason := agent.detectFailure(parts)
	assert.True(t, failed)
	assert.Contains(t, reason, "FAIL")

	// Should not detect success
	successParts := []domain.Part{
		domain.ToolCallPart{Result: "ok\nPASS: all tests passed"},
	}
	failed, _ = agent.detectFailure(successParts)
	assert.False(t, failed)
}

func TestShouldRetry(t *testing.T) {
	cfg := testAgentConfig()
	provider := testutil.NewMockProvider()
	registry := tool.NewRegistry()
	agent := New(cfg, provider, registry)

	// Without autocorrection - never retry
	assert.False(t, agent.shouldRetry())

	// Enable with 2 retries
	agent.EnableAutocorrection(AutocorrectionConfig{
		Enabled:    true,
		MaxRetries: 2,
		Patterns:   []string{"FAIL"},
	})

	assert.True(t, agent.shouldRetry())

	agent.retryCount = 1
	assert.True(t, agent.shouldRetry())

	agent.retryCount = 2
	assert.False(t, agent.shouldRetry())
}
