package agent

import (
	"fmt"
	"sort"
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

// ScoredAttempt represents a previous attempt with its quality score
// Inspired by Poetiq's ARCAGISolution with soft scoring
type ScoredAttempt struct {
	Code     string  // The code/command that was attempted
	Output   string  // Raw output
	Error    string  // Error message if any
	Score    float64 // Soft score 0.0 (worst) to 1.0 (best)
	Feedback string  // Structured feedback for LLM
}

// Autocorrector handles failure detection and retry logic
// Enhanced with Poetiq-style structured feedback and soft scoring
type Autocorrector struct {
	config     AutocorrectionConfig
	retryCount int

	// Poetiq-inspired: track previous attempts for iterative refinement
	attempts       []ScoredAttempt
	lastError      string
	lastOutput     string
	improvingOrder bool // Show best attempts last (recency bias for LLM)
}

// NewAutocorrector creates an autocorrector with default config
func NewAutocorrector() *Autocorrector {
	return &Autocorrector{
		config:         DefaultAutocorrection(),
		attempts:       make([]ScoredAttempt, 0),
		improvingOrder: true, // Best attempts last for LLM recency bias
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

// ResetRetries resets retry counter and clears attempt history
func (a *Autocorrector) ResetRetries() {
	a.retryCount = 0
	a.attempts = make([]ScoredAttempt, 0)
	a.lastError = ""
	a.lastOutput = ""
}

// RecordAttempt stores a scored attempt for feedback (Poetiq-style)
func (a *Autocorrector) RecordAttempt(code, output, errMsg string, score float64) {
	feedback := a.buildStructuredFeedback(output, errMsg, score)
	attempt := ScoredAttempt{
		Code:     code,
		Output:   output,
		Error:    errMsg,
		Score:    score,
		Feedback: feedback,
	}
	a.attempts = append(a.attempts, attempt)
	a.lastError = errMsg
	a.lastOutput = output
}

// BestAttempt returns the highest-scored attempt, or nil if none
func (a *Autocorrector) BestAttempt() *ScoredAttempt {
	if len(a.attempts) == 0 {
		return nil
	}
	best := &a.attempts[0]
	for i := range a.attempts {
		if a.attempts[i].Score > best.Score {
			best = &a.attempts[i]
		}
	}
	return best
}

// Attempts returns all recorded attempts
func (a *Autocorrector) Attempts() []ScoredAttempt {
	return a.attempts
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
// Enhanced with Poetiq-style structured feedback showing previous attempts
func (a *Autocorrector) CorrectionPrompt() string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf(`
⚠️ FAILURE DETECTED - AUTOCORRECTION ATTEMPT %d/%d

`, a.retryCount, a.config.MaxRetries))

	// Include previous attempts with structured feedback (Poetiq-style)
	if len(a.attempts) > 0 {
		b.WriteString("**EXISTING PARTIAL/INCORRECT SOLUTIONS:**\n\n")
		b.WriteString("Following are some of the best, though not completely correct, solutions so far.\n")
		b.WriteString("Study these solutions and corresponding feedback and produce a new solution fixing all issues.\n\n")

		// Sort by score and optionally reorder
		attempts := a.getSortedAttempts()

		for i, attempt := range attempts {
			b.WriteString(fmt.Sprintf("<attempt_%d>\n", i+1))
			if attempt.Code != "" {
				b.WriteString("<code>\n")
				b.WriteString(truncateOutput(attempt.Code, 500))
				b.WriteString("\n</code>\n")
			}
			b.WriteString("<evaluation>\n")
			b.WriteString(attempt.Feedback)
			b.WriteString("\n</evaluation>\n")
			b.WriteString(fmt.Sprintf("<score>%.2f</score>\n", attempt.Score))
			b.WriteString(fmt.Sprintf("</attempt_%d>\n\n", i+1))
		}
	} else {
		// Fallback to simple error display
		b.WriteString("The previous command failed. Please:\n")
		b.WriteString("1. Analyze the error output above\n")
		b.WriteString("2. Identify the root cause\n")
		b.WriteString("3. Fix the issue\n")
		b.WriteString("4. Run the tests again to verify\n")
	}

	b.WriteString("\nDo not give up - fix the error and retry.\n")

	return b.String()
}

// getSortedAttempts returns attempts sorted by score
// If improvingOrder is true, best attempts come last (for LLM recency bias)
func (a *Autocorrector) getSortedAttempts() []ScoredAttempt {
	if len(a.attempts) == 0 {
		return nil
	}

	sorted := make([]ScoredAttempt, len(a.attempts))
	copy(sorted, a.attempts)

	sort.Slice(sorted, func(i, j int) bool {
		if a.improvingOrder {
			return sorted[i].Score < sorted[j].Score // Worst first, best last
		}
		return sorted[i].Score > sorted[j].Score // Best first
	})

	// Only return top 3 attempts (Poetiq uses max_solutions)
	if len(sorted) > 3 {
		sorted = sorted[len(sorted)-3:]
	}

	return sorted
}

// buildStructuredFeedback creates detailed feedback for an attempt (Poetiq-style)
func (a *Autocorrector) buildStructuredFeedback(output, errMsg string, score float64) string {
	var b strings.Builder

	if errMsg != "" {
		b.WriteString(fmt.Sprintf("Error: %s\n\n", truncateOutput(errMsg, 300)))
	}

	if output != "" {
		// Extract key failure indicators
		lines := strings.Split(output, "\n")
		failLines := make([]string, 0)
		for _, line := range lines {
			for _, pattern := range a.config.Patterns {
				if strings.Contains(line, pattern) {
					failLines = append(failLines, strings.TrimSpace(line))
					break
				}
			}
		}

		if len(failLines) > 0 {
			b.WriteString("Failure indicators found:\n")
			for _, fl := range failLines[:min(5, len(failLines))] {
				b.WriteString(fmt.Sprintf("  - %s\n", truncateOutput(fl, 100)))
			}
			b.WriteString("\n")
		}

		// Show relevant output snippet
		b.WriteString("Output snippet:\n```\n")
		b.WriteString(truncateOutput(output, 500))
		b.WriteString("\n```\n")
	}

	b.WriteString(fmt.Sprintf("\nAccuracy estimate: %.2f (0 is worst, 1 is best)\n", score))

	return b.String()
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

// truncateOutput truncates long output strings
func truncateOutput(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// min returns the minimum of two ints
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
