package session

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/joss/urp/internal/opencode/domain"
	"github.com/joss/urp/pkg/llm"
)

const titlePrompt = `Generate a short title (3-5 words) for this conversation based on the first message.
The title should describe what the user wants to accomplish.
Output ONLY the title, no quotes, no punctuation at end.

First message: %s`

// TitleGenerator generates session titles from first message
type TitleGenerator struct {
	provider llm.Provider
	model    string
}

func NewTitleGenerator(provider llm.Provider, model string) *TitleGenerator {
	return &TitleGenerator{
		provider: provider,
		model:    model,
	}
}

// Generate creates a title from the first user message
func (t *TitleGenerator) Generate(ctx context.Context, firstMessage string) (string, error) {
	// Truncate long messages
	if len(firstMessage) > 500 {
		firstMessage = firstMessage[:500] + "..."
	}

	prompt := fmt.Sprintf(titlePrompt, firstMessage)

	req := &llm.ChatRequest{
		Model: t.model,
		Messages: []domain.Message{
			{
				ID:        "title-req",
				Role:      domain.RoleUser,
				Parts:     []domain.Part{domain.TextPart{Text: prompt}},
				Timestamp: time.Now(),
			},
		},
		MaxTokens: 50,
	}

	events, err := t.provider.Chat(ctx, req)
	if err != nil {
		return "", fmt.Errorf("chat: %w", err)
	}

	var title strings.Builder
	for event := range events {
		if event.Type == domain.StreamEventText {
			title.WriteString(event.Content)
		}
		if event.Type == domain.StreamEventError {
			return "", event.Error
		}
	}

	result := strings.TrimSpace(title.String())
	// Ensure max length
	if len(result) > 100 {
		result = result[:97] + "..."
	}

	return result, nil
}

// GenerateSimple creates a simple title without LLM
func GenerateSimple(firstMessage string) string {
	// Remove newlines and extra spaces
	msg := strings.Join(strings.Fields(firstMessage), " ")

	// Truncate to first sentence or 50 chars
	if idx := strings.IndexAny(msg, ".!?"); idx > 0 && idx < 50 {
		msg = msg[:idx]
	} else if len(msg) > 50 {
		// Find word boundary
		if idx := strings.LastIndex(msg[:50], " "); idx > 20 {
			msg = msg[:idx]
		} else {
			msg = msg[:47] + "..."
		}
	}

	return msg
}
