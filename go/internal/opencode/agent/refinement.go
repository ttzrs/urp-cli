// Package agent provides Poetiq-style iterative refinement
// The LLM learns from its own mistakes within a single task
package agent

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// RefinementConfig configures the iterative refinement loop
// Inspired by Poetiq's solve_coding configuration
type RefinementConfig struct {
	MaxIterations       int     // Maximum refinement attempts (default: 5)
	SelectionProb       float64 // Probability of including each past attempt (default: 0.8)
	MaxSolutions        int     // Max past solutions to show in feedback (default: 3)
	ImprovingOrder      bool    // Show best attempts last for recency bias (default: true)
	ReturnBestOnTimeout bool    // Return best result if max iterations reached (default: true)
	SuccessThreshold    float64 // Score threshold to consider success (default: 1.0)
}

// DefaultRefinementConfig returns sensible defaults
func DefaultRefinementConfig() RefinementConfig {
	return RefinementConfig{
		MaxIterations:       5,
		SelectionProb:       0.8,
		MaxSolutions:        3,
		ImprovingOrder:      true,
		ReturnBestOnTimeout: true,
		SuccessThreshold:    1.0,
	}
}

// RefinementResult holds the outcome of a refinement loop
type RefinementResult struct {
	Success    bool
	Output     any
	Score      float64
	Iterations int
	Duration   time.Duration
	Attempts   []RefinementAttempt
	Error      error
}

// RefinementAttempt records a single attempt during refinement
type RefinementAttempt struct {
	Iteration int
	Input     string
	Output    any
	Score     float64
	Feedback  string
	Duration  time.Duration
}

// RefinementLoop manages iterative improvement with soft scoring
// Core Poetiq pattern: LLM learns from mistakes within same session
type RefinementLoop struct {
	config RefinementConfig
	scorer SoftScorer

	// State
	attempts  []RefinementAttempt
	bestScore float64
	bestIdx   int
}

// NewRefinementLoop creates a refinement loop with default config
func NewRefinementLoop(scorer SoftScorer) *RefinementLoop {
	return &RefinementLoop{
		config:    DefaultRefinementConfig(),
		scorer:    scorer,
		attempts:  make([]RefinementAttempt, 0),
		bestScore: -1,
		bestIdx:   -1,
	}
}

// WithConfig sets the refinement configuration
func (r *RefinementLoop) WithConfig(config RefinementConfig) *RefinementLoop {
	r.config = config
	return r
}

// Reset clears all state for a new refinement task
func (r *RefinementLoop) Reset() {
	r.attempts = make([]RefinementAttempt, 0)
	r.bestScore = -1
	r.bestIdx = -1
}

// RecordAttempt records an attempt and updates best tracking
func (r *RefinementLoop) RecordAttempt(input string, output any, expected any, duration time.Duration) float64 {
	score := r.scorer.Score(expected, output)
	feedback := r.scorer.BuildFeedback(expected, output)

	attempt := RefinementAttempt{
		Iteration: len(r.attempts) + 1,
		Input:     input,
		Output:    output,
		Score:     score,
		Feedback:  feedback,
		Duration:  duration,
	}
	r.attempts = append(r.attempts, attempt)

	if score > r.bestScore {
		r.bestScore = score
		r.bestIdx = len(r.attempts) - 1
	}

	return score
}

// IsSuccess checks if the latest attempt meets success threshold
func (r *RefinementLoop) IsSuccess() bool {
	if len(r.attempts) == 0 {
		return false
	}
	return r.attempts[len(r.attempts)-1].Score >= r.config.SuccessThreshold
}

// ShouldContinue checks if we should do another iteration
func (r *RefinementLoop) ShouldContinue() bool {
	if len(r.attempts) >= r.config.MaxIterations {
		return false
	}
	if len(r.attempts) > 0 && r.IsSuccess() {
		return false
	}
	return true
}

// BestAttempt returns the highest-scoring attempt
func (r *RefinementLoop) BestAttempt() *RefinementAttempt {
	if r.bestIdx < 0 || r.bestIdx >= len(r.attempts) {
		return nil
	}
	return &r.attempts[r.bestIdx]
}

// Attempts returns all recorded attempts
func (r *RefinementLoop) Attempts() []RefinementAttempt {
	return r.attempts
}

// IterationCount returns current iteration number
func (r *RefinementLoop) IterationCount() int {
	return len(r.attempts)
}

// BuildFeedbackPrompt creates a Poetiq-style feedback prompt
// Shows previous attempts sorted by score with detailed feedback
func (r *RefinementLoop) BuildFeedbackPrompt() string {
	if len(r.attempts) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("**EXISTING PARTIAL/INCORRECT SOLUTIONS:**\n\n")
	b.WriteString("Following are some of the best, though not completely correct, solutions so far.\n")
	b.WriteString("For each solution, its code, corresponding feedback, and a numeric score between 0 (worst) and 1 (best) is provided.\n")
	b.WriteString("Study these solutions and produce a new solution fixing all issues.\n\n")

	// Get selected attempts for feedback
	selected := r.selectAttempts()

	for i, attempt := range selected {
		b.WriteString(fmt.Sprintf("<solution_%d>\n", i+1))

		// Code/input
		if attempt.Input != "" {
			b.WriteString("<solution_code>\n```\n")
			b.WriteString(truncateOutput(attempt.Input, 1000))
			b.WriteString("\n```\n</solution_code>\n")
		}

		// Evaluation/feedback
		b.WriteString("<solution_evaluation>\n")
		b.WriteString(attempt.Feedback)
		b.WriteString("\n</solution_evaluation>\n")

		// Score
		b.WriteString(fmt.Sprintf("<solution_score>%.2f</solution_score>\n", attempt.Score))

		b.WriteString(fmt.Sprintf("</solution_%d>\n\n", i+1))
	}

	return b.String()
}

// selectAttempts returns attempts for feedback with selection probability
// and optional improving order (best last)
func (r *RefinementLoop) selectAttempts() []RefinementAttempt {
	if len(r.attempts) == 0 {
		return nil
	}

	// Apply selection probability (simple: take most recent up to max)
	maxToSelect := r.config.MaxSolutions
	if maxToSelect > len(r.attempts) {
		maxToSelect = len(r.attempts)
	}

	// Copy for sorting
	selected := make([]RefinementAttempt, len(r.attempts))
	copy(selected, r.attempts)

	// Sort by score
	sort.Slice(selected, func(i, j int) bool {
		if r.config.ImprovingOrder {
			return selected[i].Score < selected[j].Score // Worst first, best last
		}
		return selected[i].Score > selected[j].Score // Best first
	})

	// Take top N (or bottom N with improving order)
	if len(selected) > maxToSelect {
		if r.config.ImprovingOrder {
			selected = selected[len(selected)-maxToSelect:]
		} else {
			selected = selected[:maxToSelect]
		}
	}

	return selected
}

// Result builds the final refinement result
func (r *RefinementLoop) Result(startTime time.Time) *RefinementResult {
	result := &RefinementResult{
		Iterations: len(r.attempts),
		Duration:   time.Since(startTime),
		Attempts:   r.attempts,
	}

	if len(r.attempts) == 0 {
		result.Success = false
		result.Error = fmt.Errorf("no attempts made")
		return result
	}

	// Check if last attempt was successful
	lastAttempt := r.attempts[len(r.attempts)-1]
	if lastAttempt.Score >= r.config.SuccessThreshold {
		result.Success = true
		result.Output = lastAttempt.Output
		result.Score = lastAttempt.Score
		return result
	}

	// Return best attempt if configured
	if r.config.ReturnBestOnTimeout && r.bestIdx >= 0 {
		best := r.attempts[r.bestIdx]
		result.Output = best.Output
		result.Score = best.Score
		result.Success = best.Score >= r.config.SuccessThreshold
	}

	return result
}

// RefinementExecutor runs a task with iterative refinement
// This is the main entry point, similar to Poetiq's solve_coding
type RefinementExecutor struct {
	loop     *RefinementLoop
	taskFunc func(ctx context.Context, prompt string) (any, error)
	expected any
}

// NewRefinementExecutor creates an executor for iterative refinement
func NewRefinementExecutor(
	scorer SoftScorer,
	taskFunc func(ctx context.Context, prompt string) (any, error),
	expected any,
) *RefinementExecutor {
	return &RefinementExecutor{
		loop:     NewRefinementLoop(scorer),
		taskFunc: taskFunc,
		expected: expected,
	}
}

// WithConfig sets the refinement configuration
func (e *RefinementExecutor) WithConfig(config RefinementConfig) *RefinementExecutor {
	e.loop.WithConfig(config)
	return e
}

// Run executes the refinement loop
func (e *RefinementExecutor) Run(ctx context.Context, initialPrompt string) (*RefinementResult, error) {
	startTime := time.Now()
	e.loop.Reset()

	currentPrompt := initialPrompt

	for e.loop.ShouldContinue() {
		select {
		case <-ctx.Done():
			return e.loop.Result(startTime), ctx.Err()
		default:
		}

		// Execute task
		iterStart := time.Now()
		output, err := e.taskFunc(ctx, currentPrompt)
		iterDuration := time.Since(iterStart)

		if err != nil {
			// Record failed attempt with zero score
			e.loop.RecordAttempt(currentPrompt, nil, e.expected, iterDuration)
			continue
		}

		// Score and record
		score := e.loop.RecordAttempt(currentPrompt, output, e.expected, iterDuration)

		// Check for success
		if score >= e.loop.config.SuccessThreshold {
			break
		}

		// Build feedback prompt for next iteration
		feedbackPrompt := e.loop.BuildFeedbackPrompt()
		currentPrompt = initialPrompt + "\n\n" + feedbackPrompt
	}

	return e.loop.Result(startTime), nil
}
