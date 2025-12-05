// Package provider implements LLM provider factories and interfaces.
package provider

import (
	"fmt"
	"net/http"
	"os"
	"sync"

	"github.com/joss/urp/pkg/llm"
)

// ProviderType identifies supported LLM providers.
type ProviderType string

const (
	ProviderAnthropic ProviderType = "anthropic"
	ProviderOpenAI    ProviderType = "openai"
	ProviderGoogle    ProviderType = "google"
)

// Config holds provider configuration.
type Config struct {
	APIKey     string
	BaseURL    string
	HTTPClient HTTPClient
}

// ConfigOption modifies provider configuration.
type ConfigOption func(*Config)

// WithAPIKey sets the API key.
func WithAPIKey(key string) ConfigOption {
	return func(c *Config) { c.APIKey = key }
}

// WithBaseURL sets the base URL.
func WithBaseURL(url string) ConfigOption {
	return func(c *Config) { c.BaseURL = url }
}

// WithHTTPClient sets the HTTP client.
func WithHTTPClient(client HTTPClient) ConfigOption {
	return func(c *Config) { c.HTTPClient = client }
}

// Factory creates LLM providers.
type Factory struct {
	mu       sync.RWMutex
	cache    map[string]llm.Provider
	builders map[ProviderType]ProviderBuilder
}

// ProviderBuilder constructs a provider from config.
type ProviderBuilder func(cfg Config) llm.Provider

// NewFactory creates a factory with default builders.
func NewFactory() *Factory {
	f := &Factory{
		cache:    make(map[string]llm.Provider),
		builders: make(map[ProviderType]ProviderBuilder),
	}
	f.RegisterDefaults()
	return f
}

// RegisterDefaults registers the built-in provider builders.
func (f *Factory) RegisterDefaults() {
	f.Register(ProviderAnthropic, func(cfg Config) llm.Provider {
		return NewAnthropicWithClient(cfg.APIKey, cfg.BaseURL, cfg.HTTPClient)
	})
	f.Register(ProviderOpenAI, func(cfg Config) llm.Provider {
		return NewOpenAIWithClient(cfg.APIKey, cfg.BaseURL, cfg.HTTPClient)
	})
	f.Register(ProviderGoogle, func(cfg Config) llm.Provider {
		return NewGoogleWithClient(cfg.APIKey, cfg.HTTPClient)
	})
}

// Register adds a provider builder. Allows extension with custom providers.
func (f *Factory) Register(pt ProviderType, builder ProviderBuilder) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.builders[pt] = builder
}

// Create returns a provider instance, caching by type+config hash.
func (f *Factory) Create(pt ProviderType, opts ...ConfigOption) (llm.Provider, error) {
	cfg := Config{
		HTTPClient: &http.Client{},
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	// Apply environment defaults
	if cfg.APIKey == "" {
		cfg.APIKey = envKey(pt)
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = envBaseURL(pt)
	}

	cacheKey := fmt.Sprintf("%s:%s:%s", pt, cfg.APIKey[:min(8, len(cfg.APIKey))], cfg.BaseURL)

	f.mu.RLock()
	if p, ok := f.cache[cacheKey]; ok {
		f.mu.RUnlock()
		return p, nil
	}
	f.mu.RUnlock()

	f.mu.Lock()
	defer f.mu.Unlock()

	// Double-check after acquiring write lock
	if p, ok := f.cache[cacheKey]; ok {
		return p, nil
	}

	builder, ok := f.builders[pt]
	if !ok {
		return nil, fmt.Errorf("unknown provider type: %s", pt)
	}

	p := builder(cfg)
	f.cache[cacheKey] = p
	return p, nil
}

// CreateByID creates a provider from string ID.
func (f *Factory) CreateByID(id string, opts ...ConfigOption) (llm.Provider, error) {
	switch id {
	case "anthropic", "claude":
		return f.Create(ProviderAnthropic, opts...)
	case "openai", "gpt":
		return f.Create(ProviderOpenAI, opts...)
	case "google", "gemini":
		return f.Create(ProviderGoogle, opts...)
	default:
		return nil, fmt.Errorf("unknown provider: %s", id)
	}
}

// Clear removes cached providers.
func (f *Factory) Clear() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cache = make(map[string]llm.Provider)
}

// Default is the global factory instance.
var Default = NewFactory()

// envKey returns environment variable for API key.
func envKey(pt ProviderType) string {
	switch pt {
	case ProviderAnthropic:
		return os.Getenv("ANTHROPIC_API_KEY")
	case ProviderOpenAI:
		return os.Getenv("OPENAI_API_KEY")
	case ProviderGoogle:
		if k := os.Getenv("GOOGLE_API_KEY"); k != "" {
			return k
		}
		return os.Getenv("GEMINI_API_KEY")
	}
	return ""
}

// envBaseURL returns environment variable for base URL.
func envBaseURL(pt ProviderType) string {
	switch pt {
	case ProviderAnthropic:
		return os.Getenv("ANTHROPIC_BASE_URL")
	case ProviderOpenAI:
		return os.Getenv("OPENAI_BASE_URL")
	}
	return ""
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
