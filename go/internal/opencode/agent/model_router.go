// Package agent provides intelligent model routing for LLM selection
package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/joss/urp/internal/opencode/model"
	"gopkg.in/yaml.v3"
)

// ModelSelection represents the result of model routing
type ModelSelection struct {
	ModelID    string  // Selected model ID
	Confidence float64 // Confidence in selection (0-1)
	Reason     string  // Human-readable reason for selection
	EstCost    float64 // Estimated cost for this task
	RuleName   string  // Name of rule that matched (if any)
}

// RoutingRule defines a condition for model selection
type RoutingRule struct {
	Name       string `yaml:"name"`
	Condition  string `yaml:"condition"` // Expression: "task_type == 'explore'"
	Model      string `yaml:"model"`     // Model ID or alias
	Priority   int    `yaml:"priority"`  // Lower = higher priority
	MinQuality int    `yaml:"min_quality,omitempty"` // Minimum quality tier
}

// RoutingConfig holds routing configuration from YAML
type RoutingConfig struct {
	Enabled bool `yaml:"enabled"`

	Proxy struct {
		BaseURL string `yaml:"base_url"`
		APIKey  string `yaml:"api_key"`
	} `yaml:"proxy"`

	Budget struct {
		DailyLimit   float64 `yaml:"daily_limit"`
		SessionLimit float64 `yaml:"session_limit"`
		MaxPerTask   float64 `yaml:"max_per_task"`
	} `yaml:"budget"`

	Weights struct {
		Quality float64 `yaml:"quality"`
		Cost    float64 `yaml:"cost"`
		Speed   float64 `yaml:"speed"`
	} `yaml:"weights"`

	Rules         []RoutingRule `yaml:"rules"`
	FallbackChain []string      `yaml:"fallback_chain"`

	Retry struct {
		MaxAttempts int `yaml:"max_attempts"`
		BackoffMs   int `yaml:"backoff_ms"`
		TimeoutMs   int `yaml:"timeout_ms"`
	} `yaml:"retry"`

	DeepSeek struct {
		BaseURL string `yaml:"base_url"`
	} `yaml:"deepseek"`
}

// ModelRouter selects the best model for a given task
type ModelRouter struct {
	mu       sync.RWMutex
	config   *RoutingConfig
	registry *model.ModelRegistry
	learning *ModelLearningStore // Optional learning component
	budget   *BudgetTracker      // Optional budget component
	fallback string              // Default fallback model
}

// NewModelRouter creates a model router with default configuration
func NewModelRouter(registry *model.ModelRegistry) *ModelRouter {
	if registry == nil {
		registry = model.DefaultModelRegistry
	}

	router := &ModelRouter{
		registry: registry,
		config:   defaultRoutingConfig(),
		fallback: "claude-sonnet-4-20250514",
	}

	return router
}

// NewModelRouterWithConfig creates a model router and loads user config
func NewModelRouterWithConfig(registry *model.ModelRegistry) *ModelRouter {
	router := NewModelRouter(registry)
	// Try to load user config
	router.LoadConfigFromDefault()
	return router
}

// defaultRoutingConfig returns sensible defaults
func defaultRoutingConfig() *RoutingConfig {
	return &RoutingConfig{
		Enabled: true,
		Budget: struct {
			DailyLimit   float64 `yaml:"daily_limit"`
			SessionLimit float64 `yaml:"session_limit"`
			MaxPerTask   float64 `yaml:"max_per_task"`
		}{
			DailyLimit:   5.0,
			SessionLimit: 1.0,
			MaxPerTask:   0.20,
		},
		Weights: struct {
			Quality float64 `yaml:"quality"`
			Cost    float64 `yaml:"cost"`
			Speed   float64 `yaml:"speed"`
		}{
			Quality: 0.5,
			Cost:    0.3,
			Speed:   0.2,
		},
		Rules: defaultRules(),
		FallbackChain: []string{
			"deepseek-chat",
			"claude-3-5-haiku-20241022",
			"claude-sonnet-4-20250514",
		},
	}
}

// defaultRules returns built-in routing rules
func defaultRules() []RoutingRule {
	return []RoutingRule{
		{Name: "vision-required", Condition: "has_images", Model: "claude-sonnet-4-20250514", Priority: 1},
		{Name: "long-context", Condition: "tokens > 100000", Model: "claude-sonnet-4-20250514", Priority: 2},
		{Name: "complex-task", Condition: "complexity > 0.8", Model: "claude-opus-4-20250514", Priority: 3},
		{Name: "explore-fast", Condition: "task_type == 'explore'", Model: "deepseek-coder", Priority: 10},
		{Name: "explain-cheap", Condition: "task_type == 'explain' && complexity < 0.5", Model: "deepseek-chat", Priority: 11},
		{Name: "bugfix-simple", Condition: "task_type == 'bugfix' && complexity < 0.5", Model: "deepseek-coder", Priority: 12},
		{Name: "bugfix-balanced", Condition: "task_type == 'bugfix'", Model: "claude-sonnet-4-20250514", Priority: 20},
		{Name: "refactor-quality", Condition: "task_type == 'refactor'", Model: "claude-sonnet-4-20250514", Priority: 21},
		{Name: "feature-balanced", Condition: "task_type == 'feature'", Model: "claude-sonnet-4-20250514", Priority: 22},
		{Name: "default", Condition: "true", Model: "claude-sonnet-4-20250514", Priority: 100},
	}
}

// LoadConfigFromDefault loads config from ~/.urp/routing.yaml
func (r *ModelRouter) LoadConfigFromDefault() error {
	home, _ := os.UserHomeDir()
	configPath := filepath.Join(home, ".urp", "routing.yaml")
	return r.LoadConfigFromFile(configPath)
}

// routingConfigWrapper wraps RoutingConfig for YAML parsing
type routingConfigWrapper struct {
	Routing RoutingConfig `yaml:"routing"`
}

// LoadConfigFromFile loads routing configuration from a YAML file
func (r *ModelRouter) LoadConfigFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err // File doesn't exist, use defaults
	}

	// Try parsing with "routing:" wrapper first
	var wrapper routingConfigWrapper
	if err := yaml.Unmarshal(data, &wrapper); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}
	cfg := wrapper.Routing

	r.mu.Lock()
	defer r.mu.Unlock()

	// Merge with defaults for missing fields
	if len(cfg.Rules) > 0 {
		r.config.Rules = cfg.Rules
	}
	if len(cfg.FallbackChain) > 0 {
		r.config.FallbackChain = cfg.FallbackChain
	}
	if cfg.Budget.DailyLimit > 0 {
		r.config.Budget = cfg.Budget
	}
	if cfg.Weights.Quality > 0 || cfg.Weights.Cost > 0 || cfg.Weights.Speed > 0 {
		r.config.Weights = cfg.Weights
	}
	if cfg.Proxy.BaseURL != "" {
		r.config.Proxy = cfg.Proxy
	}
	r.config.Enabled = cfg.Enabled

	return nil
}

// WithLearning sets the learning store for adaptive routing
func (r *ModelRouter) WithLearning(learning *ModelLearningStore) *ModelRouter {
	r.learning = learning
	return r
}

// WithBudget sets the budget tracker
func (r *ModelRouter) WithBudget(budget *BudgetTracker) *ModelRouter {
	r.budget = budget
	return r
}

// SelectModel chooses the best model for a task
func (r *ModelRouter) SelectModel(ctx context.Context, task *TaskClassification) *ModelSelection {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// If routing disabled, use fallback
	if !r.config.Enabled {
		return &ModelSelection{
			ModelID:    r.fallback,
			Confidence: 1.0,
			Reason:     "routing disabled",
		}
	}

	// 1. Check budget constraints
	if r.budget != nil {
		if !r.budget.CanAffordTask(r.config.Budget.MaxPerTask) {
			// Use cheapest model
			cheap := r.registry.CheapestWithCap("code", 1)
			if cheap != nil {
				return &ModelSelection{
					ModelID:    cheap.ID,
					Confidence: 0.8,
					Reason:     "budget constraint - using cheapest model",
				}
			}
		}
	}

	// 2. Apply static rules in priority order
	// Note: Rules are explicit user configuration, so we trust them and skip capability checks
	// Sort rules by priority (lower priority value = higher precedence)
	rules := make([]RoutingRule, len(r.config.Rules))
	copy(rules, r.config.Rules)
	sortRulesByPriority(rules)
	
	for _, rule := range rules {
		if r.evaluateRule(rule, task) {
			modelInfo := r.registry.Get(rule.Model)
			if modelInfo == nil || !modelInfo.Enabled {
				continue // Model not available, try next rule
			}

			// Check if model meets minimum quality (if specified)
			if rule.MinQuality > 0 && modelInfo.QualityTier < rule.MinQuality {
				continue
			}

			return &ModelSelection{
				ModelID:    modelInfo.ID,
				Confidence: 0.9,
				Reason:     fmt.Sprintf("rule: %s", rule.Name),
				RuleName:   rule.Name,
				EstCost:    modelInfo.EstimateCost(task.EstTokens, task.EstTokens/2),
			}
		}
	}

	// 3. Query learning store for historical best
	if r.learning != nil {
		learned := r.learning.GetBestModel(string(task.TaskType), task.Environment, task.Complexity)
		if learned != "" {
			modelInfo := r.registry.Get(learned)
			if modelInfo != nil && modelInfo.Enabled {
				return &ModelSelection{
					ModelID:    modelInfo.ID,
					Confidence: 0.7,
					Reason:     "learned from history",
					EstCost:    modelInfo.EstimateCost(task.EstTokens, task.EstTokens/2),
				}
			}
		}
	}

	// 4. Score-based selection from candidates
	selection := r.scoredSelection(task)
	if selection != nil {
		return selection
	}

	// 5. Fallback chain
	for _, modelID := range r.config.FallbackChain {
		modelInfo := r.registry.Get(modelID)
		if modelInfo != nil && modelInfo.Enabled {
			return &ModelSelection{
				ModelID:    modelInfo.ID,
				Confidence: 0.5,
				Reason:     "fallback chain",
				EstCost:    modelInfo.EstimateCost(task.EstTokens, task.EstTokens/2),
			}
		}
	}

	// 6. Ultimate fallback
	return &ModelSelection{
		ModelID:    r.fallback,
		Confidence: 0.3,
		Reason:     "ultimate fallback",
	}
}

// evaluateRule checks if a rule condition matches the task
func (r *ModelRouter) evaluateRule(rule RoutingRule, task *TaskClassification) bool {
	condition := rule.Condition

	switch condition {
	case "true":
		return true
	case "has_images":
		return task.HasImages
	}

	// Task type conditions
	if condition == fmt.Sprintf("task_type == '%s'", task.TaskType) {
		return true
	}

	// Token conditions (check first - high priority)
	if containsWord(condition, "tokens >") {
		threshold := parseTokenThreshold(condition)
		result := task.EstTokens > threshold
		return result
	}

	// Combined conditions (simple parsing)
	// "task_type == 'explain' && complexity < 0.5"
	if containsTaskTypeMatch(condition, string(task.TaskType)) {
		// Check additional conditions with && logic
		if containsWord(condition, "&&") {
			// Must satisfy ALL conditions
			if containsWord(condition, "complexity") {
				if !containsComplexityCondition(condition, task.Complexity) {
					return false // Complexity condition failed
				}
			}
			// All conditions satisfied
			return true
		}
		// No additional conditions, just task type match is enough
		return true
	}

	// Complexity conditions alone
	if containsWord(condition, "complexity >") {
		threshold := parseComplexityThreshold(condition)
		if task.Complexity > threshold {
			return true
		}
	}

	return false
}

// hasRequiredCaps checks if model has all required capabilities
func (r *ModelRouter) hasRequiredCaps(model *model.ModelInfo, required []string) bool {
	for _, cap := range required {
		if !model.HasCapability(cap) {
			return false
		}
	}
	return true
}

// scoredSelection ranks models by weighted score
func (r *ModelRouter) scoredSelection(task *TaskClassification) *ModelSelection {
	candidates := r.registry.ListEnabled()
	if len(candidates) == 0 {
		return nil
	}

	// Filter by required capabilities
	var filtered []*model.ModelInfo
	for _, m := range candidates {
		if r.hasRequiredCaps(m, task.RequiredCaps) {
			filtered = append(filtered, m)
		}
	}

	if len(filtered) == 0 {
		filtered = candidates // Fall back to all candidates
	}

	// Score each model
	var bestModel *model.ModelInfo
	var bestScore float64

	weights := r.config.Weights
	if weights.Quality == 0 && weights.Cost == 0 && weights.Speed == 0 {
		weights.Quality = 0.5
		weights.Cost = 0.3
		weights.Speed = 0.2
	}

	for _, m := range filtered {
		// Normalize scores to 0-1
		qualityScore := float64(m.QualityTier) / 3.0
		speedScore := float64(m.SpeedTier) / 3.0

		// Cost score: cheaper is better (inverted)
		// Normalize: cheapest=$0.075 (gemini-flash) to most expensive=$75 (opus)
		maxCost := 75.0
		costScore := 1.0 - (m.InputCost+m.OutputCost)/(2*maxCost)
		if costScore < 0 {
			costScore = 0
		}

		// Complexity adjustment: complex tasks need higher quality
		if task.Complexity > 0.7 && m.QualityTier < 2 {
			qualityScore *= 0.5 // Penalize low-tier models for complex tasks
		}

		// Weighted score
		score := weights.Quality*qualityScore + weights.Cost*costScore + weights.Speed*speedScore

		if score > bestScore {
			bestScore = score
			bestModel = m
		}
	}

	if bestModel == nil {
		return nil
	}

	return &ModelSelection{
		ModelID:    bestModel.ID,
		Confidence: 0.6,
		Reason:     fmt.Sprintf("scored selection (%.2f)", bestScore),
		EstCost:    bestModel.EstimateCost(task.EstTokens, task.EstTokens/2),
	}
}

// RecordOutcome records the result of using a model (for learning)
func (r *ModelRouter) RecordOutcome(outcome *ModelOutcome) {
	if r.learning != nil {
		r.learning.Record(outcome)
	}
	if r.budget != nil && outcome.Cost > 0 {
		r.budget.Record(outcome.Cost)
	}
}

// GetConfig returns current routing config
func (r *ModelRouter) GetConfig() *RoutingConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.config
}

// SetFallback sets the default fallback model
func (r *ModelRouter) SetFallback(modelID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fallback = modelID
}

// IsEnabled returns whether routing is enabled
func (r *ModelRouter) IsEnabled() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.config.Enabled
}

// Helper functions

func sortRulesByPriority(rules []RoutingRule) {
	// Simple bubble sort - rules list is small
	n := len(rules)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if rules[j].Priority > rules[j+1].Priority {
				rules[j], rules[j+1] = rules[j+1], rules[j]
			}
		}
	}
}

func containsWord(s, word string) bool {
	return len(s) > 0 && len(word) > 0 &&
		(s == word || containsSubstr(s, word))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func containsTaskTypeMatch(condition, taskType string) bool {
	pattern := fmt.Sprintf("task_type == '%s'", taskType)
	return containsSubstr(condition, pattern)
}

func containsComplexityCondition(condition string, complexity float64) bool {
	// Check for "complexity < X"
	if containsSubstr(condition, "complexity < ") {
		threshold := parseComplexityThreshold(condition)
		return complexity < threshold
	}
	// Check for "complexity > X"
	if containsSubstr(condition, "complexity > ") {
		threshold := parseComplexityThreshold(condition)
		return complexity > threshold
	}
	return false
}

func parseTokenThreshold(condition string) int {
	// Simple parsing: "tokens > 100000"
	var threshold int
	fmt.Sscanf(condition, "tokens > %d", &threshold)
	return threshold
}

func parseComplexityThreshold(condition string) float64 {
	// Find "complexity > X" or "complexity < X" anywhere in condition
	var threshold float64

	// Look for "complexity > " pattern
	gtIdx := findSubstr(condition, "complexity > ")
	if gtIdx >= 0 {
		fmt.Sscanf(condition[gtIdx+len("complexity > "):], "%f", &threshold)
		return threshold
	}

	// Look for "complexity < " pattern
	ltIdx := findSubstr(condition, "complexity < ")
	if ltIdx >= 0 {
		fmt.Sscanf(condition[ltIdx+len("complexity < "):], "%f", &threshold)
		return threshold
	}

	return threshold
}

// findSubstr returns index of substr in s, or -1 if not found
func findSubstr(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// GetNextFallback returns the next model in the fallback chain after the current one
// Returns empty string if no more fallbacks available
func (r *ModelRouter) GetNextFallback(currentModel string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	chain := r.config.FallbackChain
	if len(chain) == 0 {
		return ""
	}

	// Find current position
	currentIdx := -1
	for i, m := range chain {
		if m == currentModel {
			currentIdx = i
			break
		}
	}

	// If not found, start from beginning
	if currentIdx == -1 {
		// Return first enabled model in chain
		for _, modelID := range chain {
			if info := r.registry.Get(modelID); info != nil && info.Enabled {
				return modelID
			}
		}
		return ""
	}

	// Return next enabled model in chain
	for i := currentIdx + 1; i < len(chain); i++ {
		modelID := chain[i]
		if info := r.registry.Get(modelID); info != nil && info.Enabled {
			return modelID
		}
	}

	return ""
}

// GetFallbackChain returns the full fallback chain
func (r *ModelRouter) GetFallbackChain() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]string, len(r.config.FallbackChain))
	copy(result, r.config.FallbackChain)
	return result
}

// GetRetryConfig returns retry configuration
func (r *ModelRouter) GetRetryConfig() (maxAttempts, backoffMs, timeoutMs int) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	maxAttempts = r.config.Retry.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 3
	}

	backoffMs = r.config.Retry.BackoffMs
	if backoffMs <= 0 {
		backoffMs = 1000
	}

	timeoutMs = r.config.Retry.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = 60000
	}

	return
}

// GetModelProvider returns the provider type for a model
func (r *ModelRouter) GetModelProvider(modelID string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	info := r.registry.Get(modelID)
	if info == nil {
		return "proxy"
	}
	return info.GetProvider()
}

// DefaultModelRouter is the global model router instance
var DefaultModelRouter *ModelRouter

func init() {
	DefaultModelRouter = NewModelRouter(nil)
}
