package agent

import (
	"context"
	"strings"
	"testing"
	"time"
)

// mockScorer implements SoftScorer for testing
type mockScorer struct {
	scores map[string]float64
}

func newMockScorer(scores map[string]float64) *mockScorer {
	return &mockScorer{scores: scores}
}

func (m *mockScorer) Score(expected, actual any) float64 {
	if s, ok := actual.(string); ok {
		if score, exists := m.scores[s]; exists {
			return score
		}
	}
	return 0.0
}

func (m *mockScorer) BuildFeedback(expected, actual any) string {
	if s, ok := actual.(string); ok {
		score := m.Score(expected, actual)
		return "Feedback for: " + s + " (score: " + string(rune(int(score*100))) + "%)"
	}
	return "No feedback"
}

func TestNewRefinementLoop(t *testing.T) {
	scorer := newMockScorer(nil)
	loop := NewRefinementLoop(scorer)

	if loop == nil {
		t.Fatal("NewRefinementLoop returned nil")
	}
	if loop.IterationCount() != 0 {
		t.Errorf("initial iteration count = %d, want 0", loop.IterationCount())
	}
	if loop.config.MaxIterations != 5 {
		t.Errorf("default max iterations = %d, want 5", loop.config.MaxIterations)
	}
}

func TestRefinementLoop_RecordAttempt(t *testing.T) {
	scorer := newMockScorer(map[string]float64{
		"output1": 0.5,
		"output2": 0.8,
	})
	loop := NewRefinementLoop(scorer)

	score1 := loop.RecordAttempt("input1", "output1", "expected", time.Second)
	if score1 != 0.5 {
		t.Errorf("score1 = %f, want 0.5", score1)
	}
	if loop.IterationCount() != 1 {
		t.Errorf("iteration count = %d, want 1", loop.IterationCount())
	}

	score2 := loop.RecordAttempt("input2", "output2", "expected", time.Second)
	if score2 != 0.8 {
		t.Errorf("score2 = %f, want 0.8", score2)
	}

	// Best should be second attempt
	best := loop.BestAttempt()
	if best == nil {
		t.Fatal("BestAttempt returned nil")
	}
	if best.Score != 0.8 {
		t.Errorf("best score = %f, want 0.8", best.Score)
	}
}

func TestRefinementLoop_IsSuccess(t *testing.T) {
	scorer := newMockScorer(map[string]float64{
		"partial": 0.7,
		"perfect": 1.0,
	})
	loop := NewRefinementLoop(scorer)

	// No attempts
	if loop.IsSuccess() {
		t.Error("IsSuccess should be false with no attempts")
	}

	// Partial success
	loop.RecordAttempt("", "partial", "", time.Second)
	if loop.IsSuccess() {
		t.Error("IsSuccess should be false for score < 1.0")
	}

	// Perfect success
	loop.RecordAttempt("", "perfect", "", time.Second)
	if !loop.IsSuccess() {
		t.Error("IsSuccess should be true for score = 1.0")
	}
}

func TestRefinementLoop_ShouldContinue(t *testing.T) {
	scorer := newMockScorer(map[string]float64{
		"partial": 0.5,
		"perfect": 1.0,
	})
	loop := NewRefinementLoop(scorer).WithConfig(RefinementConfig{
		MaxIterations:    3,
		SuccessThreshold: 1.0,
	})

	// Initially should continue
	if !loop.ShouldContinue() {
		t.Error("should continue initially")
	}

	// After partial success, should continue
	loop.RecordAttempt("", "partial", "", time.Second)
	if !loop.ShouldContinue() {
		t.Error("should continue after partial success")
	}

	// After perfect success, should stop
	loop.RecordAttempt("", "perfect", "", time.Second)
	if loop.ShouldContinue() {
		t.Error("should not continue after success")
	}
}

func TestRefinementLoop_ShouldContinue_MaxIterations(t *testing.T) {
	scorer := newMockScorer(map[string]float64{"partial": 0.5})
	loop := NewRefinementLoop(scorer).WithConfig(RefinementConfig{
		MaxIterations:    2,
		SuccessThreshold: 1.0,
	})

	loop.RecordAttempt("", "partial", "", time.Second)
	loop.RecordAttempt("", "partial", "", time.Second)

	if loop.ShouldContinue() {
		t.Error("should not continue after max iterations")
	}
}

func TestRefinementLoop_BuildFeedbackPrompt(t *testing.T) {
	scorer := newMockScorer(map[string]float64{
		"output1": 0.3,
		"output2": 0.7,
	})
	loop := NewRefinementLoop(scorer)

	// No attempts
	if prompt := loop.BuildFeedbackPrompt(); prompt != "" {
		t.Error("feedback should be empty with no attempts")
	}

	// Add attempts
	loop.RecordAttempt("code1", "output1", "", time.Second)
	loop.RecordAttempt("code2", "output2", "", time.Second)

	prompt := loop.BuildFeedbackPrompt()

	if !strings.Contains(prompt, "EXISTING PARTIAL") {
		t.Error("prompt should contain header")
	}
	if !strings.Contains(prompt, "<solution_1>") {
		t.Error("prompt should contain solution tags")
	}
	if !strings.Contains(prompt, "<solution_code>") {
		t.Error("prompt should contain code tags")
	}
	if !strings.Contains(prompt, "<solution_evaluation>") {
		t.Error("prompt should contain evaluation tags")
	}
	if !strings.Contains(prompt, "<solution_score>") {
		t.Error("prompt should contain score tags")
	}
}

func TestRefinementLoop_SelectAttempts_ImprovingOrder(t *testing.T) {
	scorer := newMockScorer(map[string]float64{
		"best":   0.9,
		"worst":  0.1,
		"middle": 0.5,
	})
	loop := NewRefinementLoop(scorer).WithConfig(RefinementConfig{
		MaxSolutions:   3,
		ImprovingOrder: true,
	})

	loop.RecordAttempt("", "best", "", time.Second)
	loop.RecordAttempt("", "worst", "", time.Second)
	loop.RecordAttempt("", "middle", "", time.Second)

	selected := loop.selectAttempts()

	// With improving order, best should be last
	if selected[len(selected)-1].Score != 0.9 {
		t.Errorf("last selected score = %f, want 0.9 (best last)", selected[len(selected)-1].Score)
	}
}

func TestRefinementLoop_SelectAttempts_MaxSolutions(t *testing.T) {
	scorer := newMockScorer(nil)
	loop := NewRefinementLoop(scorer).WithConfig(RefinementConfig{
		MaxSolutions: 2,
	})

	for i := 0; i < 5; i++ {
		loop.RecordAttempt("", "", "", time.Second)
	}

	selected := loop.selectAttempts()
	if len(selected) != 2 {
		t.Errorf("selected count = %d, want 2", len(selected))
	}
}

func TestRefinementLoop_Reset(t *testing.T) {
	scorer := newMockScorer(map[string]float64{"output": 0.5})
	loop := NewRefinementLoop(scorer)

	loop.RecordAttempt("input", "output", "", time.Second)
	loop.Reset()

	if loop.IterationCount() != 0 {
		t.Errorf("iteration count after reset = %d, want 0", loop.IterationCount())
	}
	if loop.BestAttempt() != nil {
		t.Error("best attempt should be nil after reset")
	}
}

func TestRefinementLoop_Result_Success(t *testing.T) {
	scorer := newMockScorer(map[string]float64{"perfect": 1.0})
	loop := NewRefinementLoop(scorer)

	startTime := time.Now()
	loop.RecordAttempt("input", "perfect", "", time.Second)

	result := loop.Result(startTime)

	if !result.Success {
		t.Error("result should be successful")
	}
	if result.Score != 1.0 {
		t.Errorf("result score = %f, want 1.0", result.Score)
	}
	if result.Iterations != 1 {
		t.Errorf("iterations = %d, want 1", result.Iterations)
	}
}

func TestRefinementLoop_Result_ReturnBest(t *testing.T) {
	scorer := newMockScorer(map[string]float64{
		"good":   0.8,
		"worse":  0.5,
	})
	loop := NewRefinementLoop(scorer).WithConfig(RefinementConfig{
		MaxIterations:       2,
		ReturnBestOnTimeout: true,
		SuccessThreshold:    1.0,
	})

	startTime := time.Now()
	loop.RecordAttempt("", "good", "", time.Second)  // Score 0.8
	loop.RecordAttempt("", "worse", "", time.Second) // Score 0.5

	result := loop.Result(startTime)

	// Should return best (0.8) not last (0.5)
	if result.Score != 0.8 {
		t.Errorf("result score = %f, want 0.8 (best)", result.Score)
	}
}

func TestRefinementExecutor_Run_Success(t *testing.T) {
	scorer := newMockScorer(map[string]float64{
		"partial": 0.5,
		"perfect": 1.0,
	})

	iteration := 0
	taskFunc := func(ctx context.Context, prompt string) (any, error) {
		iteration++
		if iteration < 2 {
			return "partial", nil
		}
		return "perfect", nil
	}

	executor := NewRefinementExecutor(scorer, taskFunc, "expected")
	result, err := executor.Run(context.Background(), "initial prompt")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Error("result should be successful")
	}
	if result.Iterations != 2 {
		t.Errorf("iterations = %d, want 2", result.Iterations)
	}
}

func TestRefinementExecutor_Run_MaxIterations(t *testing.T) {
	scorer := newMockScorer(map[string]float64{
		"partial": 0.5,
	})

	taskFunc := func(ctx context.Context, prompt string) (any, error) {
		return "partial", nil
	}

	executor := NewRefinementExecutor(scorer, taskFunc, "expected").WithConfig(RefinementConfig{
		MaxIterations:       3,
		ReturnBestOnTimeout: true,
		SuccessThreshold:    1.0,
	})

	result, err := executor.Run(context.Background(), "initial prompt")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("result should not be successful")
	}
	if result.Iterations != 3 {
		t.Errorf("iterations = %d, want 3", result.Iterations)
	}
	if result.Score != 0.5 {
		t.Errorf("score = %f, want 0.5 (best partial)", result.Score)
	}
}

func TestRefinementExecutor_Run_ContextCancel(t *testing.T) {
	scorer := newMockScorer(map[string]float64{"partial": 0.5})

	taskFunc := func(ctx context.Context, prompt string) (any, error) {
		time.Sleep(100 * time.Millisecond)
		return "partial", nil
	}

	executor := NewRefinementExecutor(scorer, taskFunc, "expected")

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	result, err := executor.Run(ctx, "prompt")

	if err != context.DeadlineExceeded {
		t.Errorf("error = %v, want context.DeadlineExceeded", err)
	}
	if result == nil {
		t.Error("result should not be nil even on cancel")
	}
}

func TestDefaultRefinementConfig(t *testing.T) {
	config := DefaultRefinementConfig()

	if config.MaxIterations != 5 {
		t.Errorf("MaxIterations = %d, want 5", config.MaxIterations)
	}
	if config.SelectionProb != 0.8 {
		t.Errorf("SelectionProb = %f, want 0.8", config.SelectionProb)
	}
	if config.MaxSolutions != 3 {
		t.Errorf("MaxSolutions = %d, want 3", config.MaxSolutions)
	}
	if !config.ImprovingOrder {
		t.Error("ImprovingOrder should be true")
	}
	if !config.ReturnBestOnTimeout {
		t.Error("ReturnBestOnTimeout should be true")
	}
	if config.SuccessThreshold != 1.0 {
		t.Errorf("SuccessThreshold = %f, want 1.0", config.SuccessThreshold)
	}
}

func TestRefinementAttempt_Fields(t *testing.T) {
	attempt := RefinementAttempt{
		Iteration: 1,
		Input:     "test input",
		Output:    "test output",
		Score:     0.75,
		Feedback:  "test feedback",
		Duration:  time.Second,
	}

	if attempt.Iteration != 1 {
		t.Errorf("Iteration = %d, want 1", attempt.Iteration)
	}
	if attempt.Score != 0.75 {
		t.Errorf("Score = %f, want 0.75", attempt.Score)
	}
	if attempt.Duration != time.Second {
		t.Errorf("Duration = %v, want 1s", attempt.Duration)
	}
}
