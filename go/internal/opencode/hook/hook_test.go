package hook

import (
	"context"
	"errors"
	"testing"

	"github.com/joss/urp/internal/opencode/domain"
	"github.com/stretchr/testify/assert"
)

// --- Registry Tests ---

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	assert.NotNil(t, r)
	assert.NotNil(t, r.hooks)
}

func TestRegister(t *testing.T) {
	r := NewRegistry()

	hook := func(ctx context.Context, hctx *Context) Result {
		return Result{Continue: true}
	}

	r.Register(HookPreToolExec, hook)

	assert.True(t, r.Has(HookPreToolExec))
	assert.False(t, r.Has(HookPostToolExec))
}

func TestRegisterMultiple(t *testing.T) {
	r := NewRegistry()

	called := []int{}
	r.Register(HookPreToolExec, func(ctx context.Context, hctx *Context) Result {
		called = append(called, 1)
		return Result{Continue: true}
	})
	r.Register(HookPreToolExec, func(ctx context.Context, hctx *Context) Result {
		called = append(called, 2)
		return Result{Continue: true}
	})

	hctx := &Context{Type: HookPreToolExec}
	r.Run(context.Background(), hctx)

	assert.Equal(t, []int{1, 2}, called)
}

func TestRunNoHooks(t *testing.T) {
	r := NewRegistry()

	hctx := &Context{Type: HookPreToolExec}
	result := r.Run(context.Background(), hctx)

	assert.True(t, result.Continue)
	assert.NoError(t, result.Error)
}

func TestRunStopsOnContinueFalse(t *testing.T) {
	r := NewRegistry()

	called := []int{}
	r.Register(HookPreToolExec, func(ctx context.Context, hctx *Context) Result {
		called = append(called, 1)
		return Result{Continue: false}
	})
	r.Register(HookPreToolExec, func(ctx context.Context, hctx *Context) Result {
		called = append(called, 2) // Should not be called
		return Result{Continue: true}
	})

	hctx := &Context{Type: HookPreToolExec}
	result := r.Run(context.Background(), hctx)

	assert.False(t, result.Continue)
	assert.Equal(t, []int{1}, called)
}

func TestRunStopsOnError(t *testing.T) {
	r := NewRegistry()
	testErr := errors.New("test error")

	r.Register(HookPreToolExec, func(ctx context.Context, hctx *Context) Result {
		return Result{Continue: true, Error: testErr}
	})

	hctx := &Context{Type: HookPreToolExec}
	result := r.Run(context.Background(), hctx)

	assert.Equal(t, testErr, result.Error)
}

func TestHas(t *testing.T) {
	r := NewRegistry()

	assert.False(t, r.Has(HookPreToolExec))

	r.Register(HookPreToolExec, func(ctx context.Context, hctx *Context) Result {
		return Result{Continue: true}
	})

	assert.True(t, r.Has(HookPreToolExec))
	assert.False(t, r.Has(HookPostToolExec))
}

func TestClear(t *testing.T) {
	r := NewRegistry()

	r.Register(HookPreToolExec, func(ctx context.Context, hctx *Context) Result {
		return Result{Continue: true}
	})
	r.Register(HookPostToolExec, func(ctx context.Context, hctx *Context) Result {
		return Result{Continue: true}
	})

	r.Clear(HookPreToolExec)

	assert.False(t, r.Has(HookPreToolExec))
	assert.True(t, r.Has(HookPostToolExec))
}

func TestClearAll(t *testing.T) {
	r := NewRegistry()

	r.Register(HookPreToolExec, func(ctx context.Context, hctx *Context) Result {
		return Result{Continue: true}
	})
	r.Register(HookPostToolExec, func(ctx context.Context, hctx *Context) Result {
		return Result{Continue: true}
	})

	r.ClearAll()

	assert.False(t, r.Has(HookPreToolExec))
	assert.False(t, r.Has(HookPostToolExec))
}

// --- Context Tests ---

func TestContextFields(t *testing.T) {
	msg := &domain.Message{ID: "msg-123"}
	tc := &domain.ToolCallPart{Name: "bash"}
	err := errors.New("test error")

	ctx := &Context{
		Type:      HookPreToolExec,
		SessionID: "sess-123",
		Message:   msg,
		ToolCall:  tc,
		Error:     err,
	}

	assert.Equal(t, HookPreToolExec, ctx.Type)
	assert.Equal(t, "sess-123", ctx.SessionID)
	assert.Equal(t, msg, ctx.Message)
	assert.Equal(t, tc, ctx.ToolCall)
	assert.Equal(t, err, ctx.Error)
}

// --- Predefined Hooks Tests ---

func TestLoggingHook(t *testing.T) {
	var logged []string
	logger := func(format string, args ...any) {
		logged = append(logged, format)
	}

	hook := LoggingHook(logger)

	// Test pre_tool_exec
	result := hook(context.Background(), &Context{
		Type:     HookPreToolExec,
		ToolCall: &domain.ToolCallPart{Name: "bash"},
	})
	assert.True(t, result.Continue)
	assert.Len(t, logged, 1)
	assert.Contains(t, logged[0], "pre_tool_exec")

	// Test post_tool_exec
	logged = nil
	result = hook(context.Background(), &Context{
		Type:     HookPostToolExec,
		ToolCall: &domain.ToolCallPart{Name: "edit"},
	})
	assert.True(t, result.Continue)
	assert.Contains(t, logged[0], "post_tool_exec")

	// Test pre_message
	logged = nil
	result = hook(context.Background(), &Context{
		Type:      HookPreMessage,
		SessionID: "sess-123",
	})
	assert.True(t, result.Continue)
	assert.Contains(t, logged[0], "pre_message")
}

func TestValidationHook(t *testing.T) {
	testErr := errors.New("validation failed")

	// Passing validation
	passHook := ValidationHook(func(hctx *Context) error {
		return nil
	})
	result := passHook(context.Background(), &Context{})
	assert.True(t, result.Continue)
	assert.NoError(t, result.Error)

	// Failing validation
	failHook := ValidationHook(func(hctx *Context) error {
		return testErr
	})
	result = failHook(context.Background(), &Context{})
	assert.False(t, result.Continue)
	assert.Equal(t, testErr, result.Error)
}

func TestTransformHook(t *testing.T) {
	hook := TransformHook(func(hctx *Context) {
		hctx.SessionID = "transformed"
	})

	hctx := &Context{SessionID: "original"}
	result := hook(context.Background(), hctx)

	assert.True(t, result.Continue)
	assert.True(t, result.Modified)
	assert.Equal(t, "transformed", hctx.SessionID)
}

// --- Hook Types ---

func TestHookTypes(t *testing.T) {
	types := []HookType{
		HookPreToolExec,
		HookPreMessage,
		HookPreSend,
		HookPostToolExec,
		HookPostMessage,
		HookPostResponse,
		HookSessionStart,
		HookSessionEnd,
	}

	// All types should be unique non-empty strings
	seen := make(map[HookType]bool)
	for _, ht := range types {
		assert.NotEmpty(t, string(ht))
		assert.False(t, seen[ht], "duplicate hook type: %s", ht)
		seen[ht] = true
	}
}
