package gate

import (
	"context"
)

// GateResult contains the filtered output plus usage metrics.
type GateResult struct {
	FilteredText string
	InputTokens  int
	OutputTokens int
	Model        string
	Filtered     bool // true if logs were filtered (not NO_SIGNAL)
}

// GateClient defines the behavior of the "Noise Filter".
// It acts as the Sigmoid Gate, reducing thousands of logs to just the signal.
type GateClient interface {
	// FilterNoise takes raw text (logs, manual pages) and returns only the critical info.
	// Returns empty string if the input is irrelevant (Sparsity).
	FilterNoise(ctx context.Context, goal string, rawInput string) (string, error)

	// FilterNoiseWithUsage is like FilterNoise but returns detailed usage info.
	FilterNoiseWithUsage(ctx context.Context, goal string, rawInput string) (*GateResult, error)

	// Model returns the configured model name.
	Model() string
}
