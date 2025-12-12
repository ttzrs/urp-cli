package session

import (
	"testing"

	"github.com/joss/urp/internal/opencode/domain"
)

func TestMessageCompactor_CompactMessages_SmallHistory(t *testing.T) {
	compactor := NewMessageCompactor(5)

	// Small history should not be compacted
	messages := []domain.Message{
		{Role: domain.RoleUser, Parts: []domain.Part{domain.TextPart{Text: "msg1"}}},
		{Role: domain.RoleAssistant, Parts: []domain.Part{domain.TextPart{Text: "resp1"}}},
	}

	result := compactor.CompactMessages(messages, nil)
	if len(result) != 2 {
		t.Errorf("Small history should not be compacted, got %d messages", len(result))
	}
}

func TestMessageCompactor_ExtractFailures(t *testing.T) {
	compactor := NewMessageCompactor(5)

	messages := []domain.Message{
		{
			Role: domain.RoleAssistant,
			Parts: []domain.Part{
				domain.ToolCallPart{
					Name:  "bash",
					Error: "command not found",
				},
			},
		},
		{
			Role: domain.RoleAssistant,
			Parts: []domain.Part{
				domain.ToolCallPart{
					Name:   "read_file",
					Result: "success",
					Error:  "",
				},
			},
		},
	}

	failures := compactor.ExtractFailures(messages)
	if len(failures) != 1 {
		t.Errorf("Expected 1 failure, got %d", len(failures))
	}
	if len(failures) > 0 && !contains(failures[0], "bash") {
		t.Errorf("Failure should mention 'bash' tool, got: %s", failures[0])
	}
}

func TestMessageCompactor_ExtractConstraints(t *testing.T) {
	compactor := NewMessageCompactor(5)

	messages := []domain.Message{
		{
			Role: domain.RoleAssistant,
			Parts: []domain.Part{
				domain.TextPart{Text: "You must use Python 3.9 or higher for this project."},
			},
		},
		{
			Role: domain.RoleAssistant,
			Parts: []domain.Part{
				domain.TextPart{Text: "The API cannot be modified without approval."},
			},
		},
	}

	constraints := compactor.ExtractConstraints(messages)
	if len(constraints) < 1 {
		t.Errorf("Expected at least 1 constraint, got %d", len(constraints))
	}
}

func TestMessageCompactor_ShouldCompact_BelowThreshold(t *testing.T) {
	compactor := NewMessageCompactor(5)

	messages := []domain.Message{
		{Role: domain.RoleUser, Parts: []domain.Part{domain.TextPart{Text: "msg1"}}},
		{Role: domain.RoleAssistant, Parts: []domain.Part{domain.TextPart{Text: "resp1"}}},
	}

	if compactor.ShouldCompact(messages, 0.8) {
		t.Error("Should not compact small history")
	}
}

func TestMessageCompactor_ShouldCompact_AboveThreshold(t *testing.T) {
	compactor := NewMessageCompactor(5)

	// Create messages that exceed threshold (2x maxHistoryMessages)
	messages := make([]domain.Message, 15)
	for i := range messages {
		messages[i] = domain.Message{
			Role:  domain.RoleUser,
			Parts: []domain.Part{domain.TextPart{Text: "message"}},
		}
	}

	if !compactor.ShouldCompact(messages, 0.8) {
		t.Error("Should compact when message count exceeds threshold")
	}
}

func TestMessageCompactor_CompactMessages_PreservesRecent(t *testing.T) {
	compactor := NewMessageCompactor(3)

	// Create history with more than maxHistoryMessages
	messages := []domain.Message{
		{Role: domain.RoleUser, Parts: []domain.Part{domain.TextPart{Text: "old1"}}},
		{Role: domain.RoleAssistant, Parts: []domain.Part{domain.TextPart{Text: "old_response1"}}},
		{Role: domain.RoleUser, Parts: []domain.Part{domain.TextPart{Text: "old2"}}},
		{Role: domain.RoleAssistant, Parts: []domain.Part{domain.TextPart{Text: "old_response2"}}},
		{Role: domain.RoleUser, Parts: []domain.Part{domain.TextPart{Text: "recent1"}}},
		{Role: domain.RoleAssistant, Parts: []domain.Part{domain.TextPart{Text: "recent_response"}}},
	}

	result := compactor.CompactMessages(messages, nil)

	// Check that recent messages are preserved
	var hasRecent bool
	for _, msg := range result {
		for _, part := range msg.Parts {
			if tp, ok := part.(domain.TextPart); ok && contains(tp.Text, "recent") {
				hasRecent = true
			}
		}
	}

	if !hasRecent {
		t.Error("Compacted history should preserve recent messages")
	}
}

func TestMessageCompactor_ExtractCausalChain(t *testing.T) {
	compactor := NewMessageCompactor(5)

	messages := []domain.Message{
		{
			Role: domain.RoleAssistant,
			Parts: []domain.Part{
				domain.TextPart{Text: "I will try this approach"},
				domain.ToolCallPart{Name: "bash", Args: map[string]any{}},
			},
		},
		{
			Role: domain.RoleUser,
			Parts: []domain.Part{
				domain.TextPart{Text: "Exit code 0, success"},
			},
		},
		{
			Role: domain.RoleAssistant,
			Parts: []domain.Part{
				domain.TextPart{Text: "Great! The command worked"},
			},
		},
	}

	// extractCausalChain implementation is optional, just verify it doesn't panic
	chain := compactor.ExtractCausalChain(messages)
	// Chain extraction is a best-effort feature, not critical
	_ = chain
}

// helper
func contains(s, substr string) bool {
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
