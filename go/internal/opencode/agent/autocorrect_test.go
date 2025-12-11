package agent

import (
	"strings"
	"testing"

	"github.com/joss/urp/internal/opencode/domain"
)

func TestNewAutocorrector(t *testing.T) {
	ac := NewAutocorrector()

	if ac == nil {
		t.Fatal("NewAutocorrector returned nil")
	}
	if ac.RetryCount() != 0 {
		t.Errorf("initial retry count = %d, want 0", ac.RetryCount())
	}
	if ac.MaxRetries() != 3 {
		t.Errorf("default max retries = %d, want 3", ac.MaxRetries())
	}
	if !ac.improvingOrder {
		t.Error("improvingOrder should be true by default")
	}
}

func TestAutocorrector_RecordAttempt(t *testing.T) {
	ac := NewAutocorrector()

	// Record first attempt
	ac.RecordAttempt("go test ./...", "FAIL: TestFoo", "exit status 1", 0.3)

	attempts := ac.Attempts()
	if len(attempts) != 1 {
		t.Fatalf("attempts = %d, want 1", len(attempts))
	}

	a := attempts[0]
	if a.Code != "go test ./..." {
		t.Errorf("code = %q, want %q", a.Code, "go test ./...")
	}
	if a.Score != 0.3 {
		t.Errorf("score = %f, want 0.3", a.Score)
	}
	if a.Feedback == "" {
		t.Error("feedback should not be empty")
	}
}

func TestAutocorrector_BestAttempt(t *testing.T) {
	ac := NewAutocorrector()

	// No attempts
	if ac.BestAttempt() != nil {
		t.Error("BestAttempt should return nil when no attempts")
	}

	// Add multiple attempts with different scores
	ac.RecordAttempt("attempt1", "output1", "error1", 0.3)
	ac.RecordAttempt("attempt2", "output2", "error2", 0.8)
	ac.RecordAttempt("attempt3", "output3", "error3", 0.5)

	best := ac.BestAttempt()
	if best == nil {
		t.Fatal("BestAttempt returned nil")
	}
	if best.Score != 0.8 {
		t.Errorf("best score = %f, want 0.8", best.Score)
	}
	if best.Code != "attempt2" {
		t.Errorf("best code = %q, want %q", best.Code, "attempt2")
	}
}

func TestAutocorrector_ResetRetries(t *testing.T) {
	ac := NewAutocorrector()

	ac.IncrementRetry()
	ac.IncrementRetry()
	ac.RecordAttempt("test", "output", "error", 0.5)

	ac.ResetRetries()

	if ac.RetryCount() != 0 {
		t.Errorf("retry count after reset = %d, want 0", ac.RetryCount())
	}
	if len(ac.Attempts()) != 0 {
		t.Errorf("attempts after reset = %d, want 0", len(ac.Attempts()))
	}
}

func TestAutocorrector_DetectFailure(t *testing.T) {
	ac := NewAutocorrector()

	tests := []struct {
		name       string
		parts      []domain.Part
		wantFailed bool
	}{
		{
			name: "success - no failure patterns",
			parts: []domain.Part{
				domain.ToolCallPart{Result: "ok\nPASS\nall tests passed"},
			},
			wantFailed: false,
		},
		{
			name: "failure - FAIL pattern",
			parts: []domain.Part{
				domain.ToolCallPart{Result: "--- FAIL: TestFoo"},
			},
			wantFailed: true,
		},
		{
			name: "failure - error pattern",
			parts: []domain.Part{
				domain.ToolCallPart{Result: "error: something went wrong"},
			},
			wantFailed: true,
		},
		{
			name: "failure - panic pattern",
			parts: []domain.Part{
				domain.ToolCallPart{Result: "panic: runtime error"},
			},
			wantFailed: true,
		},
		{
			name: "failure in error field",
			parts: []domain.Part{
				domain.ToolCallPart{Result: "ok", Error: "exit status 1 FAIL"},
			},
			wantFailed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			failed, _ := ac.DetectFailure(tt.parts)
			if failed != tt.wantFailed {
				t.Errorf("DetectFailure() = %v, want %v", failed, tt.wantFailed)
			}
		})
	}
}

func TestAutocorrector_ShouldRetry(t *testing.T) {
	ac := NewAutocorrector()

	// Initially should retry
	if !ac.ShouldRetry() {
		t.Error("ShouldRetry should be true initially")
	}

	// After max retries, should not retry
	for i := 0; i < 3; i++ {
		ac.IncrementRetry()
	}
	if ac.ShouldRetry() {
		t.Error("ShouldRetry should be false after max retries")
	}

	// Disabled config
	ac2 := NewAutocorrector()
	ac2.Configure(AutocorrectionConfig{Enabled: false, MaxRetries: 3})
	if ac2.ShouldRetry() {
		t.Error("ShouldRetry should be false when disabled")
	}
}

func TestAutocorrector_CorrectionPrompt_NoAttempts(t *testing.T) {
	ac := NewAutocorrector()
	ac.IncrementRetry()

	prompt := ac.CorrectionPrompt()

	if !strings.Contains(prompt, "FAILURE DETECTED") {
		t.Error("prompt should contain failure header")
	}
	if !strings.Contains(prompt, "1/3") {
		t.Error("prompt should contain retry count")
	}
	if !strings.Contains(prompt, "Analyze the error") {
		t.Error("prompt should contain fallback instructions")
	}
}

func TestAutocorrector_CorrectionPrompt_WithAttempts(t *testing.T) {
	ac := NewAutocorrector()

	// Add some attempts
	ac.RecordAttempt("go test", "FAIL: TestA\n--- FAIL: TestA", "exit 1", 0.3)
	ac.RecordAttempt("go test -v", "PASS: TestB\nFAIL: TestA", "", 0.7)
	ac.IncrementRetry()

	prompt := ac.CorrectionPrompt()

	// Should contain structured feedback elements
	if !strings.Contains(prompt, "EXISTING PARTIAL/INCORRECT SOLUTIONS") {
		t.Error("prompt should contain solutions header")
	}
	if !strings.Contains(prompt, "<attempt_") {
		t.Error("prompt should contain attempt tags")
	}
	if !strings.Contains(prompt, "<code>") {
		t.Error("prompt should contain code tags")
	}
	if !strings.Contains(prompt, "<evaluation>") {
		t.Error("prompt should contain evaluation tags")
	}
	if !strings.Contains(prompt, "<score>") {
		t.Error("prompt should contain score tags")
	}
	if !strings.Contains(prompt, "0.70") || !strings.Contains(prompt, "0.30") {
		t.Error("prompt should contain actual scores")
	}
}

func TestAutocorrector_getSortedAttempts_ImprovingOrder(t *testing.T) {
	ac := NewAutocorrector()
	ac.improvingOrder = true

	ac.RecordAttempt("best", "", "", 0.9)
	ac.RecordAttempt("worst", "", "", 0.1)
	ac.RecordAttempt("middle", "", "", 0.5)

	sorted := ac.getSortedAttempts()

	if len(sorted) != 3 {
		t.Fatalf("sorted length = %d, want 3", len(sorted))
	}

	// With improving order, best should be last
	if sorted[0].Score != 0.1 {
		t.Errorf("first score = %f, want 0.1 (worst first)", sorted[0].Score)
	}
	if sorted[2].Score != 0.9 {
		t.Errorf("last score = %f, want 0.9 (best last)", sorted[2].Score)
	}
}

func TestAutocorrector_getSortedAttempts_LimitTo3(t *testing.T) {
	ac := NewAutocorrector()

	// Add 5 attempts
	for i := 0; i < 5; i++ {
		ac.RecordAttempt("code", "", "", float64(i)*0.2)
	}

	sorted := ac.getSortedAttempts()

	if len(sorted) != 3 {
		t.Errorf("sorted length = %d, want 3 (max)", len(sorted))
	}
}

func TestAutocorrector_buildStructuredFeedback(t *testing.T) {
	ac := NewAutocorrector()

	feedback := ac.buildStructuredFeedback(
		"FAIL: TestFoo\n--- FAIL: TestFoo (0.01s)\nError: expected 5, got 3",
		"exit status 1",
		0.4,
	)

	// Should contain error
	if !strings.Contains(feedback, "exit status 1") {
		t.Error("feedback should contain error message")
	}

	// Should contain failure indicators
	if !strings.Contains(feedback, "Failure indicators") {
		t.Error("feedback should contain failure indicators section")
	}

	// Should contain output snippet
	if !strings.Contains(feedback, "Output snippet") {
		t.Error("feedback should contain output snippet")
	}

	// Should contain accuracy estimate
	if !strings.Contains(feedback, "Accuracy estimate: 0.40") {
		t.Error("feedback should contain accuracy estimate")
	}
}

func TestScoredAttempt_Fields(t *testing.T) {
	attempt := ScoredAttempt{
		Code:     "test code",
		Output:   "test output",
		Error:    "test error",
		Score:    0.75,
		Feedback: "test feedback",
	}

	if attempt.Code != "test code" {
		t.Errorf("Code = %q, want %q", attempt.Code, "test code")
	}
	if attempt.Score != 0.75 {
		t.Errorf("Score = %f, want 0.75", attempt.Score)
	}
}

func TestTruncateOutput(t *testing.T) {
	tests := []struct {
		input  string
		maxLen int
		want   string
	}{
		{"short", 10, "short"},
		{"longer string", 10, "longer ..."},
		{"exactly10!", 10, "exactly10!"},
		{"", 5, ""},
	}

	for _, tt := range tests {
		got := truncateOutput(tt.input, tt.maxLen)
		if got != tt.want {
			t.Errorf("truncateOutput(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
		}
	}
}

func TestMin(t *testing.T) {
	if min(3, 5) != 3 {
		t.Error("min(3, 5) should be 3")
	}
	if min(5, 3) != 3 {
		t.Error("min(5, 3) should be 3")
	}
	if min(3, 3) != 3 {
		t.Error("min(3, 3) should be 3")
	}
}
