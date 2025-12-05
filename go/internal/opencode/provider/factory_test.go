package provider

import (
	"context"
	"testing"

	"github.com/joss/urp/internal/opencode/domain"
	"github.com/joss/urp/pkg/llm"
)

func TestFactory_Create(t *testing.T) {
	f := NewFactory()

	tests := []struct {
		name    string
		pt      ProviderType
		opts    []ConfigOption
		wantID  string
		wantErr bool
	}{
		{"anthropic", ProviderAnthropic, []ConfigOption{WithAPIKey("test-key")}, "anthropic", false},
		{"openai", ProviderOpenAI, []ConfigOption{WithAPIKey("test-key")}, "openai", false},
		{"google", ProviderGoogle, []ConfigOption{WithAPIKey("test-key")}, "google", false},
		{"unknown", ProviderType("unknown"), nil, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := f.Create(tt.pt, tt.opts...)
			if (err != nil) != tt.wantErr {
				t.Errorf("Create() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil && p.ID() != tt.wantID {
				t.Errorf("Create().ID() = %v, want %v", p.ID(), tt.wantID)
			}
		})
	}
}

func TestFactory_CreateByID(t *testing.T) {
	f := NewFactory()

	tests := []struct {
		id      string
		wantID  string
		wantErr bool
	}{
		{"anthropic", "anthropic", false},
		{"claude", "anthropic", false},
		{"openai", "openai", false},
		{"gpt", "openai", false},
		{"google", "google", false},
		{"gemini", "google", false},
		{"unknown", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			p, err := f.CreateByID(tt.id, WithAPIKey("test"))
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateByID(%q) error = %v, wantErr %v", tt.id, err, tt.wantErr)
				return
			}
			if err == nil && p.ID() != tt.wantID {
				t.Errorf("CreateByID(%q).ID() = %v, want %v", tt.id, p.ID(), tt.wantID)
			}
		})
	}
}

func TestFactory_Caching(t *testing.T) {
	f := NewFactory()

	p1, _ := f.Create(ProviderAnthropic, WithAPIKey("same-key"))
	p2, _ := f.Create(ProviderAnthropic, WithAPIKey("same-key"))

	// Should return same instance
	if p1 != p2 {
		t.Error("expected cached provider to be reused")
	}

	// Different key = different instance
	p3, _ := f.Create(ProviderAnthropic, WithAPIKey("diff-key1"))
	if p1 == p3 {
		t.Error("expected different providers for different keys")
	}
}

func TestFactory_Register(t *testing.T) {
	f := NewFactory()

	// Register custom provider type
	f.Register(ProviderType("custom"), func(cfg Config) llm.Provider {
		return &mockProvider{id: "custom", apiKey: cfg.APIKey}
	})

	p, err := f.Create(ProviderType("custom"), WithAPIKey("test"))
	if err != nil {
		t.Fatalf("Create(custom) error = %v", err)
	}
	if p.ID() != "custom" {
		t.Errorf("ID() = %v, want custom", p.ID())
	}
}

func TestFactory_Clear(t *testing.T) {
	f := NewFactory()

	p1, _ := f.Create(ProviderAnthropic, WithAPIKey("key1"))
	f.Clear()
	p2, _ := f.Create(ProviderAnthropic, WithAPIKey("key1"))

	if p1 == p2 {
		t.Error("expected new instance after Clear()")
	}
}

// mockProvider for testing custom registration
type mockProvider struct {
	id     string
	apiKey string
}

func (m *mockProvider) ID() string                                            { return m.id }
func (m *mockProvider) Name() string                                          { return m.id }
func (m *mockProvider) Models() []domain.Model                                { return nil }
func (m *mockProvider) Chat(ctx context.Context, r *llm.ChatRequest) (<-chan domain.StreamEvent, error) {
	return nil, nil
}
