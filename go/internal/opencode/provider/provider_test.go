package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/joss/urp/internal/opencode/domain"
	"github.com/joss/urp/pkg/llm"
)

func TestOpenAIStreamText(t *testing.T) {
	// Mock SSE response
	sseData := `data: {"choices":[{"delta":{"content":"Hello"},"finish_reason":null}]}

data: {"choices":[{"delta":{"content":" world"},"finish_reason":null}]}

data: {"choices":[{"delta":{},"finish_reason":"stop"}]}

`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("Missing or wrong Authorization header")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(sseData))
	}))
	defer server.Close()

	provider := NewOpenAICompatible("test-key", server.URL)

	req := &llm.ChatRequest{
		Model: "gpt-4",
		Messages: []domain.Message{
			{Role: domain.RoleUser, Parts: []domain.Part{domain.TextPart{Text: "Hi"}}},
		},
	}

	events, err := provider.Chat(context.Background(), req)
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}

	var text string
	for event := range events {
		if event.Type == domain.StreamEventText {
			text += event.Content
		}
	}

	if text != "Hello world" {
		t.Errorf("Text = %q, want %q", text, "Hello world")
	}
}

func TestOpenAIStreamToolCall(t *testing.T) {
	// Mock SSE response with tool call
	sseData := `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_123","type":"function","function":{"name":"bash","arguments":""}}]},"finish_reason":null}]}

data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"command\":"}}]},"finish_reason":null}]}

data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"ls\"}"}}]},"finish_reason":null}]}

data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}

`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(sseData))
	}))
	defer server.Close()

	provider := NewOpenAICompatible("test-key", server.URL)

	req := &llm.ChatRequest{
		Model: "gpt-4",
		Messages: []domain.Message{
			{Role: domain.RoleUser, Parts: []domain.Part{domain.TextPart{Text: "list files"}}},
		},
		Tools: []domain.Tool{
			{Name: "bash", Description: "Run bash command"},
		},
	}

	events, err := provider.Chat(context.Background(), req)
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}

	var toolCalls []domain.ToolCallPart
	for event := range events {
		if event.Type == domain.StreamEventToolCall {
			if tc, ok := event.Part.(domain.ToolCallPart); ok {
				toolCalls = append(toolCalls, tc)
			}
		}
	}

	if len(toolCalls) != 1 {
		t.Fatalf("Got %d tool calls, want 1", len(toolCalls))
	}

	if toolCalls[0].Name != "bash" {
		t.Errorf("Tool name = %q, want %q", toolCalls[0].Name, "bash")
	}
	if toolCalls[0].ToolID != "call_123" {
		t.Errorf("Tool ID = %q, want %q", toolCalls[0].ToolID, "call_123")
	}
}

func TestOpenAIAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"Invalid API key"}}`))
	}))
	defer server.Close()

	provider := NewOpenAICompatible("bad-key", server.URL)

	req := &llm.ChatRequest{
		Model: "gpt-4",
		Messages: []domain.Message{
			{Role: domain.RoleUser, Parts: []domain.Part{domain.TextPart{Text: "Hi"}}},
		},
	}

	_, err := provider.Chat(context.Background(), req)
	if err == nil {
		t.Error("Expected error for unauthorized request")
	}
}

func TestOpenAIMessageConversion(t *testing.T) {
	var capturedBody openaiRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedBody)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	provider := NewOpenAICompatible("test-key", server.URL)

	req := &llm.ChatRequest{
		Model:        "gpt-4",
		SystemPrompt: "You are helpful",
		Messages: []domain.Message{
			{Role: domain.RoleUser, Parts: []domain.Part{domain.TextPart{Text: "Hello"}}},
			{Role: domain.RoleAssistant, Parts: []domain.Part{domain.TextPart{Text: "Hi there"}}},
			{Role: domain.RoleUser, Parts: []domain.Part{domain.TextPart{Text: "How are you?"}}},
		},
		MaxTokens: 1000,
	}

	events, err := provider.Chat(context.Background(), req)
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}

	// Drain events
	for range events {
	}

	// Verify message structure
	if capturedBody.Model != "gpt-4" {
		t.Errorf("Model = %q, want %q", capturedBody.Model, "gpt-4")
	}

	if len(capturedBody.Messages) != 4 { // system + 3 messages
		t.Errorf("Messages count = %d, want 4", len(capturedBody.Messages))
	}

	if capturedBody.Messages[0].Role != "system" {
		t.Errorf("First message role = %q, want %q", capturedBody.Messages[0].Role, "system")
	}

	if capturedBody.MaxTokens != 1000 {
		t.Errorf("MaxTokens = %d, want 1000", capturedBody.MaxTokens)
	}

	if !capturedBody.Stream {
		t.Error("Stream should be true")
	}
}

func TestOpenAIModels(t *testing.T) {
	provider := NewOpenAI("", "")
	models := provider.Models()

	if len(models) == 0 {
		t.Error("Models() should return at least one model")
	}

	// Check gpt-4o exists
	found := false
	for _, m := range models {
		if m.ID == "gpt-4o" {
			found = true
			if m.ContextSize != 128000 {
				t.Errorf("gpt-4o ContextSize = %d, want 128000", m.ContextSize)
			}
			break
		}
	}
	if !found {
		t.Error("Models() should include gpt-4o")
	}
}

func TestOpenAIID(t *testing.T) {
	provider := NewOpenAI("", "")
	if provider.ID() != "openai" {
		t.Errorf("ID() = %q, want %q", provider.ID(), "openai")
	}
	if provider.Name() != "OpenAI" {
		t.Errorf("Name() = %q, want %q", provider.Name(), "OpenAI")
	}
}

func TestAnthropicModels(t *testing.T) {
	provider := NewAnthropic("", "")
	models := provider.Models()

	if len(models) == 0 {
		t.Error("Models() should return at least one model")
	}

	// Check claude-sonnet-4-20250514 exists
	found := false
	for _, m := range models {
		if m.ID == "claude-sonnet-4-20250514" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Models() should include claude-sonnet-4-20250514")
	}
}

func TestAnthropicID(t *testing.T) {
	provider := NewAnthropic("", "")
	if provider.ID() != "anthropic" {
		t.Errorf("ID() = %q, want %q", provider.ID(), "anthropic")
	}
	if provider.Name() != "Anthropic" {
		t.Errorf("Name() = %q, want %q", provider.Name(), "Anthropic")
	}
}

func TestAnthropicBaseURL(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		want    string
	}{
		{"empty uses default", "", "https://api.anthropic.com/v1/messages"},
		// Anthropic API uses /messages (not /v1/messages like OpenAI)
		{"adds /messages", "http://localhost:8080", "http://localhost:8080/messages"},
		{"adds /messages to /v1", "http://localhost:8080/v1", "http://localhost:8080/v1/messages"},
		{"removes trailing slash", "http://localhost:8080/", "http://localhost:8080/messages"},
		{"keeps full path", "http://localhost:8080/v1/messages", "http://localhost:8080/v1/messages"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewAnthropic("key", tt.baseURL)
			if p.baseURL != tt.want {
				t.Errorf("baseURL = %q, want %q", p.baseURL, tt.want)
			}
		})
	}
}

func TestOpenAIBaseURL(t *testing.T) {
	// Save and restore env
	oldEnv := os.Getenv("OPENAI_BASE_URL")
	defer os.Setenv("OPENAI_BASE_URL", oldEnv)
	os.Unsetenv("OPENAI_BASE_URL")

	tests := []struct {
		name    string
		baseURL string
		want    string
	}{
		{"empty uses default", "", "https://api.openai.com/v1/chat/completions"},
		{"adds /v1/chat/completions", "http://localhost:8080", "http://localhost:8080/v1/chat/completions"},
		{"adds /chat/completions to /v1", "http://localhost:8080/v1", "http://localhost:8080/v1/chat/completions"},
		{"removes trailing slash", "http://localhost:8080/v1/", "http://localhost:8080/v1/chat/completions"},
		{"keeps full path", "http://localhost:8080/v1/chat/completions", "http://localhost:8080/v1/chat/completions"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewOpenAI("key", tt.baseURL)
			if p.baseURL != tt.want {
				t.Errorf("baseURL = %q, want %q", p.baseURL, tt.want)
			}
		})
	}
}

func TestGoogleModels(t *testing.T) {
	provider := NewGoogle("")
	models := provider.Models()

	if len(models) == 0 {
		t.Error("Models() should return at least one model")
	}

	// Check gemini-1.5-pro exists
	found := false
	for _, m := range models {
		if m.ID == "gemini-1.5-pro" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Models() should include gemini-1.5-pro")
	}
}

func TestGoogleID(t *testing.T) {
	provider := NewGoogle("")
	if provider.ID() != "google" {
		t.Errorf("ID() = %q, want %q", provider.ID(), "google")
	}
	if provider.Name() != "Google" {
		t.Errorf("Name() = %q, want %q", provider.Name(), "Google")
	}
}
