// Package provider implements LLM provider integrations
// DeepSeek Direct API provider - bypasses proxy for direct API access
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

	"github.com/joss/urp/internal/opencode/domain"
	"github.com/joss/urp/pkg/llm"
)

const (
	deepseekBaseURL   = "https://api.deepseek.com/v1/chat/completions"
	deepseekAPIKeyEnv = "DEEPSEEK_API_KEY"
)

// DeepSeekProvider implements llm.Provider for DeepSeek direct API
type DeepSeekProvider struct {
	apiKey  string
	baseURL string
	client  HTTPClient
}

// NewDeepSeekProvider creates a new DeepSeek provider
func NewDeepSeekProvider() *DeepSeekProvider {
	apiKey := os.Getenv(deepseekAPIKeyEnv)
	baseURL := os.Getenv("DEEPSEEK_BASE_URL")
	if baseURL == "" {
		baseURL = deepseekBaseURL
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

	return &DeepSeekProvider{
		apiKey:  apiKey,
		baseURL: baseURL,
		client:  client,
	}
}

// ID returns the provider identifier
func (p *DeepSeekProvider) ID() string {
	return "deepseek-direct"
}

// Name returns the provider display name
func (p *DeepSeekProvider) Name() string {
	return "DeepSeek Direct"
}

// IsConfigured returns true if API key is available
func (p *DeepSeekProvider) IsConfigured() bool {
	return p.apiKey != ""
}

// Models returns available DeepSeek models
func (p *DeepSeekProvider) Models() []domain.Model {
	return []domain.Model{
		{
			ID:          "deepseek-chat",
			Name:        "DeepSeek Chat",
			ShortCode:   "dsc",
			ContextSize: 64000,
			InputCost:   0.14,
			OutputCost:  0.28,
		},
		{
			ID:          "deepseek-coder",
			Name:        "DeepSeek Coder",
			ShortCode:   "dco",
			ContextSize: 64000,
			InputCost:   0.14,
			OutputCost:  0.28,
		},
		{
			ID:          "deepseek-reasoner",
			Name:        "DeepSeek Reasoner",
			ShortCode:   "dsr",
			ContextSize: 64000,
			InputCost:   0.55,
			OutputCost:  2.19,
		},
	}
}

// Chat sends a chat completion request to DeepSeek
func (p *DeepSeekProvider) Chat(ctx context.Context, req *llm.ChatRequest) (<-chan domain.StreamEvent, error) {
	if !p.IsConfigured() {
		return nil, fmt.Errorf("DeepSeek API key not configured (set %s)", deepseekAPIKeyEnv)
	}

	// Build request (reuse unified format)
	dsReq := p.buildRequest(req)

	body, err := json.Marshal(dsReq)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("DeepSeek API error %d: %s", resp.StatusCode, string(bodyBytes))
	}

	events := make(chan domain.StreamEvent, 100)
	go p.streamResponse(ctx, resp, events, req.Model)

	return events, nil
}

// buildRequest converts llm.ChatRequest to OpenAI-compatible format
func (p *DeepSeekProvider) buildRequest(req *llm.ChatRequest) *unifiedRequest {
	dsReq := &unifiedRequest{
		Model:  p.mapModelName(req.Model),
		Stream: true,
		StreamOptions: &unifiedStreamOpts{
			IncludeUsage: true,
		},
	}

	// Max tokens
	if req.MaxTokens > 0 {
		dsReq.MaxTokens = req.MaxTokens
	} else {
		dsReq.MaxTokens = 8192
	}

	// Temperature
	if req.Temperature > 0 {
		dsReq.Temperature = req.Temperature
	}

	// System prompt
	if req.SystemPrompt != "" {
		dsReq.Messages = append(dsReq.Messages, unifiedMessage{
			Role:    "system",
			Content: req.SystemPrompt,
		})
	}

	// Messages
	for _, msg := range req.Messages {
		oaiMsg := p.convertMessage(msg)
		dsReq.Messages = append(dsReq.Messages, oaiMsg)
	}

	// Tools
	for _, tool := range req.Tools {
		dsReq.Tools = append(dsReq.Tools, unifiedTool{
			Type: "function",
			Function: unifiedFunction{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.Parameters,
			},
		})
	}

	return dsReq
}

// convertMessage converts domain.Message to OpenAI format
func (p *DeepSeekProvider) convertMessage(msg domain.Message) unifiedMessage {
	oaiMsg := unifiedMessage{
		Role: string(msg.Role),
	}

	var textParts []string
	var toolCalls []unifiedToolCall

	for _, part := range msg.Parts {
		switch pt := part.(type) {
		case domain.TextPart:
			textParts = append(textParts, pt.Text)

		case domain.ToolCallPart:
			if pt.Result != "" {
				// This is a tool result - format as separate message
				content := pt.Result
				if pt.Error != "" {
					content = "Error: " + pt.Error
				}
				// Ensure content is not empty for tool messages
				if content == "" {
					content = "No result returned from tool call."
				}
				oaiMsg.Role = "tool"
				oaiMsg.ToolCallID = pt.ToolID
				oaiMsg.Content = content
				return oaiMsg
			}

			// Pending tool call
			args, _ := json.Marshal(pt.Args)
			toolCalls = append(toolCalls, unifiedToolCall{
				ID:   pt.ToolID,
				Type: "function",
				Function: unifiedFunctionCall{
					Name:      pt.Name,
					Arguments: string(args),
				},
			})
		}
	}

	// Set content
	if len(textParts) > 0 {
		oaiMsg.Content = strings.Join(textParts, "\n")
	}

	// Set tool calls
	if len(toolCalls) > 0 {
		oaiMsg.ToolCalls = toolCalls
	}

	return oaiMsg
}

// mapModelName converts our model IDs to DeepSeek API model names
func (p *DeepSeekProvider) mapModelName(model string) string {
	switch model {
	case "deepseek-chat", "deepseek", "ds-chat":
		return "deepseek-chat"
	case "deepseek-coder", "deepseek-code", "ds-coder":
		return "deepseek-coder"
	case "deepseek-reasoner", "deepseek-r1", "ds-reasoner":
		return "deepseek-reasoner"
	default:
		return model
	}
}

// streamResponse processes SSE stream from DeepSeek
func (p *DeepSeekProvider) streamResponse(ctx context.Context, resp *http.Response, events chan<- domain.StreamEvent, modelID string) {
	defer resp.Body.Close()
	defer close(events)

	reader := bufio.NewReader(resp.Body)

	for {
		select {
		case <-ctx.Done():
			events <- domain.StreamEvent{
				Type:  domain.StreamEventError,
				Error: ctx.Err(),
			}
			return
		default:
		}

		line, err := reader.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				events <- domain.StreamEvent{
					Type:  domain.StreamEventError,
					Error: err,
				}
			}
			return
		}

		line = strings.TrimSpace(line)
		if line == "" || line == "data: [DONE]" {
			continue
		}

		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		var chunk unifiedStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		if len(chunk.Choices) == 0 {
			continue
		}

		choice := chunk.Choices[0]
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
			if tc.Function.Name != "" || tc.Function.Arguments != "" {
				var args map[string]any
				if tc.Function.Arguments != "" {
					json.Unmarshal([]byte(tc.Function.Arguments), &args)
				}
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

		// Usage info (usually at the end)
		if chunk.Usage != nil {
			events <- domain.StreamEvent{
				Type: domain.StreamEventUsage,
				Usage: &domain.Usage{
					InputTokens:  chunk.Usage.PromptTokens,
					OutputTokens: chunk.Usage.CompletionTokens,
				},
			}
		}

		// Finish
		if choice.FinishReason != "" {
			events <- domain.StreamEvent{
				Type: domain.StreamEventDone,
				Done: true,
			}
		}
	}
}

// DefaultDeepSeekProvider is the singleton instance
var DefaultDeepSeekProvider = NewDeepSeekProvider()
