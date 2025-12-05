// Package audit provides audit event tracking and analysis.
package audit

import (
	"context"
	"sort"
	"time"
)

// Service provides audit business logic.
// Separates business logic from persistence (Store) and presentation (CLI).
type Service struct {
	store  *Store
	logger *Logger
}

// NewService creates a new audit service.
func NewService(store *Store, logger *Logger) *Service {
	return &Service{
		store:  store,
		logger: logger,
	}
}

// Stats represents aggregate audit statistics.
type Stats struct {
	Total         int           `json:"total"`
	Success       int           `json:"success"`
	Errors        int           `json:"errors"`
	Warnings      int           `json:"warnings"`
	Timeouts      int           `json:"timeouts"`
	AvgDurationMs float64       `json:"avg_duration_ms"`
	MaxDurationMs int64         `json:"max_duration_ms"`
	FirstEvent    time.Time     `json:"first_event,omitempty"`
	LastEvent     time.Time     `json:"last_event,omitempty"`
	SuccessRate   float64       `json:"success_rate"`
	ByCategory    CategoryStats `json:"by_category,omitempty"`
}

// CategoryStats maps category to its statistics.
type CategoryStats map[string]*CategoryStat

// CategoryStat holds per-category statistics.
type CategoryStat struct {
	Total         int     `json:"total"`
	Success       int     `json:"success"`
	Errors        int     `json:"errors"`
	AvgDurationMs float64 `json:"avg_duration_ms"`
	SuccessRate   float64 `json:"success_rate"`
}

// GetRecentEvents returns the most recent audit events.
func (s *Service) GetRecentEvents(ctx context.Context, limit int) ([]AuditEvent, error) {
	if s.store == nil {
		return nil, nil
	}
	return s.store.Query(ctx, QueryFilter{Limit: limit})
}

// GetErrors returns recent error events with optional filtering.
func (s *Service) GetErrors(ctx context.Context, since time.Time, category string, limit int) ([]AuditEvent, error) {
	if s.store == nil {
		return nil, nil
	}

	filter := QueryFilter{
		Status: StatusError,
		Limit:  limit,
	}
	if !since.IsZero() {
		filter.Since = since
	}
	if category != "" {
		filter.Category = Category(category)
	}

	return s.store.Query(ctx, filter)
}

// GetStats returns aggregate statistics.
func (s *Service) GetStats(ctx context.Context) (*Stats, error) {
	if s.store == nil {
		return &Stats{}, nil
	}

	// Get overall stats
	rawStats, err := s.store.GetStats(ctx)
	if err != nil {
		return nil, err
	}

	stats := &Stats{
		Total:         toInt(rawStats["total"]),
		Success:       toInt(rawStats["success"]),
		Errors:        toInt(rawStats["errors"]),
		Warnings:      toInt(rawStats["warnings"]),
		Timeouts:      toInt(rawStats["timeouts"]),
		AvgDurationMs: toFloat(rawStats["avg_duration_ms"]),
		MaxDurationMs: toInt64(rawStats["max_duration_ms"]),
	}

	// Calculate success rate
	if stats.Total > 0 {
		stats.SuccessRate = float64(stats.Success) / float64(stats.Total) * 100
	}

	// Parse times
	if first, ok := rawStats["first_event"].(string); ok && first != "" {
		if t, err := time.Parse(time.RFC3339, first); err == nil {
			stats.FirstEvent = t
		}
	}
	if last, ok := rawStats["last_event"].(string); ok && last != "" {
		if t, err := time.Parse(time.RFC3339, last); err == nil {
			stats.LastEvent = t
		}
	}

	// Get by-category stats
	catStats, err := s.store.GetStatsByCategory(ctx)
	if err == nil && len(catStats) > 0 {
		stats.ByCategory = make(CategoryStats)
		for cat, raw := range catStats {
			cs := &CategoryStat{
				Total:         toInt(raw["total"]),
				Success:       toInt(raw["success"]),
				Errors:        toInt(raw["errors"]),
				AvgDurationMs: toFloat(raw["avg_duration_ms"]),
			}
			if cs.Total > 0 {
				cs.SuccessRate = float64(cs.Success) / float64(cs.Total) * 100
			}
			stats.ByCategory[cat] = cs
		}
	}

	return stats, nil
}

// GetEventsByCategory returns events filtered by category.
func (s *Service) GetEventsByCategory(ctx context.Context, category string, limit int) ([]AuditEvent, error) {
	if s.store == nil {
		return nil, nil
	}
	return s.store.Query(ctx, QueryFilter{
		Category: Category(category),
		Limit:    limit,
	})
}

// GetEventsByCommit returns events associated with a specific git commit.
func (s *Service) GetEventsByCommit(ctx context.Context, commitHash string) ([]AuditEvent, error) {
	if s.store == nil {
		return nil, nil
	}
	return s.store.GetEventsByCommit(ctx, commitHash)
}

// GetEventsByTimeRange returns events within a time range.
func (s *Service) GetEventsByTimeRange(ctx context.Context, since, until time.Time, limit int) ([]AuditEvent, error) {
	if s.store == nil {
		return nil, nil
	}
	return s.store.Query(ctx, QueryFilter{
		Since: since,
		Until: until,
		Limit: limit,
	})
}

// EventSummary provides a summary of an event for display.
type EventSummary struct {
	EventID   string        `json:"event_id"`
	Category  string        `json:"category"`
	Operation string        `json:"operation"`
	Status    string        `json:"status"`
	Duration  time.Duration `json:"duration"`
	Error     string        `json:"error,omitempty"`
	Timestamp time.Time     `json:"timestamp"`
}

// SummarizeEvents converts events to summaries for display.
func (s *Service) SummarizeEvents(events []AuditEvent) []EventSummary {
	summaries := make([]EventSummary, len(events))
	for i, e := range events {
		summaries[i] = EventSummary{
			EventID:   e.EventID,
			Category:  string(e.Category),
			Operation: e.Operation,
			Status:    string(e.Status),
			Duration:  e.Duration,
			Error:     e.ErrorMessage,
			Timestamp: e.StartedAt,
		}
	}
	return summaries
}

// TopErrorCategories returns categories sorted by error count.
func (s *Service) TopErrorCategories(ctx context.Context, limit int) ([]string, error) {
	stats, err := s.GetStats(ctx)
	if err != nil {
		return nil, err
	}

	type catError struct {
		category string
		errors   int
	}
	var cats []catError
	for cat, cs := range stats.ByCategory {
		if cs.Errors > 0 {
			cats = append(cats, catError{cat, cs.Errors})
		}
	}

	sort.Slice(cats, func(i, j int) bool {
		return cats[i].errors > cats[j].errors
	})

	if limit > 0 && len(cats) > limit {
		cats = cats[:limit]
	}

	result := make([]string, len(cats))
	for i, c := range cats {
		result[i] = c.category
	}
	return result, nil
}

// Helper functions for type conversion.
func toInt(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	}
	return 0
}

func toInt64(v any) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case int:
		return int64(x)
	case float64:
		return int64(x)
	}
	return 0
}

func toFloat(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int:
		return float64(x)
	case int64:
		return float64(x)
	}
	return 0
}
