// Package agent provides soft scoring for iterative refinement
// Inspired by Poetiq's soft_score for partial credit
package agent

import (
	"regexp"
	"strings"
)

// SoftScorer provides partial credit scoring for task results
// Allows ranking even completely failed solutions (Poetiq insight)
type SoftScorer interface {
	// Score returns a value from 0.0 (worst) to 1.0 (best)
	Score(expected, actual any) float64

	// BuildFeedback creates structured feedback for the LLM
	BuildFeedback(expected, actual any) string
}

// TestOutputScorer scores test output based on pass/fail ratio
// Similar to Poetiq's pixel-level accuracy but for test results
type TestOutputScorer struct {
	PassPatterns []string
	FailPatterns []string
}

// NewTestOutputScorer creates a scorer with common test patterns
func NewTestOutputScorer() *TestOutputScorer {
	return &TestOutputScorer{
		PassPatterns: []string{
			"PASS",
			"ok ",
			"passed",
			"✓",
			"SUCCESS",
		},
		FailPatterns: []string{
			"FAIL",
			"--- FAIL:",
			"failed",
			"panic:",
			"error:",
			"Error:",
			"✗",
			"FAILED",
		},
	}
}

// Score calculates the pass ratio from test output
func (s *TestOutputScorer) Score(expected, actual any) float64 {
	output, ok := actual.(string)
	if !ok {
		return 0.0
	}

	lines := strings.Split(output, "\n")
	if len(lines) == 0 {
		return 0.0
	}

	passCount := 0
	failCount := 0

	for _, line := range lines {
		for _, pattern := range s.PassPatterns {
			if strings.Contains(line, pattern) {
				passCount++
				break
			}
		}
		for _, pattern := range s.FailPatterns {
			if strings.Contains(line, pattern) {
				failCount++
				break
			}
		}
	}

	total := passCount + failCount
	if total == 0 {
		// No recognizable test results - give minimal credit
		return 0.1
	}

	return float64(passCount) / float64(total)
}

// BuildFeedback creates detailed feedback for test failures
func (s *TestOutputScorer) BuildFeedback(expected, actual any) string {
	output, ok := actual.(string)
	if !ok {
		return "No output available"
	}

	var b strings.Builder
	lines := strings.Split(output, "\n")

	// Extract failed test names
	failedTests := make([]string, 0)
	failPattern := regexp.MustCompile(`--- FAIL: (\S+)`)
	for _, line := range lines {
		if matches := failPattern.FindStringSubmatch(line); len(matches) > 1 {
			failedTests = append(failedTests, matches[1])
		}
	}

	if len(failedTests) > 0 {
		b.WriteString("Failed tests:\n")
		for _, t := range failedTests[:min(10, len(failedTests))] {
			b.WriteString("  - " + t + "\n")
		}
		if len(failedTests) > 10 {
			b.WriteString("  ... and more\n")
		}
		b.WriteString("\n")
	}

	// Extract error messages
	errorPattern := regexp.MustCompile(`(?i)error[:\s]+(.+)`)
	errors := make([]string, 0)
	for _, line := range lines {
		if matches := errorPattern.FindStringSubmatch(line); len(matches) > 1 {
			errors = append(errors, strings.TrimSpace(matches[1]))
		}
	}

	if len(errors) > 0 {
		b.WriteString("Error messages:\n")
		for _, e := range errors[:min(5, len(errors))] {
			b.WriteString("  - " + truncateOutput(e, 100) + "\n")
		}
		b.WriteString("\n")
	}

	return b.String()
}

// ExitCodeScorer scores based on command exit code
type ExitCodeScorer struct{}

// NewExitCodeScorer creates an exit code scorer
func NewExitCodeScorer() *ExitCodeScorer {
	return &ExitCodeScorer{}
}

// Score returns 1.0 for success (exit 0), 0.0 for failure
func (s *ExitCodeScorer) Score(expected, actual any) float64 {
	exitCode, ok := actual.(int)
	if !ok {
		// Try string parsing
		if str, ok := actual.(string); ok {
			if strings.Contains(str, "exit status 0") || strings.Contains(str, "succeeded") {
				return 1.0
			}
			if strings.Contains(str, "exit status") {
				return 0.0
			}
		}
		return 0.5 // Unknown
	}

	if exitCode == 0 {
		return 1.0
	}
	return 0.0
}

// BuildFeedback creates feedback for exit code results
func (s *ExitCodeScorer) BuildFeedback(expected, actual any) string {
	exitCode, ok := actual.(int)
	if !ok {
		return "Exit code unknown"
	}

	if exitCode == 0 {
		return "Command succeeded (exit 0)"
	}
	return "Command failed with exit code: " + string(rune(exitCode))
}

// DiffScorer scores based on text similarity (line-level)
type DiffScorer struct{}

// NewDiffScorer creates a diff-based scorer
func NewDiffScorer() *DiffScorer {
	return &DiffScorer{}
}

// Score calculates similarity between expected and actual output
func (s *DiffScorer) Score(expected, actual any) float64 {
	expStr, ok1 := expected.(string)
	actStr, ok2 := actual.(string)
	if !ok1 || !ok2 {
		return 0.0
	}

	expLines := strings.Split(expStr, "\n")
	actLines := strings.Split(actStr, "\n")

	if len(expLines) == 0 && len(actLines) == 0 {
		return 1.0 // Both empty = match
	}

	// Count matching lines (simplified LCS-style comparison)
	matchCount := 0
	actSet := make(map[string]bool)
	for _, line := range actLines {
		actSet[strings.TrimSpace(line)] = true
	}

	for _, line := range expLines {
		if actSet[strings.TrimSpace(line)] {
			matchCount++
		}
	}

	return float64(matchCount) / float64(max(len(expLines), len(actLines)))
}

// BuildFeedback creates a visual diff between expected and actual
func (s *DiffScorer) BuildFeedback(expected, actual any) string {
	expStr, ok1 := expected.(string)
	actStr, ok2 := actual.(string)
	if !ok1 || !ok2 {
		return "Cannot generate diff"
	}

	var b strings.Builder
	b.WriteString("Diff (expected vs actual):\n")

	expLines := strings.Split(expStr, "\n")
	actLines := strings.Split(actStr, "\n")

	maxLines := max(len(expLines), len(actLines))
	if maxLines > 20 {
		maxLines = 20
	}

	for i := 0; i < maxLines; i++ {
		expLine := ""
		actLine := ""
		if i < len(expLines) {
			expLine = expLines[i]
		}
		if i < len(actLines) {
			actLine = actLines[i]
		}

		if expLine == actLine {
			b.WriteString("  " + expLine + "\n")
		} else {
			if expLine != "" {
				b.WriteString("- " + expLine + "\n")
			}
			if actLine != "" {
				b.WriteString("+ " + actLine + "\n")
			}
		}
	}

	return b.String()
}

// CompositeScorer combines multiple scorers with weights
type CompositeScorer struct {
	scorers []weightedScorer
}

type weightedScorer struct {
	scorer SoftScorer
	weight float64
}

// NewCompositeScorer creates a scorer that combines multiple scorers
func NewCompositeScorer() *CompositeScorer {
	return &CompositeScorer{
		scorers: make([]weightedScorer, 0),
	}
}

// Add adds a scorer with a weight
func (c *CompositeScorer) Add(scorer SoftScorer, weight float64) *CompositeScorer {
	c.scorers = append(c.scorers, weightedScorer{scorer: scorer, weight: weight})
	return c
}

// Score returns weighted average of all scorers
func (c *CompositeScorer) Score(expected, actual any) float64 {
	if len(c.scorers) == 0 {
		return 0.0
	}

	totalWeight := 0.0
	totalScore := 0.0

	for _, ws := range c.scorers {
		score := ws.scorer.Score(expected, actual)
		totalScore += score * ws.weight
		totalWeight += ws.weight
	}

	if totalWeight == 0 {
		return 0.0
	}

	return totalScore / totalWeight
}

// BuildFeedback combines feedback from all scorers
func (c *CompositeScorer) BuildFeedback(expected, actual any) string {
	var b strings.Builder
	for _, ws := range c.scorers {
		fb := ws.scorer.BuildFeedback(expected, actual)
		if fb != "" {
			b.WriteString(fb)
			b.WriteString("\n")
		}
	}
	return b.String()
}

// max returns the maximum of two ints
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
