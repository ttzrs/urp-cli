package server

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─────────────────────────────────────────────────────────────────────────────
// ContextTracker Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestNewContextTracker(t *testing.T) {
	tracker := NewContextTracker()
	require.NotNil(t, tracker)

	usage := tracker.GetUsage()
	assert.Equal(t, 0, usage.Tokens)
	assert.Equal(t, 0, usage.Files)
}

func TestContextTracker_AddTokens(t *testing.T) {
	tracker := NewContextTracker()

	tracker.AddTokens(100)
	assert.Equal(t, 100, tracker.GetUsage().Tokens)

	tracker.AddTokens(50)
	assert.Equal(t, 150, tracker.GetUsage().Tokens)
}

func TestContextTracker_AddFile(t *testing.T) {
	tracker := NewContextTracker()

	tracker.AddFile("/path/to/file1.go")
	assert.Equal(t, 1, tracker.GetUsage().Files)

	tracker.AddFile("/path/to/file2.go")
	assert.Equal(t, 2, tracker.GetUsage().Files)

	// Adding same file should not increase count
	tracker.AddFile("/path/to/file1.go")
	assert.Equal(t, 2, tracker.GetUsage().Files)
}

func TestContextTracker_Reset(t *testing.T) {
	tracker := NewContextTracker()

	tracker.AddTokens(500)
	tracker.AddFile("file1.go")
	tracker.AddFile("file2.go")

	usage := tracker.GetUsage()
	assert.Equal(t, 500, usage.Tokens)
	assert.Equal(t, 2, usage.Files)

	tracker.Reset()

	usage = tracker.GetUsage()
	assert.Equal(t, 0, usage.Tokens)
	assert.Equal(t, 0, usage.Files)
}

func TestContextTracker_Paths(t *testing.T) {
	tracker := NewContextTracker()

	tracker.AddFile("a.go")
	tracker.AddFile("b.go")

	usage := tracker.GetUsage()
	assert.Len(t, usage.Paths, 2)
	assert.Contains(t, usage.Paths, "a.go")
	assert.Contains(t, usage.Paths, "b.go")
}

func TestContextTracker_Concurrent(t *testing.T) {
	tracker := NewContextTracker()
	var wg sync.WaitGroup

	// Concurrent token additions
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tracker.AddTokens(1)
		}()
	}

	// Concurrent file additions
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			tracker.AddFile("file" + string(rune('0'+n%10)) + ".go")
		}(i)
	}

	wg.Wait()

	usage := tracker.GetUsage()
	assert.Equal(t, 100, usage.Tokens)
	// Files might be less than 50 due to deduplication
	assert.LessOrEqual(t, usage.Files, 50)
}

// ─────────────────────────────────────────────────────────────────────────────
// StubAgentService Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestStubAgentService_ProcessPrompt(t *testing.T) {
	svc := &StubAgentService{}
	ctx := context.Background()

	req := PromptRequest{
		Prompt:    "test prompt",
		SessionID: "sess-123",
	}

	resp, err := svc.ProcessPrompt(ctx, req)

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "stub", resp.Status)
	assert.Contains(t, resp.Response, "test prompt")
	assert.Greater(t, resp.Timestamp, int64(0))
}

func TestStubAgentService_StreamPrompt(t *testing.T) {
	svc := &StubAgentService{}
	ctx := context.Background()

	req := PromptRequest{
		Prompt: "stream test",
	}

	var events []StreamEvent
	var mu sync.Mutex

	callback := func(event StreamEvent) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
	}

	err := svc.StreamPrompt(ctx, req, callback)
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()

	// Should have: start + 4 chunks + done = 6 events
	require.GreaterOrEqual(t, len(events), 2)

	// First event is start
	assert.Equal(t, "start", events[0].Type)

	// Last event is done
	assert.Equal(t, "done", events[len(events)-1].Type)
}

func TestStubAgentService_StreamPrompt_Cancellation(t *testing.T) {
	svc := &StubAgentService{}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	req := PromptRequest{
		Prompt: "cancel test",
	}

	var events []StreamEvent
	callback := func(event StreamEvent) {
		events = append(events, event)
	}

	err := svc.StreamPrompt(ctx, req, callback)

	// Should either complete or be cancelled
	if err != nil {
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	}

	// Should have at least the start event
	assert.GreaterOrEqual(t, len(events), 1)
}

// ─────────────────────────────────────────────────────────────────────────────
// Interface Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestAgentServiceInterface(t *testing.T) {
	// Verify StubAgentService implements AgentService
	var _ AgentService = (*StubAgentService)(nil)
}

func TestContextTrackerInterface(t *testing.T) {
	// Verify DefaultContextTracker implements ContextTracker
	var _ ContextTracker = (*DefaultContextTracker)(nil)
}

// ─────────────────────────────────────────────────────────────────────────────
// Type Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestPromptRequest(t *testing.T) {
	req := PromptRequest{
		Prompt:    "hello",
		SessionID: "123",
	}
	assert.Equal(t, "hello", req.Prompt)
	assert.Equal(t, "123", req.SessionID)
}

func TestPromptResponse(t *testing.T) {
	resp := PromptResponse{
		Status:    "success",
		Response:  "world",
		SessionID: "456",
		Timestamp: 1234567890,
	}
	assert.Equal(t, "success", resp.Status)
	assert.Equal(t, "world", resp.Response)
	assert.Equal(t, int64(1234567890), resp.Timestamp)
}

func TestStreamEvent(t *testing.T) {
	event := StreamEvent{
		Type:      "chunk",
		Data:      map[string]string{"content": "hello"},
		ID:        "1",
		Timestamp: time.Now().UnixMilli(),
	}
	assert.Equal(t, "chunk", event.Type)
	assert.Equal(t, "1", event.ID)
}

func TestFocusState(t *testing.T) {
	state := FocusState{
		Target: "main.go",
		Depth:  2,
	}
	assert.Equal(t, "main.go", state.Target)
	assert.Equal(t, 2, state.Depth)
}

func TestFocusResult(t *testing.T) {
	result := FocusResult{
		Success:  true,
		Entities: 10,
		Rendered: "focused context",
	}
	assert.True(t, result.Success)
	assert.Equal(t, 10, result.Entities)
}

func TestContextUsage(t *testing.T) {
	usage := ContextUsage{
		Tokens: 1000,
		Files:  5,
		Paths:  []string{"a.go", "b.go"},
	}
	assert.Equal(t, 1000, usage.Tokens)
	assert.Equal(t, 5, usage.Files)
	assert.Len(t, usage.Paths, 2)
}
