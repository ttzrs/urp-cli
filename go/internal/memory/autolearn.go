// Package memory provides auto-learning capabilities.
package memory

import (
	"context"
	"sync"
	"time"

	"github.com/joss/urp/internal/graph"
)

// LearningEvent represents a pattern that can be learned.
type LearningEvent struct {
	EventType    string            `json:"event_type"`    // "success", "failure", "fix", "pattern"
	Description  string            `json:"description"`   // What happened
	Context      map[string]string `json:"context"`       // File, function, error type, etc.
	Solution     string            `json:"solution"`      // What worked (if success/fix)
	Confidence   float64           `json:"confidence"`    // How certain we are (0-1)
	Timestamp    time.Time         `json:"timestamp"`     //
	SessionID    string            `json:"session_id"`    //
	RelatedIDs   []string          `json:"related_ids"`   // Related knowledge entries
	WasPromoted  bool              `json:"was_promoted"`  // Was this promoted to persistent?
	PromotedAt   *time.Time        `json:"promoted_at"`   //
	SuccessCount int               `json:"success_count"` // Times this solution worked
	FailureCount int               `json:"failure_count"` // Times it failed
}

// AutoLearner implements automatic knowledge extraction and promotion.
type AutoLearner struct {
	mu              sync.RWMutex
	pendingLearning []LearningEvent
	db              graph.Driver
	tracker         *TemporalTracker
	config          AutoLearnConfig
}

// AutoLearnConfig configures the auto-learning behavior.
type AutoLearnConfig struct {
	MinConfidenceToStore   float64       // Minimum confidence to store (default: 0.5)
	MinSuccessToPromote    int           // Successes needed to auto-promote (default: 3)
	MaxPendingEvents       int           // Max pending events before flush (default: 100)
	DecayThreshold         time.Duration // Mark as decayed after this (default: 7 days)
	PromotionCheckInterval time.Duration // How often to check for promotions (default: 1h)
}

// DefaultAutoLearnConfig returns sensible defaults.
func DefaultAutoLearnConfig() AutoLearnConfig {
	return AutoLearnConfig{
		MinConfidenceToStore:   0.5,
		MinSuccessToPromote:    3,
		MaxPendingEvents:       100,
		DecayThreshold:         7 * 24 * time.Hour,
		PromotionCheckInterval: time.Hour,
	}
}

// NewAutoLearner creates a new auto-learner.
func NewAutoLearner(db graph.Driver, tracker *TemporalTracker, config AutoLearnConfig) *AutoLearner {
	if config.MinConfidenceToStore <= 0 {
		config.MinConfidenceToStore = 0.5
	}
	if config.MinSuccessToPromote <= 0 {
		config.MinSuccessToPromote = 3
	}
	if config.MaxPendingEvents <= 0 {
		config.MaxPendingEvents = 100
	}
	if config.DecayThreshold <= 0 {
		config.DecayThreshold = 7 * 24 * time.Hour
	}

	return &AutoLearner{
		pendingLearning: make([]LearningEvent, 0),
		db:              db,
		tracker:         tracker,
		config:          config,
	}
}

// RecordSuccess records a successful outcome.
func (a *AutoLearner) RecordSuccess(description, solution, sessionID string, ctx map[string]string) {
	event := LearningEvent{
		EventType:    "success",
		Description:  description,
		Solution:     solution,
		Context:      ctx,
		Confidence:   0.7, // Start with moderate confidence
		Timestamp:    time.Now(),
		SessionID:    sessionID,
		SuccessCount: 1,
	}

	a.addEvent(event)
}

// RecordFailure records a failed attempt.
func (a *AutoLearner) RecordFailure(description, attemptedSolution, sessionID string, ctx map[string]string) {
	event := LearningEvent{
		EventType:    "failure",
		Description:  description,
		Solution:     attemptedSolution,
		Context:      ctx,
		Confidence:   0.3, // Low confidence for failures
		Timestamp:    time.Now(),
		SessionID:    sessionID,
		FailureCount: 1,
	}

	a.addEvent(event)
}

// RecordFix records a fix that resolved an issue.
func (a *AutoLearner) RecordFix(problem, fix, sessionID string, ctx map[string]string) {
	event := LearningEvent{
		EventType:    "fix",
		Description:  problem,
		Solution:     fix,
		Context:      ctx,
		Confidence:   0.8, // High confidence for fixes
		Timestamp:    time.Now(),
		SessionID:    sessionID,
		SuccessCount: 1,
	}

	a.addEvent(event)
}

// RecordPattern records a detected code pattern.
func (a *AutoLearner) RecordPattern(patternName, description, sessionID string, ctx map[string]string) {
	if ctx == nil {
		ctx = make(map[string]string)
	}
	event := LearningEvent{
		EventType:   "pattern",
		Description: description,
		Context:     ctx,
		Confidence:  0.6,
		Timestamp:   time.Now(),
		SessionID:   sessionID,
	}
	event.Context["pattern_name"] = patternName

	a.addEvent(event)
}

func (a *AutoLearner) addEvent(event LearningEvent) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Check for existing similar event and merge
	for i, existing := range a.pendingLearning {
		if a.isSimilar(existing, event) {
			// Merge: update counts and confidence
			a.pendingLearning[i].SuccessCount += event.SuccessCount
			a.pendingLearning[i].FailureCount += event.FailureCount

			// Adjust confidence based on outcomes
			total := float64(a.pendingLearning[i].SuccessCount + a.pendingLearning[i].FailureCount)
			if total > 0 {
				successRate := float64(a.pendingLearning[i].SuccessCount) / total
				a.pendingLearning[i].Confidence = 0.3 + (0.6 * successRate) // Range 0.3-0.9
			}
			return
		}
	}

	// Add new event
	a.pendingLearning = append(a.pendingLearning, event)

	// Trim if over limit
	if len(a.pendingLearning) > a.config.MaxPendingEvents {
		a.pendingLearning = a.pendingLearning[1:]
	}
}

// isSimilar checks if two events are similar enough to merge.
func (a *AutoLearner) isSimilar(a1, a2 LearningEvent) bool {
	if a1.EventType != a2.EventType {
		return false
	}

	// Compare descriptions using simple word overlap
	words1 := tokenize(a1.Description)
	words2 := tokenize(a2.Description)

	similarity := jaccardFromSets(words1, words2)
	return similarity > 0.6
}

// GetPendingEvents returns events ready for review.
func (a *AutoLearner) GetPendingEvents(minConfidence float64) []LearningEvent {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var result []LearningEvent
	for _, e := range a.pendingLearning {
		if e.Confidence >= minConfidence && !e.WasPromoted {
			cp := e
			cp.Context = make(map[string]string)
			for k, v := range e.Context {
				cp.Context[k] = v
			}
			result = append(result, cp)
		}
	}
	return result
}

// GetPromotionCandidates returns events that qualify for auto-promotion.
func (a *AutoLearner) GetPromotionCandidates() []LearningEvent {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var candidates []LearningEvent
	for _, e := range a.pendingLearning {
		if !e.WasPromoted &&
			e.Confidence >= a.config.MinConfidenceToStore &&
			e.SuccessCount >= a.config.MinSuccessToPromote {
			candidates = append(candidates, e)
		}
	}
	return candidates
}

// PromoteEvent promotes a learning event to persistent knowledge.
func (a *AutoLearner) PromoteEvent(description string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	for i, e := range a.pendingLearning {
		if e.Description == description && !e.WasPromoted {
			now := time.Now()
			a.pendingLearning[i].WasPromoted = true
			a.pendingLearning[i].PromotedAt = &now

			// Store in graph if available
			if a.db != nil {
				return a.storeInGraph(context.Background(), e)
			}
			return "", nil
		}
	}
	return "", nil
}

func (a *AutoLearner) storeInGraph(ctx context.Context, e LearningEvent) (string, error) {
	knowledgeID := generateID()

	query := `
		CREATE (k:Knowledge {
			knowledge_id: $id,
			kind: $kind,
			scope: 'instance',
			text: $text,
			context_signature: $sig,
			created_at: timestamp(),
			confidence: $confidence,
			success_count: $success_count,
			auto_learned: true
		})
		RETURN k.knowledge_id as id
	`

	text := e.Description
	if e.Solution != "" {
		text += "\n\nSolution: " + e.Solution
	}

	err := a.db.ExecuteWrite(ctx, query, map[string]any{
		"id":            knowledgeID,
		"kind":          "solution",
		"text":          text,
		"sig":           e.Context["file"],
		"confidence":    e.Confidence,
		"success_count": e.SuccessCount,
	})

	return knowledgeID, err
}

// ConfirmSuccess marks a solution as successful (reinforcement).
func (a *AutoLearner) ConfirmSuccess(description string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	for i, e := range a.pendingLearning {
		if e.Description == description {
			a.pendingLearning[i].SuccessCount++

			// Increase confidence
			total := float64(a.pendingLearning[i].SuccessCount + a.pendingLearning[i].FailureCount)
			successRate := float64(a.pendingLearning[i].SuccessCount) / total
			a.pendingLearning[i].Confidence = 0.3 + (0.6 * successRate)
			return
		}
	}
}

// ConfirmFailure marks a solution as failed (negative reinforcement).
func (a *AutoLearner) ConfirmFailure(description string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	for i, e := range a.pendingLearning {
		if e.Description == description {
			a.pendingLearning[i].FailureCount++

			// Decrease confidence
			total := float64(a.pendingLearning[i].SuccessCount + a.pendingLearning[i].FailureCount)
			successRate := float64(a.pendingLearning[i].SuccessCount) / total
			a.pendingLearning[i].Confidence = 0.3 + (0.6 * successRate)
			return
		}
	}
}

// SuggestRelated suggests knowledge related to current context.
func (a *AutoLearner) SuggestRelated(currentFile, errorType string) []LearningEvent {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var suggestions []LearningEvent

	for _, e := range a.pendingLearning {
		if e.Confidence < 0.5 || e.WasPromoted {
			continue
		}

		// Match by file
		if currentFile != "" && e.Context["file"] == currentFile {
			suggestions = append(suggestions, e)
			continue
		}

		// Match by error type
		if errorType != "" && e.Context["error_type"] == errorType {
			suggestions = append(suggestions, e)
		}
	}

	// Sort by confidence
	for i := 0; i < len(suggestions); i++ {
		for j := i + 1; j < len(suggestions); j++ {
			if suggestions[j].Confidence > suggestions[i].Confidence {
				suggestions[i], suggestions[j] = suggestions[j], suggestions[i]
			}
		}
	}

	if len(suggestions) > 5 {
		suggestions = suggestions[:5]
	}

	return suggestions
}

// Stats returns learning statistics.
func (a *AutoLearner) Stats() map[string]any {
	a.mu.RLock()
	defer a.mu.RUnlock()

	pending := 0
	promoted := 0
	totalSuccess := 0
	totalFailure := 0

	for _, e := range a.pendingLearning {
		if e.WasPromoted {
			promoted++
		} else {
			pending++
		}
		totalSuccess += e.SuccessCount
		totalFailure += e.FailureCount
	}

	successRate := 0.0
	if totalSuccess+totalFailure > 0 {
		successRate = float64(totalSuccess) / float64(totalSuccess+totalFailure)
	}

	return map[string]any{
		"pending_events": pending,
		"promoted":       promoted,
		"total_events":   len(a.pendingLearning),
		"total_success":  totalSuccess,
		"total_failure":  totalFailure,
		"success_rate":   successRate,
	}
}

// Flush persists pending high-confidence events to graph.
func (a *AutoLearner) Flush(ctx context.Context) (int, error) {
	candidates := a.GetPromotionCandidates()

	count := 0
	for _, c := range candidates {
		_, err := a.PromoteEvent(c.Description)
		if err != nil {
			return count, err
		}
		count++
	}

	return count, nil
}

// Clear removes all pending events.
func (a *AutoLearner) Clear() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.pendingLearning = a.pendingLearning[:0]
}

// generateID creates a unique knowledge ID.
func generateID() string {
	return "k-" + time.Now().Format("20060102150405") + "-" + randomSuffix()
}

func randomSuffix() string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 6)
	for i := range b {
		b[i] = chars[time.Now().UnixNano()%int64(len(chars))]
	}
	return string(b)
}
