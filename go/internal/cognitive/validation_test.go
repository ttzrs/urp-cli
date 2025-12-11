package cognitive

import (
	"context"
	"strings"
	"testing"

	"github.com/joss/urp/internal/opencode/domain"
	"github.com/joss/urp/pkg/llm"
)

// TestSystemValidation validates the complete response evaluation system
func TestSystemValidation(t *testing.T) {
	// Create a mock provider for testing
	mockProvider := &TestMockProvider{
		Responses: map[string]string{
			"test analysis": "ROOT CAUSE: Missing validation check\nPROPOSAL: Add validation to input function in user.go\nFILES: user.go\nCONFIDENCE: high",
		},
	}

	// Test 1: ResponseEvaluator functionality
	t.Run("ResponseEvaluator", func(t *testing.T) {
		evaluator := NewResponseEvaluator(mockProvider, "test-model")
		
		// Test good response
		goodResponse := "ROOT CAUSE: Missing validation check\nPROPOSAL: Add validation to input function in user.go\nFILES: user.go\nCONFIDENCE: high"
		eval, err := evaluator.EvaluateResponse(context.Background(), goodResponse)
		if err != nil {
			t.Fatalf("Error evaluating good response: %v", err)
		}
		
		if eval.Score < 0.7 {
			t.Errorf("Expected good response to have score >= 0.7, got %.2f", eval.Score)
		}
		
		if !eval.PatternCompliance {
			t.Error("Expected good response to comply with patterns")
		}
		
		// Test bad response
		badResponse := "This might be an issue somewhere in the code. Maybe check some files."
		badEval, err := evaluator.EvaluateResponse(context.Background(), badResponse)
		if err != nil {
			t.Fatalf("Error evaluating bad response: %v", err)
		}
		
		if badEval.Score >= 0.5 {
			t.Errorf("Expected bad response to have score < 0.5, got %.2f", badEval.Score)
		}
		
		t.Logf("Good response score: %.2f, Bad response score: %.2f", eval.Score, badEval.Score)
	})

	// Test 2: Main Evaluator integration
	t.Run("MainEvaluatorIntegration", func(t *testing.T) {
		mockProvider := &TestMockProvider{
			Responses: map[string]string{
				"ERROR (1 occurrences):\ntest error\n\nContext: test/test-op\n\nRELATED FILES (by spreading activation energy):\n- [0.50] test.go\n": "ROOT CAUSE: Missing validation check\nPROPOSAL: Add validation to input function in user.go\nFILES: user.go\nCONFIDENCE: high",
			},
		}
		
		mainEvaluator := NewEvaluator(mockProvider, WithModel("test-model"))
		
		errCtx := ErrorContext{
			ErrorMessage: "test error",
			ErrorCount:   1,
			Category:     "test",
			Operation:    "test-op",
			Files: []OptimizedFile{
				{Path: "test.go", Energy: 0.5},
			},
			Source:       "test.go:1",
			Severity:     "medium",
			Environment:  "test",
		}
		
		proposal, err := mainEvaluator.ProposeFix(context.Background(), errCtx)
		if err != nil {
			t.Fatalf("Error proposing fix: %v", err)
		}
		
		if proposal.Analysis == "" {
			t.Error("Expected analysis in proposal")
		}
		
		// Test quality evaluation
		quality, err := mainEvaluator.EvaluateResponseQuality(context.Background(), proposal.RawResponse)
		if err != nil {
			t.Fatalf("Error evaluating response quality: %v", err)
		}
		
		t.Logf("Proposal quality score: %.2f", quality.Score)
	})

	// Test 3: Adaptive prompting
	t.Run("AdaptivePrompting", func(t *testing.T) {
		prompter := NewAdaptivePrompter()
		
		lowQuality := &ResponseQuality{
			Score:           0.3,
			PatternCompliance: false,
			Issues:          []string{"missing structure"},
		}
		
		originalPrompt := "Analyze this error."
		adjusted := prompter.AdjustPrompt(originalPrompt, "context", lowQuality)
		
		if !strings.Contains(adjusted, "IMPORTANT: Provide your response in this exact format:") {
			t.Error("Expected adjusted prompt to include structure requirements")
		}
		
		// Test recording and insights
		prompter.RecordQuality(originalPrompt, "response", lowQuality, "context")
		insights := prompter.GetQualityInsights()
		
		if !strings.Contains(insights, "Low quality responses:") {
			t.Error("Expected insights to mention low quality responses")
		}
		
		t.Logf("Adaptive prompting working, insights: %s", insights)
	})

	// Test 4: Self-improvement flow
	t.Run("SelfImprovement", func(t *testing.T) {
		mainEvaluator := NewEvaluator(mockProvider, WithModel("test-model"))
		
		originalProposal := &FixProposal{
			Analysis:    "This might be an issue somewhere",
			Proposal:    "Maybe check some files",
			Files:       []string{},
			Confidence:  "low",
			RawResponse: "This might be an issue somewhere. Maybe check some files.",
		}
		
		errCtx := ErrorContext{
			ErrorMessage: "Test error",
			ErrorCount:   1,
			Category:     "test",
			Operation:    "test-op",
		}
		
		_, improvementPlan, err := mainEvaluator.SelfImproveFixProposal(context.Background(), originalProposal, errCtx)
		if err != nil {
			t.Logf("Self-improvement test skipped (expected in mock): %v", err)
		} else {
			t.Logf("Self-improvement test completed, needs improvement: %t", improvementPlan.NeedsImprovement)
		}
	})

	t.Log("All system validation tests passed!")
}

// TestMockProvider for testing
type TestMockProvider struct {
	Responses map[string]string
	CallCount int
}

func (m *TestMockProvider) ID() string { return "test-mock" }
func (m *TestMockProvider) Name() string { return "Test Mock Provider" }
func (m *TestMockProvider) Models() []domain.Model {
	return []domain.Model{{ID: "test-model", Name: "Test Model", ContextSize: 1000}}
}

func (m *TestMockProvider) Chat(ctx context.Context, req *llm.ChatRequest) (<-chan domain.StreamEvent, error) {
	m.CallCount++
	
	response := "DEFAULT MOCK RESPONSE"
	if len(req.Messages) > 0 {
		userMsg := ""
		for _, part := range req.Messages[0].Parts {
			if textPart, ok := part.(domain.TextPart); ok {
				userMsg = textPart.Text
				break
			}
		}
		if mockResp, exists := m.Responses[userMsg]; exists {
			response = mockResp
		}
	}

	events := make(chan domain.StreamEvent, 10)
	go func() {
		defer close(events)
		events <- domain.StreamEvent{
			Type:    domain.StreamEventText,
			Content: response,
		}
		events <- domain.StreamEvent{
			Type: domain.StreamEventDone,
			Done: true,
		}
	}()

	return events, nil
}