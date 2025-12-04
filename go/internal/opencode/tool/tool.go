package tool

import (
	"context"

	"github.com/joss/urp/internal/opencode/domain"
)

// Executor is the interface all tools must implement
type Executor interface {
	Info() domain.Tool
	Execute(ctx context.Context, args map[string]any) (*Result, error)
}

// Result holds the output of a tool execution
type Result struct {
	Title       string
	Output      string
	Metadata    map[string]any
	Attachments []domain.FilePart
	Images      []domain.ImagePart // Images to include in tool result for vision
	Error       error
}

// ToolRegistry defines the interface for tool management (DIP)
type ToolRegistry interface {
	Get(name string) (Executor, bool)
	All() []domain.Tool
	Execute(ctx context.Context, name string, args map[string]any) (*Result, error)
}

// Registry holds all available tools
type Registry struct {
	tools map[string]Executor
}

func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Executor),
	}
}

func (r *Registry) Register(t Executor) {
	info := t.Info()
	r.tools[info.Name] = t
}

func (r *Registry) Get(name string) (Executor, bool) {
	t, ok := r.tools[name]
	return t, ok
}

func (r *Registry) All() []domain.Tool {
	result := make([]domain.Tool, 0, len(r.tools))
	for _, t := range r.tools {
		result = append(result, t.Info())
	}
	return result
}

func (r *Registry) Execute(ctx context.Context, name string, args map[string]any) (*Result, error) {
	t, ok := r.tools[name]
	if !ok {
		return &Result{Error: ErrToolNotFound}, ErrToolNotFound
	}
	return t.Execute(ctx, args)
}

// Default registry with all built-in tools
func DefaultRegistry(workDir string) *Registry {
	r := NewRegistry()
	r.Register(NewBash(workDir))
	r.Register(NewRead())
	r.Register(NewWrite())
	r.Register(NewEdit())
	r.Register(NewGlob(workDir))
	r.Register(NewGrep(workDir))
	r.Register(NewLS(workDir))
	r.Register(NewWebFetch())
	r.Register(NewWebSearch())
	r.Register(NewScreenshot())
	r.Register(NewScreenCapture())
	r.Register(NewComputer())
	return r
}

type ToolError string

func (e ToolError) Error() string { return string(e) }

const (
	ErrToolNotFound ToolError = "tool not found"
	ErrInvalidArgs  ToolError = "invalid arguments"
)

// Verify Registry implements ToolRegistry
var _ ToolRegistry = (*Registry)(nil)
