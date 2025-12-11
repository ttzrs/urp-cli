package agent

import (
	"strings"
	"testing"
)

func TestTestOutputScorer_Score(t *testing.T) {
	scorer := NewTestOutputScorer()

	tests := []struct {
		name     string
		output   string
		wantMin  float64
		wantMax  float64
	}{
		{
			name:    "all pass",
			output:  "PASS\nok   github.com/test/pkg\nPASS",
			wantMin: 0.9,
			wantMax: 1.0,
		},
		{
			name:    "all fail",
			output:  "--- FAIL: TestFoo\n--- FAIL: TestBar\nFAIL",
			wantMin: 0.0,
			wantMax: 0.1,
		},
		{
			name:    "mixed results",
			output:  "PASS\n--- FAIL: TestFoo\nPASS\nok ",
			wantMin: 0.4,
			wantMax: 0.8,
		},
		{
			name:    "no recognizable output",
			output:  "building...\ncompiling...\ndone",
			wantMin: 0.0,
			wantMax: 0.2, // Minimal credit
		},
		{
			name:    "empty output",
			output:  "",
			wantMin: 0.0,
			wantMax: 0.11, // Minimal credit for no recognizable patterns
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := scorer.Score(nil, tt.output)
			if score < tt.wantMin || score > tt.wantMax {
				t.Errorf("Score() = %f, want between %f and %f", score, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestTestOutputScorer_Score_NonString(t *testing.T) {
	scorer := NewTestOutputScorer()

	if score := scorer.Score(nil, 123); score != 0.0 {
		t.Errorf("Score(int) = %f, want 0.0", score)
	}
	if score := scorer.Score(nil, nil); score != 0.0 {
		t.Errorf("Score(nil) = %f, want 0.0", score)
	}
}

func TestTestOutputScorer_BuildFeedback(t *testing.T) {
	scorer := NewTestOutputScorer()

	output := `
=== RUN   TestFoo
--- FAIL: TestFoo (0.01s)
    foo_test.go:10: expected 5, got 3
=== RUN   TestBar
--- FAIL: TestBar (0.02s)
    bar_test.go:20: error: connection refused
FAIL
`

	feedback := scorer.BuildFeedback(nil, output)

	if !strings.Contains(feedback, "Failed tests") {
		t.Error("feedback should contain failed tests section")
	}
	if !strings.Contains(feedback, "TestFoo") {
		t.Error("feedback should contain TestFoo")
	}
	if !strings.Contains(feedback, "TestBar") {
		t.Error("feedback should contain TestBar")
	}
	if !strings.Contains(feedback, "Error messages") {
		t.Error("feedback should contain error messages section")
	}
}

func TestExitCodeScorer_Score(t *testing.T) {
	scorer := NewExitCodeScorer()

	tests := []struct {
		name   string
		actual any
		want   float64
	}{
		{"exit 0", 0, 1.0},
		{"exit 1", 1, 0.0},
		{"exit 2", 2, 0.0},
		{"string success", "exit status 0", 1.0},
		{"string fail", "exit status 1", 0.0},
		{"string succeeded", "command succeeded", 1.0},
		{"unknown", "some output", 0.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := scorer.Score(nil, tt.actual)
			if score != tt.want {
				t.Errorf("Score(%v) = %f, want %f", tt.actual, score, tt.want)
			}
		})
	}
}

func TestDiffScorer_Score(t *testing.T) {
	scorer := NewDiffScorer()

	tests := []struct {
		name     string
		expected string
		actual   string
		wantMin  float64
		wantMax  float64
	}{
		{
			name:     "exact match",
			expected: "line1\nline2\nline3",
			actual:   "line1\nline2\nline3",
			wantMin:  0.99,
			wantMax:  1.0,
		},
		{
			name:     "partial match",
			expected: "line1\nline2\nline3",
			actual:   "line1\nline2\nline4",
			wantMin:  0.5,
			wantMax:  0.8,
		},
		{
			name:     "no match",
			expected: "aaa\nbbb",
			actual:   "ccc\nddd",
			wantMin:  0.0,
			wantMax:  0.1,
		},
		{
			name:     "both empty",
			expected: "",
			actual:   "",
			wantMin:  0.99,
			wantMax:  1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := scorer.Score(tt.expected, tt.actual)
			if score < tt.wantMin || score > tt.wantMax {
				t.Errorf("Score() = %f, want between %f and %f", score, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestDiffScorer_BuildFeedback(t *testing.T) {
	scorer := NewDiffScorer()

	expected := "line1\nline2\nline3"
	actual := "line1\nchanged\nline3"

	feedback := scorer.BuildFeedback(expected, actual)

	if !strings.Contains(feedback, "Diff") {
		t.Error("feedback should contain diff header")
	}
	if !strings.Contains(feedback, "line1") {
		t.Error("feedback should contain matching line")
	}
	if !strings.Contains(feedback, "-") || !strings.Contains(feedback, "+") {
		t.Error("feedback should contain diff markers")
	}
}

func TestCompositeScorer(t *testing.T) {
	composite := NewCompositeScorer().
		Add(NewTestOutputScorer(), 0.7).
		Add(NewExitCodeScorer(), 0.3)

	// Test output with mixed results
	testOutput := "PASS\n--- FAIL: TestFoo\nPASS"
	// ExitCodeScorer expects int but we're passing string

	score := composite.Score(nil, testOutput)
	// TestOutputScorer should give ~0.66, ExitCodeScorer gets string (gives 0.5)
	// Result should be weighted

	if score < 0.2 || score > 0.8 {
		t.Errorf("composite score = %f, expected moderate value", score)
	}
}

func TestCompositeScorer_Empty(t *testing.T) {
	composite := NewCompositeScorer()

	if score := composite.Score(nil, "test"); score != 0.0 {
		t.Errorf("empty composite score = %f, want 0.0", score)
	}
}

func TestCompositeScorer_BuildFeedback(t *testing.T) {
	composite := NewCompositeScorer().
		Add(NewTestOutputScorer(), 1.0).
		Add(NewDiffScorer(), 1.0)

	output := "--- FAIL: TestFoo\nerror: something broke"
	feedback := composite.BuildFeedback("expected", output)

	// Should combine feedback from both scorers
	if feedback == "" {
		t.Error("feedback should not be empty")
	}
}

func TestMax(t *testing.T) {
	if max(3, 5) != 5 {
		t.Error("max(3, 5) should be 5")
	}
	if max(5, 3) != 5 {
		t.Error("max(5, 3) should be 5")
	}
	if max(3, 3) != 3 {
		t.Error("max(3, 3) should be 3")
	}
}

func TestSoftScorer_Interface(t *testing.T) {
	// Verify all scorers implement the interface
	var _ SoftScorer = NewTestOutputScorer()
	var _ SoftScorer = NewExitCodeScorer()
	var _ SoftScorer = NewDiffScorer()
	var _ SoftScorer = NewCompositeScorer()
}
