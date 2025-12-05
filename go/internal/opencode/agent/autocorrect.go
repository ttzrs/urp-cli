package agent

import (
	"fmt"
	"strings"

	"github.com/joss/urp/internal/opencode/domain"
)

// AutocorrectionConfig defines behavior for automatic retry on test failures
type AutocorrectionConfig struct {
	Enabled    bool     // Enable autocorrection loop
	MaxRetries int      // Maximum retry attempts (default: 3)
	Patterns   []string // Patterns that trigger retry (e.g., "FAIL", "error:", "panic:")
}

// DefaultAutocorrection returns sensible defaults
func DefaultAutocorrection() AutocorrectionConfig {
	return AutocorrectionConfig{
		Enabled:    true,
		MaxRetries: 3,
		Patterns:   []string{"FAIL", "--- FAIL:", "panic:", "error:", "Error:", "failed"},
	}
}

// Autocorrector handles failure detection and retry logic
type Autocorrector struct {
	config     AutocorrectionConfig
	retryCount int
}

// NewAutocorrector creates an autocorrector with default config
func NewAutocorrector() *Autocorrector {
	return &Autocorrector{
		config: DefaultAutocorrection(),
	}
}

// Configure sets the autocorrection config
func (a *Autocorrector) Configure(config AutocorrectionConfig) {
	a.config = config
}

// RetryCount returns current retry count
func (a *Autocorrector) RetryCount() int {
	return a.retryCount
}

// MaxRetries returns max allowed retries
func (a *Autocorrector) MaxRetries() int {
	return a.config.MaxRetries
}

// IncrementRetry increments retry counter
func (a *Autocorrector) IncrementRetry() {
	a.retryCount++
}

// ResetRetries resets retry counter
func (a *Autocorrector) ResetRetries() {
	a.retryCount = 0
}

// DetectFailure checks if any tool result contains failure patterns
// Returns (failed, reason)
func (a *Autocorrector) DetectFailure(parts []domain.Part) (bool, string) {
	if !a.config.Enabled {
		return false, ""
	}

	for _, part := range parts {
		tc, ok := part.(domain.ToolCallPart)
		if !ok {
			continue
		}

		// Check both result and error
		output := tc.Result
		if tc.Error != "" {
			output += "\n" + tc.Error
		}

		for _, pattern := range a.config.Patterns {
			if strings.Contains(output, pattern) {
				snippet := extractSnippet(output, pattern)
				return true, fmt.Sprintf("Detected '%s' in output: ...%s...", pattern, snippet)
			}
		}
	}
	return false, ""
}

// ShouldRetry checks if we should trigger autocorrection
func (a *Autocorrector) ShouldRetry() bool {
	if !a.config.Enabled {
		return false
	}
	return a.retryCount < a.config.MaxRetries
}

// CorrectionPrompt returns the instruction to add for retry
func (a *Autocorrector) CorrectionPrompt() string {
	return fmt.Sprintf(`
⚠️ FAILURE DETECTED - AUTOCORRECTION ATTEMPT %d/%d

The previous command failed. Please:
1. Analyze the error output above
2. Identify the root cause
3. Fix the issue
4. Run the tests again to verify

Do not give up - fix the error and retry.
`, a.retryCount, a.config.MaxRetries)
}

// extractSnippet extracts context around a pattern match
func extractSnippet(output, pattern string) string {
	idx := strings.Index(output, pattern)
	if idx == -1 {
		return ""
	}

	start := idx - 100
	if start < 0 {
		start = 0
	}
	end := idx + 200
	if end > len(output) {
		end = len(output)
	}
	return output[start:end]
}
