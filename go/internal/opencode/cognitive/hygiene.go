// Package cognitive implements memory hygiene (auto-compaction)
package cognitive

import (
	"strings"

	"github.com/joss/urp/internal/opencode/domain"
)

// HygieneConfig configures memory cleanup behavior
type HygieneConfig struct {
	// MaxToolOutputLen is the max length for tool output before compression
	MaxToolOutputLen int
	// ForgetFailedAfterSuccess removes failed attempts after task succeeds
	ForgetFailedAfterSuccess bool
	// CompactSuccessfulTools summarizes successful tool outputs
	CompactSuccessfulTools bool
}

// DefaultHygieneConfig returns sensible defaults
func DefaultHygieneConfig() HygieneConfig {
	return HygieneConfig{
		MaxToolOutputLen:         500,
		ForgetFailedAfterSuccess: true,
		CompactSuccessfulTools:   true,
	}
}

// Hygiene handles message cleanup and compression
type Hygiene struct {
	config HygieneConfig
}

// NewHygiene creates a new hygiene handler
func NewHygiene(config HygieneConfig) *Hygiene {
	return &Hygiene{config: config}
}

// CleanMessages processes messages to reduce token usage
// This is the "forgetting" mechanism
func (h *Hygiene) CleanMessages(messages []domain.Message, taskSolved bool) []domain.Message {
	result := make([]domain.Message, 0, len(messages))

	for _, msg := range messages {
		cleaned := h.cleanMessage(msg, taskSolved)
		if cleaned != nil {
			result = append(result, *cleaned)
		}
	}

	return result
}

// cleanMessage processes a single message
func (h *Hygiene) cleanMessage(msg domain.Message, taskSolved bool) *domain.Message {
	newParts := make([]domain.Part, 0, len(msg.Parts))

	for _, part := range msg.Parts {
		switch p := part.(type) {
		case domain.ToolCallPart:
			cleaned := h.cleanToolPart(p, taskSolved)
			if cleaned != nil {
				newParts = append(newParts, *cleaned)
			}
		case domain.TextPart:
			// Keep text parts as-is (user prompts, assistant responses)
			newParts = append(newParts, p)
		default:
			newParts = append(newParts, part)
		}
	}

	if len(newParts) == 0 {
		return nil
	}

	cleaned := msg
	cleaned.Parts = newParts
	return &cleaned
}

// cleanToolPart compresses or removes tool call parts
func (h *Hygiene) cleanToolPart(tc domain.ToolCallPart, taskSolved bool) *domain.ToolCallPart {
	// Rule: If task is solved and this was a failed attempt, forget it
	if h.config.ForgetFailedAfterSuccess && taskSolved && tc.Error != "" {
		return nil
	}

	// Rule: Compress successful tool outputs
	if h.config.CompactSuccessfulTools && tc.Error == "" {
		tc = h.compressToolOutput(tc)
	}

	return &tc
}

// compressToolOutput reduces the size of tool output
func (h *Hygiene) compressToolOutput(tc domain.ToolCallPart) domain.ToolCallPart {
	if len(tc.Result) <= h.config.MaxToolOutputLen {
		return tc
	}

	// Summarize based on tool type
	switch tc.Name {
	case "bash", "execute", "shell":
		tc.Result = h.compressBashOutput(tc.Result)
	case "read", "cat":
		tc.Result = h.compressFileRead(tc.Result)
	case "grep", "search":
		tc.Result = h.compressSearchOutput(tc.Result)
	case "write", "edit":
		tc.Result = "✅ File modified successfully"
	default:
		// Generic truncation
		tc.Result = tc.Result[:h.config.MaxToolOutputLen-50] + "\n... [truncated]"
	}

	return tc
}

// compressBashOutput summarizes bash command output
func (h *Hygiene) compressBashOutput(output string) string {
	lines := strings.Split(output, "\n")

	// For short output, keep as-is
	if len(lines) <= 10 {
		return output
	}

	// Check for common patterns
	if strings.Contains(output, "PASS") && strings.Contains(output, "ok") {
		// Test output - summarize
		passCount := strings.Count(output, "PASS")
		return "✅ Tests passed: " + countSummary(passCount, "test")
	}

	if strings.Contains(output, "Successfully") {
		// Build/install output
		return "✅ Command completed successfully"
	}

	// Keep first and last lines with truncation indicator
	var b strings.Builder
	for i := 0; i < 5 && i < len(lines); i++ {
		b.WriteString(lines[i] + "\n")
	}
	b.WriteString("... [" + countSummary(len(lines)-10, "line") + " truncated]\n")
	for i := len(lines) - 5; i < len(lines); i++ {
		if i >= 0 {
			b.WriteString(lines[i] + "\n")
		}
	}

	return b.String()
}

// compressFileRead summarizes file read output
func (h *Hygiene) compressFileRead(output string) string {
	lines := strings.Split(output, "\n")
	if len(lines) <= 20 {
		return output
	}

	return "📄 File content: " + countSummary(len(lines), "line") + "\n[Content available in context]"
}

// compressSearchOutput summarizes grep/search output
func (h *Hygiene) compressSearchOutput(output string) string {
	lines := strings.Split(output, "\n")
	if len(lines) <= 15 {
		return output
	}

	return "🔍 Found " + countSummary(len(lines), "match") + "\n[Top results shown in context]"
}

// countSummary formats a count summary
func countSummary(count int, unit string) string {
	if count == 1 {
		return "1 " + unit
	}
	return strings.TrimSpace(strings.ReplaceAll("%d "+unit+"s", "%d", string(rune('0'+count%10))))
}

// EstimateTokenSavings estimates tokens saved by cleaning
func (h *Hygiene) EstimateTokenSavings(original, cleaned []domain.Message) int {
	originalTokens := estimateTokens(original)
	cleanedTokens := estimateTokens(cleaned)
	return originalTokens - cleanedTokens
}

// estimateTokens gives a rough token count
func estimateTokens(messages []domain.Message) int {
	total := 0
	for _, msg := range messages {
		for _, part := range msg.Parts {
			switch p := part.(type) {
			case domain.TextPart:
				total += len(p.Text) / 4
			case domain.ToolCallPart:
				total += len(p.Result) / 4
			}
		}
	}
	return total
}
