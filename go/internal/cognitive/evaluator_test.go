package cognitive

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestParseFixProposal(t *testing.T) {
	tests := []struct {
		name     string
		response string
		want     *FixProposal
	}{
		{
			name: "complete response",
			response: `ROOT CAUSE: The file path validation is missing before attempting to open the file.
PROPOSAL: Add os.Stat() check before opening files to validate existence.
FILES: internal/ingest/ingester.go, internal/ingest/parser.go
CONFIDENCE: high`,
			want: &FixProposal{
				Analysis:   "The file path validation is missing before attempting to open the file.",
				Proposal:   "Add os.Stat() check before opening files to validate existence.",
				Files:      []string{"internal/ingest/ingester.go", "internal/ingest/parser.go"},
				Confidence: "high",
			},
		},
		{
			name: "multiline analysis",
			response: `ROOT CAUSE: The error occurs because the function
does not handle null inputs properly.
PROPOSAL: Add nil check at the start.
FILES: main.go
CONFIDENCE: medium`,
			want: &FixProposal{
				Analysis:   "The error occurs because the function does not handle null inputs properly.",
				Proposal:   "Add nil check at the start.",
				Files:      []string{"main.go"},
				Confidence: "medium",
			},
		},
		{
			name: "missing confidence",
			response: `ROOT CAUSE: Buffer overflow in parsing.
PROPOSAL: Use safe parsing function.
FILES: parser.go`,
			want: &FixProposal{
				Analysis:   "Buffer overflow in parsing.",
				Proposal:   "Use safe parsing function.",
				Files:      []string{"parser.go"},
				Confidence: "medium", // default
			},
		},
		{
			name:     "empty response",
			response: "",
			want: &FixProposal{
				Confidence: "medium",
			},
		},
		{
			name: "no files",
			response: `ROOT CAUSE: Configuration error.
PROPOSAL: Update config file.
CONFIDENCE: low`,
			want: &FixProposal{
				Analysis:   "Configuration error.",
				Proposal:   "Update config file.",
				Files:      nil,
				Confidence: "low",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseFixProposal(tt.response)
			assert.Equal(t, tt.want.Analysis, got.Analysis)
			assert.Equal(t, tt.want.Proposal, got.Proposal)
			assert.Equal(t, tt.want.Files, got.Files)
			assert.Equal(t, tt.want.Confidence, got.Confidence)
		})
	}
}

func TestErrorContext(t *testing.T) {
	ctx := ErrorContext{
		ErrorMessage: "connection refused",
		ErrorCount:   5,
		Category:     "network",
		Operation:    "connect",
		Files: []OptimizedFile{
			{Path: "internal/client/client.go", Energy: 0.95},
			{Path: "internal/client/pool.go", Energy: 0.80},
		},
		Timestamp: time.Now(),
	}

	assert.Equal(t, "connection refused", ctx.ErrorMessage)
	assert.Equal(t, 5, ctx.ErrorCount)
	assert.Len(t, ctx.Files, 2)
	assert.Equal(t, "internal/client/client.go", ctx.Files[0].Path)
}

func TestNewEvaluator(t *testing.T) {
	// Test with nil provider (should still create evaluator)
	e := NewEvaluator(nil)
	assert.NotNil(t, e)
	assert.Equal(t, "claude-sonnet-4-5-20250929", e.model)
}

func TestNewEvaluatorWithModel(t *testing.T) {
	e := NewEvaluator(nil, WithModel("gpt-4"))
	assert.Equal(t, "gpt-4", e.model)
}

func TestProposeFixPrompt(t *testing.T) {
	// Just verify the prompt is reasonable
	assert.Contains(t, ProposeFixPrompt, "ROOT CAUSE")
	assert.Contains(t, ProposeFixPrompt, "PROPOSAL")
	assert.Contains(t, ProposeFixPrompt, "FILES")
	assert.Contains(t, ProposeFixPrompt, "CONFIDENCE")
}
