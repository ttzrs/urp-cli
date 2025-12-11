// Package cognitive provides AI-powered analysis and self-improvement tools.
package cognitive

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/joss/urp/internal/opencode/domain"
	"github.com/joss/urp/pkg/llm"
)

// ResponseEvaluator analyzes LLM responses for quality and pattern compliance.
type ResponseEvaluator struct {
	provider llm.Provider
	model    string
}

// ResponseEvaluation contains the analysis of an LLM response.
type ResponseEvaluation struct {
	Score           float64         // 0.0-1.0, higher is better
	PatternCompliance bool          // Whether response follows expected patterns
	Issues          []string        // List of detected issues
	Suggestions     []string        // Suggestions for improvement
	Confidence      string          // high/medium/low confidence in evaluation
	Analysis        string          // Detailed analysis
	Timestamp       time.Time       // When evaluation was performed
}

// Pattern defines expected response patterns.
type Pattern struct {
	Name        string          // Name of the pattern
	Description string          // What the pattern checks for
	Regex       *regexp.Regexp  // Regular expression to match
	Required    bool           // Whether this pattern is required
	Weight      float64        // How much this pattern affects the score
}

// NewResponseEvaluator creates a response evaluator with the given LLM provider.
func NewResponseEvaluator(p llm.Provider, model string) *ResponseEvaluator {
	return &ResponseEvaluator{
		provider: p,
		model:    model,
	}
}

// DefaultPatterns returns common patterns that LLM responses should follow.
func DefaultPatterns() []Pattern {
	patterns := []Pattern{
		{
			Name:        "structured_format",
			Description: "Response follows a structured format with clear sections",
			Regex:       regexp.MustCompile(`(?i)(?s)(analysis|root cause|problem|issue):.*?(\n\n|\n[A-Z][a-z]+:)`),
			Required:    true,
			Weight:      0.3,
		},
		{
			Name:        "actionable_content",
			Description: "Response contains actionable information",
			Regex:       regexp.MustCompile(`(?i)(should|must|need to|recommend|suggest|propose|fix|solution|approach)`),
			Required:    true,
			Weight:      0.25,
		},
		{
			Name:        "evidence_based",
			Description: "Response provides evidence or reasoning",
			Regex:       regexp.MustCompile(`(?i)(because|since|due to|as a result|therefore|implies|indicates|shows)`),
			Required:    false,
			Weight:      0.15,
		},
		{
			Name:        "specific_details",
			Description: "Response includes specific details rather than generic advice",
			Regex:       regexp.MustCompile(`(?i)(file:|line:|function:|method:|class:|variable:|parameter:)`),
			Required:    false,
			Weight:      0.15,
		},
		{
			Name:        "no_vague_responses",
			Description: "Response avoids vague terms",
			Regex:       regexp.MustCompile(`(?i)(something|somewhere|someone|somehow|maybe|perhaps|possibly|might|could)`),
			Required:    false,
			Weight:      -0.1, // Negative weight - indicates issue
		},
	}

	return patterns
}

// EvaluateResponse analyzes an LLM response against expected patterns.
func (re *ResponseEvaluator) EvaluateResponse(ctx context.Context, response string) (*ResponseEvaluation, error) {
	if re.provider == nil {
		return nil, fmt.Errorf("no LLM provider configured")
	}

	eval := &ResponseEvaluation{
		Timestamp: time.Now(),
	}

	// Apply pattern matching
	patterns := DefaultPatterns()
	score := 1.0 // Start with perfect score
	issues := []string{}
	suggestions := []string{}

	for _, pattern := range patterns {
		matches := pattern.Regex.MatchString(response)

		if pattern.Required && !matches {
			issues = append(issues, fmt.Sprintf("Missing required pattern: %s (%s)", pattern.Name, pattern.Description))
			score -= pattern.Weight
		} else if matches {
			if pattern.Weight > 0 {
				score += pattern.Weight
			} else {
				// Negative weight means this is an issue when present
				issues = append(issues, fmt.Sprintf("Detected issue pattern: %s (%s)", pattern.Name, pattern.Description))
				score += pattern.Weight // Negative weight will reduce score
			}
		}
	}

	// Normalize score to 0.0-1.0 range
	if score > 1.0 {
		score = 1.0
	} else if score < 0.0 {
		score = 0.0
	}

	eval.Score = score
	eval.PatternCompliance = score > 0.7 // Consider compliant if score > 0.7
	eval.Issues = issues

	// Add suggestions based on issues
	for _, issue := range issues {
		if strings.Contains(issue, "Missing required pattern") {
			if strings.Contains(issue, "structured_format") {
				suggestions = append(suggestions, "Structure your response with clear sections like 'Analysis:', 'Solution:', and 'Next Steps:'")
			} else if strings.Contains(issue, "actionable_content") {
				suggestions = append(suggestions, "Provide specific, actionable recommendations")
			}
		}
	}
	eval.Suggestions = suggestions

	// Determine confidence level
	if score >= 0.8 {
		eval.Confidence = "high"
	} else if score >= 0.5 {
		eval.Confidence = "medium"
	} else {
		eval.Confidence = "low"
	}

	// Perform AI-powered analysis if score is low
	if score < 0.5 {
		aiAnalysis, err := re.analyzeWithAI(ctx, response)
		if err != nil {
			// If AI analysis fails, provide basic analysis
			eval.Analysis = fmt.Sprintf("Low-scoring response detected (score: %.2f). Contains %d issues.", score, len(issues))
		} else {
			eval.Analysis = aiAnalysis
		}
	} else {
		eval.Analysis = fmt.Sprintf("Response quality is acceptable (score: %.2f).", score)
	}

	return eval, nil
}

// analyzeWithAI uses an LLM to provide detailed analysis of a response.
func (re *ResponseEvaluator) analyzeWithAI(ctx context.Context, response string) (string, error) {
	prompt := `Analyze this LLM response and provide feedback on its quality:

RESPONSE TO ANALYZE:
%s

Provide feedback focusing on:
1. Structure and organization
2. Actionability of the content
3. Specificity vs vagueness
4. Logical flow and reasoning
5. Missing elements that would improve the response

Return your analysis in this format:
STRUCTURE: <comment>
ACTIONABILITY: <comment>
SPECIFICITY: <comment>
REASONING: <comment>
MISSING: <comma-separated list of improvements>

SCORE: <0-10>`

	req := &llm.ChatRequest{
		Model:        re.model,
		SystemPrompt: prompt,
		Messages: []domain.Message{
			{
				Role:  domain.RoleUser,
				Parts: []domain.Part{domain.TextPart{Text: response}},
			},
		},
		MaxTokens:   512,
		Temperature: 0.2,
	}

	events, err := re.provider.Chat(ctx, req)
	if err != nil {
		return "", err
	}

	var analysis strings.Builder
	for event := range events {
		switch event.Type {
		case domain.StreamEventText:
			analysis.WriteString(event.Content)
		case domain.StreamEventError:
			return "", event.Error
		}
	}

	return analysis.String(), nil
}

// SelfImprove evaluates a response and suggests improvements for future responses.
func (re *ResponseEvaluator) SelfImprove(ctx context.Context, response string, contextInfo string) (*ImprovementPlan, error) {
	eval, err := re.EvaluateResponse(ctx, response)
	if err != nil {
		return nil, err
	}

	// If response is already good, no improvement needed
	if eval.Score >= 0.8 {
		return &ImprovementPlan{
			NeedsImprovement: false,
			CurrentScore:     eval.Score,
			Suggestions:      []string{"Response quality is good, no major improvements needed"},
		}, nil
	}

	// Generate improvement suggestions using AI
	prompt := fmt.Sprintf(`Based on this context and response, suggest improvements for future responses:

CONTEXT: %s

RESPONSE: %s

EVALUATION: %s

Current score: %.2f

Suggest specific improvements to increase the response quality and make it more helpful.`, contextInfo, response, eval.Analysis, eval.Score)

	req := &llm.ChatRequest{
		Model:        re.model,
		SystemPrompt: prompt,
		Messages: []domain.Message{
			{
				Role:  domain.RoleUser,
				Parts: []domain.Part{domain.TextPart{Text: "Provide specific improvement suggestions."}},
			},
		},
		MaxTokens:   512,
		Temperature: 0.3,
	}

	events, err := re.provider.Chat(ctx, req)
	if err != nil {
		return nil, err
	}

	var suggestions strings.Builder
	for event := range events {
		switch event.Type {
		case domain.StreamEventText:
			suggestions.WriteString(event.Content)
		case domain.StreamEventError:
			return nil, event.Error
		}
	}

	return &ImprovementPlan{
		NeedsImprovement: true,
		CurrentScore:     eval.Score,
		Analysis:         eval.Analysis,
		Suggestions:      []string{suggestions.String()},
	}, nil
}

// ImprovementPlan contains suggestions for improving response quality.
type ImprovementPlan struct {
	NeedsImprovement bool
	CurrentScore     float64
	Analysis         string
	Suggestions      []string
	NextSteps        []string
}

// EvaluateAndImprove combines evaluation and improvement suggestion.
func (re *ResponseEvaluator) EvaluateAndImprove(ctx context.Context, response, contextInfo string) (*ResponseEvaluation, *ImprovementPlan, error) {
	eval, err := re.EvaluateResponse(ctx, response)
	if err != nil {
		return nil, nil, err
	}

	plan, err := re.SelfImprove(ctx, response, contextInfo)
	if err != nil {
		return eval, nil, err
	}

	return eval, plan, nil
}