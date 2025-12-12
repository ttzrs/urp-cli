// Package config contains utility functions for model configuration
package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/joss/urp/internal/opencode/model"
	"github.com/joss/urp/internal/opencode/provider"
	"github.com/joss/urp/pkg/llm"
)

// ModelConfig holds configuration for a specific model with fallback options
type ModelConfig struct {
	ModelID   string
	URL       string
	APIKey    string
	Fallbacks []string
}

// GetModelWithFallback attempts to get a working model with fallback options
func GetModelWithFallback(modelID string, fallbackModels []string, configOptions ...provider.ConfigOption) (llm.Provider, string, error) {

	// SPECIAL CASE: If model is zai-glm-4.6 and we have proxy credentials, use unified provider directly
	if modelID == "zai-glm-4.6" {
		// Check for proxy credentials in the environment directly
		apiKey := os.Getenv("UNIFIED_API_KEY")
		if apiKey == "" {
			apiKey = os.Getenv("PROXY_API_KEY")
		}
		baseURL := os.Getenv("UNIFIED_BASE_URL")
		if baseURL == "" {
			baseURL = os.Getenv("PROXY_BASE_URL")
		}

		if apiKey != "" && baseURL != "" && strings.Contains(baseURL, "tizz.win") {
			fmt.Printf("[DEBUG] GetModelWithFallback: Detected zai-glm-4.6 with proxy config, creating unified provider\n")
			// Create unified provider directly for zai-glm-4.6 with proxy settings
			prov := provider.NewUnifiedProvider(apiKey, baseURL, model.DefaultModelRegistry)
			return prov, modelID, nil
		}
	}

	// First try the primary model with default provider
	if modelID != "" {
		prov, resolvedModelID, err := provider.Default.CreateForModel(modelID, configOptions...)
		if err == nil && prov != nil {
			return prov, resolvedModelID, nil
		}
		// If primary model fails, continue to try fallbacks
	}

	// SPECIAL CASE for fallback: If fallback is zai-glm-4.6 and proxy credentials available
	for _, fallbackModel := range fallbackModels {
		if fallbackModel == "zai-glm-4.6" {
			// Check for proxy credentials in the environment directly
			apiKey := os.Getenv("UNIFIED_API_KEY")
			if apiKey == "" {
				apiKey = os.Getenv("PROXY_API_KEY")
			}
			baseURL := os.Getenv("UNIFIED_BASE_URL")
			if baseURL == "" {
				baseURL = os.Getenv("PROXY_BASE_URL")
			}

			if apiKey != "" && baseURL != "" && strings.Contains(baseURL, "tizz.win") {
				fmt.Printf("[DEBUG] GetModelWithFallback: Detected fallback zai-glm-4.6 with proxy config, creating unified provider\n")
				// Create unified provider directly for fallback zai-glm-4.6 with proxy settings
				prov := provider.NewUnifiedProvider(apiKey, baseURL, model.DefaultModelRegistry)
				return prov, fallbackModel, nil
			}
		}

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
		ModelID: GetEnvOrDefault("URP_MASTER_MODEL_ID", ""),
		URL:     GetEnvOrDefault("URP_MASTER_MODEL_URL", ""),
		APIKey:  GetEnvOrDefault("URP_MASTER_MODEL_API_KEY", ""),
		Fallbacks: []string{
			GetEnvOrDefault("URP_DEFAULT_MASTER_MODEL", "claude-opus-4-5-20251101"),
			"gpt-5.1",
			"claude-sonnet-4-5-20250929",
		},
	}
}

// GetGateModelConfig returns the configuration for the gate model with appropriate fallbacks
func GetGateModelConfig() ModelConfig {
	return ModelConfig{
		ModelID: GetEnvOrDefault("URP_GATE_MODEL_ID", ""),
		URL:     GetEnvOrDefault("URP_GATE_MODEL_URL", ""),
		APIKey:  GetEnvOrDefault("URP_GATE_MODEL_API_KEY", ""),
		Fallbacks: []string{
			GetEnvOrDefault("URP_DEFAULT_GATE_MODEL", "claude-haiku-4-5-20251001"),
			"claude-3-5-haiku-20241022",
			GetEnvOrDefault("URP_DEFAULT_MASTER_MODEL", "claude-opus-4-5-20251101"),
		},
	}
}

// GetWorkerModelConfig returns the configuration for the worker model with appropriate fallbacks
func GetWorkerModelConfig() ModelConfig {
	return ModelConfig{
		ModelID: GetEnvOrDefault("URP_WORKER_MODEL_ID", ""),
		URL:     GetEnvOrDefault("URP_WORKER_MODEL_URL", ""),
		APIKey:  GetEnvOrDefault("URP_WORKER_MODEL_API_KEY", ""),
		Fallbacks: []string{
			GetEnvOrDefault("URP_CODING_MODEL_ID", "claude-opus-4-5-20251101"),
			"gpt-5.1",
			GetEnvOrDefault("URP_DEFAULT_MASTER_MODEL", "claude-opus-4-5-20251101"),
			"claude-sonnet-4-5-20250929",
		},
	}
}

// GetCodingModelConfig returns the configuration for the coding model with appropriate fallbacks
func GetCodingModelConfig() ModelConfig {
	return ModelConfig{
		ModelID: GetEnvOrDefault("URP_CODING_MODEL_ID", ""),
		URL:     GetEnvOrDefault("URP_CODING_MODEL_URL", ""),
		APIKey:  GetEnvOrDefault("URP_CODING_MODEL_API_KEY", ""),
		Fallbacks: []string{
			"qwen3-coder-plus",
			"gpt-5.1",
			GetEnvOrDefault("URP_DEFAULT_MASTER_MODEL", "claude-opus-4-5-20251101"),
		},
	}
}

// GetReasoningModelConfig returns the configuration for the reasoning model with appropriate fallbacks
func GetReasoningModelConfig() ModelConfig {
	return ModelConfig{
		ModelID: GetEnvOrDefault("URP_REASONING_MODEL_ID", ""),
		URL:     GetEnvOrDefault("URP_REASONING_MODEL_URL", ""),
		APIKey:  GetEnvOrDefault("URP_REASONING_MODEL_API_KEY", ""),
		Fallbacks: []string{
			"claude-opus-4-5-thinking",
			GetEnvOrDefault("URP_DEFAULT_MASTER_MODEL", "claude-opus-4-5-20251101"),
			"gpt-5.1",
		},
	}
}

// GetFastModelConfig returns the configuration for the fast model with appropriate fallbacks
func GetFastModelConfig() ModelConfig {
	return ModelConfig{
		ModelID: GetEnvOrDefault("URP_FAST_MODEL_ID", ""),
		URL:     GetEnvOrDefault("URP_FAST_MODEL_URL", ""),
		APIKey:  GetEnvOrDefault("URP_FAST_MODEL_API_KEY", ""),
		Fallbacks: []string{
			"claude-haiku-4-5-20251001",
			"claude-3-5-haiku-20241022",
			GetEnvOrDefault("URP_DEFAULT_GATE_MODEL", "claude-haiku-4-5-20251001"),
		},
	}
}
