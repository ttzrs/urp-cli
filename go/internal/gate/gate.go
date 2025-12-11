package gate

import (
	"context"
)

// GateClient defines the behavior of the "Noise Filter".
// It acts as the Sigmoid Gate, reducing thousands of logs to just the signal.
type GateClient interface {
	// FilterNoise takes raw text (logs, manual pages) and returns only the critical info.
	// Returns empty string if the input is irrelevant (Sparsity).
	FilterNoise(ctx context.Context, goal string, rawInput string) (string, error)
}
