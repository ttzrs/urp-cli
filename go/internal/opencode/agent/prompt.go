package agent

import (
	"context"
	"fmt"

	"github.com/joss/urp/internal/compiler"
	"github.com/joss/urp/internal/opencode/domain"
)

// PromptBuilder constructs system prompts for agent sessions
type PromptBuilder struct {
	customPrompt string
	taskContext  *TaskContext
	compiler     *compiler.ContextCompiler
}

// NewPromptBuilder creates a new prompt builder
func NewPromptBuilder() *PromptBuilder {
	return &PromptBuilder{}
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

// Build constructs the full system prompt for a session
func (p *PromptBuilder) Build(ctx context.Context, session *domain.Session) string {
	// 1. Try to use Context Compiler (Architecture V2)
	if p.compiler != nil {
		goal := "Perform engineering task"
		if p.taskContext != nil {
			// Extract goal from task context (Objective is usually the first input)
			// For now, we use a generic goal if not available, or the session title
			if session.Title != "" {
				goal = session.Title
			}
		}

		// Compile the context (logs are empty for now as we don't have a structured log store yet)
		// We use session.ID as the session ID
		compiled, err := p.compiler.Compile(ctx, session.ID, goal, "", session.Directory)
		if err == nil {
			// Success! Return the compiled view.
			// We append the custom prompt and task context as before, 
			// although the Compiler should ideally handle this.
			// For transition, we'll append custom prompt if set.
			if p.customPrompt != "" {
				compiled += "\n" + p.customPrompt
			}
			return compiled
		}
		// Fallback to V1 on error
		fmt.Printf("Context Compiler failed, falling back to V1: %v\n", err)
	}

	// 2. Fallback to V1 (Legacy)
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

	return prompt
}
