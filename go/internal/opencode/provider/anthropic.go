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

const (
	anthropicAPIURL = "https://api.anthropic.com/v1/messages"
	anthropicVersion = "2023-06-01"
)

type Anthropic struct {
	apiKey string
	client HTTPClient
}

func NewAnthropic(apiKey string) *Anthropic {
	return NewAnthropicWithClient(apiKey, &http.Client{})
}

func NewAnthropicWithClient(apiKey string, client HTTPClient) *Anthropic {
	if apiKey == "" {
		apiKey = os.Getenv("ANTHROPIC_API_KEY")
	}
	return &Anthropic{
		apiKey: apiKey,
		client: client,
	}
}

func (a *Anthropic) ID() string   { return "anthropic" }
func (a *Anthropic) Name() string { return "Anthropic" }

func (a *Anthropic) Models() []domain.Model {
	return []domain.Model{
		{ID: "claude-sonnet-4-20250514", Name: "Claude Sonnet 4", ContextSize: 200000, InputCost: 3, OutputCost: 15},
		{ID: "claude-opus-4-20250514", Name: "Claude Opus 4", ContextSize: 200000, InputCost: 15, OutputCost: 75},
		{ID: "claude-3-5-sonnet-20241022", Name: "Claude 3.5 Sonnet", ContextSize: 200000, InputCost: 3, OutputCost: 15},
		{ID: "claude-3-5-haiku-20241022", Name: "Claude 3.5 Haiku", ContextSize: 200000, InputCost: 0.8, OutputCost: 4},
	}
}

type anthropicRequest struct {
	Model       string              `json:"model"`
	MaxTokens   int                 `json:"max_tokens"`
	System      []systemContent     `json:"system,omitempty"`
	Messages    []anthropicMessage  `json:"messages"`
	Tools       []anthropicTool     `json:"tools,omitempty"`
	Stream      bool                `json:"stream"`
	Temperature float64             `json:"temperature,omitempty"`
	Thinking    *thinkingConfig     `json:"thinking,omitempty"`
}

type thinkingConfig struct {
	Type         string `json:"type"`          // "enabled"
	BudgetTokens int    `json:"budget_tokens"` // max tokens for thinking
}

type systemContent struct {
	Type         string        `json:"type"`
	Text         string        `json:"text"`
	CacheControl *cacheControl `json:"cache_control,omitempty"`
}

type cacheControl struct {
	Type string `json:"type"` // "ephemeral"
}

type anthropicMessage struct {
	Role    string        `json:"role"`
	Content []contentPart `json:"content"`
}

type contentPart struct {
	Type      string         `json:"type"`
	Text      string         `json:"text,omitempty"`
	ID        string         `json:"id,omitempty"`
	Name      string         `json:"name,omitempty"`
	Input     map[string]any `json:"input,omitempty"`
	ToolUseID string         `json:"tool_use_id,omitempty"`
	Content   string         `json:"content,omitempty"`
	Source    *imageSource   `json:"source,omitempty"`
}

type imageSource struct {
	Type      string `json:"type"`       // "base64"
	MediaType string `json:"media_type"` // "image/png", "image/jpeg", etc
	Data      string `json:"data"`       // base64 encoded
}

type anthropicTool struct {
	Name        string              `json:"name"`
	Description string              `json:"description"`
	InputSchema domain.JSONSchema   `json:"input_schema"`
}

type streamEvent struct {
	Type  string          `json:"type"`
	Index int             `json:"index,omitempty"`
	Delta json.RawMessage `json:"delta,omitempty"`
	ContentBlock *struct {
		Type      string `json:"type"`
		ID        string `json:"id,omitempty"`
		Name      string `json:"name,omitempty"`
		Thinking  string `json:"thinking,omitempty"`
	} `json:"content_block,omitempty"`
	Usage *anthropicUsage `json:"usage,omitempty"`
}

type anthropicUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
}

type textDelta struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type inputJSONDelta struct {
	Type        string `json:"type"`
	PartialJSON string `json:"partial_json"`
}

func (a *Anthropic) Chat(ctx context.Context, req *llm.ChatRequest) (<-chan domain.StreamEvent, error) {
	// Convert messages
	msgs := make([]anthropicMessage, 0, len(req.Messages))
	for _, m := range req.Messages {
		if m.Role == domain.RoleSystem {
			continue // System handled separately
		}

		var content []contentPart
		for _, p := range m.Parts {
			switch part := p.(type) {
			case domain.TextPart:
				content = append(content, contentPart{Type: "text", Text: part.Text})
			case domain.ImagePart:
				content = append(content, contentPart{
					Type: "image",
					Source: &imageSource{
						Type:      "base64",
						MediaType: part.MediaType,
						Data:      part.Base64,
					},
				})
			case domain.ToolCallPart:
				if m.Role == domain.RoleAssistant {
					content = append(content, contentPart{
						Type:  "tool_use",
						ID:    part.ToolID,
						Name:  part.Name,
						Input: part.Args,
					})
				} else {
					// Tool result from user
					content = append(content, contentPart{
						Type:      "tool_result",
						ToolUseID: part.ToolID,
						Content:   part.Result,
					})
				}
			}
		}

		if len(content) > 0 {
			msgs = append(msgs, anthropicMessage{
				Role:    string(m.Role),
				Content: content,
			})
		}
	}

	// Convert tools
	var tools []anthropicTool
	for _, t := range req.Tools {
		tools = append(tools, anthropicTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.Parameters,
		})
	}

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 8192
	}

	// Build system content with caching
	var system []systemContent
	if req.SystemPrompt != "" {
		system = []systemContent{
			{
				Type:         "text",
				Text:         req.SystemPrompt,
				CacheControl: &cacheControl{Type: "ephemeral"},
			},
		}
	}

	body := anthropicRequest{
		Model:       req.Model,
		MaxTokens:   maxTokens,
		System:      system,
		Messages:    msgs,
		Tools:       tools,
		Stream:      true,
		Temperature: req.Temperature,
	}

	// Enable extended thinking if budget > 0
	if req.ThinkingBudget > 0 {
		body.Thinking = &thinkingConfig{
			Type:         "enabled",
			BudgetTokens: req.ThinkingBudget,
		}
		// Temperature must be 1 when thinking is enabled
		body.Temperature = 1
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", anthropicAPIURL, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", a.apiKey)
	httpReq.Header.Set("anthropic-version", anthropicVersion)

	// Beta features: prompt caching and extended thinking
	betaFeatures := "prompt-caching-2024-07-31"
	if req.ThinkingBudget > 0 {
		betaFeatures += ",extended-thinking-2025-04-11"
	}
	httpReq.Header.Set("anthropic-beta", betaFeatures)

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("anthropic API error %d: %s", resp.StatusCode, string(body))
	}

	events := make(chan domain.StreamEvent, 100)
	go a.streamResponse(resp.Body, events)
	return events, nil
}

func (a *Anthropic) streamResponse(body io.ReadCloser, events chan<- domain.StreamEvent) {
	defer close(events)
	defer body.Close()

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	var currentToolID, currentToolName string
	var toolInputBuffer bytes.Buffer
	var inThinkingBlock bool

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || line == "event: ping" {
			continue
		}

		if len(line) > 6 && line[:6] == "data: " {
			data := line[6:]
			if data == "[DONE]" {
				events <- domain.StreamEvent{Type: domain.StreamEventDone, Done: true}
				return
			}

			var event streamEvent
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				continue
			}

			switch event.Type {
			case "content_block_start":
				if event.ContentBlock != nil {
					switch event.ContentBlock.Type {
					case "tool_use":
						currentToolID = event.ContentBlock.ID
						currentToolName = event.ContentBlock.Name
						toolInputBuffer.Reset()
					case "thinking":
						inThinkingBlock = true
					}
				}

			case "content_block_delta":
				var delta struct {
					Type        string `json:"type"`
					Text        string `json:"text"`
					Thinking    string `json:"thinking"`
					PartialJSON string `json:"partial_json"`
				}
				json.Unmarshal(event.Delta, &delta)

				switch delta.Type {
				case "text_delta":
					if delta.Text != "" {
						events <- domain.StreamEvent{
							Type:    domain.StreamEventText,
							Content: delta.Text,
						}
					}
				case "thinking_delta":
					if delta.Thinking != "" {
						events <- domain.StreamEvent{
							Type:    domain.StreamEventThinking,
							Content: delta.Thinking,
						}
					}
				case "input_json_delta":
					if delta.PartialJSON != "" {
						toolInputBuffer.WriteString(delta.PartialJSON)
					}
				}

			case "content_block_stop":
				if currentToolID != "" {
					var args map[string]any
					json.Unmarshal(toolInputBuffer.Bytes(), &args)

					events <- domain.StreamEvent{
						Type: domain.StreamEventToolCall,
						Part: domain.ToolCallPart{
							ToolID: currentToolID,
							Name:   currentToolName,
							Args:   args,
						},
					}
					currentToolID = ""
					currentToolName = ""
				}
				inThinkingBlock = false

			case "message_stop":
				events <- domain.StreamEvent{Type: domain.StreamEventDone, Done: true}
				return

			case "message_delta":
				// Final usage stats
				if event.Usage != nil {
					events <- domain.StreamEvent{
						Type: domain.StreamEventUsage,
						Usage: &domain.Usage{
							InputTokens:  event.Usage.InputTokens,
							OutputTokens: event.Usage.OutputTokens,
							CacheRead:    event.Usage.CacheReadInputTokens,
							CacheWrite:   event.Usage.CacheCreationInputTokens,
						},
					}
				}
			}
		}
	}
	_ = inThinkingBlock // reserved for future use
}

var _ llm.Provider = (*Anthropic)(nil)
