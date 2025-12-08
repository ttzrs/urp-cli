// Package agent provides task similarity search via embeddings
package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/joss/urp/internal/vector"
)

// TaskEmbedding stores a task's vector representation
type TaskEmbedding struct {
	TaskID       string    `json:"task_id"`
	Objective    string    `json:"objective"`
	Environment  string    `json:"environment"`
	TaskType     string    `json:"task_type"`
	StrategyUsed string    `json:"strategy_used"`
	Success      bool      `json:"success"`
	Tokens       int       `json:"tokens"`
	Turns        int       `json:"turns"`
	CreatedAt    time.Time `json:"created_at"`
}

// EmbeddingStore provides vector similarity search for tasks
type EmbeddingStore struct {
	store    vector.Store
	embedder vector.Embedder
}

// NewEmbeddingStore creates an embedding store with LanceDB backend
func NewEmbeddingStore(dbPath string) (*EmbeddingStore, error) {
	embedder := vector.GetDefaultEmbedder()

	store, err := vector.NewLanceStore(dbPath)
	if err != nil {
		return nil, fmt.Errorf("create store: %w", err)
	}

	return &EmbeddingStore{
		store:    store,
		embedder: embedder,
	}, nil
}

// IndexTask stores a task embedding for future similarity search
func (e *EmbeddingStore) IndexTask(ctx context.Context, task TaskEmbedding) error {
	// Generate embedding from objective text
	embedding, err := e.embedder.Embed(ctx, task.Objective)
	if err != nil {
		return fmt.Errorf("embed objective: %w", err)
	}

	// Build metadata as string map
	metadata := map[string]string{
		"task_id":       task.TaskID,
		"objective":     task.Objective,
		"environment":   task.Environment,
		"task_type":     task.TaskType,
		"strategy_used": task.StrategyUsed,
		"success":       fmt.Sprintf("%v", task.Success),
		"tokens":        fmt.Sprintf("%d", task.Tokens),
		"turns":         fmt.Sprintf("%d", task.Turns),
		"created_at":    task.CreatedAt.Format(time.RFC3339),
	}

	// Store in vector DB using VectorEntry
	entry := vector.VectorEntry{
		ID:       task.TaskID,
		Text:     task.Objective,
		Vector:   embedding,
		Kind:     "task",
		Metadata: metadata,
	}
	return e.store.Add(ctx, entry)
}

// FindSimilarTasks finds tasks similar to a given objective
func (e *EmbeddingStore) FindSimilarTasks(ctx context.Context, objective string, limit int) ([]SimilarTask, error) {
	// Generate embedding for query
	embedding, err := e.embedder.Embed(ctx, objective)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}

	// Search vector store for "task" kind
	results, err := e.store.Search(ctx, embedding, limit, "task")
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}

	var similar []SimilarTask
	for _, r := range results {
		task := SimilarTask{
			TaskID:       getStringFromMeta(r.Entry.Metadata, "task_id"),
			Objective:    getStringFromMeta(r.Entry.Metadata, "objective"),
			Environment:  getStringFromMeta(r.Entry.Metadata, "environment"),
			TaskType:     getStringFromMeta(r.Entry.Metadata, "task_type"),
			StrategyUsed: getStringFromMeta(r.Entry.Metadata, "strategy_used"),
			Success:      getStringFromMeta(r.Entry.Metadata, "success") == "true",
			Tokens:       getIntFromMetaStr(r.Entry.Metadata, "tokens"),
			Turns:        getIntFromMetaStr(r.Entry.Metadata, "turns"),
			Similarity:   float64(r.Score),
		}
		similar = append(similar, task)
	}

	return similar, nil
}

// FindSimilarSuccessful finds successful tasks similar to objective
func (e *EmbeddingStore) FindSimilarSuccessful(ctx context.Context, objective, targetEnv string, limit int) ([]SimilarTask, error) {
	similar, err := e.FindSimilarTasks(ctx, objective, limit*3) // Get more to filter
	if err != nil {
		return nil, err
	}

	// Filter and re-rank
	var filtered []SimilarTask
	for _, t := range similar {
		if !t.Success {
			continue // Only successful tasks
		}

		// Adjust similarity by environment proximity
		envSim := GetEnvSimilarity(t.Environment, targetEnv)
		t.Similarity *= envSim

		filtered = append(filtered, t)
	}

	// Sort by adjusted similarity
	// Simple bubble sort for small list
	for i := 0; i < len(filtered)-1; i++ {
		for j := i + 1; j < len(filtered); j++ {
			if filtered[j].Similarity > filtered[i].Similarity {
				filtered[i], filtered[j] = filtered[j], filtered[i]
			}
		}
	}

	// Limit results
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}

	return filtered, nil
}

// SimilarTask represents a task found via similarity search
type SimilarTask struct {
	TaskID       string  `json:"task_id"`
	Objective    string  `json:"objective"`
	Environment  string  `json:"environment"`
	TaskType     string  `json:"task_type"`
	StrategyUsed string  `json:"strategy_used"`
	Success      bool    `json:"success"`
	Tokens       int     `json:"tokens"`
	Turns        int     `json:"turns"`
	Similarity   float64 `json:"similarity"`
}

// GetStrategyFromSimilar extracts the best strategy from similar tasks
func GetStrategyFromSimilar(similar []SimilarTask) (string, float64) {
	if len(similar) == 0 {
		return "", 0
	}

	// Count strategy usage weighted by similarity
	strategyScores := make(map[string]float64)
	for _, t := range similar {
		if t.StrategyUsed != "" {
			strategyScores[t.StrategyUsed] += t.Similarity
		}
	}

	// Find best
	var bestStrategy string
	var bestScore float64
	for strat, score := range strategyScores {
		if score > bestScore {
			bestScore = score
			bestStrategy = strat
		}
	}

	// Normalize score
	if len(similar) > 0 {
		bestScore /= float64(len(similar))
	}

	return bestStrategy, bestScore
}

// GetExpectedMetrics estimates tokens/turns from similar tasks
func GetExpectedMetrics(similar []SimilarTask) (avgTokens, avgTurns float64) {
	if len(similar) == 0 {
		return 3000, 4 // Default estimates
	}

	var totalTokens, totalTurns float64
	var weightSum float64

	for _, t := range similar {
		weight := t.Similarity
		totalTokens += float64(t.Tokens) * weight
		totalTurns += float64(t.Turns) * weight
		weightSum += weight
	}

	if weightSum > 0 {
		avgTokens = totalTokens / weightSum
		avgTurns = totalTurns / weightSum
	}

	return avgTokens, avgTurns
}

// Close releases resources
func (e *EmbeddingStore) Close() error {
	return e.store.Close()
}

// Helper functions for string metadata
func getStringFromMeta(m map[string]string, key string) string {
	if v, ok := m[key]; ok {
		return v
	}
	return ""
}

func getIntFromMetaStr(m map[string]string, key string) int {
	if v, ok := m[key]; ok {
		var n int
		fmt.Sscanf(v, "%d", &n)
		return n
	}
	return 0
}
