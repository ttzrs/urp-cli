// Package cognitive provides AI-powered analysis tools.
package cognitive

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/joss/urp/internal/opencode/domain"
	"github.com/joss/urp/internal/opencode/provider"
	"github.com/joss/urp/pkg/llm"
)

// Evaluator analyzes errors and proposes fixes using LLM.
type Evaluator struct {
	provider llm.Provider
	model    string
}

// EvaluatorOption configures Evaluator.
type EvaluatorOption func(*Evaluator)

// WithModel sets the model to use for evaluation.
func WithModel(model string) EvaluatorOption {
	return func(e *Evaluator) { e.model = model }
}

// NewEvaluator creates an evaluator with the given LLM provider.
func NewEvaluator(p llm.Provider, opts ...EvaluatorOption) *Evaluator {
	e := &Evaluator{
		provider: p,
		model:    "claude-sonnet-4-5-20250929", // Default model
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// DefaultEvaluator creates an evaluator using environment configuration.
// Tries Anthropic first, then OpenAI.
func DefaultEvaluator() (*Evaluator, error) {
	var p llm.Provider

	// Try Anthropic first (Claude is better at code analysis)
	if apiKey := os.Getenv("ANTHROPIC_API_KEY"); apiKey != "" {
		baseURL := os.Getenv("ANTHROPIC_BASE_URL")
		p = provider.NewAnthropic(apiKey, baseURL)
	} else if apiKey := os.Getenv("OPENAI_API_KEY"); apiKey != "" {
		// Fallback to OpenAI
		baseURL := os.Getenv("OPENAI_BASE_URL")
		p = provider.NewOpenAI(apiKey, baseURL)
	}

	if p == nil {
		return nil, fmt.Errorf("no LLM provider available: set ANTHROPIC_API_KEY or OPENAI_API_KEY")
	}

	model := os.Getenv("URP_MODEL")
	if model == "" {
		model = "claude-sonnet-4-5-20250929"
	}

	return NewEvaluator(p, WithModel(model)), nil
}

// ErrorContext contains information about an error and its code context.
type ErrorContext struct {
	ErrorMessage string
	ErrorCount   int
	Category     string
	Operation    string
	Files        []OptimizedFile
	Timestamp    time.Time
}

// FixProposal is the LLM's analysis and proposed fix.
type FixProposal struct {
	Analysis    string   // Root cause analysis
	Proposal    string   // Suggested fix
	Files       []string // Files to modify
	Confidence  string   // high/medium/low
	RawResponse string   // Full LLM response
}

// ProposeFixPrompt is the system prompt for error analysis.
const ProposeFixPrompt = `You are an expert software engineer analyzing errors in a codebase.

Given an error message and related code files, provide:
1. ROOT CAUSE: Brief analysis of why this error occurred
2. PROPOSAL: Specific fix with code changes needed
3. FILES: List files that need modification
4. CONFIDENCE: Your confidence level (high/medium/low)

Be concise but specific. Focus on actionable fixes.
Output format:
ROOT CAUSE: <analysis>
PROPOSAL: <fix>
FILES: <comma-separated list>
CONFIDENCE: <level>`

// ProposeFix analyzes an error and proposes a fix.
func (e *Evaluator) ProposeFix(ctx context.Context, errCtx ErrorContext) (*FixProposal, error) {
	if e.provider == nil {
		return nil, fmt.Errorf("no LLM provider configured")
	}

	// Build the user message
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("ERROR (%d occurrences):\n%s\n\n", errCtx.ErrorCount, errCtx.ErrorMessage))

	if errCtx.Category != "" || errCtx.Operation != "" {
		sb.WriteString(fmt.Sprintf("Context: %s/%s\n\n", errCtx.Category, errCtx.Operation))
	}

	if len(errCtx.Files) > 0 {
		sb.WriteString("RELATED FILES (by spreading activation energy):\n")
		for _, f := range errCtx.Files {
			sb.WriteString(fmt.Sprintf("- [%.2f] %s\n", f.Energy, f.Path))
		}
	}

	// Create request
	req := &llm.ChatRequest{
		Model:        e.model,
		SystemPrompt: ProposeFixPrompt,
		Messages: []domain.Message{
			{
				Role:  domain.RoleUser,
				Parts: []domain.Part{domain.TextPart{Text: sb.String()}},
			},
		},
		MaxTokens:   1024,
		Temperature: 0.3, // Lower temperature for more focused analysis
	}

	// Get streaming response
	events, err := e.provider.Chat(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("chat request: %w", err)
	}

	// Consume stream and collect text
	var response strings.Builder
	for event := range events {
		switch event.Type {
		case domain.StreamEventText:
			response.WriteString(event.Content)
		case domain.StreamEventError:
			return nil, fmt.Errorf("stream error: %w", event.Error)
		}
	}

	rawResponse := response.String()
	proposal := parseFixProposal(rawResponse)
	proposal.RawResponse = rawResponse

	return proposal, nil
}

// parseFixProposal extracts structured data from LLM response.
func parseFixProposal(response string) *FixProposal {
	p := &FixProposal{}

	lines := strings.Split(response, "\n")
	var currentSection string

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "ROOT CAUSE:") {
			currentSection = "analysis"
			p.Analysis = strings.TrimPrefix(line, "ROOT CAUSE:")
			p.Analysis = strings.TrimSpace(p.Analysis)
		} else if strings.HasPrefix(line, "PROPOSAL:") {
			currentSection = "proposal"
			p.Proposal = strings.TrimPrefix(line, "PROPOSAL:")
			p.Proposal = strings.TrimSpace(p.Proposal)
		} else if strings.HasPrefix(line, "FILES:") {
			currentSection = "files"
			filesStr := strings.TrimPrefix(line, "FILES:")
			filesStr = strings.TrimSpace(filesStr)
			if filesStr != "" {
				for _, f := range strings.Split(filesStr, ",") {
					f = strings.TrimSpace(f)
					if f != "" {
						p.Files = append(p.Files, f)
					}
				}
			}
		} else if strings.HasPrefix(line, "CONFIDENCE:") {
			currentSection = "confidence"
			p.Confidence = strings.TrimPrefix(line, "CONFIDENCE:")
			p.Confidence = strings.TrimSpace(strings.ToLower(p.Confidence))
		} else if line != "" && currentSection != "" {
			// Continuation of previous section
			switch currentSection {
			case "analysis":
				p.Analysis += " " + line
			case "proposal":
				p.Proposal += " " + line
			}
		}
	}

	// Default confidence if not parsed
	if p.Confidence == "" {
		p.Confidence = "medium"
	}

	return p
}
