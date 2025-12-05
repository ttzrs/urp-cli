package agent

import (
	"fmt"

	"github.com/joss/urp/internal/opencode/domain"
)

// PromptBuilder constructs system prompts for agent sessions
type PromptBuilder struct {
	customPrompt string
}

// NewPromptBuilder creates a new prompt builder
func NewPromptBuilder() *PromptBuilder {
	return &PromptBuilder{}
}

// SetCustomPrompt sets additional custom instructions
func (p *PromptBuilder) SetCustomPrompt(prompt string) {
	p.customPrompt = prompt
}

// Build constructs the full system prompt for a session
func (p *PromptBuilder) Build(session *domain.Session) string {
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

	if p.customPrompt != "" {
		prompt += "\n" + p.customPrompt
	}

	return prompt
}
