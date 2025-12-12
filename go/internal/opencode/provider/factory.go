// Package provider implements LLM provider factories and interfaces.
package provider

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/joss/urp/internal/opencode/model"
	"github.com/joss/urp/internal/opencode/modelservice"
	"github.com/joss/urp/pkg/llm"
)

// ProviderType identifies supported LLM providers.
type ProviderType string

const (
	ProviderAnthropic ProviderType = "anthropic"
	ProviderOpenAI    ProviderType = "openai"
	ProviderGoogle    ProviderType = "google"
	ProviderDeepSeek  ProviderType = "deepseek"
	ProviderUnified   ProviderType = "unified"
)

// providerTypeFromSource maps modelservice.Source to ProviderType.
func providerTypeFromSource(source modelservice.Source) ProviderType {
	switch source {
	case modelservice.SourceProxy:
		return ProviderUnified
	case modelservice.SourceOpenAI:
		return ProviderOpenAI
	case modelservice.SourceAnthropic:
		return ProviderAnthropic
	case modelservice.SourceGoogle:
		return ProviderGoogle
	case modelservice.SourceDeepSeek:
		return ProviderDeepSeek
	default:
		return ProviderUnified // fallback
	}
}

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
	f.Register(ProviderDeepSeek, func(cfg Config) llm.Provider {
		return NewDeepSeekProvider()
	})
	f.Register(ProviderUnified, func(cfg Config) llm.Provider {
		return NewUnifiedProvider(cfg.APIKey, cfg.BaseURL, model.DefaultModelRegistry)
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
	case "deepseek", "deepseek-chat", "deepseek-coder":
		return f.Create(ProviderDeepSeek, opts...)
	case "unified", "proxy":
		return f.Create(ProviderUnified, opts...)
	default:
		return nil, fmt.Errorf("unknown provider: %s", id)
	}
}

// CreateForModel creates a provider appropriate for the given model ID or shortcode.
// Returns the provider, the resolved model ID, and any error.
//
// Logic:
// 1. If proxy credentials (PROXY_BASE_URL + PROXY_API_KEY) are set:
//    - For Anthropic models: Use AnthropicProvider with proxy URL
//    - For other models: Use UnifiedProvider (OpenAI-compatible proxy)
// 2. Otherwise: Use the model's registered source provider
func (f *Factory) CreateForModel(modelIDOrShortcode string, opts ...ConfigOption) (llm.Provider, string, error) {
	fmt.Printf("[DEBUG] CreateForModel called for model: %s\n", modelIDOrShortcode)
	svc := modelservice.DefaultService
	// Try shortcode first
	modelWithSource, ok := svc.ResolveShortCode(modelIDOrShortcode)
	if !ok {
		// Try as model ID
		modelWithSource, ok = svc.FindModel(modelIDOrShortcode)
	}
	if !ok {
		return nil, "", fmt.Errorf("model not found: %s", modelIDOrShortcode)
	}
	fmt.Printf("[DEBUG] Model found: %s, Source: %s\n", modelWithSource.ID, modelWithSource.Source)

	// Apply all options to get the final configuration
	cfg := Config{}
	for _, opt := range opts {
		opt(&cfg)
	}

	// Load proxy credentials from environment if not provided
	proxyAPIKey := cfg.APIKey
	proxyBaseURL := cfg.BaseURL
	if proxyAPIKey == "" {
		proxyAPIKey = os.Getenv("PROXY_API_KEY")
		if proxyAPIKey == "" {
			proxyAPIKey = os.Getenv("UNIFIED_API_KEY")
		}
	}
	if proxyBaseURL == "" {
		proxyBaseURL = os.Getenv("PROXY_BASE_URL")
		if proxyBaseURL == "" {
			proxyBaseURL = os.Getenv("UNIFIED_BASE_URL")
		}
	}

	fmt.Printf("[DEBUG] Config for model %s: APIKey='%s', BaseURL='%s', ProxyKey='%s', ProxyURL='%s'\n",
		modelWithSource.ID, cfg.APIKey, cfg.BaseURL, proxyAPIKey, proxyBaseURL)

	// LOGIC: If proxy is configured, check model source
	if proxyBaseURL != "" && proxyAPIKey != "" {
		// Proxy is configured
		switch modelWithSource.Source {
		case modelservice.SourceAnthropic:
			// Check if proxy URL already has /v1/chat or /chat pattern
			// If not, assume it's OpenAI-compatible and add /v1/chat/completions
			hasOpenAIPattern := strings.Contains(proxyBaseURL, "/v1/chat") ||
				strings.Contains(proxyBaseURL, "/chat/completions")

			if !hasOpenAIPattern {
				// Proxy is likely OpenAI-compatible base URL
				// Add OpenAI-compatible endpoint
				openaiProxyURL := proxyBaseURL
				if openaiProxyURL[len(openaiProxyURL)-1] == '/' {
					openaiProxyURL = openaiProxyURL[:len(openaiProxyURL)-1]
				}
				openaiProxyURL = openaiProxyURL + "/v1/chat/completions"

				fmt.Printf("[DEBUG] Detected OpenAI-compatible proxy for Anthropic model: %s, using OpenAI provider with URL: %s\n", modelWithSource.ID, openaiProxyURL)
				proxyOpts := []ConfigOption{
					WithAPIKey(proxyAPIKey),
					WithBaseURL(openaiProxyURL),
				}
				provider, err := f.Create(ProviderOpenAI, proxyOpts...)
				if err != nil {
					fmt.Printf("[DEBUG] Failed to create OpenAI provider for Anthropic model: %v\n", err)
					return nil, "", err
				}
				return provider, modelWithSource.ID, nil
			}

			// Use Anthropic provider but with proxy URL
			fmt.Printf("[DEBUG] Using Anthropic provider with proxy URL for model: %s\n", modelWithSource.ID)
			proxyOpts := []ConfigOption{
				WithAPIKey(proxyAPIKey),
				WithBaseURL(proxyBaseURL),
			}
			provider, err := f.Create(ProviderAnthropic, proxyOpts...)
			if err != nil {
				fmt.Printf("[DEBUG] Failed to create Anthropic provider with proxy: %v\n", err)
				return nil, "", err
			}
			return provider, modelWithSource.ID, nil

		case modelservice.SourceOpenAI:
			// Use OpenAI provider but with proxy URL
			fmt.Printf("[DEBUG] Using OpenAI provider with proxy URL for model: %s\n", modelWithSource.ID)
			proxyOpts := []ConfigOption{
				WithAPIKey(proxyAPIKey),
				WithBaseURL(proxyBaseURL),
			}
			provider, err := f.Create(ProviderOpenAI, proxyOpts...)
			if err != nil {
				fmt.Printf("[DEBUG] Failed to create OpenAI provider with proxy: %v\n", err)
				return nil, "", err
			}
			return provider, modelWithSource.ID, nil

		default:
			// For other models (DeepSeek, Google, etc.), use unified/proxy provider
			fmt.Printf("[DEBUG] Using unified provider for model: %s\n", modelWithSource.ID)
			proxyOpts := []ConfigOption{
				WithAPIKey(proxyAPIKey),
				WithBaseURL(proxyBaseURL),
			}
			provider, err := f.Create(ProviderUnified, proxyOpts...)
			if err != nil {
				fmt.Printf("[DEBUG] Failed to create unified provider: %v\n", err)
				return nil, "", err
			}
			return provider, modelWithSource.ID, nil
		}
	}

	// No proxy configured - use original source provider
	fmt.Printf("[DEBUG] No proxy configured, using original source provider: %s\n", modelWithSource.Source)
	providerType := providerTypeFromSource(modelWithSource.Source)
	provider, err := f.Create(providerType, opts...)
	if err != nil {
		return nil, "", err
	}
	return provider, modelWithSource.ID, nil
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
	case ProviderDeepSeek:
		return os.Getenv("DEEPSEEK_API_KEY")
	case ProviderUnified:
		if k := os.Getenv("UNIFIED_API_KEY"); k != "" {
			return k
		}
		return os.Getenv("PROXY_API_KEY")
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
	case ProviderDeepSeek:
		return os.Getenv("DEEPSEEK_BASE_URL")
	case ProviderUnified:
		if u := os.Getenv("UNIFIED_BASE_URL"); u != "" {
			return u
		}
		return os.Getenv("PROXY_BASE_URL")
	}
	return ""
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
