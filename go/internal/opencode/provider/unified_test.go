package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/joss/urp/internal/opencode/model"
	"github.com/joss/urp/internal/opencode/domain"
	"github.com/joss/urp/pkg/llm"
)

func TestNewUnifiedProvider(t *testing.T) {
	registry := model.NewModelRegistry()
	registry.Register(&model.ModelInfo{
		ID:          "test-model",
		InputCost:   1.0,
		OutputCost:  2.0,
		ContextSize: 10000,
		Enabled:     true,
	})

	p := NewUnifiedProvider("test-key", "http://localhost:8000/v1", registry)
	if p == nil {
		t.Fatal("NewUnifiedProvider returned nil")
	}

	if p.ID() != "unified" {
		t.Errorf("ID = %s, want unified", p.ID())
	}
	if p.Name() != "Unified Proxy" {
		t.Errorf("Name = %s, want 'Unified Proxy'", p.Name())
	}
}

func TestUnifiedProvider_Models(t *testing.T) {
	registry := model.NewModelRegistry()
	registry.Register(&model.ModelInfo{
		ID:          "model-a",
		InputCost:   1.0,
		OutputCost:  2.0,
		ContextSize: 10000,
		Enabled:     true,
	})
	registry.Register(&model.ModelInfo{
		ID:          "model-b",
		InputCost:   0.5,
		OutputCost:  1.0,
		ContextSize: 50000,
		Enabled:     true,
	})
	registry.Register(&model.ModelInfo{
		ID:      "disabled-model",
		Enabled: false,
	})

	p := NewUnifiedProvider("key", "http://localhost:8000", registry)
	models := p.Models()

	// Should only return enabled models
	if len(models) != 2 {
		t.Errorf("expected 2 enabled models, got %d", len(models))
	}

	// Check model details propagate
	for _, m := range models {
		if m.ID == "model-a" {
			if m.InputCost != 1.0 {
				t.Errorf("model-a InputCost = %f, want 1.0", m.InputCost)
			}
		}
	}
}

func TestUnifiedProvider_URLNormalization(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"http://localhost:8000", "http://localhost:8000/v1/chat/completions"},
		{"http://localhost:8000/", "http://localhost:8000/v1/chat/completions"},
		{"http://localhost:8000/v1", "http://localhost:8000/v1/chat/completions"},
		{"http://localhost:8000/v1/", "http://localhost:8000/v1/chat/completions"},
		{"http://localhost:8000/v1/chat/completions", "http://localhost:8000/v1/chat/completions"},
	}

	for _, tt := range tests {
		p := NewUnifiedProvider("key", tt.input, nil)
		if p.baseURL != tt.expected {
			t.Errorf("URL %s -> %s, want %s", tt.input, p.baseURL, tt.expected)
		}
	}
}

func TestUnifiedProvider_BuildRequest_Basic(t *testing.T) {
	registry := model.DefaultModelRegistry
	p := NewUnifiedProvider("key", "http://localhost:8000", registry)

	req := &llm.ChatRequest{
		Model:        "test-model",
		SystemPrompt: "You are helpful",
		Messages: []domain.Message{
			{
				Role:  domain.RoleUser,
				Parts: []domain.Part{domain.TextPart{Text: "Hello"}},
			},
		},
		MaxTokens:   4096,
		Temperature: 0.7,
	}

	oaiReq := p.buildRequest(req)

	if oaiReq.Model != "test-model" {
		t.Errorf("Model = %s, want test-model", oaiReq.Model)
	}
	if !oaiReq.Stream {
		t.Error("Stream should be true")
	}
	if oaiReq.MaxTokens != 4096 {
		t.Errorf("MaxTokens = %d, want 4096", oaiReq.MaxTokens)
	}
	if oaiReq.Temperature != 0.7 {
		t.Errorf("Temperature = %f, want 0.7", oaiReq.Temperature)
	}

	// Should have system message first
	if len(oaiReq.Messages) < 2 {
		t.Fatal("expected at least 2 messages (system + user)")
	}
	if oaiReq.Messages[0].Role != "system" {
		t.Errorf("first message role = %s, want system", oaiReq.Messages[0].Role)
	}
}

func TestUnifiedProvider_BuildRequest_WithTools(t *testing.T) {
	p := NewUnifiedProvider("key", "http://localhost:8000", nil)

	req := &llm.ChatRequest{
		Model: "test-model",
		Messages: []domain.Message{
			{Role: domain.RoleUser, Parts: []domain.Part{domain.TextPart{Text: "Test"}}},
		},
		Tools: []domain.Tool{
			{
				Name:        "read_file",
				Description: "Read a file",
				Parameters: domain.JSONSchema{
					"type": "object",
					"properties": map[string]any{
						"path": map[string]any{"type": "string"},
					},
				},
			},
		},
	}

	oaiReq := p.buildRequest(req)

	if len(oaiReq.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(oaiReq.Tools))
	}

	tool := oaiReq.Tools[0]
	if tool.Type != "function" {
		t.Errorf("tool type = %s, want function", tool.Type)
	}
	if tool.Function.Name != "read_file" {
		t.Errorf("tool name = %s, want read_file", tool.Function.Name)
	}
}

func TestUnifiedProvider_ConvertMessage_Text(t *testing.T) {
	p := NewUnifiedProvider("key", "http://localhost:8000", nil)

	msg := domain.Message{
		Role:  domain.RoleUser,
		Parts: []domain.Part{domain.TextPart{Text: "Hello world"}},
	}

	oaiMsg, _ := p.convertMessageWithToolResults(msg)

	if oaiMsg.Role != "user" {
		t.Errorf("role = %s, want user", oaiMsg.Role)
	}
	if oaiMsg.Content != "Hello world" {
		t.Errorf("content = %v, want 'Hello world'", oaiMsg.Content)
	}
}

func TestUnifiedProvider_ConvertMessage_ToolCall(t *testing.T) {
	p := NewUnifiedProvider("key", "http://localhost:8000", nil)

	msg := domain.Message{
		Role: domain.RoleAssistant,
		Parts: []domain.Part{
			domain.ToolCallPart{
				ToolID: "call_123",
				Name:   "read_file",
				Args:   map[string]any{"path": "/test.txt"},
			},
		},
	}

	oaiMsg, _ := p.convertMessageWithToolResults(msg)

	if len(oaiMsg.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(oaiMsg.ToolCalls))
	}
	if oaiMsg.ToolCalls[0].ID != "call_123" {
		t.Errorf("tool call ID = %s, want call_123", oaiMsg.ToolCalls[0].ID)
	}
	if oaiMsg.ToolCalls[0].Function.Name != "read_file" {
		t.Errorf("tool call name = %s, want read_file", oaiMsg.ToolCalls[0].Function.Name)
	}
}

func TestUnifiedProvider_Chat_NoBaseURL(t *testing.T) {
	p := &UnifiedProvider{
		apiKey:   "key",
		baseURL:  "",
		registry: model.DefaultModelRegistry,
	}

	_, err := p.Chat(context.Background(), &llm.ChatRequest{Model: "test"})
	if err == nil {
		t.Error("expected error for missing base URL")
	}
}

func TestUnifiedProvider_Chat_NoAPIKey(t *testing.T) {
	p := &UnifiedProvider{
		apiKey:   "",
		baseURL:  "http://localhost:8000/v1/chat/completions",
		registry: model.DefaultModelRegistry,
	}

	_, err := p.Chat(context.Background(), &llm.ChatRequest{Model: "test"})
	if err == nil {
		t.Error("expected error for missing API key")
	}
}

// mockHTTPClient for testing
type mockHTTPClient struct {
	response *http.Response
	err      error
}

func (m *mockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return m.response, m.err
}

func TestUnifiedProvider_Chat_StreamResponse(t *testing.T) {
	// Create mock SSE response
	sseData := `data: {"id":"1","choices":[{"delta":{"content":"Hello"}}]}

data: {"id":"2","choices":[{"delta":{"content":" world"}}]}

data: {"id":"3","choices":[{"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5}}

data: [DONE]
`
	mockClient := &mockHTTPClient{
		response: &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewBufferString(sseData)),
		},
	}

	registry := model.NewModelRegistry()
	registry.Register(&model.ModelInfo{
		ID:         "test-model",
		InputCost:  3.0,
		OutputCost: 15.0,
		Enabled:    true,
	})

	p := NewUnifiedProviderWithClient("key", "http://localhost:8000", registry, mockClient)

	events, err := p.Chat(context.Background(), &llm.ChatRequest{
		Model: "test-model",
		Messages: []domain.Message{
			{Role: domain.RoleUser, Parts: []domain.Part{domain.TextPart{Text: "Hi"}}},
		},
	})
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}

	var textContent string
	var gotUsage bool
	var gotDone bool

	for event := range events {
		switch event.Type {
		case domain.StreamEventText:
			textContent += event.Content
		case domain.StreamEventUsage:
			gotUsage = true
			if event.Usage.InputTokens != 10 {
				t.Errorf("InputTokens = %d, want 10", event.Usage.InputTokens)
			}
		case domain.StreamEventDone:
			gotDone = true
		}
	}

	if textContent != "Hello world" {
		t.Errorf("text content = %q, want 'Hello world'", textContent)
	}
	if !gotUsage {
		t.Error("expected usage event")
	}
	if !gotDone {
		t.Error("expected done event")
	}
}

func TestUnifiedProvider_Chat_ToolCallStream(t *testing.T) {
	// Mock tool call streaming (split across chunks)
	sseData := `data: {"id":"1","choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"read_file","arguments":""}}]}}]}

data: {"id":"2","choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"path\":"}}]}}]}

data: {"id":"3","choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"/test.txt\"}"}}]}}]}

data: {"id":"4","choices":[{"finish_reason":"tool_calls"}]}

data: [DONE]
`
	mockClient := &mockHTTPClient{
		response: &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewBufferString(sseData)),
		},
	}

	p := NewUnifiedProviderWithClient("key", "http://localhost:8000", nil, mockClient)

	events, err := p.Chat(context.Background(), &llm.ChatRequest{
		Model:    "test",
		Messages: []domain.Message{{Role: domain.RoleUser, Parts: []domain.Part{domain.TextPart{Text: "Test"}}}},
	})
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}

	var gotToolCall bool
	for event := range events {
		if event.Type == domain.StreamEventToolCall {
			gotToolCall = true
			tc, ok := event.Part.(domain.ToolCallPart)
			if !ok {
				t.Fatal("expected ToolCallPart")
			}
			if tc.Name != "read_file" {
				t.Errorf("tool name = %s, want read_file", tc.Name)
			}
			if tc.Args["path"] != "/test.txt" {
				t.Errorf("tool args path = %v, want /test.txt", tc.Args["path"])
			}
		}
	}

	if !gotToolCall {
		t.Error("expected tool call event")
	}
}

func TestUnifiedProvider_Chat_HTTPError(t *testing.T) {
	mockClient := &mockHTTPClient{
		response: &http.Response{
			StatusCode: 500,
			Body:       io.NopCloser(bytes.NewBufferString(`{"error":"internal server error"}`)),
		},
	}

	p := NewUnifiedProviderWithClient("key", "http://localhost:8000", nil, mockClient)

	_, err := p.Chat(context.Background(), &llm.ChatRequest{
		Model:    "test",
		Messages: []domain.Message{{Role: domain.RoleUser, Parts: []domain.Part{domain.TextPart{Text: "Hi"}}}},
	})

	if err == nil {
		t.Error("expected error for HTTP 500")
	}
}

func TestShortCode(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"claude-sonnet-4", "cs4"},
		{"deepseek-chat", "dcc"},  // d-c-c (last part is "chat")
		{"gpt-4o", "g44"},         // g-4-4 (last part is "4o")
		{"ab", "ab"},
		{"a", "a"},
	}

	for _, tt := range tests {
		got := shortCode(tt.input)
		if got != tt.want {
			t.Errorf("shortCode(%s) = %s, want %s", tt.input, got, tt.want)
		}
	}
}

func TestUnifiedProvider_UsesDefaultRegistry(t *testing.T) {
	p := NewUnifiedProvider("key", "http://localhost:8000", nil)

	// Should use DefaultModelRegistry
	models := p.Models()
	if len(models) == 0 {
		t.Error("expected models from default registry")
	}

	// Should be able to find common models
	found := false
	for _, m := range models {
		if m.ID == "claude-sonnet-4-20250514" || m.ID == "deepseek-chat" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected to find common models in default registry")
	}
}

func TestUnifiedProvider_CostCalculation(t *testing.T) {
	registry := model.NewModelRegistry()
	registry.Register(&model.ModelInfo{
		ID:         "test-model",
		InputCost:  3.0,  // $3/1M
		OutputCost: 15.0, // $15/1M
		Enabled:    true,
	})

	// Simulate usage event with cost calculation
	usage := &domain.Usage{
		InputTokens:  1000,
		OutputTokens: 500,
	}

	model := registry.Get("test-model")
	if model != nil {
		usage.InputCost = float64(usage.InputTokens) * model.InputCost / 1_000_000
		usage.OutputCost = float64(usage.OutputTokens) * model.OutputCost / 1_000_000
		usage.TotalCost = usage.InputCost + usage.OutputCost
	}

	expectedInput := 1000 * 3.0 / 1_000_000
	expectedOutput := 500 * 15.0 / 1_000_000

	if usage.InputCost != expectedInput {
		t.Errorf("InputCost = %f, want %f", usage.InputCost, expectedInput)
	}
	if usage.OutputCost != expectedOutput {
		t.Errorf("OutputCost = %f, want %f", usage.OutputCost, expectedOutput)
	}
}

// Verify interface compliance
func TestUnifiedProvider_ImplementsProvider(t *testing.T) {
	var _ llm.Provider = (*UnifiedProvider)(nil)
}

func TestUnifiedProvider_BuildRequest_MultipleTextParts(t *testing.T) {
	p := NewUnifiedProvider("key", "http://localhost:8000", nil)

	msg := domain.Message{
		Role: domain.RoleUser,
		Parts: []domain.Part{
			domain.TextPart{Text: "Part 1"},
			domain.TextPart{Text: "Part 2"},
		},
	}

	oaiMsg, _ := p.convertMessageWithToolResults(msg)

	// Should concatenate text parts
	content, ok := oaiMsg.Content.(string)
	if !ok {
		t.Fatal("expected string content")
	}
	if content != "Part 1\nPart 2" {
		t.Errorf("content = %q, want 'Part 1\\nPart 2'", content)
	}
}

func TestUnifiedProvider_RequestJSON(t *testing.T) {
	p := NewUnifiedProvider("key", "http://localhost:8000", nil)

	req := &llm.ChatRequest{
		Model: "test-model",
		Messages: []domain.Message{
			{Role: domain.RoleUser, Parts: []domain.Part{domain.TextPart{Text: "Hello"}}},
		},
		Tools: []domain.Tool{
			{Name: "test_tool", Description: "A test tool"},
		},
	}

	oaiReq := p.buildRequest(req)

	// Should serialize to valid JSON
	data, err := json.Marshal(oaiReq)
	if err != nil {
		t.Fatalf("failed to marshal request: %v", err)
	}

	// Verify it's valid JSON by unmarshaling
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal request: %v", err)
	}

	if parsed["model"] != "test-model" {
		t.Errorf("model = %v, want test-model", parsed["model"])
	}
}
