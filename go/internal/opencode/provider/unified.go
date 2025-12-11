// Package provider implements a unified LLM provider for proxy gateways
// Compatible with LiteLLM, OpenRouter, cliploxyapi, and similar OpenAI-compatible proxies
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
	"time"

	"github.com/joss/urp/internal/opencode/model"
	"github.com/joss/urp/internal/opencode/domain"
	"github.com/joss/urp/pkg/llm"
)

// UnifiedProvider is a single provider that routes to multiple backends via a proxy
// It uses OpenAI-compatible API format and the ModelRegistry for model metadata
type UnifiedProvider struct {
	apiKey   string
	baseURL  string
	client   HTTPClient
	registry *model.ModelRegistry
}

// NewUnifiedProvider creates a unified provider for a proxy gateway
func NewUnifiedProvider(apiKey, baseURL string, registry *model.ModelRegistry) *UnifiedProvider {
	if apiKey == "" {
		apiKey = os.Getenv("UNIFIED_API_KEY")
		if apiKey == "" {
			apiKey = os.Getenv("PROXY_API_KEY")
		}
	}
	if baseURL == "" {
		baseURL = os.Getenv("UNIFIED_BASE_URL")
		if baseURL == "" {
			baseURL = os.Getenv("PROXY_BASE_URL")
		}
	}

	// Normalize URL
	if baseURL != "" {
		baseURL = strings.TrimSuffix(baseURL, "/")
		if !strings.HasSuffix(baseURL, "/chat/completions") {
			if strings.HasSuffix(baseURL, "/v1") {
				baseURL = baseURL + "/chat/completions"
			} else {
				baseURL = baseURL + "/v1/chat/completions"
			}
		}
	}

	if registry == nil {
		registry = model.DefaultModelRegistry
	}

	transport := &http.Transport{
		MaxIdleConns:        10,
		MaxIdleConnsPerHost: 5,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  true,
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   5 * time.Minute,
	}

	return &UnifiedProvider{
		apiKey:   apiKey,
		baseURL:  baseURL,
		client:   client,
		registry: registry,
	}
}

// NewUnifiedProviderWithClient creates a unified provider with custom HTTP client
func NewUnifiedProviderWithClient(apiKey, baseURL string, registry *model.ModelRegistry, client HTTPClient) *UnifiedProvider {
	p := NewUnifiedProvider(apiKey, baseURL, registry)
	p.client = client
	return p
}

func (u *UnifiedProvider) ID() string   { return "unified" }
func (u *UnifiedProvider) Name() string { return "Unified Proxy" }

// Models returns all enabled models from the registry
func (u *UnifiedProvider) Models() []domain.Model {
	models := u.registry.ListEnabled()
	result := make([]domain.Model, 0, len(models))
	for _, m := range models {
		result = append(result, domain.Model{
			ID:          m.ID,
			Name:        m.ID,
			ShortCode:   shortCode(m.ID),
			ContextSize: m.ContextSize,
			InputCost:   m.InputCost,
			OutputCost:  m.OutputCost,
		})
	}
	return result
}

// shortCode generates a 3-letter code from model ID
func shortCode(id string) string {
	parts := strings.Split(id, "-")
	if len(parts) >= 2 {
		// claude-sonnet-4 -> cs4
		return string(parts[0][0]) + string(parts[1][0]) + string(parts[len(parts)-1][0])
	}
	if len(id) >= 3 {
		return id[:3]
	}
	return id
}

// Chat implements the llm.Provider interface
func (u *UnifiedProvider) Chat(ctx context.Context, req *llm.ChatRequest) (<-chan domain.StreamEvent, error) {
	if u.baseURL == "" {
		return nil, fmt.Errorf("unified provider: base URL not configured")
	}
	if u.apiKey == "" {
		return nil, fmt.Errorf("unified provider: API key not configured")
	}

	// Build request
	oaiReq := u.buildRequest(req)

	body, err := json.Marshal(oaiReq)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", u.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+u.apiKey)
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := u.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("http %d: %s", resp.StatusCode, string(bodyBytes))
	}

	events := make(chan domain.StreamEvent, 100)
	go u.streamResponse(ctx, resp, events, req.Model)

	return events, nil
}

// buildRequest converts llm.ChatRequest to OpenAI format
func (u *UnifiedProvider) buildRequest(req *llm.ChatRequest) *unifiedRequest {
	oaiReq := &unifiedRequest{
		Model:  req.Model,
		Stream: true,
		StreamOptions: &unifiedStreamOpts{
			IncludeUsage: true,
		},
	}

	// Max tokens
	if req.MaxTokens > 0 {
		oaiReq.MaxTokens = req.MaxTokens
	} else {
		oaiReq.MaxTokens = 8192
	}

	// Temperature
	if req.Temperature > 0 {
		oaiReq.Temperature = req.Temperature
	}

	// System prompt
	if req.SystemPrompt != "" {
		oaiReq.Messages = append(oaiReq.Messages, unifiedMessage{
			Role:    "system",
			Content: req.SystemPrompt,
		})
	}

	// Messages
	for _, msg := range req.Messages {
		oaiMsg, toolResults := u.convertMessageWithToolResults(msg)
		oaiReq.Messages = append(oaiReq.Messages, oaiMsg)
		// Add any tool result messages
		oaiReq.Messages = append(oaiReq.Messages, toolResults...)
	}

	// Tools
	for _, tool := range req.Tools {
		oaiReq.Tools = append(oaiReq.Tools, unifiedTool{
			Type: "function",
			Function: unifiedFunction{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.Parameters,
			},
		})
	}

	return oaiReq
}

// convertMessageWithToolResults converts domain.Message to OpenAI format and returns the main message and any tool result messages
func (u *UnifiedProvider) convertMessageWithToolResults(msg domain.Message) (unifiedMessage, []unifiedMessage) {
	oaiMsg := unifiedMessage{
		Role: string(msg.Role),
	}

	// Check if message has tool calls or tool results
	var textParts []string
	var toolCalls []unifiedToolCall
	var toolResults []unifiedMessage

	for _, part := range msg.Parts {
		switch p := part.(type) {
		case domain.TextPart:
			textParts = append(textParts, p.Text)

		case domain.ImagePart:
			// Add as content array with base64 data URL
			if oaiMsg.Content == nil {
				oaiMsg.Content = []unifiedContentPart{}
			}
			parts := oaiMsg.Content.([]unifiedContentPart)
			// Construct data URL from base64
			dataURL := "data:" + p.MediaType + ";base64," + p.Base64
			parts = append(parts, unifiedContentPart{
				Type: "image_url",
				ImageURL: &unifiedImageURL{
					URL:    dataURL,
					Detail: "auto",
				},
			})
			oaiMsg.Content = parts

		case domain.ToolCallPart:
			if p.Result != "" {
				// This is a tool result - needs separate message
				// Ensure content is not empty for tool messages
				content := p.Result
				if content == "" {
					content = "No result returned from tool call."
				}
				toolResults = append(toolResults, unifiedMessage{
					Role:       "tool",
					Content:    content,
					ToolCallID: p.ToolID,
				})
			} else {
				// This is a tool call from assistant
				argsJSON, _ := json.Marshal(p.Args)
				toolCalls = append(toolCalls, unifiedToolCall{
					ID:   p.ToolID,
					Type: "function",
					Function: unifiedFunctionCall{
						Name:      p.Name,
						Arguments: string(argsJSON),
					},
				})
			}
		}
	}

	// Set content
	if len(textParts) > 0 {
		if oaiMsg.Content == nil {
			oaiMsg.Content = strings.Join(textParts, "\n")
		} else {
			// Already has image parts
			parts := oaiMsg.Content.([]unifiedContentPart)
			for _, text := range textParts {
				parts = append([]unifiedContentPart{{
					Type: "text",
					Text: text,
				}}, parts...)
			}
			oaiMsg.Content = parts
		}
	} else if len(toolCalls) == 0 && oaiMsg.Content == nil {
		// For user and system roles, content may be empty if only tool calls are present
		// But we should ensure we never send completely empty content for non-assistant roles
		// when there are no text parts or tool calls
		if string(msg.Role) == "assistant" {
			oaiMsg.Content = ""
		} else if string(msg.Role) == "tool" {
			// Tool messages should always have content (handled separately in toolResults)
		} else if len(toolCalls) == 0 {
			// For user/system messages with no text parts and no tool calls,
			// set content to an empty string to satisfy API requirements
			oaiMsg.Content = ""
		}
	}

	// Set tool calls for assistant messages
	if len(toolCalls) > 0 {
		oaiMsg.ToolCalls = toolCalls
	}

	return oaiMsg, toolResults
}

// streamResponse reads SSE stream and emits events
func (u *UnifiedProvider) streamResponse(ctx context.Context, resp *http.Response, events chan<- domain.StreamEvent, modelID string) {
	defer resp.Body.Close()
	defer close(events)

	reader := bufio.NewReader(resp.Body)
	var toolCallAccumulators = make(map[int]*unifiedToolCall)

	for {
		select {
		case <-ctx.Done():
			events <- domain.StreamEvent{Type: domain.StreamEventError, Error: ctx.Err()}
			return
		default:
		}

		line, err := reader.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				events <- domain.StreamEvent{Type: domain.StreamEventError, Error: err}
			}
			return
		}

		line = strings.TrimSpace(line)
		if line == "" || line == ":" {
			continue
		}

		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			events <- domain.StreamEvent{Type: domain.StreamEventDone, Done: true}
			return
		}

		var chunk unifiedStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		// Handle usage
		if chunk.Usage != nil {
			usage := &domain.Usage{
				InputTokens:  chunk.Usage.PromptTokens,
				OutputTokens: chunk.Usage.CompletionTokens,
			}
			// Calculate cost from registry
			if model := u.registry.Get(modelID); model != nil {
				usage.InputCost = float64(usage.InputTokens) * model.InputCost / 1_000_000
				usage.OutputCost = float64(usage.OutputTokens) * model.OutputCost / 1_000_000
				usage.TotalCost = usage.InputCost + usage.OutputCost
			}
			events <- domain.StreamEvent{Type: domain.StreamEventUsage, Usage: usage}
		}

		// Handle choices
		for _, choice := range chunk.Choices {
			delta := choice.Delta

			// Text content
			if delta.Content != "" {
				events <- domain.StreamEvent{
					Type:    domain.StreamEventText,
					Content: delta.Content,
				}
			}

			// Tool calls
			for _, tc := range delta.ToolCalls {
				acc, exists := toolCallAccumulators[tc.Index]
				if !exists {
					acc = &unifiedToolCall{Index: tc.Index}
					toolCallAccumulators[tc.Index] = acc
				}

				if tc.ID != "" {
					acc.ID = tc.ID
				}
				if tc.Type != "" {
					acc.Type = tc.Type
				}
				if tc.Function.Name != "" {
					acc.Function.Name = tc.Function.Name
				}
				acc.Function.Arguments += tc.Function.Arguments
			}

			// Finish reason
			if choice.FinishReason != "" {
				// Emit accumulated tool calls
				for _, tc := range toolCallAccumulators {
					var args map[string]any
					json.Unmarshal([]byte(tc.Function.Arguments), &args)

					events <- domain.StreamEvent{
						Type: domain.StreamEventToolCall,
						Part: domain.ToolCallPart{
							ToolID: tc.ID,
							Name:   tc.Function.Name,
							Args:   args,
						},
					}
				}
			}
		}
	}
}

// Request/Response types for OpenAI-compatible API
type unifiedRequest struct {
	Model         string             `json:"model"`
	Messages      []unifiedMessage   `json:"messages"`
	Tools         []unifiedTool      `json:"tools,omitempty"`
	Stream        bool               `json:"stream"`
	StreamOptions *unifiedStreamOpts `json:"stream_options,omitempty"`
	MaxTokens     int                `json:"max_tokens,omitempty"`
	Temperature   float64            `json:"temperature,omitempty"`
}

type unifiedStreamOpts struct {
	IncludeUsage bool `json:"include_usage"`
}

type unifiedMessage struct {
	Role       string             `json:"role"`
	Content    any                `json:"content,omitempty"`
	ToolCalls  []unifiedToolCall  `json:"tool_calls,omitempty"`
	ToolCallID string             `json:"tool_call_id,omitempty"`
}

type unifiedContentPart struct {
	Type     string           `json:"type"`
	Text     string           `json:"text,omitempty"`
	ImageURL *unifiedImageURL `json:"image_url,omitempty"`
}

type unifiedImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}

type unifiedToolCall struct {
	Index    int                 `json:"index"`
	ID       string              `json:"id"`
	Type     string              `json:"type"`
	Function unifiedFunctionCall `json:"function"`
}

type unifiedFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type unifiedTool struct {
	Type     string          `json:"type"`
	Function unifiedFunction `json:"function"`
}

type unifiedFunction struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Parameters  domain.JSONSchema `json:"parameters"`
}

type unifiedStreamChunk struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index        int `json:"index"`
		Delta        struct {
			Role      string            `json:"role,omitempty"`
			Content   string            `json:"content,omitempty"`
			ToolCalls []unifiedToolCall `json:"tool_calls,omitempty"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason,omitempty"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage,omitempty"`
}

// Ensure UnifiedProvider implements llm.Provider
var _ llm.Provider = (*UnifiedProvider)(nil)
