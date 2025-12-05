package tokens

import (
	"testing"

	"github.com/joss/urp/internal/opencode/domain"
)

func TestCount(t *testing.T) {
	tests := []struct {
		name string
		text string
		min  int // minimum expected tokens
		max  int // maximum expected tokens
	}{
		{"empty", "", 0, 0},
		{"hello", "hello", 1, 2},
		{"sentence", "The quick brown fox jumps over the lazy dog.", 8, 12},
		{"code", "func main() { fmt.Println(\"hello\") }", 10, 20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Count(tt.text)
			if got < tt.min || got > tt.max {
				t.Errorf("Count(%q) = %d, want between %d and %d", tt.text, got, tt.min, tt.max)
			}
		})
	}
}

func TestCountMessage(t *testing.T) {
	msg := domain.Message{
		ID:   "test",
		Role: domain.RoleUser,
		Parts: []domain.Part{
			domain.TextPart{Text: "Hello, how are you?"},
		},
	}

	tokens := CountMessage(msg)
	if tokens < 5 || tokens > 20 {
		t.Errorf("CountMessage = %d, want between 5 and 20", tokens)
	}
}

func TestCountMessages(t *testing.T) {
	msgs := []domain.Message{
		{
			ID:    "1",
			Role:  domain.RoleUser,
			Parts: []domain.Part{domain.TextPart{Text: "Hello"}},
		},
		{
			ID:    "2",
			Role:  domain.RoleAssistant,
			Parts: []domain.Part{domain.TextPart{Text: "Hi there!"}},
		},
	}

	tokens := CountMessages(msgs)
	if tokens < 10 {
		t.Errorf("CountMessages = %d, want at least 10", tokens)
	}
}

func TestShouldCompact(t *testing.T) {
	msgs := []domain.Message{
		{
			ID:    "1",
			Role:  domain.RoleUser,
			Parts: []domain.Part{domain.TextPart{Text: "Hello"}},
		},
	}

	if ShouldCompact(msgs, 100) {
		t.Error("ShouldCompact should be false for small messages")
	}

	if !ShouldCompact(msgs, 1) {
		t.Error("ShouldCompact should be true when threshold is 1")
	}
}

func TestToolCallPart(t *testing.T) {
	msg := domain.Message{
		ID:   "test",
		Role: domain.RoleAssistant,
		Parts: []domain.Part{
			domain.ToolCallPart{
				ToolID: "123",
				Name:   "read",
				Args:   map[string]any{"file_path": "/path/to/file.go"},
				Result: "file content here",
			},
		},
	}

	tokens := CountMessage(msg)
	if tokens < 10 {
		t.Errorf("CountMessage with tool call = %d, want at least 10", tokens)
	}
}
