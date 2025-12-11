package cognitive

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/joss/urp/pkg/llm"
	"github.com/joss/urp/internal/opencode/domain"
)

// MockProvider is a mock implementation of llm.Provider for testing
type MockProvider struct {
	Responses map[string]string
}

func (m *MockProvider) ID() string   { return "mock" }
func (m *MockProvider) Name() string { return "Mock Provider" }
func (m *MockProvider) Models() []domain.Model {
	return []domain.Model{{ID: "mock-model", Name: "Mock Model", ContextSize: 1000}}
}

func (m *MockProvider) Chat(ctx context.Context, req *llm.ChatRequest) (<-chan domain.StreamEvent, error) {
	response := "MOCK RESPONSE"
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

	events := make(chan domain.StreamEvent, 1)
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

func TestResponseEvaluator(t *testing.T) {
	// Create a mock provider
	mockProvider := &MockProvider{
		Responses: map[string]string{
			"test good response": "ROOT CAUSE: Missing validation check\nPROPOSAL: Add validation to input function in user.go\nFILES: user.go\nCONFIDENCE: high",
			"test bad response":  "This might be an issue somewhere in the code. Maybe check some files.",
		},
	}

	// Create evaluator
	evaluator := NewResponseEvaluator(mockProvider, "mock-model")

	ctx := context.Background()

	// Test good response
	goodResponse := "ROOT CAUSE: Missing validation check\nPROPOSAL: Add validation to input function in user.go\nFILES: user.go\nCONFIDENCE: high"
	goodEval, err := evaluator.EvaluateResponse(ctx, goodResponse)
	if err != nil {
		t.Errorf("Error evaluating good response: %v", err)
	}

	if goodEval.Score < 0.7 {
		t.Errorf("Expected good response to have score >= 0.7, got %.2f", goodEval.Score)
	}

	if !goodEval.PatternCompliance {
		t.Error("Expected good response to comply with patterns")
	}

	// Test bad response
	badResponse := "This might be an issue somewhere in the code. Maybe check some files."
	badEval, err := evaluator.EvaluateResponse(ctx, badResponse)
	if err != nil {
		t.Errorf("Error evaluating bad response: %v", err)
	}

	if badEval.Score >= 0.5 {
		t.Errorf("Expected bad response to have score < 0.5, got %.2f", badEval.Score)
	}

	if badEval.PatternCompliance {
		t.Error("Expected bad response to not comply with patterns")
	}

	// Test improvement suggestions
	if len(badEval.Suggestions) == 0 {
		t.Error("Expected bad response to have improvement suggestions")
	}
}

func TestAdaptivePrompter(t *testing.T) {
	prompter := NewAdaptivePrompter()

	// Simulate a low quality response
	lowQuality := &ResponseQuality{
		Score:           0.3,
		PatternCompliance: false,
		Issues:          []string{"missing structure", "vague content"},
		Suggestions:     []string{"use structured format", "be more specific"},
	}

	// Adjust a prompt based on low quality
	basePrompt := "Analyze this error and provide a fix."
	adjustedPrompt := prompter.AdjustPrompt(basePrompt, "error context", lowQuality)

	if !strings.Contains(adjustedPrompt, "IMPORTANT: Provide your response in this exact format:") {
		t.Error("Expected adjusted prompt to include structure requirements for low quality response")
	}

	// Record the quality
	prompter.RecordQuality(basePrompt, "test response", &ResponseQuality{
		Score:           lowQuality.Score,
		PatternCompliance: lowQuality.PatternCompliance,
		Issues:          lowQuality.Issues,
		Suggestions:     lowQuality.Suggestions,
		Confidence:      lowQuality.Confidence,
		Analysis:        lowQuality.Analysis,
	}, "error context")

	// Get insights
	insights := prompter.GetQualityInsights()
	if !strings.Contains(insights, "Low quality responses:") {
		t.Error("Expected insights to mention low quality responses")
	}
}

func ExampleUsage() {
	fmt.Println("=== URP Response Quality Evaluation Example ===")
	
	// Create a mock provider (in real usage, this would be a real LLM provider)
	mockProvider := &MockProvider{
		Responses: map[string]string{
			"Fix the authentication error": "ROOT CAUSE: The authentication middleware is not properly handling expired tokens. The issue is in auth.go line 45 where expired tokens are not being checked.\nPROPOSAL: Update the token validation function to check for expiration before allowing access. Modify auth.go to include expiration validation.\nFILES: auth.go, middleware/auth.go\nCONFIDENCE: high",
		},
	}
	
	// Create response evaluator
	evaluator := NewResponseEvaluator(mockProvider, "claude-sonnet-4-5-20250929")
	
	// Simulate a response from an LLM
	response := "ROOT CAUSE: The authentication middleware is not properly handling expired tokens. The issue is in auth.go line 45 where expired tokens are not being checked.\nPROPOSAL: Update the token validation function to check for expiration before allowing access. Modify auth.go to include expiration validation.\nFILES: auth.go, middleware/auth.go\nCONFIDENCE: high"
	
	ctx := context.Background()
	
	// Evaluate the response quality
	quality, err := evaluator.EvaluateResponse(ctx, response)
	if err != nil {
		fmt.Printf("Error evaluating response: %v\n", err)
		return
	}
	
	fmt.Printf("Response Quality Score: %.2f\n", quality.Score)
	fmt.Printf("Pattern Compliance: %t\n", quality.PatternCompliance)
	
	if len(quality.Issues) > 0 {
		fmt.Println("Issues found:")
		for _, issue := range quality.Issues {
			fmt.Printf("  - %s\n", issue)
		}
	}
	
	if len(quality.Suggestions) > 0 {
		fmt.Println("Suggestions:")
		for _, suggestion := range quality.Suggestions {
			fmt.Printf("  - %s\n", suggestion)
		}
	}
	
	// Create adaptive prompter for learning
	prompter := NewAdaptivePrompter()
	prompter.RecordQuality("original prompt", response, &ResponseQuality{
		Score:           quality.Score,
		PatternCompliance: quality.PatternCompliance,
		Issues:          quality.Issues,
		Suggestions:     quality.Suggestions,
		Confidence:      quality.Confidence,
		Analysis:        quality.Analysis,
	}, "auth error context")
	
	fmt.Println("\nQuality Insights:")
	fmt.Println(prompter.GetQualityInsights())
	
	// Simulate self-improvement process
	fmt.Println("\n=== Self-Improvement Process ===")
	fmt.Println("The system can automatically detect when responses don't meet quality standards")
	fmt.Println("and suggest improvements or generate better responses using the same principles.")
	fmt.Println("This creates a feedback loop for continuous improvement.")
}

// Demonstrate the integration with the main Evaluator
func ShowIntegrationExample() {
	fmt.Println("=== Integration with Main Evaluator ===")
	
	// This shows how the response evaluation would be integrated
	// with the existing Evaluator system
	
	mockProvider := &MockProvider{}
	_ = NewEvaluator(mockProvider, WithModel("mock-model"))
	
	// The evaluator already has the new methods we added:
	// - EvaluateResponseQuality
	// - SelfImproveFixProposal
	// - StoreQualityMetrics
	
	fmt.Println("Response evaluation and self-improvement are now integrated")
	fmt.Println("into the main cognitive evaluation system.")
	
	// Example error context
	errCtx := ErrorContext{
		ErrorMessage: "Authentication failed for user",
		ErrorCount:   5,
		Category:     "security",
		Operation:    "login",
		Timestamp:    time.Now(),
		Source:       "auth.go:45",
		Severity:     "high",
		Environment:  "production",
	}
	
	// In a real scenario, this would call the LLM and then evaluate the response quality
	fmt.Printf("Processing error: %s\n", errCtx.ErrorMessage)
	fmt.Printf("Context: %s/%s (severity: %s)\n", errCtx.Category, errCtx.Operation, errCtx.Severity)
	fmt.Println("System would now call LLM for analysis, evaluate response quality, and apply improvements if needed.")
}