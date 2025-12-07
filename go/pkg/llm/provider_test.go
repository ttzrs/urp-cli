package llm

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/joss/urp/internal/opencode/domain"
)

// ─────────────────────────────────────────────────────────────────────────────
// Mock Provider for Testing (DIP in action)
// ─────────────────────────────────────────────────────────────────────────────

type MockProvider struct {
	id     string
	name   string
	models []domain.Model

	// Chat tracking
	ChatCalled int
	LastReq    *ChatRequest
	ChatErr    error
}

func NewMockProvider(id, name string) *MockProvider {
	return &MockProvider{
		id:   id,
		name: name,
		models: []domain.Model{
			{ID: "model-1", Name: "Model 1"},
			{ID: "model-2", Name: "Model 2"},
		},
	}
}

func (m *MockProvider) ID() string {
	return m.id
}

func (m *MockProvider) Name() string {
	return m.name
}

func (m *MockProvider) Models() []domain.Model {
	return m.models
}

func (m *MockProvider) Chat(ctx context.Context, req *ChatRequest) (<-chan domain.StreamEvent, error) {
	m.ChatCalled++
	m.LastReq = req

	if m.ChatErr != nil {
		return nil, m.ChatErr
	}

	// Return a channel with a simple response
	ch := make(chan domain.StreamEvent, 1)
	ch <- domain.StreamEvent{
		Type: domain.StreamEventDone,
	}
	close(ch)

	return ch, nil
}

// Verify MockProvider implements Provider
var _ Provider = (*MockProvider)(nil)

// ─────────────────────────────────────────────────────────────────────────────
// ProviderRegistry Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	require.NotNil(t, r)
	assert.Empty(t, r.List())
}

func TestRegistry_Register(t *testing.T) {
	r := NewRegistry()
	provider := NewMockProvider("anthropic", "Anthropic")

	r.Register(provider)

	retrieved, ok := r.Get("anthropic")
	assert.True(t, ok)
	assert.Equal(t, provider, retrieved)
}

func TestRegistry_Get_NotFound(t *testing.T) {
	r := NewRegistry()

	_, ok := r.Get("nonexistent")
	assert.False(t, ok)
}

func TestRegistry_List(t *testing.T) {
	r := NewRegistry()
	r.Register(NewMockProvider("anthropic", "Anthropic"))
	r.Register(NewMockProvider("openai", "OpenAI"))

	list := r.List()
	assert.Len(t, list, 2)
}

func TestRegistry_MultipleProviders(t *testing.T) {
	r := NewRegistry()

	providers := []struct {
		id   string
		name string
	}{
		{"anthropic", "Anthropic"},
		{"openai", "OpenAI"},
		{"ollama", "Ollama"},
	}

	for _, p := range providers {
		r.Register(NewMockProvider(p.id, p.name))
	}

	for _, p := range providers {
		retrieved, ok := r.Get(p.id)
		assert.True(t, ok, "provider %s should exist", p.id)
		assert.Equal(t, p.name, retrieved.Name())
	}
}

func TestRegistry_Overwrite(t *testing.T) {
	r := NewRegistry()

	// Register first provider
	p1 := NewMockProvider("anthropic", "Anthropic V1")
	r.Register(p1)

	// Register with same ID - should overwrite
	p2 := NewMockProvider("anthropic", "Anthropic V2")
	r.Register(p2)

	retrieved, ok := r.Get("anthropic")
	assert.True(t, ok)
	assert.Equal(t, "Anthropic V2", retrieved.Name())
	assert.Len(t, r.List(), 1)
}

// ─────────────────────────────────────────────────────────────────────────────
// Provider Interface Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestMockProvider_ID(t *testing.T) {
	p := NewMockProvider("test-id", "Test Provider")
	assert.Equal(t, "test-id", p.ID())
}

func TestMockProvider_Name(t *testing.T) {
	p := NewMockProvider("test-id", "Test Provider")
	assert.Equal(t, "Test Provider", p.Name())
}

func TestMockProvider_Models(t *testing.T) {
	p := NewMockProvider("test-id", "Test Provider")
	models := p.Models()
	assert.Len(t, models, 2)
	assert.Equal(t, "model-1", models[0].ID)
	assert.Equal(t, "model-2", models[1].ID)
}

func TestMockProvider_Chat(t *testing.T) {
	p := NewMockProvider("test-id", "Test Provider")
	ctx := context.Background()

	req := &ChatRequest{
		Model:        "claude-3-sonnet",
		MaxTokens:    1000,
		Temperature:  0.7,
		SystemPrompt: "You are helpful",
	}

	ch, err := p.Chat(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, ch)

	// Verify tracking
	assert.Equal(t, 1, p.ChatCalled)
	assert.Equal(t, req, p.LastReq)

	// Read response
	event := <-ch
	assert.Equal(t, domain.StreamEventDone, event.Type)
}

func TestMockProvider_ChatError(t *testing.T) {
	p := NewMockProvider("test-id", "Test Provider")
	p.ChatErr = assert.AnError

	ctx := context.Background()
	req := &ChatRequest{Model: "test"}

	ch, err := p.Chat(ctx, req)
	assert.Error(t, err)
	assert.Nil(t, ch)
}

// ─────────────────────────────────────────────────────────────────────────────
// ChatRequest Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestChatRequest_Fields(t *testing.T) {
	req := &ChatRequest{
		Model:          "claude-3-sonnet",
		Messages:       []domain.Message{{ID: "msg1"}},
		Tools:          []domain.Tool{{Name: "bash"}},
		MaxTokens:      4096,
		Temperature:    0.5,
		SystemPrompt:   "System prompt",
		ThinkingBudget: 10000,
	}

	assert.Equal(t, "claude-3-sonnet", req.Model)
	assert.Len(t, req.Messages, 1)
	assert.Len(t, req.Tools, 1)
	assert.Equal(t, 4096, req.MaxTokens)
	assert.Equal(t, 0.5, req.Temperature)
	assert.Equal(t, "System prompt", req.SystemPrompt)
	assert.Equal(t, 10000, req.ThinkingBudget)
}

func TestChatRequest_Defaults(t *testing.T) {
	req := &ChatRequest{}

	assert.Empty(t, req.Model)
	assert.Nil(t, req.Messages)
	assert.Nil(t, req.Tools)
	assert.Zero(t, req.MaxTokens)
	assert.Zero(t, req.Temperature)
	assert.Empty(t, req.SystemPrompt)
	assert.Zero(t, req.ThinkingBudget)
}
