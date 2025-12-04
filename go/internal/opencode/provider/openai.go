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
	"strings"

	"github.com/joss/urp/internal/opencode/domain"
	"github.com/joss/urp/pkg/llm"
)

const openaiAPIURL = "https://api.openai.com/v1/chat/completions"

type OpenAI struct {
	apiKey  string
	baseURL string
	client  HTTPClient
}

func NewOpenAI(apiKey string, baseURLOverride string) *OpenAI {
	return NewOpenAIWithClient(apiKey, baseURLOverride, &http.Client{})
}

func NewOpenAIWithClient(apiKey string, baseURLOverride string, client HTTPClient) *OpenAI {
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}
	baseURL := baseURLOverride
	if baseURL == "" {
		baseURL = os.Getenv("OPENAI_BASE_URL")
	}
	if baseURL == "" {
		baseURL = openaiAPIURL
	} else {
		// Normalize: remove trailing slash
		if baseURL[len(baseURL)-1] == '/' {
			baseURL = baseURL[:len(baseURL)-1]
		}
		// Ensure it ends with /v1/chat/completions
		if !strings.HasSuffix(baseURL, "/chat/completions") {
			// If baseURL ends with /v1, append /chat/completions
			if strings.HasSuffix(baseURL, "/v1") {
				baseURL = baseURL + "/chat/completions"
			} else {
				// Otherwise, append /v1/chat/completions
				baseURL = baseURL + "/v1/chat/completions"
			}
		}
	}
	return &OpenAI{
		apiKey:  apiKey,
		baseURL: baseURL,
		client:  client,
	}
}

func NewOpenAICompatible(apiKey, baseURL string) *OpenAI {
	return NewOpenAICompatibleWithClient(apiKey, baseURL, &http.Client{})
}

func NewOpenAICompatibleWithClient(apiKey, baseURL string, client HTTPClient) *OpenAI {
	return &OpenAI{
		apiKey:  apiKey,
		baseURL: baseURL,
		client:  client,
	}
}

func (o *OpenAI) ID() string   { return "openai" }
func (o *OpenAI) Name() string { return "OpenAI" }

func (o *OpenAI) Models() []domain.Model {
	return []domain.Model{
		{ID: "gpt-4o", Name: "GPT-4o", ShortCode: "4o", ContextSize: 128000, InputCost: 2.5, OutputCost: 10},
		{ID: "gpt-4o-mini", Name: "GPT-4o Mini", ShortCode: "4om", ContextSize: 128000, InputCost: 0.15, OutputCost: 0.6},
		{ID: "gpt-4-turbo", Name: "GPT-4 Turbo", ShortCode: "4tb", ContextSize: 128000, InputCost: 10, OutputCost: 30},
		{ID: "o1", Name: "o1", ShortCode: "o1", ContextSize: 200000, InputCost: 15, OutputCost: 60},
		{ID: "o1-mini", Name: "o1 Mini", ShortCode: "o1m", ContextSize: 128000, InputCost: 3, OutputCost: 12},
	}
}

type openaiRequest struct {
	Model         string             `json:"model"`
	Messages      []openaiMessage    `json:"messages"`
	Tools         []openaiTool       `json:"tools,omitempty"`
	Stream        bool               `json:"stream"`
	StreamOptions *openaiStreamOpts  `json:"stream_options,omitempty"`
	MaxTokens     int                `json:"max_tokens,omitempty"`
	Temperature   float64            `json:"temperature,omitempty"`
}

type openaiStreamOpts struct {
	IncludeUsage bool `json:"include_usage"`
}

type openaiMessage struct {
	Role       string           `json:"role"`
	Content    any              `json:"content,omitempty"`
	ToolCalls  []openaiToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

type openaiContentPart struct {
	Type     string            `json:"type"`
	Text     string            `json:"text,omitempty"`
	ImageURL *openaiImageURL   `json:"image_url,omitempty"`
}

type openaiImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"` // "auto", "low", "high"
}

type openaiToolCall struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type openaiTool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string            `json:"name"`
		Description string            `json:"description"`
		Parameters  domain.JSONSchema `json:"parameters"`
	} `json:"function"`
}

func (o *OpenAI) Chat(ctx context.Context, req *llm.ChatRequest) (<-chan domain.StreamEvent, error) {
	// Convert messages
	msgs := make([]openaiMessage, 0, len(req.Messages)+1)

	// Add system prompt
	if req.SystemPrompt != "" {
		msgs = append(msgs, openaiMessage{Role: "system", Content: req.SystemPrompt})
	}

	for _, m := range req.Messages {
		if m.Role == domain.RoleSystem {
			continue
		}

		msg := openaiMessage{Role: string(m.Role)}
		var contentParts []openaiContentPart
		hasImage := false

		for _, p := range m.Parts {
			switch part := p.(type) {
			case domain.TextPart:
				contentParts = append(contentParts, openaiContentPart{
					Type: "text",
					Text: part.Text,
				})
			case domain.ImagePart:
				hasImage = true
				dataURL := "data:" + part.MediaType + ";base64," + part.Base64
				contentParts = append(contentParts, openaiContentPart{
					Type: "image_url",
					ImageURL: &openaiImageURL{
						URL:    dataURL,
						Detail: "auto",
					},
				})
			case domain.ToolCallPart:
				if m.Role == domain.RoleAssistant {
					msg.ToolCalls = append(msg.ToolCalls, openaiToolCall{
						ID:   part.ToolID,
						Type: "function",
						Function: struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						}{
							Name:      part.Name,
							Arguments: mustJSON(part.Args),
						},
					})
				} else {
					// Tool result - needs separate message
					msgs = append(msgs, openaiMessage{
						Role:       "tool",
						Content:    part.Result,
						ToolCallID: part.ToolID,
					})
					continue
				}
			}
		}

		// Use array format when there are images, string when just text
		if hasImage || len(contentParts) > 1 {
			msg.Content = contentParts
		} else if len(contentParts) == 1 {
			msg.Content = contentParts[0].Text
		}

		if msg.Content != nil || len(msg.ToolCalls) > 0 {
			msgs = append(msgs, msg)
		}
	}

	// Convert tools
	var tools []openaiTool
	for _, t := range req.Tools {
		tool := openaiTool{Type: "function"}
		tool.Function.Name = t.Name
		tool.Function.Description = t.Description
		tool.Function.Parameters = t.Parameters
		tools = append(tools, tool)
	}

	reqBody := map[string]interface{}{
		"model":    req.Model,
		"messages": msgs,
		"stream":   true,
	}

	if len(tools) > 0 {
		reqBody["tools"] = tools
	}

	if req.MaxTokens > 0 {
		// Newer O1/GPT-5 models require max_completion_tokens
		if strings.HasPrefix(req.Model, "o1") || strings.HasPrefix(req.Model, "gpt-5") {
			reqBody["max_completion_tokens"] = req.MaxTokens
		} else {
			reqBody["max_tokens"] = req.MaxTokens
		}
	}

	if req.Temperature > 0 {
		reqBody["temperature"] = req.Temperature
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", o.baseURL, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+o.apiKey)

	resp, err := o.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("OpenAI API error %d: %s", resp.StatusCode, string(body))
	}

	events := make(chan domain.StreamEvent, 100)
	go o.streamResponse(resp.Body, events)
	return events, nil
}

type openaiStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content   string           `json:"content"`
			ToolCalls []openaiToolCall `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *openaiUsage `json:"usage,omitempty"`
}

type openaiUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

func (o *OpenAI) streamResponse(body io.ReadCloser, events chan<- domain.StreamEvent) {
	defer close(events)
	defer body.Close()

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	toolCalls := make(map[int]*domain.ToolCallPart)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		if len(line) > 6 && line[:6] == "data: " {
			data := line[6:]
			if data == "[DONE]" {
				events <- domain.StreamEvent{Type: domain.StreamEventDone, Done: true}
				return
			}

			var chunk openaiStreamChunk
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}

			// Emit usage if present (OpenAI sends in final chunk with stream_options.include_usage)
			if chunk.Usage != nil {
				events <- domain.StreamEvent{
					Type: domain.StreamEventUsage,
					Usage: &domain.Usage{
						InputTokens:  chunk.Usage.PromptTokens,
						OutputTokens: chunk.Usage.CompletionTokens,
					},
				}
			}

			for _, choice := range chunk.Choices {
				if choice.Delta.Content != "" {
					events <- domain.StreamEvent{
						Type:    domain.StreamEventText,
						Content: choice.Delta.Content,
					}
				}

				for _, tc := range choice.Delta.ToolCalls {
					if tc.ID != "" {
						toolCalls[tc.Index] = &domain.ToolCallPart{
							ToolID: tc.ID,
							Name:   tc.Function.Name,
						}
					}
					if tc.Function.Arguments != "" {
						if call, ok := toolCalls[tc.Index]; ok {
							if call.Args == nil {
								call.Args = make(map[string]any)
							}
							// Accumulate arguments
							var args map[string]any
							json.Unmarshal([]byte(tc.Function.Arguments), &args)
							for k, v := range args {
								call.Args[k] = v
							}
						}
					}
				}

				if choice.FinishReason == "tool_calls" {
					for _, call := range toolCalls {
						events <- domain.StreamEvent{
							Type: domain.StreamEventToolCall,
							Part: *call,
						}
					}
				}

				if choice.FinishReason == "stop" {
					events <- domain.StreamEvent{Type: domain.StreamEventDone, Done: true}
					return
				}
			}
		}
	}
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

var _ llm.Provider = (*OpenAI)(nil)
