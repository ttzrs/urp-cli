// Package model provides model registry for dynamic LLM model configuration
package model

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"gopkg.in/yaml.v3"
)

// ModelInfo describes an LLM model's capabilities and costs
type ModelInfo struct {
	ID           string   `yaml:"id"`
	Alias        []string `yaml:"alias,omitempty"`
	Capabilities []string `yaml:"capabilities"`
	QualityTier  int      `yaml:"quality_tier"` // 1=basic, 2=good, 3=excellent
	SpeedTier    int      `yaml:"speed_tier"`   // 1=slow, 2=medium, 3=fast
	InputCost    float64  `yaml:"input_cost"`   // $/1M tokens
	OutputCost   float64  `yaml:"output_cost"`  // $/1M tokens
	ContextSize  int      `yaml:"context_size"`
	Enabled      bool     `yaml:"enabled"`
	Provider     string   `yaml:"provider,omitempty"` // Optional: "deepseek-direct", "proxy" (default)
}

// HasCapability checks if model has a specific capability
func (m *ModelInfo) HasCapability(cap string) bool {
	for _, c := range m.Capabilities {
		if c == cap {
			return true
		}
	}
	return false
}

// GetProvider returns the provider for this model (default: "proxy")
func (m *ModelInfo) GetProvider() string {
	if m.Provider == "" {
		return "proxy"
	}
	return m.Provider
}

// IsDeepSeekDirect returns true if model uses DeepSeek direct API
func (m *ModelInfo) IsDeepSeekDirect() bool {
	return m.Provider == "deepseek-direct"
}

// EstimateCost estimates cost for given token counts
func (m *ModelInfo) EstimateCost(inputTokens, outputTokens int) float64 {
	return float64(inputTokens)*m.InputCost/1_000_000 +
		float64(outputTokens)*m.OutputCost/1_000_000
}

// ModelRegistry manages available models loaded from config
type ModelRegistry struct {
	mu      sync.RWMutex
	models  map[string]*ModelInfo
	aliases map[string]string // alias → model_id
	byTier  map[int][]*ModelInfo
}

// NewModelRegistry creates an empty registry
func NewModelRegistry() *ModelRegistry {
	return &ModelRegistry{
		models:  make(map[string]*ModelInfo),
		aliases: make(map[string]string),
		byTier:  make(map[int][]*ModelInfo),
	}
}

// modelsConfig is the YAML structure for models.yaml
type modelsConfig struct {
	Models []ModelInfo `yaml:"models"`
}

// LoadFromFile loads models from a YAML config file
func (r *ModelRegistry) LoadFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}

	var cfg modelsConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Clear existing
	r.models = make(map[string]*ModelInfo)
	r.aliases = make(map[string]string)
	r.byTier = make(map[int][]*ModelInfo)

	// Load models
	for i := range cfg.Models {
		model := &cfg.Models[i]
		if model.ID == "" {
			continue
		}

		// Default enabled to true if not specified
		if !model.Enabled && model.InputCost > 0 {
			model.Enabled = true
		}

		r.models[model.ID] = model

		// Register aliases
		for _, alias := range model.Alias {
			r.aliases[alias] = model.ID
		}

		// Index by tier
		r.byTier[model.QualityTier] = append(r.byTier[model.QualityTier], model)
	}

	return nil
}

// LoadFromDefault loads from ~/.urp/models.yaml or uses builtin defaults
func (r *ModelRegistry) LoadFromDefault() error {
	// Try user config first
	home, _ := os.UserHomeDir()
	userConfig := filepath.Join(home, ".urp", "models.yaml")
	if _, err := os.Stat(userConfig); err == nil {
		if loadErr := r.LoadFromFile(userConfig); loadErr == nil {
			// Successfully loaded from user config
			return nil
		}
		// Config exists but failed to load - fall through to defaults
	}

	// Fall back to builtin defaults
	r.loadBuiltinDefaults()
	return nil
}

// loadBuiltinDefaults sets up default models
func (r *ModelRegistry) loadBuiltinDefaults() {
	r.mu.Lock()
	defer r.mu.Unlock()

	defaults := []ModelInfo{
		// Tier 3: Premium
		{
			ID:           "claude-opus-4-20250514",
			Alias:        []string{"opus", "claude-opus"},
			QualityTier:  3,
			SpeedTier:    1,
			InputCost:    15,
			OutputCost:   75,
			ContextSize:  200000,
			Capabilities: []string{"tool_use", "code", "reasoning", "vision"},
			Enabled:      true,
		},
		{
			ID:           "o1",
			Alias:        []string{"o1"},
			QualityTier:  3,
			SpeedTier:    1,
			InputCost:    15,
			OutputCost:   60,
			ContextSize:  200000,
			Capabilities: []string{"code", "reasoning"},
			Enabled:      true,
		},

		// Tier 2: Balanced
		{
			ID:           "claude-sonnet-4-20250514",
			Alias:        []string{"sonnet", "claude-sonnet"},
			QualityTier:  2,
			SpeedTier:    2,
			InputCost:    3,
			OutputCost:   15,
			ContextSize:  200000,
			Capabilities: []string{"tool_use", "code", "reasoning", "vision"},
			Enabled:      true,
		},
		{
			ID:           "gpt-4o",
			Alias:        []string{"gpt4o", "4o"},
			QualityTier:  2,
			SpeedTier:    2,
			InputCost:    2.5,
			OutputCost:   10,
			ContextSize:  128000,
			Capabilities: []string{"tool_use", "code", "reasoning", "vision"},
			Enabled:      true,
		},
		{
			ID:           "deepseek-chat",
			Alias:        []string{"deepseek"},
			QualityTier:  2,
			SpeedTier:    2,
			InputCost:    0.14,
			OutputCost:   0.28,
			ContextSize:  64000,
			Capabilities: []string{"tool_use", "code", "reasoning"},
			Enabled:      true,
		},
		{
			ID:           "qwen-max",
			Alias:        []string{"qwen"},
			QualityTier:  2,
			SpeedTier:    2,
			InputCost:    0.4,
			OutputCost:   1.2,
			ContextSize:  32000,
			Capabilities: []string{"tool_use", "code", "reasoning"},
			Enabled:      true,
		},
		{
			ID:           "gemini-1.5-pro",
			Alias:        []string{"gemini-pro"},
			QualityTier:  2,
			SpeedTier:    2,
			InputCost:    1.25,
			OutputCost:   5,
			ContextSize:  2000000,
			Capabilities: []string{"tool_use", "code", "reasoning", "vision", "long_context"},
			Enabled:      true,
		},

		// Tier 1: Fast/Cheap
		{
			ID:           "claude-3-5-haiku-20241022",
			Alias:        []string{"haiku", "claude-haiku"},
			QualityTier:  1,
			SpeedTier:    3,
			InputCost:    0.8,
			OutputCost:   4,
			ContextSize:  200000,
			Capabilities: []string{"tool_use", "code"},
			Enabled:      true,
		},
		{
			ID:           "gpt-4o-mini",
			Alias:        []string{"mini", "4o-mini"},
			QualityTier:  1,
			SpeedTier:    3,
			InputCost:    0.15,
			OutputCost:   0.6,
			ContextSize:  128000,
			Capabilities: []string{"tool_use", "code"},
			Enabled:      true,
		},
		{
			ID:           "deepseek-coder",
			Alias:        []string{"deepseek-code"},
			QualityTier:  1,
			SpeedTier:    3,
			InputCost:    0.14,
			OutputCost:   0.28,
			ContextSize:  64000,
			Capabilities: []string{"code"},
			Enabled:      true,
		},
		{
			ID:           "gemini-1.5-flash",
			Alias:        []string{"flash", "gemini-flash"},
			QualityTier:  1,
			SpeedTier:    3,
			InputCost:    0.075,
			OutputCost:   0.3,
			ContextSize:  1000000,
			Capabilities: []string{"tool_use", "code", "vision", "long_context"},
			Enabled:      true,
		},
	}

	r.models = make(map[string]*ModelInfo)
	r.aliases = make(map[string]string)
	r.byTier = make(map[int][]*ModelInfo)

	for i := range defaults {
		model := &defaults[i]
		r.models[model.ID] = model

		for _, alias := range model.Alias {
			r.aliases[alias] = model.ID
		}

		r.byTier[model.QualityTier] = append(r.byTier[model.QualityTier], model)
	}
}

// Get retrieves a model by ID or alias
func (r *ModelRegistry) Get(idOrAlias string) *ModelInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Try direct lookup
	if model, ok := r.models[idOrAlias]; ok {
		return model
	}

	// Try alias
	if modelID, ok := r.aliases[idOrAlias]; ok {
		return r.models[modelID]
	}

	return nil
}

// ListByTier returns all models of a given quality tier
func (r *ModelRegistry) ListByTier(tier int) []*ModelInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	models := r.byTier[tier]
	result := make([]*ModelInfo, len(models))
	copy(result, models)
	return result
}

// ListEnabled returns all enabled models
func (r *ModelRegistry) ListEnabled() []*ModelInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*ModelInfo
	for _, model := range r.models {
		if model.Enabled {
			result = append(result, model)
		}
	}
	return result
}

// ListAll returns all models
func (r *ModelRegistry) ListAll() []*ModelInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*ModelInfo, 0, len(r.models))
	for _, model := range r.models {
		result = append(result, model)
	}
	return result
}

// CheapestWithCap finds the cheapest model with a required capability and minimum tier
func (r *ModelRegistry) CheapestWithCap(capability string, minTier int) *ModelInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var candidates []*ModelInfo
	for _, model := range r.models {
		if !model.Enabled || model.QualityTier < minTier {
			continue
		}
		if model.HasCapability(capability) {
			candidates = append(candidates, model)
		}
	}

	if len(candidates) == 0 {
		return nil
	}

	// Sort by input cost (primary) then output cost (secondary)
	sort.Slice(candidates, func(i, j int) bool {
		costI := candidates[i].InputCost + candidates[i].OutputCost
		costJ := candidates[j].InputCost + candidates[j].OutputCost
		return costI < costJ
	})

	return candidates[0]
}

// CheapestForContext finds cheapest model that can handle given context size
func (r *ModelRegistry) CheapestForContext(tokens int, minTier int) *ModelInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var candidates []*ModelInfo
	for _, model := range r.models {
		if !model.Enabled || model.QualityTier < minTier {
			continue
		}
		if model.ContextSize >= tokens {
			candidates = append(candidates, model)
		}
	}

	if len(candidates) == 0 {
		return nil
	}

	sort.Slice(candidates, func(i, j int) bool {
		costI := candidates[i].InputCost + candidates[i].OutputCost
		costJ := candidates[j].InputCost + candidates[j].OutputCost
		return costI < costJ
	})

	return candidates[0]
}

// Register adds or updates a model in the registry
func (r *ModelRegistry) Register(model *ModelInfo) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.models[model.ID] = model

	for _, alias := range model.Alias {
		r.aliases[alias] = model.ID
	}

	// Update tier index
	r.byTier[model.QualityTier] = append(r.byTier[model.QualityTier], model)
}

// Count returns number of registered models
func (r *ModelRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.models)
}

// DefaultModelRegistry is the global model registry
var DefaultModelRegistry = NewModelRegistry()

func init() {
	// Try to load from user config, fall back to builtin defaults
	DefaultModelRegistry.LoadFromDefault()
}
