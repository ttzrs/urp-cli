package llm

import (
	"context"

	"github.com/joss/urp/internal/opencode/domain"
)

// Provider is the interface all LLM providers must implement
type Provider interface {
	ID() string
	Name() string
	Models() []domain.Model

	// Chat sends messages and returns a streaming response
	Chat(ctx context.Context, req *ChatRequest) (<-chan domain.StreamEvent, error)
}

// ChatRequest represents a request to the LLM
type ChatRequest struct {
	Model          string
	Messages       []domain.Message
	Tools          []domain.Tool
	MaxTokens      int
	Temperature    float64
	SystemPrompt   string
	ThinkingBudget int // Extended thinking token budget (0 = disabled)
}

// ProviderRegistry holds all available providers
type ProviderRegistry struct {
	providers map[string]Provider
}

func NewRegistry() *ProviderRegistry {
	return &ProviderRegistry{
		providers: make(map[string]Provider),
	}
}

func (r *ProviderRegistry) Register(p Provider) {
	r.providers[p.ID()] = p
}

func (r *ProviderRegistry) Get(id string) (Provider, bool) {
	p, ok := r.providers[id]
	return p, ok
}

func (r *ProviderRegistry) List() []Provider {
	result := make([]Provider, 0, len(r.providers))
	for _, p := range r.providers {
		result = append(result, p)
	}
	return result
}
