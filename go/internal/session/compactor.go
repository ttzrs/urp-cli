package session

import (
	"fmt"
	"strings"

	"github.com/joss/urp/internal/opencode/domain"
)

// MessageCompactor handles summarization of long message histories
// preserving critical information while reducing token usage
type MessageCompactor struct {
	maxHistoryMessages int // Keep this many recent messages
}

// NewMessageCompactor creates a new message compactor
func NewMessageCompactor(maxHistoryMessages int) *MessageCompactor {
	if maxHistoryMessages < 5 {
		maxHistoryMessages = 5 // Minimum to maintain context
	}
	return &MessageCompactor{
		maxHistoryMessages: maxHistoryMessages,
	}
}

// CompactMessages reduces message history while preserving critical context
// Strategy: Keep recent messages + summarized history of failures and constraints
func (mc *MessageCompactor) CompactMessages(messages []domain.Message, taskContext any) []domain.Message {
	if len(messages) <= mc.maxHistoryMessages {
		return messages // Already small enough
	}

	// Extract critical information from old messages
	oldMessages := messages[:len(messages)-mc.maxHistoryMessages]
	recentMessages := messages[len(messages)-mc.maxHistoryMessages:]

	// Build summary of critical events
	failures := mc.ExtractFailures(oldMessages)
	constraints := mc.ExtractConstraints(oldMessages)
	causalChain := mc.ExtractCausalChain(oldMessages)

	// Create summary message with preserved information
	summaryParts := []domain.Part{}

	if len(failures) > 0 {
		failureText := "[COMPACTED HISTORY - FAILURES]\n" + strings.Join(failures, "\n")
		summaryParts = append(summaryParts, domain.TextPart{Text: failureText})
	}

	if len(constraints) > 0 {
		constraintText := "[COMPACTED HISTORY - CONSTRAINTS]\n" + strings.Join(constraints, "\n")
		summaryParts = append(summaryParts, domain.TextPart{Text: constraintText})
	}

	if len(causalChain) > 0 {
		causalText := "[COMPACTED HISTORY - DECISIONS]\n" + strings.Join(causalChain, "\n")
		summaryParts = append(summaryParts, domain.TextPart{Text: causalText})
	}

	// If we have summary content, prepend it to recent messages
	if len(summaryParts) > 0 {
		summaryMsg := domain.Message{
			ID:       "compacted-summary",
			Role:     domain.RoleSystem,
			Parts:    summaryParts,
		}
		return append([]domain.Message{summaryMsg}, recentMessages...)
	}

	// Fallback: just return recent messages
	return recentMessages
}

// ExtractFailures preserves all tool execution failures
// Scans for ToolCallPart with non-empty Error field
func (mc *MessageCompactor) ExtractFailures(messages []domain.Message) []string {
	var failures []string
	seenErrors := make(map[string]bool) // Deduplicate

	for _, msg := range messages {
		if msg.Role != domain.RoleUser && msg.Role != domain.RoleAssistant {
			continue
		}

		for _, part := range msg.Parts {
			if toolCall, ok := part.(domain.ToolCallPart); ok {
				if toolCall.Error != "" {
					key := fmt.Sprintf("%s:%s", toolCall.Name, toolCall.Error)
					if !seenErrors[key] {
						errorMsg := fmt.Sprintf("- %s failed: %s", toolCall.Name, toolCall.Error)
						failures = append(failures, errorMsg)
						seenErrors[key] = true
					}
				}
			}
		}
	}

	return failures
}

// ExtractConstraints finds assistant-stated requirements and constraints
// Looks for explicit statements like "must", "must not", "required", "constraint"
func (mc *MessageCompactor) ExtractConstraints(messages []domain.Message) []string {
	var constraints []string
	seenConstraints := make(map[string]bool) // Deduplicate

	constraintKeywords := []string{"must", "must not", "required", "cannot", "constraint", "cannot be", "should not", "forbidden"}

	for _, msg := range messages {
		if msg.Role != domain.RoleAssistant {
			continue
		}

		for _, part := range msg.Parts {
			if textPart, ok := part.(domain.TextPart); ok {
				sentences := strings.Split(textPart.Text, ".")
				for _, sentence := range sentences {
					lowerSentence := strings.ToLower(strings.TrimSpace(sentence))
					for _, keyword := range constraintKeywords {
						if strings.Contains(lowerSentence, keyword) {
							if !seenConstraints[lowerSentence] {
								constraints = append(constraints, "- "+strings.TrimSpace(sentence))
								seenConstraints[lowerSentence] = true
							}
							break
						}
					}
				}
			}
		}
	}

	return constraints
}

// ExtractCausalChain preserves decision lineage
// Maps tool calls to their outcomes and the reasoning that followed
func (mc *MessageCompactor) ExtractCausalChain(messages []domain.Message) []string {
	var chain []string
	seenDecisions := make(map[string]bool)

	// Iterate pairs: assistant message → tool calls → user message → response
	for i := 0; i < len(messages)-2; i++ {
		// Look for pattern: assistant with tool calls → next message with results/reasoning
		msg := messages[i]
		if msg.Role != domain.RoleAssistant {
			continue
		}

		// Check if this message has tool calls or important reasoning
		hasToolCalls := false
		var toolNames []string

		for _, part := range msg.Parts {
			if toolCall, ok := part.(domain.ToolCallPart); ok {
				hasToolCalls = true
				toolNames = append(toolNames, toolCall.Name)
			}
		}

		if hasToolCalls && i+1 < len(messages) {
			nextMsg := messages[i+1]
			// Extract reasoning from next message if it exists
			for _, part := range nextMsg.Parts {
				if textPart, ok := part.(domain.TextPart); ok {
					text := strings.TrimSpace(textPart.Text)
					if len(text) > 0 && len(text) < 200 { // Preserve concise decisions
						key := fmt.Sprintf("%v:%s", toolNames, text[:min(50, len(text))])
						if !seenDecisions[key] {
							decision := fmt.Sprintf("- Tried %v, resulted in: %s", toolNames, text[:min(100, len(text))])
							chain = append(chain, decision)
							seenDecisions[key] = true
						}
					}
					break
				}
			}
		}
	}

	return chain
}

// ShouldCompact determines if message history should be compacted
// Returns true if:
// - Message count > threshold
// - Estimated token count > budget
func (mc *MessageCompactor) ShouldCompact(messages []domain.Message, tokenBudgetPercent float64) bool {
	if len(messages) <= mc.maxHistoryMessages {
		return false
	}

	// Rule 1: Too many messages
	if len(messages) > mc.maxHistoryMessages*2 {
		return true
	}

	// Rule 2: Estimated token overflow (rough heuristic: 4 chars ≈ 1 token)
	estimatedTokens := 0
	for _, msg := range messages {
		for _, part := range msg.Parts {
			if textPart, ok := part.(domain.TextPart); ok {
				estimatedTokens += len(textPart.Text) / 4
			}
		}
	}

	// Trigger if > 80% of a 16K token window (12,800 tokens)
	return estimatedTokens > int(16384*tokenBudgetPercent)
}

// helper
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
