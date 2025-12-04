package agent

import (
	"context"
	"sync"
	"time"

	"github.com/joss/urp/internal/opencode/domain"
	"github.com/joss/urp/internal/opencode/hook"
	"github.com/joss/urp/internal/opencode/permission"
	"github.com/joss/urp/internal/opencode/tool"
)

// ToolExecutor handles tool execution with permissions and hooks
type ToolExecutor struct {
	tools       tool.ToolRegistry
	permissions *permission.Manager
	hooks       *hook.Registry
	permMu      sync.Mutex // Serializes permission dialogs to avoid UI conflicts
}

// NewToolExecutor creates a new executor
func NewToolExecutor(tools tool.ToolRegistry, perms *permission.Manager) *ToolExecutor {
	return &ToolExecutor{
		tools:       tools,
		permissions: perms,
		hooks:       hook.NewRegistry(),
	}
}

// WithHooks sets the hook registry
func (e *ToolExecutor) WithHooks(hooks *hook.Registry) *ToolExecutor {
	e.hooks = hooks
	return e
}

// Hooks returns the hook registry for external registration
func (e *ToolExecutor) Hooks() *hook.Registry {
	return e.hooks
}

// ExecuteResult contains the result of a tool execution
type ExecuteResult struct {
	Part    domain.ToolCallPart
	Images  []domain.ImagePart // Images to include in response (for vision)
	Allowed bool
}

// Execute runs a tool call with permission checking
// Returns the result via events channel, handles permission flow
func (e *ToolExecutor) Execute(
	ctx context.Context,
	tc domain.ToolCallPart,
	startTime time.Time,
	events chan<- domain.StreamEvent,
) ExecuteResult {
	// Run pre-tool hooks
	if e.hooks != nil {
		hctx := &hook.Context{
			Type:     hook.HookPreToolExec,
			ToolCall: &tc,
		}
		result := e.hooks.Run(ctx, hctx)
		if !result.Continue {
			tc.Error = "blocked by hook"
			if result.Error != nil {
				tc.Error = result.Error.Error()
			}
			tc.Duration = time.Since(startTime)
			events <- domain.StreamEvent{
				Type: domain.StreamEventToolDone,
				Part: tc,
			}
			return ExecuteResult{Part: tc, Allowed: false}
		}
	}

	// Check permission before executing
	if e.permissions != nil {
		req := e.buildPermissionRequest(tc)
		perm, reason := e.permissions.Check(req)

		if perm == domain.PermissionDeny {
			tc.Error = "permission denied"
			tc.Duration = time.Since(startTime)
			events <- domain.StreamEvent{
				Type: domain.StreamEventToolDone,
				Part: tc,
			}
			return ExecuteResult{Part: tc, Allowed: false}
		}

		if perm == domain.PermissionAsk {
			// Serialize permission dialogs to avoid UI conflicts
			// Lock ensures only one dialog is shown at a time
			e.permMu.Lock()

			// Ask user for permission
			respChan := make(chan bool, 1)
			events <- domain.StreamEvent{
				Type: domain.StreamEventPermissionAsk,
				PermissionReq: &domain.PermissionRequest{
					ID:      tc.ToolID,
					Tool:    tc.Name,
					Command: getCommand(tc.Args),
					Path:    getPath(tc.Args),
					Reason:  reason,
				},
				PermissionResp: respChan,
			}

			// Wait for response
			var allowed bool
			select {
			case allowed = <-respChan:
			case <-ctx.Done():
				e.permMu.Unlock()
				tc.Error = "cancelled"
				tc.Duration = time.Since(startTime)
				events <- domain.StreamEvent{
					Type: domain.StreamEventToolDone,
					Part: tc,
				}
				return ExecuteResult{Part: tc, Allowed: false}
			}

			e.permMu.Unlock()

			if !allowed {
				tc.Error = "permission denied by user"
				tc.Duration = time.Since(startTime)
				events <- domain.StreamEvent{
					Type: domain.StreamEventToolDone,
					Part: tc,
				}
				return ExecuteResult{Part: tc, Allowed: false}
			}
		}
	}

	// Execute the tool
	result, err := e.tools.Execute(ctx, tc.Name, tc.Args)

	tc.Duration = time.Since(startTime)
	if err != nil {
		tc.Error = err.Error()
	}

	var images []domain.ImagePart
	if result != nil {
		tc.Result = result.Output
		images = result.Images
	}

	// Run post-tool hooks
	if e.hooks != nil {
		hctx := &hook.Context{
			Type:     hook.HookPostToolExec,
			ToolCall: &tc,
			Error:    err,
		}
		e.hooks.Run(ctx, hctx)
		// Post hooks don't block, but can log/record
	}

	events <- domain.StreamEvent{
		Type: domain.StreamEventToolDone,
		Part: tc,
	}

	return ExecuteResult{Part: tc, Images: images, Allowed: true}
}

func (e *ToolExecutor) buildPermissionRequest(tc domain.ToolCallPart) permission.Request {
	return permission.Request{
		Tool:    tc.Name,
		Command: getCommand(tc.Args),
		Path:    getPath(tc.Args),
		Args:    tc.Args,
	}
}

func getCommand(args map[string]any) string {
	if cmd, ok := args["command"].(string); ok {
		return cmd
	}
	return ""
}

func getPath(args map[string]any) string {
	if p, ok := args["path"].(string); ok {
		return p
	}
	if p, ok := args["file_path"].(string); ok {
		return p
	}
	return ""
}
