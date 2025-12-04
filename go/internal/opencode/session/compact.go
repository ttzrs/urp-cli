package session

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/joss/urp/internal/opencode/domain"
	"github.com/joss/urp/pkg/llm"
)

const compactPrompt = `You are a conversation summarizer. Summarize the following conversation between a user and an AI assistant.

Focus on:
- Key decisions made
- Important code changes or files modified
- Problems solved and solutions used
- Current state of the task

Keep the summary concise but complete enough to continue the conversation.
Do NOT include pleasantries or meta-commentary about the conversation.
Output ONLY the summary, nothing else.

Conversation to summarize:
%s`

// Compactor handles session compaction
type Compactor struct {
	manager  *Manager
	provider llm.Provider
	model    string
}

func NewCompactor(manager *Manager, provider llm.Provider, model string) *Compactor {
	return &Compactor{
		manager:  manager,
		provider: provider,
		model:    model,
	}
}

// Compact summarizes old messages and replaces them with a summary
func (c *Compactor) Compact(ctx context.Context, sessionID string, keepLast int) (*CompactResult, error) {
	messages, err := c.manager.GetMessages(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("get messages: %w", err)
	}

	if len(messages) <= keepLast {
		return &CompactResult{
			OriginalCount: len(messages),
			NewCount:      len(messages),
			Skipped:       true,
		}, nil
	}

	// Split messages: old ones to summarize, recent ones to keep
	toSummarize := messages[:len(messages)-keepLast]
	toKeep := messages[len(messages)-keepLast:]

	// Build conversation text
	var sb strings.Builder
	for _, msg := range toSummarize {
		role := string(msg.Role)
		if msg.Role == domain.RoleUser {
			role = "User"
		} else if msg.Role == domain.RoleAssistant {
			role = "Assistant"
		}

		for _, part := range msg.Parts {
			switch p := part.(type) {
			case domain.TextPart:
				sb.WriteString(fmt.Sprintf("%s: %s\n\n", role, p.Text))
			case domain.ToolCallPart:
				if p.Result != "" {
					sb.WriteString(fmt.Sprintf("Assistant used tool '%s'\n", p.Name))
				}
			}
		}
	}

	// Generate summary
	summary, err := c.generateSummary(ctx, sb.String())
	if err != nil {
		return nil, fmt.Errorf("generate summary: %w", err)
	}

	// Create summary message
	summaryMsg := &domain.Message{
		ID:        ulid.Make().String(),
		SessionID: sessionID,
		Role:      domain.RoleSystem,
		Parts: []domain.Part{
			domain.TextPart{Text: fmt.Sprintf("[Conversation Summary]\n\n%s", summary)},
		},
		Timestamp: toSummarize[0].Timestamp, // Use first summarized message timestamp
	}

	// Delete old messages and add summary
	// Note: In a production system, we'd do this in a transaction
	for _, msg := range toSummarize {
		if err := c.manager.store.DeleteMessage(ctx, msg.ID); err != nil {
			return nil, fmt.Errorf("delete message %s: %w", msg.ID, err)
		}
	}

	if err := c.manager.AddMessage(ctx, summaryMsg); err != nil {
		return nil, fmt.Errorf("add summary: %w", err)
	}

	return &CompactResult{
		OriginalCount:   len(messages),
		SummarizedCount: len(toSummarize),
		NewCount:        len(toKeep) + 1, // kept + summary
		Summary:         summary,
	}, nil
}

func (c *Compactor) generateSummary(ctx context.Context, conversation string) (string, error) {
	prompt := fmt.Sprintf(compactPrompt, conversation)

	req := &llm.ChatRequest{
		Model: c.model,
		Messages: []domain.Message{
			{
				ID:        "compact-req",
				Role:      domain.RoleUser,
				Parts:     []domain.Part{domain.TextPart{Text: prompt}},
				Timestamp: time.Now(),
			},
		},
		MaxTokens: 2048,
	}

	events, err := c.provider.Chat(ctx, req)
	if err != nil {
		return "", fmt.Errorf("chat: %w", err)
	}

	var summary strings.Builder
	for event := range events {
		if event.Type == domain.StreamEventText {
			summary.WriteString(event.Content)
		}
		if event.Type == domain.StreamEventError {
			return "", event.Error
		}
	}

	return strings.TrimSpace(summary.String()), nil
}

// CompactResult holds the result of compaction
type CompactResult struct {
	OriginalCount   int
	SummarizedCount int
	NewCount        int
	Summary         string
	Skipped         bool
}

// EstimateTokens gives a rough estimate of tokens in messages
func EstimateTokens(messages []*domain.Message) int {
	total := 0
	for _, msg := range messages {
		for _, part := range msg.Parts {
			switch p := part.(type) {
			case domain.TextPart:
				// Rough estimate: 1 token per 4 characters
				total += len(p.Text) / 4
			case domain.ToolCallPart:
				total += len(p.Result) / 4
			}
		}
	}
	return total
}
