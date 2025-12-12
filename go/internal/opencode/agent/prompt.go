package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/joss/urp/internal/compiler"
	"github.com/joss/urp/internal/gate"
	"github.com/joss/urp/internal/opencode/domain"
)

// PromptBuilder constructs system prompts for agent sessions
type PromptBuilder struct {
	customPrompt string
	taskContext  *TaskContext
	compiler     *compiler.ContextCompiler

	// Accumulated tool logs for gate filtering
	toolLogs strings.Builder

	// Last compilation metadata
	lastGateResult *gate.GateResult

	// Max tool logs size before forcing clear (prevents unbounded growth)
	maxToolLogsSize int
}

// NewPromptBuilder creates a new prompt builder
func NewPromptBuilder() *PromptBuilder {
	return &PromptBuilder{
		maxToolLogsSize: 50000, // ~50KB max, ~12k tokens
	}
}

// SetCustomPrompt sets additional custom instructions
func (p *PromptBuilder) SetCustomPrompt(prompt string) {
	p.customPrompt = prompt
}

// SetTaskContext sets the current task context for injection
func (p *PromptBuilder) SetTaskContext(tc *TaskContext) {
	p.taskContext = tc
}

// SetCompiler injects the ContextCompiler
func (p *PromptBuilder) SetCompiler(c *compiler.ContextCompiler) {
	p.compiler = c
}

// SetMaxToolLogsSize sets the maximum size for accumulated tool logs.
func (p *PromptBuilder) SetMaxToolLogsSize(size int) {
	p.maxToolLogsSize = size
}

// AddToolLog adds a tool execution result to the logs buffer.
// These logs will be passed to the Gate for filtering on next Build().
// Automatically truncates if logs exceed max size.
func (p *PromptBuilder) AddToolLog(toolName string, result string, errMsg string) {
	// Check if we're at capacity - if so, clear oldest logs
	if p.toolLogs.Len() > p.maxToolLogsSize {
		// Keep only the last 25% of logs (most recent)
		current := p.toolLogs.String()
		keepFrom := len(current) * 3 / 4
		p.toolLogs.Reset()
		p.toolLogs.WriteString("... (older logs truncated)\n")
		p.toolLogs.WriteString(current[keepFrom:])
	}

	if errMsg != "" {
		p.toolLogs.WriteString(fmt.Sprintf("[%s] ERROR: %s\n", toolName, truncateLog(errMsg, 1000)))
	} else if result != "" {
		p.toolLogs.WriteString(fmt.Sprintf("[%s] %s\n", toolName, truncateLog(result, 1000)))
	}
}

// truncateLog truncates a log entry to maxLen characters
func truncateLog(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "... (truncated)"
}

// ClearToolLogs clears the accumulated tool logs.
func (p *PromptBuilder) ClearToolLogs() {
	p.toolLogs.Reset()
}

// ToolLogsSize returns the current size of accumulated tool logs.
func (p *PromptBuilder) ToolLogsSize() int {
	return p.toolLogs.Len()
}

// LastGateResult returns the result of the last gate call (for UI display).
func (p *PromptBuilder) LastGateResult() *gate.GateResult {
	return p.lastGateResult
}

// GateModel returns the configured gate model name.
func (p *PromptBuilder) GateModel() string {
	if p.compiler != nil {
		return p.compiler.GateModel()
	}
	return ""
}

// BuildResult contains the built prompt and metadata.
type BuildResult struct {
	Prompt     string
	GateResult *gate.GateResult
}

// Build constructs the full system prompt for a session
func (p *PromptBuilder) Build(ctx context.Context, session *domain.Session) string {
	result := p.BuildWithMetadata(ctx, session)
	return result.Prompt
}

// BuildWithMetadata constructs the prompt and returns metadata about compilation.
func (p *PromptBuilder) BuildWithMetadata(ctx context.Context, session *domain.Session) *BuildResult {
	result := &BuildResult{}
	p.lastGateResult = nil

	// 1. Try to use Context Compiler (Architecture V2)
	if p.compiler != nil {
		goal := "Perform engineering task"
		if p.taskContext != nil {
			if session.Title != "" {
				goal = session.Title
			}
		}

		// Get accumulated tool logs
		rawLogs := p.toolLogs.String()

		// Compile the context with logs
		compileResult, err := p.compiler.CompileWithMetadata(ctx, session.ID, goal, rawLogs, session.Directory)
		if err == nil {
			// Success! Store gate result for UI
			result.GateResult = compileResult.GateResult
			p.lastGateResult = compileResult.GateResult

			// ALWAYS clear logs after compilation attempt (success or filtered)
			p.toolLogs.Reset()

			// Append custom prompt if set
			compiled := compileResult.Prompt
			if p.customPrompt != "" {
				compiled += "\n" + p.customPrompt
			}
			result.Prompt = compiled
			return result
		}
		// Fallback to V1 on error - still clear logs to prevent unbounded growth
		fmt.Printf("Context Compiler failed, falling back to V1: %v\n", err)
		p.toolLogs.Reset()
	}

	// 2. Fallback to V1 (Legacy) - No gate, logs are cleared above
	basePrompt := `You are an AI coding assistant. You help users with software engineering tasks.

Working directory: %s

## Memory-First Approach
You have access to a graph database with indexed code structure. Use it BEFORE reading files:

1. **graph_search** - Find files, functions, classes by name. Much faster than grep for structure.
2. **code_deps** - Find what calls a function or what a function calls.
3. **graph_stats** - See what's indexed (file count, function count, etc.)
4. **context_search** - Semantic search to find relevant code for a task.
5. **memory_recall** - Recall notes/decisions from this or previous sessions.
6. **knowledge_query** - Search learned patterns, rules, and fixes.
7. **wisdom** - Find similar past errors and their solutions.
8. **memory_add** - Store important context for later.

## Workflow
1. Use graph_search/context_search to find relevant files FIRST
2. Only read files that are actually needed
3. Use wisdom when encountering errors
4. Store important decisions with memory_add

## CRITICAL: Persist Key Context
After completing significant work, use memory_add to save:
- Decisions made ("decision": why X approach was chosen over Y)
- Files modified ("note": list of files changed and why)
- Errors solved ("observation": what error occurred and how it was fixed)

This ensures context survives session compaction.

## Guidelines
- Be concise and direct
- Use tools to accomplish tasks
- Read files before editing them
- Prefer editing existing files over creating new ones
- Don't create documentation unless explicitly asked
`
	prompt := fmt.Sprintf(basePrompt, session.Directory)

	if p.customPrompt != "" {
		prompt += "\n" + p.customPrompt
	}

	// Inject task context if available (keeps agent focused)
	if p.taskContext != nil && p.taskContext.TurnCount > 0 {
		prompt += "\n\n" + p.taskContext.BuildReminder()
	}

	result.Prompt = prompt
	return result
}
