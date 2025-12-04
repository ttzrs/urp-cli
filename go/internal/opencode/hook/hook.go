package hook

import (
	"context"

	"github.com/joss/urp/internal/opencode/domain"
)

// HookType identifies when a hook should be called
type HookType string

const (
	// Pre-execution hooks
	HookPreToolExec   HookType = "pre_tool_exec"
	HookPreMessage    HookType = "pre_message"
	HookPreSend       HookType = "pre_send"

	// Post-execution hooks
	HookPostToolExec  HookType = "post_tool_exec"
	HookPostMessage   HookType = "post_message"
	HookPostResponse  HookType = "post_response"

	// Lifecycle hooks
	HookSessionStart  HookType = "session_start"
	HookSessionEnd    HookType = "session_end"
)

// Context passed to hooks
type Context struct {
	Type      HookType
	SessionID string
	Message   *domain.Message
	ToolCall  *domain.ToolCallPart
	Error     error
}

// Result returned by hooks
type Result struct {
	Continue bool   // Whether to continue processing
	Error    error  // Error to propagate
	Modified bool   // Whether the hook modified the context
}

// Hook is a function called at specific points in execution
type Hook func(ctx context.Context, hctx *Context) Result

// Registry manages hooks
type Registry struct {
	hooks map[HookType][]Hook
}

// NewRegistry creates a new hook registry
func NewRegistry() *Registry {
	return &Registry{
		hooks: make(map[HookType][]Hook),
	}
}

// Register adds a hook for a specific type
func (r *Registry) Register(hookType HookType, hook Hook) {
	r.hooks[hookType] = append(r.hooks[hookType], hook)
}

// Run executes all hooks of a given type
func (r *Registry) Run(ctx context.Context, hctx *Context) Result {
	hooks, ok := r.hooks[hctx.Type]
	if !ok || len(hooks) == 0 {
		return Result{Continue: true}
	}

	for _, hook := range hooks {
		result := hook(ctx, hctx)
		if !result.Continue {
			return result
		}
		if result.Error != nil {
			return result
		}
	}

	return Result{Continue: true}
}

// Has checks if any hooks are registered for a type
func (r *Registry) Has(hookType HookType) bool {
	hooks, ok := r.hooks[hookType]
	return ok && len(hooks) > 0
}

// Clear removes all hooks of a type
func (r *Registry) Clear(hookType HookType) {
	delete(r.hooks, hookType)
}

// ClearAll removes all hooks
func (r *Registry) ClearAll() {
	r.hooks = make(map[HookType][]Hook)
}

// Predefined hooks

// LoggingHook logs hook invocations (for debugging)
func LoggingHook(logger func(string, ...any)) Hook {
	return func(ctx context.Context, hctx *Context) Result {
		switch hctx.Type {
		case HookPreToolExec:
			if hctx.ToolCall != nil {
				logger("hook: pre_tool_exec tool=%s", hctx.ToolCall.Name)
			}
		case HookPostToolExec:
			if hctx.ToolCall != nil {
				logger("hook: post_tool_exec tool=%s error=%v", hctx.ToolCall.Name, hctx.Error)
			}
		case HookPreMessage:
			logger("hook: pre_message session=%s", hctx.SessionID)
		case HookPostResponse:
			logger("hook: post_response session=%s", hctx.SessionID)
		}
		return Result{Continue: true}
	}
}

// ValidationHook checks conditions before execution
func ValidationHook(validate func(*Context) error) Hook {
	return func(ctx context.Context, hctx *Context) Result {
		if err := validate(hctx); err != nil {
			return Result{Continue: false, Error: err}
		}
		return Result{Continue: true}
	}
}

// TransformHook modifies context data
func TransformHook(transform func(*Context)) Hook {
	return func(ctx context.Context, hctx *Context) Result {
		transform(hctx)
		return Result{Continue: true, Modified: true}
	}
}
