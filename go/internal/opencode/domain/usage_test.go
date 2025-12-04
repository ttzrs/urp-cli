package domain

import "testing"

func TestSimpleTokenCounter(t *testing.T) {
	counter := &SimpleTokenCounter{}

	tests := []struct {
		text     string
		expected int
	}{
		{"", 0},
		{"a", 1},
		{"test", 1},
		{"hello world", 3},
		{"This is a longer sentence with multiple words.", 12},
	}

	for _, tt := range tests {
		got := counter.Count(tt.text)
		if got != tt.expected {
			t.Errorf("Count(%q) = %d, want %d", tt.text, got, tt.expected)
		}
	}
}

func TestCalculateCost(t *testing.T) {
	model := Model{
		ID:         "test-model",
		InputCost:  3.0,  // $3 per 1M
		OutputCost: 15.0, // $15 per 1M
	}

	usage := CalculateCost(1000, 500, model)

	if usage.InputTokens != 1000 {
		t.Errorf("InputTokens = %d, want 1000", usage.InputTokens)
	}
	if usage.OutputTokens != 500 {
		t.Errorf("OutputTokens = %d, want 500", usage.OutputTokens)
	}

	expectedInputCost := 3.0 / 1000  // 1000 tokens * $3/1M
	expectedOutputCost := 7.5 / 1000 // 500 tokens * $15/1M

	if usage.InputCost < expectedInputCost-0.0001 || usage.InputCost > expectedInputCost+0.0001 {
		t.Errorf("InputCost = %f, want %f", usage.InputCost, expectedInputCost)
	}
	if usage.OutputCost < expectedOutputCost-0.0001 || usage.OutputCost > expectedOutputCost+0.0001 {
		t.Errorf("OutputCost = %f, want %f", usage.OutputCost, expectedOutputCost)
	}
}

func TestUsageAdd(t *testing.T) {
	u1 := Usage{
		InputTokens:  100,
		OutputTokens: 50,
		InputCost:    0.001,
		OutputCost:   0.002,
		TotalCost:    0.003,
	}

	u2 := Usage{
		InputTokens:  200,
		OutputTokens: 100,
		InputCost:    0.002,
		OutputCost:   0.004,
		TotalCost:    0.006,
	}

	u1.Add(u2)

	if u1.InputTokens != 300 {
		t.Errorf("InputTokens = %d, want 300", u1.InputTokens)
	}
	if u1.OutputTokens != 150 {
		t.Errorf("OutputTokens = %d, want 150", u1.OutputTokens)
	}
	if u1.TotalCost < 0.008 || u1.TotalCost > 0.01 {
		t.Errorf("TotalCost = %f, want ~0.009", u1.TotalCost)
	}
}

func TestFormatCost(t *testing.T) {
	tests := []struct {
		cost     float64
		expected string
	}{
		{0.001, "<$0.01"},
		{0.01, "$0.01"},
		{1.234, "$1.23"},
		{99.99, "$99.99"},
	}

	for _, tt := range tests {
		got := FormatCost(tt.cost)
		if got != tt.expected {
			t.Errorf("FormatCost(%f) = %q, want %q", tt.cost, got, tt.expected)
		}
	}
}

func TestFormatTokens(t *testing.T) {
	tests := []struct {
		tokens   int
		expected string
	}{
		{0, "0"},
		{100, "100"},
		{999, "999"},
		{1000, "1.0k"},
		{1500, "1.5k"},
		{10000, "10.0k"},
	}

	for _, tt := range tests {
		got := FormatTokens(tt.tokens)
		if got != tt.expected {
			t.Errorf("FormatTokens(%d) = %q, want %q", tt.tokens, got, tt.expected)
		}
	}
}

func TestCountMessages(t *testing.T) {
	counter := &SimpleTokenCounter{}

	messages := []Message{
		{
			Role: RoleUser,
			Parts: []Part{
				TextPart{Text: "Hello, how are you?"},
			},
		},
		{
			Role: RoleAssistant,
			Parts: []Part{
				TextPart{Text: "I'm doing well, thanks!"},
				ToolCallPart{Name: "read", Result: "file content here"},
			},
		},
	}

	total := counter.CountMessages(messages)
	// Should be > 0 and reasonable
	if total < 10 {
		t.Errorf("CountMessages() = %d, expected > 10", total)
	}
}
