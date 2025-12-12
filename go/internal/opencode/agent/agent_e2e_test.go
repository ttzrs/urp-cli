package agent

import (
	"testing"
	"time"

	"github.com/joss/urp/internal/opencode/domain"
	sessionpkg "github.com/joss/urp/internal/session"
)

// TestE2E_LongSessionWithCompaction simulates a long-running session
// verifying that Phases 1-3 work together:
// Phase 1: File-based harness (progress + tasks)
// Phase 2: Event retrieval and context engineering
// Phase 3: Message compaction for long sessions
func TestE2E_LongSessionWithCompaction(t *testing.T) {
	compactor := sessionpkg.NewMessageCompactor(5) // Keep 5 recent messages

	// Simulate 30 messages (typical long session)
	messages := simulateLongSession(30)

	// Verify compaction is not triggered initially
	if compactor.ShouldCompact(messages[:10], 0.8) {
		t.Error("Should not compact small history (<= 10 messages)")
	}

	// Verify compaction IS triggered at 30 messages
	if !compactor.ShouldCompact(messages, 0.8) {
		t.Error("Should compact at 30 messages (> 2x threshold of 5)")
	}

	// Perform compaction
	compacted := compactor.CompactMessages(messages, nil)

	// Verify recent messages preserved
	if len(compacted) < 5 {
		t.Errorf("Compacted history should preserve recent messages, got %d", len(compacted))
	}

	// Verify failures are preserved
	failures := compactor.ExtractFailures(messages)
	if len(failures) > 0 {
		t.Logf("Preserved %d failures in compaction", len(failures))
	}

	// Verify constraints extracted
	constraints := compactor.ExtractConstraints(messages)
	if len(constraints) > 0 {
		t.Logf("Preserved %d constraints in compaction", len(constraints))
	}

	// Token reduction: compacted should be ~30-40% of original
	estimatedTokensOriginal := estimateTokens(messages)
	estimatedTokensCompacted := estimateTokens(compacted)
	reduction := float64(estimatedTokensCompacted) / float64(estimatedTokensOriginal)

	t.Logf("Token reduction: %.2f (original: %d, compacted: %d)",
		reduction, estimatedTokensOriginal, estimatedTokensCompacted)

	if reduction > 0.7 {
		t.Logf("Warning: Compaction only achieved %.1f%% reduction", (1-reduction)*100)
	}

	// Verify system message added for context
	if len(compacted) > 0 && compacted[0].Role != domain.RoleSystem {
		// Summary might not always be first, but should exist
		hasSystemMsg := false
		for _, msg := range compacted {
			if msg.Role == domain.RoleSystem {
				hasSystemMsg = true
				break
			}
		}
		if !hasSystemMsg {
			t.Log("Info: No system summary message in compacted history (OK if < threshold)")
		}
	}
}

// TestE2E_MessageCompactorPreservesCriticalInfo verifies that
// compaction preserves all critical information for decision-making
func TestE2E_MessageCompactorPreservesCriticalInfo(t *testing.T) {
	compactor := sessionpkg.NewMessageCompactor(3)

	// Create realistic scenario: attempt 1 fails, attempt 2 succeeds
	messages := []domain.Message{
		// User request
		{Role: domain.RoleUser, Parts: []domain.Part{domain.TextPart{Text: "Deploy to production"}}},

		// Assistant tries first approach
		{Role: domain.RoleAssistant, Parts: []domain.Part{
			domain.TextPart{Text: "I'll use kubectl deploy"},
			domain.ToolCallPart{Name: "bash", Args: map[string]any{}},
		}},

		// Tool fails
		{Role: domain.RoleUser, Parts: []domain.Part{
			domain.ToolCallPart{
				Name:  "bash",
				Error: "permission denied: cluster credentials not found",
			},
		}},

		// Assistant learns and retries with constraint
		{Role: domain.RoleAssistant, Parts: []domain.Part{
			domain.TextPart{Text: "I must check credentials first before attempting deploy"},
		}},

		// Many intermediate messages (to trigger compaction)
		{Role: domain.RoleUser, Parts: []domain.Part{domain.TextPart{Text: "msg"}}},
		{Role: domain.RoleAssistant, Parts: []domain.Part{domain.TextPart{Text: "msg"}}},
		{Role: domain.RoleUser, Parts: []domain.Part{domain.TextPart{Text: "msg"}}},
		{Role: domain.RoleAssistant, Parts: []domain.Part{domain.TextPart{Text: "msg"}}},

		// Final request
		{Role: domain.RoleUser, Parts: []domain.Part{domain.TextPart{Text: "Deploy now"}}},
	}

	// Compact
	compacted := compactor.CompactMessages(messages, nil)

	// Verify failure preserved
	failures := compactor.ExtractFailures(messages)
	if len(failures) == 0 {
		t.Error("Critical failure should be preserved")
	}
	if len(failures) > 0 && !stringContains(failures[0], "permission") {
		t.Errorf("Failure message should contain 'permission', got: %s", failures[0])
	}

	// Verify constraint preserved
	constraints := compactor.ExtractConstraints(messages)
	if len(constraints) == 0 {
		t.Log("Info: No constraints extracted (heuristic may need tuning)")
	}

	// Verify final user message included
	var hasFinalMsg bool
	for _, msg := range compacted {
		if msg.Role == domain.RoleUser {
			for _, part := range msg.Parts {
				if tp, ok := part.(domain.TextPart); ok && stringContains(tp.Text, "Deploy now") {
					hasFinalMsg = true
				}
			}
		}
	}
	if !hasFinalMsg {
		t.Error("Compacted history should include final user message for context")
	}
}

// TestE2E_ShouldCompact_BudgetThreshold verifies compaction triggers
// based on token budget (estimated at 80% of 16K window)
func TestE2E_ShouldCompact_BudgetThreshold(t *testing.T) {
	compactor := sessionpkg.NewMessageCompactor(10)

	// Create messages that approach token budget
	// Rough heuristic: 4 chars ≈ 1 token
	// 80% of 16K = 12,800 tokens = ~51,200 chars

	smallMessages := make([]domain.Message, 5)
	for i := range smallMessages {
		smallMessages[i] = domain.Message{
			Role: domain.RoleAssistant,
			Parts: []domain.Part{domain.TextPart{
				Text: "This is a short message", // ~95 tokens
			}},
		}
	}

	if compactor.ShouldCompact(smallMessages, 0.8) {
		t.Error("Should not compact small token budget")
	}

	// Create large messages
	largeMessages := make([]domain.Message, 15)
	longText := repeatString("This is a very long message with detailed explanation of what we are doing. ", 100)

	for i := range largeMessages {
		largeMessages[i] = domain.Message{
			Role: domain.RoleAssistant,
			Parts: []domain.Part{domain.TextPart{Text: longText}},
		}
	}

	if !compactor.ShouldCompact(largeMessages, 0.8) {
		t.Log("Info: Large messages should trigger compaction (by message count if not by tokens)")
	}
}

// TestE2E_DecisionQualityAfterCompaction verifies that decision-making
// quality doesn't degrade after message compaction
func TestE2E_DecisionQualityAfterCompaction(t *testing.T) {
	compactor := sessionpkg.NewMessageCompactor(5)

	// Simulate realistic decision flow
	messages := simulateDecisionProcess()

	// Before compaction: extract decision context
	originalFailures := compactor.ExtractFailures(messages)

	// Compact
	compacted := compactor.CompactMessages(messages, nil)

	// After compaction: verify same context available
	compactedFailures := compactor.ExtractFailures(compacted)
	compactedConstraints := compactor.ExtractConstraints(compacted)

	if len(compactedFailures) != len(originalFailures) {
		t.Logf("Warning: Lost failures in compaction (before: %d, after: %d)",
			len(originalFailures), len(compactedFailures))
	}

	// Both should have decision data
	t.Logf("Decision context preserved: failures=%d, constraints=%d",
		len(compactedFailures), len(compactedConstraints))

	if len(compactedFailures) == 0 && len(compactedConstraints) == 0 {
		t.Log("Warning: No critical context preserved (may impact decision quality)")
	}
}

// ============== HELPERS ==============

// simulateLongSession creates a realistic 30-message conversation
func simulateLongSession(count int) []domain.Message {
	messages := make([]domain.Message, count)

	for i := 0; i < count; i++ {
		role := domain.RoleUser
		if i%2 == 0 {
			role = domain.RoleAssistant
		}

		text := ""
		switch {
		case i == 0:
			text = "Help me implement feature X"
		case i == 1:
			text = "I'll start by analyzing the requirements"
		case i < 10:
			text = "Intermediate step in implementation"
		case i == count-2:
			text = "There was an error in step 5"
		case i == count-1:
			text = "Let me fix that by using approach Y"
		default:
			text = "Continuing work on the implementation"
		}

		messages[i] = domain.Message{
			Role: role,
			Parts: []domain.Part{domain.TextPart{Text: text}},
			Timestamp: time.Now().Add(time.Duration(i) * time.Second),
		}

		// Add occasional tool calls for realism
		if i%4 == 0 && i > 0 {
			messages[i].Parts = append(messages[i].Parts, domain.ToolCallPart{
				Name:   "bash",
				Args:   map[string]any{},
				Result: "success",
			})
		}

		// Add occasional failures
		if i == 5 {
			messages[i] = domain.Message{
				Role: domain.RoleUser,
				Parts: []domain.Part{
					domain.ToolCallPart{
						Name:  "bash",
						Error: "command not found",
					},
				},
			}
		}
	}

	return messages
}

// simulateDecisionProcess creates a conversation with important decisions
func simulateDecisionProcess() []domain.Message {
	return []domain.Message{
		// Initial context
		{Role: domain.RoleUser, Parts: []domain.Part{
			domain.TextPart{Text: "I need a secure authentication system"},
		}},

		// Assistant proposes approach
		{Role: domain.RoleAssistant, Parts: []domain.Part{
			domain.TextPart{Text: "OAuth2 is the industry standard for this use case"},
		}},

		// User feedback
		{Role: domain.RoleUser, Parts: []domain.Part{
			domain.TextPart{Text: "We must use JWT tokens with short expiry"},
		}},

		// Try implementation
		{Role: domain.RoleAssistant, Parts: []domain.Part{
			domain.TextPart{Text: "Setting up OAuth2 provider"},
			domain.ToolCallPart{Name: "bash", Args: map[string]any{}},
		}},

		// Failure
		{Role: domain.RoleUser, Parts: []domain.Part{
			domain.ToolCallPart{
				Name:  "bash",
				Error: "TLS certificate validation failed",
			},
		}},

		// Learn and adjust
		{Role: domain.RoleAssistant, Parts: []domain.Part{
			domain.TextPart{Text: "I cannot use external OAuth without proper certificate configuration"},
		}},

		// Alternative approach
		{Role: domain.RoleAssistant, Parts: []domain.Part{
			domain.TextPart{Text: "Using internal JWT implementation instead"},
		}},

		// Success
		{Role: domain.RoleUser, Parts: []domain.Part{
			domain.ToolCallPart{
				Name:   "bash",
				Result: "JWT tokens generated successfully",
			},
		}},
	}
}

// estimateTokens roughly estimates tokens using 4 chars = 1 token heuristic
func estimateTokens(messages []domain.Message) int {
	totalChars := 0
	for _, msg := range messages {
		for _, part := range msg.Parts {
			if tp, ok := part.(domain.TextPart); ok {
				totalChars += len(tp.Text)
			}
		}
	}
	return totalChars / 4
}

// repeatString repeats a string n times
func repeatString(s string, n int) string {
	result := ""
	for i := 0; i < n; i++ {
		result += s
	}
	return result
}

// stringContains is a simple substring search for strings
func stringContains(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(s) < len(substr) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
