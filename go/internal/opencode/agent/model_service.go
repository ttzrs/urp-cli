// Package agent provides unified model selection
package agent

import (
	"context"
	"fmt"
	"sync"

	"github.com/joss/urp/internal/opencode/model"
	"github.com/joss/urp/internal/opencode/provider"
	"github.com/joss/urp/pkg/llm"
)

// ModelService provides a clean unified API for model selection and provisioning
// This replaces the fragmented GetModelWithFallback + CreateForModel patterns
type ModelService struct {
	mu              sync.RWMutex
	decisionEngine  *ModelDecisionEngine
	providerFactory *provider.Factory
	registry        *model.ModelRegistry
	providerCache   map[string]llm.Provider
}

// NewModelService creates the unified model selection service
func NewModelService(
	decisionEngine *ModelDecisionEngine,
	providerFactory *provider.Factory,
	registry *model.ModelRegistry,
) *ModelService {
	if providerFactory == nil {
		providerFactory = provider.Default
	}
	if registry == nil {
		registry = model.DefaultModelRegistry
	}

	return &ModelService{
		decisionEngine:  decisionEngine,
		providerFactory: providerFactory,
		registry:        registry,
		providerCache:   make(map[string]llm.Provider),
	}
}

// SelectAndProvision selects the best model and returns a ready-to-use provider
// This is the main entry point for model provisioning
func (ms *ModelService) SelectAndProvision(
	ctx context.Context,
	input *DecisionInput,
	configOpts ...provider.ConfigOption,
) (llm.Provider, *ModelSelection, *DecisionLog, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	// Step 1: Make decision
	selection, decisionLog, err := ms.decisionEngine.Decide(ctx, input)
	if err != nil {
		return nil, nil, decisionLog, fmt.Errorf("decision failed: %w", err)
	}

	if selection == nil {
		return nil, nil, decisionLog, fmt.Errorf("no model selected")
	}

	// Step 2: Create provider for selected model
	prov, _, err := ms.providerFactory.CreateForModel(selection.ModelID, configOpts...)
	if err != nil {
		// Try fallback chain
		return ms.tryFallback(ctx, selection.ModelID, input, configOpts...)
	}

	if prov == nil {
		return nil, selection, decisionLog, fmt.Errorf("provider creation returned nil")
	}

	return prov, selection, decisionLog, nil
}

// Select makes a model decision without provisioning the provider
// Useful for planning/logging decisions before actually using them
func (ms *ModelService) Select(ctx context.Context, input *DecisionInput) (*ModelSelection, *DecisionLog, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	return ms.decisionEngine.Decide(ctx, input)
}

// Provision creates a provider for a specific model ID
func (ms *ModelService) Provision(
	ctx context.Context,
	modelID string,
	configOpts ...provider.ConfigOption,
) (llm.Provider, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	// Check cache first
	cacheKey := fmt.Sprintf("%s:%v", modelID, configOpts)
	if prov, ok := ms.providerCache[cacheKey]; ok {
		return prov, nil
	}

	// Create new provider
	prov, _, err := ms.providerFactory.CreateForModel(modelID, configOpts...)
	if err != nil {
		return nil, fmt.Errorf("provision failed for %s: %w", modelID, err)
	}

	ms.providerCache[cacheKey] = prov
	return prov, nil
}

// tryFallback attempts to provision from the fallback chain
func (ms *ModelService) tryFallback(
	ctx context.Context,
	selectedModel string,
	input *DecisionInput,
	configOpts ...provider.ConfigOption,
) (llm.Provider, *ModelSelection, *DecisionLog, error) {
	fallbacks := ms.decisionEngine.config.Fallback.Default
	if byStrat, ok := ms.decisionEngine.config.Fallback.ByStrategy[input.Strategy]; ok {
		fallbacks = byStrat
	}

	var lastErr error
	for _, modelID := range fallbacks {
		if modelID == selectedModel {
			continue // Skip the one that failed
		}

		prov, _, err := ms.providerFactory.CreateForModel(modelID, configOpts...)
		if err != nil {
			lastErr = err
			continue
		}

		if prov != nil {
			return prov, &ModelSelection{
				ModelID:    modelID,
				Confidence: 0.5,
				Reason:     fmt.Sprintf("fallback from %s", selectedModel),
			}, &DecisionLog{
				Selected:  modelID,
				Fallback:  true,
				Reason:    fmt.Sprintf("fallback from %s", selectedModel),
			}, nil
		}
	}

	return nil, nil, nil, fmt.Errorf("all models failed, last error: %w", lastErr)
}

// GetDecisionAudit returns all decisions for a session
func (ms *ModelService) GetDecisionAudit(ctx context.Context, sessionID string) ([]DecisionLog, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	return ms.decisionEngine.GetDecisionLog(ctx, sessionID)
}

// RecordOutcome records the result of using a model
func (ms *ModelService) RecordOutcome(
	ctx context.Context,
	decision *DecisionLog,
	cost float64,
	success bool,
) error {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	return ms.decisionEngine.RecordOutcome(ctx, decision, cost, success)
}

// GetModelInfo returns information about a model
func (ms *ModelService) GetModelInfo(modelID string) *model.ModelInfo {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	return ms.registry.Get(modelID)
}

// ListModels returns all available models
func (ms *ModelService) ListModels() []*model.ModelInfo {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	return ms.registry.ListEnabled()
}

// ListByTier returns models of a specific quality tier
func (ms *ModelService) ListByTier(tier int) []*model.ModelInfo {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	return ms.registry.ListByTier(tier)
}

// GetStrategy exports a decision strategy
func (ms *ModelService) GetStrategy(name string) interface{} {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	return ms.decisionEngine.ExportStrategy(name)
}

// ClearCache removes cached providers
func (ms *ModelService) ClearCache() {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	ms.providerCache = make(map[string]llm.Provider)
	ms.providerFactory.Clear()
}

// Global default service instance
var DefaultModelService *ModelService

// InitDefaultModelService initializes the global service
// Call this during app bootstrap
func InitDefaultModelService(
	decisionEngine *ModelDecisionEngine,
	providerFactory *provider.Factory,
	registry *model.ModelRegistry,
) {
	DefaultModelService = NewModelService(decisionEngine, providerFactory, registry)
}
