// Package config contains utility functions for model configuration
package config

import (
	"fmt"

	"github.com/joss/urp/internal/opencode/provider"
	"github.com/joss/urp/pkg/llm"
)

// ModelConfig holds configuration for a specific model with fallback options
type ModelConfig struct {
	ModelID     string
	URL         string
	APIKey      string
	Fallbacks   []string
}

// GetModelWithFallback attempts to get a working model with fallback options
func GetModelWithFallback(modelID string, fallbackModels []string, configOptions ...provider.ConfigOption) (llm.Provider, string, error) {
	// First try the primary model
	if modelID != "" {
		prov, resolvedModelID, err := provider.Default.CreateForModel(modelID, configOptions...)
		if err == nil && prov != nil {
			return prov, resolvedModelID, nil
		}
		// If primary model fails, continue to try fallbacks
	}

	// Try each fallback model in order
	for _, fallbackModel := range fallbackModels {
		if fallbackModel == "" {
			continue
		}
		prov, resolvedModelID, err := provider.Default.CreateForModel(fallbackModel, configOptions...)
		if err == nil && prov != nil {
			return prov, resolvedModelID, nil
		}
	}

	// If all models fail, return an error
	return nil, "", fmt.Errorf("all primary and fallback models failed to initialize")
}

// GetMasterModelConfig returns the configuration for the master model with appropriate fallbacks
func GetMasterModelConfig() ModelConfig {
	return ModelConfig{
		ModelID:   GetEnvOrDefault("URP_MASTER_MODEL_ID", ""),
		URL:       GetEnvOrDefault("URP_MASTER_MODEL_URL", ""),
		APIKey:    GetEnvOrDefault("URP_MASTER_MODEL_API_KEY", ""),
		Fallbacks: []string{
			GetEnvOrDefault("URP_DEFAULT_MASTER_MODEL", "anthropic/claude-sonnet-4-5-20250929"),
			"gpt-4o-mini",
			"gpt-4o",
		},
	}
}

// GetGateModelConfig returns the configuration for the gate model with appropriate fallbacks
func GetGateModelConfig() ModelConfig {
	return ModelConfig{
		ModelID:   GetEnvOrDefault("URP_GATE_MODEL_ID", ""),
		URL:       GetEnvOrDefault("URP_GATE_MODEL_URL", ""),
		APIKey:    GetEnvOrDefault("URP_GATE_MODEL_API_KEY", ""),
		Fallbacks: []string{
			GetEnvOrDefault("URP_DEFAULT_GATE_MODEL", "gpt-4o-mini"),
			"gpt-4o-mini",
			GetEnvOrDefault("URP_DEFAULT_MASTER_MODEL", "anthropic/claude-sonnet-4-5-20250929"),
		},
	}
}

// GetWorkerModelConfig returns the configuration for the worker model with appropriate fallbacks
func GetWorkerModelConfig() ModelConfig {
	return ModelConfig{
		ModelID:   GetEnvOrDefault("URP_WORKER_MODEL_ID", ""),
		URL:       GetEnvOrDefault("URP_WORKER_MODEL_URL", ""),
		APIKey:    GetEnvOrDefault("URP_WORKER_MODEL_API_KEY", ""),
		Fallbacks: []string{
			GetEnvOrDefault("URP_CODING_MODEL_ID", "deepseek-coder"),
			"deepseek-chat",
			GetEnvOrDefault("URP_DEFAULT_MASTER_MODEL", "anthropic/claude-sonnet-4-5-20250929"),
			"gpt-4o-mini",
		},
	}
}

// GetCodingModelConfig returns the configuration for the coding model with appropriate fallbacks
func GetCodingModelConfig() ModelConfig {
	return ModelConfig{
		ModelID:   GetEnvOrDefault("URP_CODING_MODEL_ID", ""),
		URL:       GetEnvOrDefault("URP_CODING_MODEL_URL", ""),
		APIKey:    GetEnvOrDefault("URP_CODING_MODEL_API_KEY", ""),
		Fallbacks: []string{
			"deepseek-coder",
			"gpt-4o",
			GetEnvOrDefault("URP_DEFAULT_MASTER_MODEL", "anthropic/claude-sonnet-4-5-20250929"),
		},
	}
}

// GetReasoningModelConfig returns the configuration for the reasoning model with appropriate fallbacks
func GetReasoningModelConfig() ModelConfig {
	return ModelConfig{
		ModelID:   GetEnvOrDefault("URP_REASONING_MODEL_ID", ""),
		URL:       GetEnvOrDefault("URP_REASONING_MODEL_URL", ""),
		APIKey:    GetEnvOrDefault("URP_REASONING_MODEL_API_KEY", ""),
		Fallbacks: []string{
			"o1",
			GetEnvOrDefault("URP_DEFAULT_MASTER_MODEL", "anthropic/claude-sonnet-4-5-20250929"),
			"gpt-4o",
		},
	}
}

// GetFastModelConfig returns the configuration for the fast model with appropriate fallbacks
func GetFastModelConfig() ModelConfig {
	return ModelConfig{
		ModelID:   GetEnvOrDefault("URP_FAST_MODEL_ID", ""),
		URL:       GetEnvOrDefault("URP_FAST_MODEL_URL", ""),
		APIKey:    GetEnvOrDefault("URP_FAST_MODEL_API_KEY", ""),
		Fallbacks: []string{
			"gpt-4o-mini",
			"claude-3-5-haiku-20241022",
			GetEnvOrDefault("URP_DEFAULT_GATE_MODEL", "gpt-4o-mini"),
		},
	}
}