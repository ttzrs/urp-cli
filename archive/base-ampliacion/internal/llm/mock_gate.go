package llm

import (
	"context"
	"strings"
)

// MockGate simulates the behavior of Qwen/Gemini-Flash for testing.
type MockGate struct{}

func NewMockGate() *MockGate {
	return &MockGate{}
}

func (m *MockGate) FilterNoise(ctx context.Context, goal string, rawInput string) (string, error) {
	// SIMULATION OF "SIGMOID GATE" LOGIC:
	// If the log contains "CRITICAL" or "Error", keep it.
	// Otherwise, treat as noise (return empty).
	
	if strings.Contains(rawInput, "Error") || strings.Contains(rawInput, "CRITICAL") {
		// Simulate extraction: return the line with the error
		lines := strings.Split(rawInput, "\n")
		var validLines []string
		for _, line := range lines {
			if strings.Contains(line, "Error") || strings.Contains(line, "CRITICAL") {
				validLines = append(validLines, line)
			}
		}
		return strings.Join(validLines, "\n"), nil
	}
	
	// Sparsity: Return nothing if no signal found
	return "", nil
}

// QwenGate would be the real implementation using Alibaba's API
type QwenGate struct {
	ApiKey string
}
// func (q *QwenGate) FilterNoise(...) ...
