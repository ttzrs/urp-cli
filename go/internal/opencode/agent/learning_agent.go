// Package agent provides learning-enhanced agent capabilities
package agent

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// LearningAgent wraps an Agent with adaptive learning capabilities
type LearningAgent struct {
	agent   *Agent
	auditor *TaskAuditor
	embeds  *EmbeddingStore

	// Current task tracking
	currentMetrics *TaskMetrics
	strategyHint   *StrategyHint
}

// NewLearningAgent creates an agent with learning capabilities
func NewLearningAgent(agent *Agent, store StrategyStore, vectorPath string) (*LearningAgent, error) {
	// Seed builtin strategies if store supports it
	if gs, ok := store.(*GraphStrategyStore); ok {
		if err := gs.EnsureSchema(context.Background()); err != nil {
			// Non-fatal, continue without schema
		}
		_ = gs.SeedBuiltinStrategies(context.Background())
	}

	// Create auditor
	auditor := NewTaskAuditor(store)

	// Create embedding store (optional)
	var embeds *EmbeddingStore
	if vectorPath != "" {
		var err error
		embeds, err = NewEmbeddingStore(vectorPath)
		if err != nil {
			// Non-fatal, continue without embeddings
			embeds = nil
		}
	}

	return &LearningAgent{
		agent:   agent,
		auditor: auditor,
		embeds:  embeds,
	}, nil
}

// PreTask prepares for a task by consulting learned strategies
func (la *LearningAgent) PreTask(ctx context.Context, objective string) (*StrategyHint, error) {
	// Detect environment from working directory
	env := la.detectEnvironmentFromWorkdir()

	// Get strategy hint from auditor
	hint, err := la.auditor.GetHint(ctx, objective, env)
	if err != nil {
		return nil, err
	}

	// Enrich with embedding similarity search
	if la.embeds != nil {
		similar, err := la.embeds.FindSimilarSuccessful(ctx, objective, env, 5)
		if err == nil && len(similar) > 0 {
			// Get strategy from similar tasks
			stratName, confidence := GetStrategyFromSimilar(similar)
			if hint == nil || confidence > hint.Confidence {
				hint = &StrategyHint{
					Confidence: confidence,
				}
			}

			// Add suggestions from similar tasks
			for _, t := range similar[:minInt(3, len(similar))] {
				if t.StrategyUsed != "" {
					hint.Suggestions = append(hint.Suggestions,
						fmt.Sprintf("Similar task (%s, %.0f%% match) used: %s",
							t.Environment, t.Similarity*100, t.StrategyUsed))
				}
			}

			// Get expected metrics
			avgTokens, avgTurns := GetExpectedMetrics(similar)
			hint.Suggestions = append(hint.Suggestions,
				fmt.Sprintf("Expected: ~%.0f tokens, ~%.1f turns", avgTokens, avgTurns))

			_ = stratName // May use later
		}
	}

	la.strategyHint = hint
	return hint, nil
}

// PostTask analyzes completed task and learns from it
func (la *LearningAgent) PostTask(ctx context.Context, success bool) error {
	if la.currentMetrics == nil {
		return nil // No metrics to analyze
	}

	la.currentMetrics.Finish(success)

	// Audit the task
	var tc *TaskContext
	if la.agent != nil {
		tc = la.agent.taskContext
	}
	analysis, err := la.auditor.Audit(ctx, la.currentMetrics, tc)
	if err != nil {
		return fmt.Errorf("audit: %w", err)
	}

	// Learn from analysis
	if err := la.auditor.Learn(ctx, analysis); err != nil {
		return fmt.Errorf("learn: %w", err)
	}

	// Index task for similarity search
	if la.embeds != nil {
		taskEmbed := TaskEmbedding{
			TaskID:       la.currentMetrics.TaskID,
			Objective:    la.currentMetrics.Objective,
			Environment:  analysis.Environment,
			TaskType:     la.auditor.inferTaskType(la.currentMetrics.Objective),
			StrategyUsed: analysis.StrategyUsed,
			Success:      success,
			Tokens:       analysis.TotalTokens,
			Turns:        analysis.TotalTurns,
			CreatedAt:    time.Now(),
		}
		if err := la.embeds.IndexTask(ctx, taskEmbed); err != nil {
			// Non-fatal
		}
	}

	// Clear current task
	la.currentMetrics = nil
	la.strategyHint = nil

	return nil
}

// BuildEnhancedPrompt creates a prompt with strategy hints
func (la *LearningAgent) BuildEnhancedPrompt(basePrompt string) string {
	if la.strategyHint == nil {
		return basePrompt
	}

	var b strings.Builder
	b.WriteString(basePrompt)

	b.WriteString("\n\n<learned-strategy>\n")

	// Strategy info
	if la.strategyHint.Strategy != nil {
		b.WriteString(fmt.Sprintf("RECOMMENDED APPROACH: %s\n", la.strategyHint.Strategy.Name))
		b.WriteString(fmt.Sprintf("CONFIDENCE: %.0f%%\n", la.strategyHint.Confidence*100))

		if len(la.strategyHint.Strategy.Steps) > 0 {
			b.WriteString("STEPS:\n")
			for i, step := range la.strategyHint.Strategy.Steps {
				b.WriteString(fmt.Sprintf("  %d. %s\n", i+1, step))
			}
		}
	}

	// Implementation hints
	if la.strategyHint.Impl != nil {
		b.WriteString(fmt.Sprintf("TOOLCHAIN: %v\n", la.strategyHint.Impl.Toolchain))
		b.WriteString(fmt.Sprintf("EXPECTED: ~%.0f tokens, ~%.1f turns\n",
			la.strategyHint.Impl.AvgTokens, la.strategyHint.Impl.AvgTurns))
	} else if la.strategyHint.SimilarImpl != nil {
		b.WriteString(fmt.Sprintf("TRANSFER FROM %s: %v\n",
			la.strategyHint.TransferFrom, la.strategyHint.SimilarImpl.Toolchain))
	}

	// Suggestions
	if len(la.strategyHint.Suggestions) > 0 {
		b.WriteString("SUGGESTIONS:\n")
		for _, s := range la.strategyHint.Suggestions {
			b.WriteString(fmt.Sprintf("  • %s\n", s))
		}
	}

	// Anti-patterns
	if len(la.strategyHint.Avoid) > 0 {
		b.WriteString("AVOID:\n")
		for _, a := range la.strategyHint.Avoid {
			b.WriteString(fmt.Sprintf("  ✗ %s\n", a))
		}
	}

	b.WriteString("</learned-strategy>")

	return b.String()
}

// StartTask initializes metrics tracking for a new task
func (la *LearningAgent) StartTask(taskID, objective string) {
	la.currentMetrics = NewTaskMetrics(taskID, objective)
}

// RecordTurn logs metrics for a turn
func (la *LearningAgent) RecordTurn(inputTok, outputTok, toolCalls int, phase, action string) {
	if la.currentMetrics == nil {
		return
	}
	onTrack := la.isOnTrack(action)
	la.currentMetrics.RecordTurn(inputTok, outputTok, toolCalls, phase, action, onTrack)
}

// RecordError logs an error
func (la *LearningAgent) RecordError(err string) {
	if la.currentMetrics != nil {
		la.currentMetrics.RecordError(err)
	}
}

// RecordScopeCreep logs scope creep
func (la *LearningAgent) RecordScopeCreep(action string) {
	if la.currentMetrics != nil {
		la.currentMetrics.RecordScopeCreep(action)
	}
}

// isOnTrack checks if an action is aligned with the objective
func (la *LearningAgent) isOnTrack(action string) bool {
	if la.currentMetrics == nil {
		return true
	}

	// Check for scope creep indicators
	scopeCreepIndicators := []string{
		"creating documentation",
		"writing test",
		"adding readme",
		"refactoring unrelated",
		"improving style",
	}

	actionLower := strings.ToLower(action)
	for _, indicator := range scopeCreepIndicators {
		if strings.Contains(actionLower, indicator) {
			return false
		}
	}

	return true
}

// detectEnvironmentFromWorkdir detects programming environment
func (la *LearningAgent) detectEnvironmentFromWorkdir() string {
	// Check for common files in workdir
	// This would scan the actual directory in production
	// For now, default to go since urp is a go project
	return "go"
}

// GetMetrics returns current task metrics
func (la *LearningAgent) GetMetrics() *TaskMetrics {
	return la.currentMetrics
}

// GetHint returns current strategy hint
func (la *LearningAgent) GetHint() *StrategyHint {
	return la.strategyHint
}

// Agent returns the underlying agent
func (la *LearningAgent) Agent() *Agent {
	return la.agent
}

// Close releases resources
func (la *LearningAgent) Close() error {
	if la.embeds != nil {
		return la.embeds.Close()
	}
	return nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
