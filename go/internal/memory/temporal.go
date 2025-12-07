// Package memory provides temporal pattern tracking for knowledge access.
package memory

import (
	"context"
	"sync"
	"time"

	"github.com/joss/urp/internal/graph"
)

// TemporalPattern tracks access patterns over time.
type TemporalPattern struct {
	KnowledgeID   string    `json:"knowledge_id"`
	AccessCount   int       `json:"access_count"`
	LastAccess    time.Time `json:"last_access"`
	FirstAccess   time.Time `json:"first_access"`
	AvgInterval   float64   `json:"avg_interval_seconds"` // Average time between accesses
	HourlyHeat    [24]int   `json:"hourly_heat"`          // Access distribution by hour
	WeekdayHeat   [7]int    `json:"weekday_heat"`         // Access distribution by weekday
	CoAccessedIDs []string  `json:"co_accessed_ids"`      // Often accessed together
	Velocity      float64   `json:"velocity"`             // Access rate (accesses/hour)
}

// AccessEvent represents a single knowledge access.
type AccessEvent struct {
	KnowledgeID string
	Timestamp   time.Time
	QueryText   string // The query that led to this access
	SessionID   string
}

// TemporalTracker tracks and analyzes temporal patterns.
type TemporalTracker struct {
	mu           sync.RWMutex
	patterns     map[string]*TemporalPattern
	recentAccess []AccessEvent // Rolling window of recent accesses
	windowSize   int           // How many recent events to keep
	db           graph.Driver
}

// NewTemporalTracker creates a new tracker.
func NewTemporalTracker(db graph.Driver, windowSize int) *TemporalTracker {
	if windowSize <= 0 {
		windowSize = 1000
	}
	return &TemporalTracker{
		patterns:     make(map[string]*TemporalPattern),
		recentAccess: make([]AccessEvent, 0, windowSize),
		windowSize:   windowSize,
		db:           db,
	}
}

// RecordAccess records an access event and updates patterns.
func (t *TemporalTracker) RecordAccess(knowledgeID, queryText, sessionID string) {
	now := time.Now()
	event := AccessEvent{
		KnowledgeID: knowledgeID,
		Timestamp:   now,
		QueryText:   queryText,
		SessionID:   sessionID,
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	// Update pattern for this knowledge
	pattern, exists := t.patterns[knowledgeID]
	if !exists {
		pattern = &TemporalPattern{
			KnowledgeID: knowledgeID,
			FirstAccess: now,
		}
		t.patterns[knowledgeID] = pattern
	}

	// Update timing stats
	if pattern.AccessCount > 0 {
		interval := now.Sub(pattern.LastAccess).Seconds()
		// Rolling average of intervals
		pattern.AvgInterval = (pattern.AvgInterval*float64(pattern.AccessCount-1) + interval) / float64(pattern.AccessCount)
	}

	pattern.AccessCount++
	pattern.LastAccess = now
	pattern.HourlyHeat[now.Hour()]++
	pattern.WeekdayHeat[now.Weekday()]++

	// Calculate velocity (accesses per hour since first access)
	elapsed := now.Sub(pattern.FirstAccess).Hours()
	if elapsed > 0 {
		pattern.Velocity = float64(pattern.AccessCount) / elapsed
	}

	// Add to recent access window
	t.recentAccess = append(t.recentAccess, event)
	if len(t.recentAccess) > t.windowSize {
		t.recentAccess = t.recentAccess[1:]
	}

	// Update co-access relationships
	t.updateCoAccess(knowledgeID, now)
}

// updateCoAccess finds items accessed within 5 minutes of each other.
func (t *TemporalTracker) updateCoAccess(knowledgeID string, now time.Time) {
	coAccessWindow := 5 * time.Minute
	coAccessed := make(map[string]int)

	for _, event := range t.recentAccess {
		if event.KnowledgeID != knowledgeID && now.Sub(event.Timestamp) < coAccessWindow {
			coAccessed[event.KnowledgeID]++
		}
	}

	// Keep top 5 co-accessed items
	pattern := t.patterns[knowledgeID]
	type scored struct {
		id    string
		count int
	}
	var list []scored
	for id, count := range coAccessed {
		if count >= 2 { // At least 2 co-accesses
			list = append(list, scored{id, count})
		}
	}

	// Sort by count descending
	for i := 0; i < len(list); i++ {
		for j := i + 1; j < len(list); j++ {
			if list[j].count > list[i].count {
				list[i], list[j] = list[j], list[i]
			}
		}
	}

	pattern.CoAccessedIDs = make([]string, 0, 5)
	for i := 0; i < len(list) && i < 5; i++ {
		pattern.CoAccessedIDs = append(pattern.CoAccessedIDs, list[i].id)
	}
}

// GetPattern returns the temporal pattern for a knowledge entry.
func (t *TemporalTracker) GetPattern(knowledgeID string) *TemporalPattern {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if p, ok := t.patterns[knowledgeID]; ok {
		// Return copy
		cp := *p
		cp.CoAccessedIDs = make([]string, len(p.CoAccessedIDs))
		copy(cp.CoAccessedIDs, p.CoAccessedIDs)
		return &cp
	}
	return nil
}

// GetHotKnowledge returns knowledge IDs sorted by recent access velocity.
func (t *TemporalTracker) GetHotKnowledge(limit int) []string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	type scored struct {
		id       string
		velocity float64
	}

	var list []scored
	for id, p := range t.patterns {
		// Only consider recently accessed (within 24 hours)
		if time.Since(p.LastAccess) < 24*time.Hour {
			list = append(list, scored{id, p.Velocity})
		}
	}

	// Sort by velocity descending
	for i := 0; i < len(list); i++ {
		for j := i + 1; j < len(list); j++ {
			if list[j].velocity > list[i].velocity {
				list[i], list[j] = list[j], list[i]
			}
		}
	}

	result := make([]string, 0, limit)
	for i := 0; i < len(list) && i < limit; i++ {
		result = append(result, list[i].id)
	}
	return result
}

// GetDecayedKnowledge returns knowledge that hasn't been accessed recently.
func (t *TemporalTracker) GetDecayedKnowledge(olderThan time.Duration, limit int) []string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var list []string
	cutoff := time.Now().Add(-olderThan)

	for id, p := range t.patterns {
		if p.LastAccess.Before(cutoff) {
			list = append(list, id)
		}
	}

	if len(list) > limit {
		list = list[:limit]
	}
	return list
}

// PredictNextAccess estimates when knowledge will be accessed again.
func (t *TemporalTracker) PredictNextAccess(knowledgeID string) *time.Time {
	t.mu.RLock()
	defer t.mu.RUnlock()

	p, ok := t.patterns[knowledgeID]
	if !ok || p.AccessCount < 3 {
		return nil // Need at least 3 accesses for prediction
	}

	// Simple prediction: last access + average interval
	predicted := p.LastAccess.Add(time.Duration(p.AvgInterval) * time.Second)
	return &predicted
}

// GetPeakHours returns the hours with most accesses.
func (t *TemporalTracker) GetPeakHours(knowledgeID string) []int {
	t.mu.RLock()
	defer t.mu.RUnlock()

	p, ok := t.patterns[knowledgeID]
	if !ok {
		return nil
	}

	// Find top 3 peak hours
	type hourCount struct {
		hour  int
		count int
	}
	var hours []hourCount
	for h, c := range p.HourlyHeat {
		if c > 0 {
			hours = append(hours, hourCount{h, c})
		}
	}

	for i := 0; i < len(hours); i++ {
		for j := i + 1; j < len(hours); j++ {
			if hours[j].count > hours[i].count {
				hours[i], hours[j] = hours[j], hours[i]
			}
		}
	}

	result := make([]int, 0, 3)
	for i := 0; i < len(hours) && i < 3; i++ {
		result = append(result, hours[i].hour)
	}
	return result
}

// Stats returns aggregate statistics.
func (t *TemporalTracker) Stats() map[string]any {
	t.mu.RLock()
	defer t.mu.RUnlock()

	totalAccess := 0
	for _, p := range t.patterns {
		totalAccess += p.AccessCount
	}

	return map[string]any{
		"tracked_items":  len(t.patterns),
		"total_accesses": totalAccess,
		"window_size":    t.windowSize,
		"window_used":    len(t.recentAccess),
	}
}

// PersistToGraph saves temporal patterns to the graph database.
func (t *TemporalTracker) PersistToGraph(ctx context.Context) error {
	if t.db == nil {
		return nil
	}

	t.mu.RLock()
	defer t.mu.RUnlock()

	for id, p := range t.patterns {
		query := `
			MATCH (k:Knowledge {knowledge_id: $id})
			SET k.access_count = $access_count,
			    k.last_access = $last_access,
			    k.velocity = $velocity
		`
		err := t.db.ExecuteWrite(ctx, query, map[string]any{
			"id":           id,
			"access_count": p.AccessCount,
			"last_access":  p.LastAccess.Unix(),
			"velocity":     p.Velocity,
		})
		if err != nil {
			return err
		}
	}

	return nil
}

// LoadFromGraph loads temporal data from the graph database.
func (t *TemporalTracker) LoadFromGraph(ctx context.Context) error {
	if t.db == nil {
		return nil
	}

	query := `
		MATCH (k:Knowledge)
		WHERE k.access_count IS NOT NULL
		RETURN k.knowledge_id as id,
		       k.access_count as access_count,
		       k.last_access as last_access,
		       k.velocity as velocity
	`

	records, err := t.db.Execute(ctx, query, nil)
	if err != nil {
		return err
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	for _, r := range records {
		id := graph.GetString(r, "id")
		if id == "" {
			continue
		}

		accessCount := 0
		if v, ok := r["access_count"].(int64); ok {
			accessCount = int(v)
		}

		var lastAccess time.Time
		if v, ok := r["last_access"].(int64); ok {
			lastAccess = time.Unix(v, 0)
		}

		velocity := 0.0
		if v, ok := r["velocity"].(float64); ok {
			velocity = v
		}

		t.patterns[id] = &TemporalPattern{
			KnowledgeID: id,
			AccessCount: accessCount,
			LastAccess:  lastAccess,
			Velocity:    velocity,
		}
	}

	return nil
}
