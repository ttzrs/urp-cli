// Package agent provides strategy persistence in Memgraph
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/joss/urp/internal/graph"
)

// GraphStrategyStore implements StrategyStore using Memgraph
type GraphStrategyStore struct {
	db graph.Driver
}

// NewGraphStrategyStore creates a store backed by Memgraph
func NewGraphStrategyStore(db graph.Driver) *GraphStrategyStore {
	return &GraphStrategyStore{db: db}
}

// EnsureSchema creates required indexes and constraints
func (s *GraphStrategyStore) EnsureSchema(ctx context.Context) error {
	queries := []string{
		"CREATE INDEX ON :Strategy(id)",
		"CREATE INDEX ON :Strategy(task_type)",
		"CREATE INDEX ON :StrategyImpl(id)",
		"CREATE INDEX ON :StrategyImpl(strategy_id)",
		"CREATE INDEX ON :StrategyImpl(environment)",
		"CREATE INDEX ON :FailedAttempt(strategy_id)",
	}

	for _, q := range queries {
		// Ignore errors for already existing indexes
		_, _ = s.db.Execute(ctx, q, nil)
	}

	return nil
}

// SaveStrategy persists a strategy to Memgraph
func (s *GraphStrategyStore) SaveStrategy(ctx context.Context, strat *Strategy) error {
	query := `
		MERGE (s:Strategy {id: $id})
		SET s.name = $name,
		    s.description = $description,
		    s.level = $level,
		    s.task_type = $task_type,
		    s.steps = $steps,
		    s.tools = $tools,
		    s.preconditions = $preconditions,
		    s.confidence = $confidence,
		    s.usage_count = $usage_count,
		    s.success_count = $success_count,
		    s.learned_from = $learned_from,
		    s.created_at = $created_at,
		    s.updated_at = $updated_at
	`

	params := map[string]any{
		"id":            strat.ID,
		"name":          strat.Name,
		"description":   strat.Description,
		"level":         strat.Level,
		"task_type":     strat.TaskType,
		"steps":         toJSON(strat.Steps),
		"tools":         toJSON(strat.Tools),
		"preconditions": toJSON(strat.Preconditions),
		"confidence":    strat.Confidence,
		"usage_count":   strat.UsageCount,
		"success_count": strat.SuccessCount,
		"learned_from":  toJSON(strat.LearnedFrom),
		"created_at":    strat.CreatedAt.Format(time.RFC3339),
		"updated_at":    strat.UpdatedAt.Format(time.RFC3339),
	}

	_, err := s.db.Execute(ctx, query, params)
	return err
}

// GetStrategy retrieves a strategy by ID
func (s *GraphStrategyStore) GetStrategy(ctx context.Context, id string) (*Strategy, error) {
	query := `
		MATCH (s:Strategy {id: $id})
		RETURN s.id, s.name, s.description, s.level, s.task_type,
		       s.steps, s.tools, s.preconditions, s.confidence,
		       s.usage_count, s.success_count, s.learned_from,
		       s.created_at, s.updated_at
	`

	records, err := s.db.Execute(ctx, query, map[string]any{"id": id})
	if err != nil {
		return nil, err
	}

	if len(records) == 0 {
		return nil, nil
	}

	return recordToStrategy(records[0])
}

// FindStrategies finds strategies by task type
func (s *GraphStrategyStore) FindStrategies(ctx context.Context, taskType string) ([]*Strategy, error) {
	query := `
		MATCH (s:Strategy)
		WHERE s.task_type = $task_type OR s.level = 'abstract'
		RETURN s.id, s.name, s.description, s.level, s.task_type,
		       s.steps, s.tools, s.preconditions, s.confidence,
		       s.usage_count, s.success_count, s.learned_from,
		       s.created_at, s.updated_at
		ORDER BY s.confidence DESC
		LIMIT 10
	`

	records, err := s.db.Execute(ctx, query, map[string]any{"task_type": taskType})
	if err != nil {
		return nil, err
	}

	var strategies []*Strategy
	for _, r := range records {
		strat, err := recordToStrategy(r)
		if err != nil {
			continue
		}
		strategies = append(strategies, strat)
	}

	return strategies, nil
}

// FindBestStrategy finds the best strategy for a task description and environment
func (s *GraphStrategyStore) FindBestStrategy(ctx context.Context, taskDesc, env string) (*StrategyHint, error) {
	// First, find matching strategies
	query := `
		MATCH (s:Strategy)
		WHERE s.confidence > 0.5
		RETURN s.id, s.name, s.description, s.level, s.task_type,
		       s.steps, s.tools, s.preconditions, s.confidence,
		       s.usage_count, s.success_count, s.learned_from,
		       s.created_at, s.updated_at
		ORDER BY s.confidence DESC, s.usage_count DESC
		LIMIT 5
	`

	records, err := s.db.Execute(ctx, query, nil)
	if err != nil {
		return nil, err
	}

	if len(records) == 0 {
		return nil, nil
	}

	// Score each strategy against task description
	var bestStrategy *Strategy
	var bestScore float64

	for _, r := range records {
		strat, err := recordToStrategy(r)
		if err != nil {
			continue
		}

		score := strat.MatchScore(taskDesc)
		if score > bestScore {
			bestScore = score
			bestStrategy = strat
		}
	}

	if bestStrategy == nil {
		return nil, nil
	}

	hint := &StrategyHint{
		Strategy:   bestStrategy,
		Confidence: bestScore,
	}

	// Find implementation for this environment
	impl, err := s.GetImpl(ctx, bestStrategy.ID, env)
	if err == nil && impl != nil {
		hint.Impl = impl
		hint.Confidence = bestScore * impl.SuccessRate
	} else {
		// Try to find similar environment
		similarImpl, similarity, err := s.FindSimilarImpl(ctx, bestStrategy.ID, env)
		if err == nil && similarImpl != nil {
			hint.SimilarImpl = similarImpl
			hint.TransferFrom = similarImpl.Environment
			hint.Confidence = bestScore * similarity * similarImpl.SuccessRate
		}
	}

	return hint, nil
}

// SaveImpl persists a strategy implementation
func (s *GraphStrategyStore) SaveImpl(ctx context.Context, impl *StrategyImpl) error {
	query := `
		MERGE (i:StrategyImpl {id: $id})
		SET i.strategy_id = $strategy_id,
		    i.environment = $environment,
		    i.toolchain = $toolchain,
		    i.commands = $commands,
		    i.anti_patterns = $anti_patterns,
		    i.avg_tokens = $avg_tokens,
		    i.avg_turns = $avg_turns,
		    i.success_rate = $success_rate,
		    i.sample_count = $sample_count,
		    i.last_used = $last_used,
		    i.created_at = $created_at
	`

	params := map[string]any{
		"id":            impl.ID,
		"strategy_id":   impl.StrategyID,
		"environment":   impl.Environment,
		"toolchain":     toJSON(impl.Toolchain),
		"commands":      toJSON(impl.Commands),
		"anti_patterns": toJSON(impl.AntiPatterns),
		"avg_tokens":    impl.AvgTokens,
		"avg_turns":     impl.AvgTurns,
		"success_rate":  impl.SuccessRate,
		"sample_count":  impl.SampleCount,
		"last_used":     impl.LastUsed.Format(time.RFC3339),
		"created_at":    impl.CreatedAt.Format(time.RFC3339),
	}

	_, err := s.db.Execute(ctx, query, params)
	if err != nil {
		return err
	}

	// Link to strategy
	linkQuery := `
		MATCH (s:Strategy {id: $strategy_id})
		MATCH (i:StrategyImpl {id: $impl_id})
		MERGE (s)-[:IMPLEMENTED_AS]->(i)
	`
	_, _ = s.db.Execute(ctx, linkQuery, map[string]any{
		"strategy_id": impl.StrategyID,
		"impl_id":     impl.ID,
	})

	return nil
}

// GetImpl retrieves an implementation for a strategy and environment
func (s *GraphStrategyStore) GetImpl(ctx context.Context, strategyID, env string) (*StrategyImpl, error) {
	query := `
		MATCH (i:StrategyImpl {strategy_id: $strategy_id, environment: $env})
		RETURN i.id, i.strategy_id, i.environment, i.toolchain, i.commands,
		       i.anti_patterns, i.avg_tokens, i.avg_turns, i.success_rate,
		       i.sample_count, i.last_used, i.created_at
	`

	records, err := s.db.Execute(ctx, query, map[string]any{
		"strategy_id": strategyID,
		"env":         env,
	})
	if err != nil {
		return nil, err
	}

	if len(records) == 0 {
		return nil, nil
	}

	return recordToImpl(records[0])
}

// FindSimilarImpl finds an implementation from a similar environment
func (s *GraphStrategyStore) FindSimilarImpl(ctx context.Context, strategyID, targetEnv string) (*StrategyImpl, float64, error) {
	query := `
		MATCH (i:StrategyImpl {strategy_id: $strategy_id})
		WHERE i.environment <> $env
		RETURN i.id, i.strategy_id, i.environment, i.toolchain, i.commands,
		       i.anti_patterns, i.avg_tokens, i.avg_turns, i.success_rate,
		       i.sample_count, i.last_used, i.created_at
		ORDER BY i.success_rate DESC
	`

	records, err := s.db.Execute(ctx, query, map[string]any{
		"strategy_id": strategyID,
		"env":         targetEnv,
	})
	if err != nil {
		return nil, 0, err
	}

	var bestImpl *StrategyImpl
	var bestSimilarity float64

	for _, r := range records {
		impl, err := recordToImpl(r)
		if err != nil {
			continue
		}

		similarity := GetEnvSimilarity(impl.Environment, targetEnv)
		// Weight by both similarity and success rate
		score := similarity * impl.SuccessRate

		if score > bestSimilarity {
			bestSimilarity = similarity
			bestImpl = impl
		}
	}

	return bestImpl, bestSimilarity, nil
}

// SaveFailedAttempt records a failed attempt
func (s *GraphStrategyStore) SaveFailedAttempt(ctx context.Context, fa *FailedAttempt) error {
	query := `
		CREATE (f:FailedAttempt {
			id: $id,
			strategy_id: $strategy_id,
			environment: $environment,
			task_id: $task_id,
			attempted_tools: $attempted_tools,
			failure_reason: $failure_reason,
			suggestion: $suggestion,
			created_at: $created_at
		})
	`

	params := map[string]any{
		"id":              fa.ID,
		"strategy_id":     fa.StrategyID,
		"environment":     fa.Environment,
		"task_id":         fa.TaskID,
		"attempted_tools": toJSON(fa.AttemptedTools),
		"failure_reason":  fa.FailureReason,
		"suggestion":      fa.Suggestion,
		"created_at":      fa.CreatedAt.Format(time.RFC3339),
	}

	_, err := s.db.Execute(ctx, query, params)
	if err != nil {
		return err
	}

	// Link to strategy
	linkQuery := `
		MATCH (s:Strategy {id: $strategy_id})
		MATCH (f:FailedAttempt {id: $fail_id})
		MERGE (f)-[:FAILED_WITH]->(s)
	`
	_, _ = s.db.Execute(ctx, linkQuery, map[string]any{
		"strategy_id": fa.StrategyID,
		"fail_id":     fa.ID,
	})

	return nil
}

// GetFailedAttempts retrieves failed attempts for a strategy/environment
func (s *GraphStrategyStore) GetFailedAttempts(ctx context.Context, strategyID, env string) ([]*FailedAttempt, error) {
	query := `
		MATCH (f:FailedAttempt {strategy_id: $strategy_id})
		WHERE f.environment = $env OR $env = ''
		RETURN f.id, f.strategy_id, f.environment, f.task_id,
		       f.attempted_tools, f.failure_reason, f.suggestion, f.created_at
		ORDER BY f.created_at DESC
		LIMIT 10
	`

	records, err := s.db.Execute(ctx, query, map[string]any{
		"strategy_id": strategyID,
		"env":         env,
	})
	if err != nil {
		return nil, err
	}

	var failures []*FailedAttempt
	for _, r := range records {
		fa, err := recordToFailedAttempt(r)
		if err != nil {
			continue
		}
		failures = append(failures, fa)
	}

	return failures, nil
}

// Helper functions

func toJSON(v any) string {
	data, _ := json.Marshal(v)
	return string(data)
}

func fromJSON[T any](s string) T {
	var v T
	_ = json.Unmarshal([]byte(s), &v)
	return v
}

func recordToStrategy(r map[string]any) (*Strategy, error) {
	strat := &Strategy{
		ID:            getString(r, "s.id"),
		Name:          getString(r, "s.name"),
		Description:   getString(r, "s.description"),
		Level:         getString(r, "s.level"),
		TaskType:      getString(r, "s.task_type"),
		Steps:         fromJSON[[]string](getString(r, "s.steps")),
		Tools:         fromJSON[[]string](getString(r, "s.tools")),
		Preconditions: fromJSON[[]string](getString(r, "s.preconditions")),
		Confidence:    getFloat(r, "s.confidence"),
		UsageCount:    getInt(r, "s.usage_count"),
		SuccessCount:  getInt(r, "s.success_count"),
		LearnedFrom:   fromJSON[[]string](getString(r, "s.learned_from")),
	}

	if created := getString(r, "s.created_at"); created != "" {
		strat.CreatedAt, _ = time.Parse(time.RFC3339, created)
	}
	if updated := getString(r, "s.updated_at"); updated != "" {
		strat.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
	}

	return strat, nil
}

func recordToImpl(r map[string]any) (*StrategyImpl, error) {
	impl := &StrategyImpl{
		ID:           getString(r, "i.id"),
		StrategyID:   getString(r, "i.strategy_id"),
		Environment:  getString(r, "i.environment"),
		Toolchain:    fromJSON[[]string](getString(r, "i.toolchain")),
		Commands:     fromJSON[[]string](getString(r, "i.commands")),
		AntiPatterns: fromJSON[[]string](getString(r, "i.anti_patterns")),
		AvgTokens:    getFloat(r, "i.avg_tokens"),
		AvgTurns:     getFloat(r, "i.avg_turns"),
		SuccessRate:  getFloat(r, "i.success_rate"),
		SampleCount:  getInt(r, "i.sample_count"),
	}

	if lastUsed := getString(r, "i.last_used"); lastUsed != "" {
		impl.LastUsed, _ = time.Parse(time.RFC3339, lastUsed)
	}
	if created := getString(r, "i.created_at"); created != "" {
		impl.CreatedAt, _ = time.Parse(time.RFC3339, created)
	}

	return impl, nil
}

func recordToFailedAttempt(r map[string]any) (*FailedAttempt, error) {
	fa := &FailedAttempt{
		ID:             getString(r, "f.id"),
		StrategyID:     getString(r, "f.strategy_id"),
		Environment:    getString(r, "f.environment"),
		TaskID:         getString(r, "f.task_id"),
		AttemptedTools: fromJSON[[]string](getString(r, "f.attempted_tools")),
		FailureReason:  getString(r, "f.failure_reason"),
		Suggestion:     getString(r, "f.suggestion"),
	}

	if created := getString(r, "f.created_at"); created != "" {
		fa.CreatedAt, _ = time.Parse(time.RFC3339, created)
	}

	return fa, nil
}

func getString(r map[string]any, key string) string {
	if v, ok := r[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func getFloat(r map[string]any, key string) float64 {
	if v, ok := r[key]; ok {
		switch f := v.(type) {
		case float64:
			return f
		case float32:
			return float64(f)
		case int:
			return float64(f)
		case int64:
			return float64(f)
		}
	}
	return 0
}

func getInt(r map[string]any, key string) int {
	if v, ok := r[key]; ok {
		switch i := v.(type) {
		case int:
			return i
		case int64:
			return int(i)
		case float64:
			return int(i)
		}
	}
	return 0
}

// SeedBuiltinStrategies populates the database with builtin strategies
func (s *GraphStrategyStore) SeedBuiltinStrategies(ctx context.Context) error {
	for _, strat := range BuiltinStrategies() {
		if err := s.SaveStrategy(ctx, strat); err != nil {
			return fmt.Errorf("seed strategy %s: %w", strat.ID, err)
		}
	}

	for _, impl := range BuiltinImplementations() {
		if err := s.SaveImpl(ctx, impl); err != nil {
			return fmt.Errorf("seed impl %s: %w", impl.ID, err)
		}
	}

	return nil
}
