package cognitive

import (
	"testing"

	"github.com/joss/urp/internal/opencode/domain"
)

func TestHygiene_CleanMessages(t *testing.T) {
	h := NewHygiene(DefaultHygieneConfig())

	messages := []domain.Message{
		{
			ID:   "1",
			Role: domain.RoleUser,
			Parts: []domain.Part{
				domain.TextPart{Text: "Run the tests"},
			},
		},
		{
			ID:   "2",
			Role: domain.RoleAssistant,
			Parts: []domain.Part{
				domain.ToolCallPart{
					Name:   "bash",
					Result: "FAIL: TestFoo\nError: expected 1, got 2",
					Error:  "exit status 1",
				},
			},
		},
		{
			ID:   "3",
			Role: domain.RoleAssistant,
			Parts: []domain.Part{
				domain.ToolCallPart{
					Name:   "bash",
					Result: "PASS: TestFoo\nok",
					Error:  "",
				},
			},
		},
	}

	// Task solved - should forget failed attempts
	cleaned := h.CleanMessages(messages, true)

	// User message should remain
	if len(cleaned) < 1 {
		t.Fatal("should have at least user message")
	}

	// Failed tool call should be removed when task solved
	for _, msg := range cleaned {
		for _, part := range msg.Parts {
			if tc, ok := part.(domain.ToolCallPart); ok {
				if tc.Error != "" {
					t.Error("failed tool call should be removed when task solved")
				}
			}
		}
	}
}

func TestHygiene_CompressToolOutput(t *testing.T) {
	h := NewHygiene(HygieneConfig{
		MaxToolOutputLen:       100,
		CompactSuccessfulTools: true,
	})

	longOutput := ""
	for i := 0; i < 100; i++ {
		longOutput += "line of output\n"
	}

	tc := domain.ToolCallPart{
		Name:   "bash",
		Result: longOutput,
		Error:  "",
	}

	compressed := h.compressToolOutput(tc)

	if len(compressed.Result) >= len(longOutput) {
		t.Error("output should be compressed")
	}
}

func TestHygiene_CompressBashOutput(t *testing.T) {
	h := NewHygiene(DefaultHygieneConfig())

	// Test pass output
	testOutput := `=== RUN   TestFoo
--- PASS: TestFoo (0.00s)
=== RUN   TestBar
--- PASS: TestBar (0.00s)
PASS
ok  	github.com/example/pkg	0.002s`

	compressed := h.compressBashOutput(testOutput)
	if !containsSubstr(compressed, "PASS") && !containsSubstr(compressed, "Tests passed") {
		t.Errorf("test output should summarize passes, got: %s", compressed)
	}
}

func TestEstimateTokenSavings(t *testing.T) {
	h := NewHygiene(DefaultHygieneConfig())

	original := []domain.Message{
		{
			Parts: []domain.Part{
				domain.TextPart{Text: "short"},
				domain.ToolCallPart{Result: "very long output that will be compressed and forgotten"},
			},
		},
	}

	cleaned := []domain.Message{
		{
			Parts: []domain.Part{
				domain.TextPart{Text: "short"},
			},
		},
	}

	savings := h.EstimateTokenSavings(original, cleaned)
	if savings <= 0 {
		t.Error("should have positive token savings")
	}
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
