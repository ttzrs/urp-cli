package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/joss/urp/internal/opencode/domain"
	"github.com/joss/urp/pkg/llm"
)

const googleAPIURL = "https://generativelanguage.googleapis.com/v1beta/models"

type Google struct {
	apiKey string
	client HTTPClient
}

func NewGoogle(apiKey string) *Google {
	return NewGoogleWithClient(apiKey, &http.Client{})
}

func NewGoogleWithClient(apiKey string, client HTTPClient) *Google {
	if apiKey == "" {
		apiKey = os.Getenv("GOOGLE_API_KEY")
		if apiKey == "" {
			apiKey = os.Getenv("GEMINI_API_KEY")
		}
	}
	return &Google{
		apiKey: apiKey,
		client: client,
	}
}

func (g *Google) ID() string   { return "google" }
func (g *Google) Name() string { return "Google" }

func (g *Google) Models() []domain.Model {
	return []domain.Model{
		{ID: "gemini-2.0-flash-exp", Name: "Gemini 2.0 Flash", ContextSize: 1000000, InputCost: 0, OutputCost: 0},
		{ID: "gemini-1.5-pro", Name: "Gemini 1.5 Pro", ContextSize: 2000000, InputCost: 1.25, OutputCost: 5},
		{ID: "gemini-1.5-flash", Name: "Gemini 1.5 Flash", ContextSize: 1000000, InputCost: 0.075, OutputCost: 0.3},
		{ID: "gemini-1.5-flash-8b", Name: "Gemini 1.5 Flash 8B", ContextSize: 1000000, InputCost: 0.0375, OutputCost: 0.15},
	}
}

type googleRequest struct {
	Contents         []googleContent       `json:"contents"`
	SystemInstruction *googleContent       `json:"systemInstruction,omitempty"`
	Tools            []googleToolDef       `json:"tools,omitempty"`
	GenerationConfig *googleGenConfig      `json:"generationConfig,omitempty"`
}

type googleContent struct {
	Role  string       `json:"role"`
	Parts []googlePart `json:"parts"`
}

type googlePart struct {
	Text             string                `json:"text,omitempty"`
	FunctionCall     *googleFunctionCall   `json:"functionCall,omitempty"`
	FunctionResponse *googleFunctionResp   `json:"functionResponse,omitempty"`
}

type googleFunctionCall struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

type googleFunctionResp struct {
	Name     string         `json:"name"`
	Response map[string]any `json:"response"`
}

type googleToolDef struct {
	FunctionDeclarations []googleFuncDecl `json:"functionDeclarations"`
}

type googleFuncDecl struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Parameters  domain.JSONSchema `json:"parameters"`
}

type googleGenConfig struct {
	MaxOutputTokens int     `json:"maxOutputTokens,omitempty"`
	Temperature     float64 `json:"temperature,omitempty"`
}

func (g *Google) Chat(ctx context.Context, req *llm.ChatRequest) (<-chan domain.StreamEvent, error) {
	// Convert messages
	var contents []googleContent
	var systemContent *googleContent

	for _, m := range req.Messages {
		role := "user"
		if m.Role == domain.RoleAssistant {
			role = "model"
		} else if m.Role == domain.RoleSystem {
			// Handle system as systemInstruction
			for _, p := range m.Parts {
				if tp, ok := p.(domain.TextPart); ok {
					systemContent = &googleContent{
						Role:  "user",
						Parts: []googlePart{{Text: tp.Text}},
					}
				}
			}
			continue
		}

		var parts []googlePart
		for _, p := range m.Parts {
			switch part := p.(type) {
			case domain.TextPart:
				parts = append(parts, googlePart{Text: part.Text})
			case domain.ToolCallPart:
				if m.Role == domain.RoleAssistant {
					parts = append(parts, googlePart{
						FunctionCall: &googleFunctionCall{
							Name: part.Name,
							Args: part.Args,
						},
					})
				} else {
					// Tool result
					parts = append(parts, googlePart{
						FunctionResponse: &googleFunctionResp{
							Name: part.Name,
							Response: map[string]any{
								"result": part.Result,
							},
						},
					})
				}
			}
		}

		if len(parts) > 0 {
			contents = append(contents, googleContent{Role: role, Parts: parts})
		}
	}

	// Add system instruction from request if not in messages
	if systemContent == nil && req.SystemPrompt != "" {
		systemContent = &googleContent{
			Role:  "user",
			Parts: []googlePart{{Text: req.SystemPrompt}},
		}
	}

	// Convert tools
	var tools []googleToolDef
	if len(req.Tools) > 0 {
		var funcs []googleFuncDecl
		for _, t := range req.Tools {
			funcs = append(funcs, googleFuncDecl{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			})
		}
		tools = append(tools, googleToolDef{FunctionDeclarations: funcs})
	}

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 8192
	}

	body := googleRequest{
		Contents:          contents,
		SystemInstruction: systemContent,
		Tools:             tools,
		GenerationConfig: &googleGenConfig{
			MaxOutputTokens: maxTokens,
			Temperature:     req.Temperature,
		},
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/%s:streamGenerateContent?alt=sse&key=%s", googleAPIURL, req.Model, g.apiKey)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("Google API error %d: %s", resp.StatusCode, string(body))
	}

	events := make(chan domain.StreamEvent, 100)
	go g.streamResponse(resp.Body, events)
	return events, nil
}

type googleStreamResponse struct {
	Candidates []struct {
		Content struct {
			Parts []googlePart `json:"parts"`
			Role  string       `json:"role"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	UsageMetadata *googleUsage `json:"usageMetadata,omitempty"`
}

type googleUsage struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

func (g *Google) streamResponse(body io.ReadCloser, events chan<- domain.StreamEvent) {
	defer close(events)
	defer body.Close()

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		if len(line) > 6 && line[:6] == "data: " {
			data := line[6:]

			var resp googleStreamResponse
			if err := json.Unmarshal([]byte(data), &resp); err != nil {
				continue
			}

			// Emit usage if present
			if resp.UsageMetadata != nil {
				events <- domain.StreamEvent{
					Type: domain.StreamEventUsage,
					Usage: &domain.Usage{
						InputTokens:  resp.UsageMetadata.PromptTokenCount,
						OutputTokens: resp.UsageMetadata.CandidatesTokenCount,
					},
				}
			}

			for _, candidate := range resp.Candidates {
				for _, part := range candidate.Content.Parts {
					if part.Text != "" {
						events <- domain.StreamEvent{
							Type:    domain.StreamEventText,
							Content: part.Text,
						}
					}

					if part.FunctionCall != nil {
						events <- domain.StreamEvent{
							Type: domain.StreamEventToolCall,
							Part: domain.ToolCallPart{
								ToolID: part.FunctionCall.Name, // Google uses name as ID
								Name:   part.FunctionCall.Name,
								Args:   part.FunctionCall.Args,
							},
						}
					}
				}

				if candidate.FinishReason == "STOP" || candidate.FinishReason == "MAX_TOKENS" {
					events <- domain.StreamEvent{Type: domain.StreamEventDone, Done: true}
					return
				}
			}
		}
	}
}

var _ llm.Provider = (*Google)(nil)
