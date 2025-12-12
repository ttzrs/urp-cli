// Package agent provides the unified ModelDecisionEngine
// This consolidates fragmented model selection logic into a single auditable system
package agent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/joss/urp/internal/opencode/model"
)

// DecisionLog records a model selection decision for debugging and learning
type DecisionLog struct {
	Timestamp   time.Time         `json:"timestamp"`
	SessionID   string            `json:"session_id"`
	GoalID      string            `json:"goal_id"`
	TaskType    TaskType          `json:"task_type"`
	Input       DecisionInput     `json:"input"`
	Reasoning   []string          `json:"reasoning"`   // Step-by-step explanation
	Candidates  []CandidateScore  `json:"candidates"`  // All scored options
	Selected    string            `json:"selected"`    // Chosen model ID
	Confidence  float64           `json:"confidence"`  // 0-1 confidence level
	Reason      string            `json:"reason"`      // Why this model
	EstCost     float64           `json:"est_cost"`    // Estimated cost
	ActualCost  float64           `json:"actual_cost"` // Set after execution
	Success     *bool             `json:"success"`     // null=pending, true/false=known
	Fallback    bool              `json:"fallback"`    // Was this a fallback?
}

// CandidateScore represents a model option with its evaluation
type CandidateScore struct {
	ModelID    string  `json:"model_id"`
	Score      float64 `json:"score"`
	RuleMatch  string  `json:"rule_match,omitempty"`  // Which rule matched
	Reasoning  string  `json:"reasoning"`
	EstCost    float64 `json:"est_cost"`
	Viable     bool    `json:"viable"` // Could be selected
	RuledOut   string  `json:"ruled_out,omitempty"` // Why rejected
}

// DecisionInput is what the decision engine receives
type DecisionInput struct {
	SessionID     string
	GoalID        string
	TaskType      TaskType
	Complexity    float64   // 0-1, higher = more complex
	EstTokens     int       // Estimated input tokens
	HasImages     bool
	RequiredCaps  []string  // Required capabilities
	BudgetLimit   float64   // Max $ for this task
	Strategy      string    // "cost", "quality", "speed", "balanced"
	Environment   string    // "development", "production", "simulation"
}

// ModelDecisionEngine unifies all model selection logic
type ModelDecisionEngine struct {
	mu         sync.RWMutex
	registry   *model.ModelRegistry
	budget     *BudgetTracker
	learning   *ModelLearningStore
	decisionLog DecisionLogStore  // Audit trail

	// Configuration (single source of truth)
	config *DecisionConfig

	// Strategies: different decision profiles
	strategies map[string]*SelectionStrategy
}

// SelectionStrategy defines how to choose models for different scenarios
type SelectionStrategy struct {
	Name    string
	Rules   []SelectionRule
	Weights StrategyWeights
}

// SelectionRule matches task characteristics
type SelectionRule struct {
	Name       string
	Condition  func(*DecisionInput) bool
	ModelID    string
	Confidence float64
}

// StrategyWeights influence model scoring
type StrategyWeights struct {
	Quality  float64 // Quality tier importance (0-1)
	Cost     float64 // Cost optimization (0-1)
	Speed    float64 // Response speed (0-1)
	Context  float64 // Context window size (0-1)
}

// DecisionConfig holds all configuration
type DecisionConfig struct {
	Budget struct {
		DailyLimit   float64
		SessionLimit float64
		TaskLimit    float64
	}
	Fallback FallbackChain
}

// FallbackChain specifies models to try in order
type FallbackChain struct {
	Default []string // Used if everything fails
	ByStrategy map[string][]string
}

// DecisionLogStore persists decisions (interface for abstraction)
type DecisionLogStore interface {
	Store(ctx context.Context, log *DecisionLog) error
	Query(ctx context.Context, filter map[string]interface{}) ([]DecisionLog, error)
}

// NewModelDecisionEngine creates the unified decision engine
func NewModelDecisionEngine(
	registry *model.ModelRegistry,
	budget *BudgetTracker,
	learning *ModelLearningStore,
	logStore DecisionLogStore,
) *ModelDecisionEngine {
	if registry == nil {
		registry = model.DefaultModelRegistry
	}

	engine := &ModelDecisionEngine{
		registry:    registry,
		budget:      budget,
		learning:    learning,
		decisionLog: logStore,
		config:      defaultDecisionConfig(),
		strategies:  make(map[string]*SelectionStrategy),
	}

	// Initialize built-in strategies
	engine.registerBuiltinStrategies()

	return engine
}

// Decide selects the best model for a task (main entry point)
func (de *ModelDecisionEngine) Decide(ctx context.Context, input *DecisionInput) (*ModelSelection, *DecisionLog, error) {
	de.mu.RLock()
	defer de.mu.RUnlock()

	log := &DecisionLog{
		Timestamp:  time.Now(),
		SessionID:  input.SessionID,
		GoalID:     input.GoalID,
		TaskType:   input.TaskType,
		Input:      *input,
		Reasoning:  []string{},
		Candidates: []CandidateScore{},
	}

	// Step 1: Budget check
	if de.budget != nil && !de.budget.CanAffordTask(input.BudgetLimit) {
		log.Reasoning = append(log.Reasoning, fmt.Sprintf("budget constraint: limit=$%.2f", input.BudgetLimit))
		// Force cheapest model
		cheap := de.registry.CheapestWithCap("code", 1)
		if cheap != nil {
			selection := &ModelSelection{
				ModelID:    cheap.ID,
				Confidence: 0.7,
				Reason:     "budget-forced fallback",
				EstCost:    cheap.EstimateCost(input.EstTokens, input.EstTokens/2),
			}
			log.Selected = cheap.ID
			log.Confidence = selection.Confidence
			log.Reason = selection.Reason
			log.EstCost = selection.EstCost
			log.Fallback = true
			de.decisionLog.Store(ctx, log)
			return selection, log, nil
		}
	}

	// Step 2: Apply strategy
	strategy, ok := de.strategies[input.Strategy]
	if !ok {
		strategy = de.strategies["balanced"]
	}
	log.Reasoning = append(log.Reasoning, fmt.Sprintf("strategy: %s", strategy.Name))

	// Check rules in order
	for _, rule := range strategy.Rules {
		if rule.Condition(input) {
			modelInfo := de.registry.Get(rule.ModelID)
			if modelInfo != nil && modelInfo.Enabled {
				log.Reasoning = append(log.Reasoning, fmt.Sprintf("rule matched: %s → %s", rule.Name, rule.ModelID))
				selection := &ModelSelection{
					ModelID:    modelInfo.ID,
					Confidence: rule.Confidence,
					Reason:     fmt.Sprintf("rule: %s", rule.Name),
					RuleName:   rule.Name,
					EstCost:    modelInfo.EstimateCost(input.EstTokens, input.EstTokens/2),
				}
				log.Selected = modelInfo.ID
				log.Confidence = selection.Confidence
				log.Reason = selection.Reason
				log.EstCost = selection.EstCost
				log.Candidates = de.scoreAllCandidates(input, strategy, rule.ModelID)
				de.decisionLog.Store(ctx, log)
				return selection, log, nil
			}
		}
	}
	log.Reasoning = append(log.Reasoning, "no rules matched, using scoring")

	// Step 3: Query learning store
	if de.learning != nil {
		learned := de.learning.GetBestModel(string(input.TaskType), input.Environment, input.Complexity)
		if learned != "" {
			modelInfo := de.registry.Get(learned)
			if modelInfo != nil && modelInfo.Enabled {
				log.Reasoning = append(log.Reasoning, fmt.Sprintf("learned history: %s", learned))
				selection := &ModelSelection{
					ModelID:    modelInfo.ID,
					Confidence: 0.75,
					Reason:     "learned from history",
					EstCost:    modelInfo.EstimateCost(input.EstTokens, input.EstTokens/2),
				}
				log.Selected = modelInfo.ID
				log.Confidence = selection.Confidence
				log.Reason = selection.Reason
				log.EstCost = selection.EstCost
				log.Candidates = de.scoreAllCandidates(input, strategy, learned)
				de.decisionLog.Store(ctx, log)
				return selection, log, nil
			}
		}
	}

	// Step 4: Score-based selection
	log.Reasoning = append(log.Reasoning, "evaluating all models by score")
	candidates := de.scoreAllCandidates(input, strategy, "")
	log.Candidates = candidates

	if len(candidates) > 0 && candidates[0].Viable {
		modelInfo := de.registry.Get(candidates[0].ModelID)
		if modelInfo != nil {
			selection := &ModelSelection{
				ModelID:    modelInfo.ID,
				Confidence: candidates[0].Score,
				Reason:     fmt.Sprintf("scored selection (%.2f)", candidates[0].Score),
				EstCost:    modelInfo.EstimateCost(input.EstTokens, input.EstTokens/2),
			}
			log.Selected = modelInfo.ID
			log.Confidence = selection.Confidence
			log.Reason = selection.Reason
			log.EstCost = selection.EstCost
			de.decisionLog.Store(ctx, log)
			return selection, log, nil
		}
	}

	// Step 5: Fallback chain
	log.Reasoning = append(log.Reasoning, "attempting fallback chain")
	fallbacks := de.config.Fallback.Default
	if byStrategy, ok := de.config.Fallback.ByStrategy[input.Strategy]; ok {
		fallbacks = byStrategy
	}

	for _, modelID := range fallbacks {
		modelInfo := de.registry.Get(modelID)
		if modelInfo != nil && modelInfo.Enabled {
			log.Reasoning = append(log.Reasoning, fmt.Sprintf("fallback: %s", modelID))
			selection := &ModelSelection{
				ModelID:    modelInfo.ID,
				Confidence: 0.5,
				Reason:     "fallback chain",
				EstCost:    modelInfo.EstimateCost(input.EstTokens, input.EstTokens/2),
			}
			log.Selected = modelInfo.ID
			log.Confidence = selection.Confidence
			log.Reason = selection.Reason
			log.EstCost = selection.EstCost
			log.Fallback = true
			de.decisionLog.Store(ctx, log)
			return selection, log, nil
		}
	}

	// Step 6: Ultimate fallback
	log.Reasoning = append(log.Reasoning, "ultimate fallback to haiku")
	log.Selected = "claude-haiku-4-5-20251001"
	log.Confidence = 0.3
	log.Reason = "ultimate fallback"
	log.Fallback = true
	de.decisionLog.Store(ctx, log)

	return &ModelSelection{
		ModelID:    "claude-haiku-4-5-20251001",
		Confidence: 0.3,
		Reason:     "ultimate fallback",
	}, log, nil
}

// scoreAllCandidates evaluates all models using the strategy
func (de *ModelDecisionEngine) scoreAllCandidates(input *DecisionInput, strategy *SelectionStrategy, exclude string) []CandidateScore {
	candidates := de.registry.ListEnabled()
	scores := []CandidateScore{}

	weights := strategy.Weights
	if weights.Quality == 0 && weights.Cost == 0 && weights.Speed == 0 {
		weights.Quality = 0.4
		weights.Cost = 0.3
		weights.Speed = 0.2
		weights.Context = 0.1
	}

	for _, m := range candidates {
		if exclude != "" && m.ID == exclude {
			continue // Skip if already selected
		}

		// Check required capabilities
		ruledOut := ""
		for _, cap := range input.RequiredCaps {
			if !m.HasCapability(cap) {
				ruledOut = fmt.Sprintf("missing capability: %s", cap)
				break
			}
		}

		if ruledOut != "" {
			scores = append(scores, CandidateScore{
				ModelID:  m.ID,
				Viable:   false,
				RuledOut: ruledOut,
			})
			continue
		}

		// Check budget
		estCost := m.EstimateCost(input.EstTokens, input.EstTokens/2)
		if estCost > input.BudgetLimit {
			scores = append(scores, CandidateScore{
				ModelID:  m.ID,
				Viable:   false,
				RuledOut: fmt.Sprintf("exceeds budget: $%.4f > $%.4f", estCost, input.BudgetLimit),
				EstCost:  estCost,
			})
			continue
		}

		// Score viable models
		qualityScore := float64(m.QualityTier) / 3.0
		speedScore := float64(m.SpeedTier) / 3.0
		maxCost := 75.0
		costScore := 1.0 - (m.InputCost+m.OutputCost)/(2*maxCost)
		if costScore < 0 {
			costScore = 0
		}

		// Context window score: more is better
		contextScore := float64(m.ContextSize) / 2_000_000.0
		if contextScore > 1.0 {
			contextScore = 1.0
		}

		// Adjust for task complexity
		if input.Complexity > 0.7 && m.QualityTier < 2 {
			qualityScore *= 0.5
		}
		if input.Complexity < 0.3 && m.QualityTier > 1 {
			costScore *= 1.2 // Prefer cheap models for simple tasks
		}

		score := weights.Quality*qualityScore +
			weights.Cost*costScore +
			weights.Speed*speedScore +
			weights.Context*contextScore

		scores = append(scores, CandidateScore{
			ModelID:   m.ID,
			Score:     score,
			Viable:    true,
			Reasoning: fmt.Sprintf("Q=%.2f C=%.2f S=%.2f", qualityScore, costScore, speedScore),
			EstCost:   estCost,
		})
	}

	// Sort by score descending
	for i := 0; i < len(scores)-1; i++ {
		for j := i + 1; j < len(scores); j++ {
			if scores[j].Viable && (!scores[i].Viable || scores[j].Score > scores[i].Score) {
				scores[i], scores[j] = scores[j], scores[i]
			}
		}
	}

	return scores
}

// registerBuiltinStrategies defines the default decision strategies
func (de *ModelDecisionEngine) registerBuiltinStrategies() {
	// Cost-optimized strategy
	de.strategies["cost"] = &SelectionStrategy{
		Name: "cost-optimized",
		Rules: []SelectionRule{
			{
				Name: "cheap-exploration",
				Condition: func(in *DecisionInput) bool {
					return in.TaskType == TaskTypeExplore && in.Complexity < 0.5
				},
				ModelID:    "gemini-1.5-flash",
				Confidence: 0.85,
			},
			{
				Name: "cheap-simple",
				Condition: func(in *DecisionInput) bool {
					return in.Complexity < 0.3
				},
				ModelID:    "claude-haiku-4-5-20251001",
				Confidence: 0.8,
			},
			{
				Name: "cheap-moderate",
				Condition: func(in *DecisionInput) bool {
					return in.Complexity < 0.6
				},
				ModelID:    "deepseek-chat",
				Confidence: 0.75,
			},
		},
		Weights: StrategyWeights{
			Quality: 0.2,
			Cost:    0.6,
			Speed:   0.1,
			Context: 0.1,
		},
	}

	// Quality-focused strategy
	de.strategies["quality"] = &SelectionStrategy{
		Name: "quality-focused",
		Rules: []SelectionRule{
			{
				Name: "vision-required",
				Condition: func(in *DecisionInput) bool {
					return in.HasImages
				},
				ModelID:    "claude-opus-4-5-20251101",
				Confidence: 0.95,
			},
			{
				Name: "complex-reasoning",
				Condition: func(in *DecisionInput) bool {
					return in.Complexity > 0.8
				},
				ModelID:    "claude-opus-4-5-20251101",
				Confidence: 0.9,
			},
			{
				Name: "code-review",
				Condition: func(in *DecisionInput) bool {
					return in.TaskType == TaskTypeBugfix || in.TaskType == TaskTypeRefactor
				},
				ModelID:    "claude-opus-4-5-20251101",
				Confidence: 0.85,
			},
		},
		Weights: StrategyWeights{
			Quality: 0.7,
			Cost:    0.1,
			Speed:   0.1,
			Context: 0.1,
		},
	}

	// Speed-optimized strategy
	de.strategies["speed"] = &SelectionStrategy{
		Name: "speed-optimized",
		Rules: []SelectionRule{
			{
				Name: "fast-response",
				Condition: func(in *DecisionInput) bool {
					return in.Complexity < 0.4
				},
				ModelID:    "claude-haiku-4-5-20251001",
				Confidence: 0.9,
			},
		},
		Weights: StrategyWeights{
			Quality: 0.2,
			Cost:    0.3,
			Speed:   0.4,
			Context: 0.1,
		},
	}

	// Balanced strategy (default)
	de.strategies["balanced"] = &SelectionStrategy{
		Name: "balanced",
		Rules: []SelectionRule{
			{
				Name: "vision-images",
				Condition: func(in *DecisionInput) bool {
					return in.HasImages
				},
				ModelID:    "claude-opus-4-5-20251101",
				Confidence: 0.9,
			},
			{
				Name: "complex-high",
				Condition: func(in *DecisionInput) bool {
					return in.Complexity > 0.75
				},
				ModelID:    "claude-opus-4-5-20251101",
				Confidence: 0.85,
			},
			{
				Name: "complex-medium",
				Condition: func(in *DecisionInput) bool {
					return in.Complexity > 0.5
				},
				ModelID:    "claude-sonnet-4-5-20250929",
				Confidence: 0.8,
			},
			{
				Name: "simple-cheap",
				Condition: func(in *DecisionInput) bool {
					return in.Complexity < 0.4
				},
				ModelID:    "claude-haiku-4-5-20251001",
				Confidence: 0.8,
			},
		},
		Weights: StrategyWeights{
			Quality: 0.4,
			Cost:    0.3,
			Speed:   0.2,
			Context: 0.1,
		},
	}
}

// RecordOutcome updates decision quality metrics after execution
func (de *ModelDecisionEngine) RecordOutcome(ctx context.Context, decision *DecisionLog, cost float64, success bool) error {
	de.mu.Lock()
	defer de.mu.Unlock()

	decision.ActualCost = cost
	decision.Success = &success

	if de.budget != nil {
		de.budget.Record(cost)
	}

	if de.learning != nil {
		outcome := &ModelOutcome{
			ModelID:    decision.Selected,
			TaskType:   string(decision.TaskType),
			Cost:       cost,
			Success:    success,
			Score:      decision.Confidence,
			Timestamp:  time.Now(),
		}
		de.learning.Record(outcome)
	}

	return de.decisionLog.Store(ctx, decision)
}

// defaultDecisionConfig returns sensible defaults
func defaultDecisionConfig() *DecisionConfig {
	cfg := &DecisionConfig{}
	cfg.Budget.DailyLimit = 5.0
	cfg.Budget.SessionLimit = 1.0
	cfg.Budget.TaskLimit = 0.20

	cfg.Fallback.Default = []string{
		"claude-sonnet-4-5-20250929",
		"gpt-4o",
		"deepseek-chat",
		"claude-haiku-4-5-20251001",
	}

	cfg.Fallback.ByStrategy = map[string][]string{
		"cost": {
			"deepseek-chat",
			"gemini-1.5-flash",
			"claude-haiku-4-5-20251001",
		},
		"quality": {
			"claude-opus-4-5-20251101",
			"gpt-5.1",
			"claude-sonnet-4-5-20250929",
		},
		"speed": {
			"claude-haiku-4-5-20251001",
			"gemini-1.5-flash",
		},
	}

	return cfg
}

// GetDecisionLog retrieves audit trail for a session
func (de *ModelDecisionEngine) GetDecisionLog(ctx context.Context, sessionID string) ([]DecisionLog, error) {
	de.mu.RLock()
	defer de.mu.RUnlock()

	return de.decisionLog.Query(ctx, map[string]interface{}{
		"session_id": sessionID,
	})
}

// ExportStrategy exports a strategy for external use/configuration
func (de *ModelDecisionEngine) ExportStrategy(name string) interface{} {
	de.mu.RLock()
	defer de.mu.RUnlock()

	strategy, ok := de.strategies[name]
	if !ok {
		return nil
	}

	return map[string]interface{}{
		"name": strategy.Name,
		"weights": map[string]float64{
			"quality": strategy.Weights.Quality,
			"cost":    strategy.Weights.Cost,
			"speed":   strategy.Weights.Speed,
			"context": strategy.Weights.Context,
		},
	}
}
