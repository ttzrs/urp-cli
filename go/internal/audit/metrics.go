// Package audit metrics collection and aggregation.
package audit

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/joss/urp/internal/graph"
)

// MetricType identifies what's being measured.
type MetricType string

const (
	MetricLatency   MetricType = "latency_ms"
	MetricFrequency MetricType = "frequency"
	MetricErrorRate MetricType = "error_rate"
	MetricOutputSize MetricType = "output_size"
)

// Metric represents a single metric data point.
type Metric struct {
	Type      MetricType `json:"type"`
	Category  Category   `json:"category"`
	Operation string     `json:"operation"`
	Value     float64    `json:"value"`
	Timestamp time.Time  `json:"timestamp"`
	SessionID string     `json:"session_id"`
}

// MetricStats holds statistical summary for a metric.
type MetricStats struct {
	Type      MetricType `json:"type"`
	Category  Category   `json:"category"`
	Operation string     `json:"operation,omitempty"`
	Count     int        `json:"count"`
	Sum       float64    `json:"sum"`
	Mean      float64    `json:"mean"`
	Min       float64    `json:"min"`
	Max       float64    `json:"max"`
	StdDev    float64    `json:"std_dev"`
	P50       float64    `json:"p50"`
	P95       float64    `json:"p95"`
	P99       float64    `json:"p99"`
}

// MetricsCollector aggregates metrics from audit events.
type MetricsCollector struct {
	mu       sync.RWMutex
	metrics  []Metric
	window   time.Duration
	maxSize  int
}

// NewMetricsCollector creates a new collector.
func NewMetricsCollector(window time.Duration, maxSize int) *MetricsCollector {
	if window == 0 {
		window = 1 * time.Hour
	}
	if maxSize == 0 {
		maxSize = 10000
	}
	return &MetricsCollector{
		metrics: make([]Metric, 0, maxSize),
		window:  window,
		maxSize: maxSize,
	}
}

// Record adds a metric from an audit event.
func (c *MetricsCollector) Record(event *AuditEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()

	// Latency metric
	if event.DurationMs > 0 {
		c.metrics = append(c.metrics, Metric{
			Type:      MetricLatency,
			Category:  event.Category,
			Operation: event.Operation,
			Value:     float64(event.DurationMs),
			Timestamp: now,
			SessionID: event.SessionID,
		})
	}

	// Output size metric
	if event.OutputSize > 0 {
		c.metrics = append(c.metrics, Metric{
			Type:      MetricOutputSize,
			Category:  event.Category,
			Operation: event.Operation,
			Value:     float64(event.OutputSize),
			Timestamp: now,
			SessionID: event.SessionID,
		})
	}

	// Prune old metrics
	c.prune()
}

// RecordError records an error occurrence.
func (c *MetricsCollector) RecordError(category Category, operation string, sessionID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.metrics = append(c.metrics, Metric{
		Type:      MetricErrorRate,
		Category:  category,
		Operation: operation,
		Value:     1.0, // error count
		Timestamp: time.Now(),
		SessionID: sessionID,
	})

	c.prune()
}

// prune removes old metrics outside the window.
func (c *MetricsCollector) prune() {
	cutoff := time.Now().Add(-c.window)
	newMetrics := make([]Metric, 0, len(c.metrics))
	for _, m := range c.metrics {
		if m.Timestamp.After(cutoff) {
			newMetrics = append(newMetrics, m)
		}
	}
	c.metrics = newMetrics

	// Cap size
	if len(c.metrics) > c.maxSize {
		c.metrics = c.metrics[len(c.metrics)-c.maxSize:]
	}
}

// GetStats computes statistics for a metric type and category.
func (c *MetricsCollector) GetStats(metricType MetricType, category Category, operation string) *MetricStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var values []float64
	for _, m := range c.metrics {
		if m.Type != metricType {
			continue
		}
		if category != "" && m.Category != category {
			continue
		}
		if operation != "" && m.Operation != operation {
			continue
		}
		values = append(values, m.Value)
	}

	if len(values) == 0 {
		return nil
	}

	return computeStats(metricType, category, operation, values)
}

// GetAllStats returns stats for all tracked metric combinations.
func (c *MetricsCollector) GetAllStats() []MetricStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Group by type+category+operation
	groups := make(map[string][]float64)
	keys := make(map[string]struct{ t MetricType; c Category; o string })

	for _, m := range c.metrics {
		key := string(m.Type) + ":" + string(m.Category) + ":" + m.Operation
		groups[key] = append(groups[key], m.Value)
		keys[key] = struct{ t MetricType; c Category; o string }{m.Type, m.Category, m.Operation}
	}

	var stats []MetricStats
	for key, values := range groups {
		k := keys[key]
		s := computeStats(k.t, k.c, k.o, values)
		if s != nil {
			stats = append(stats, *s)
		}
	}

	return stats
}

// computeStats calculates statistical measures.
func computeStats(metricType MetricType, category Category, operation string, values []float64) *MetricStats {
	n := len(values)
	if n == 0 {
		return nil
	}

	// Sort for percentiles
	sorted := make([]float64, n)
	copy(sorted, values)
	sortFloat64(sorted)

	// Basic stats
	var sum, min, max float64
	min = sorted[0]
	max = sorted[n-1]
	for _, v := range sorted {
		sum += v
	}
	mean := sum / float64(n)

	// Std dev
	var variance float64
	for _, v := range sorted {
		variance += (v - mean) * (v - mean)
	}
	stdDev := math.Sqrt(variance / float64(n))

	return &MetricStats{
		Type:      metricType,
		Category:  category,
		Operation: operation,
		Count:     n,
		Sum:       sum,
		Mean:      mean,
		Min:       min,
		Max:       max,
		StdDev:    stdDev,
		P50:       percentile(sorted, 0.50),
		P95:       percentile(sorted, 0.95),
		P99:       percentile(sorted, 0.99),
	}
}

// sortFloat64 sorts a slice of float64 in place.
func sortFloat64(a []float64) {
	// Simple insertion sort (good enough for typical sizes)
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && a[j] < a[j-1]; j-- {
			a[j], a[j-1] = a[j-1], a[j]
		}
	}
}

// percentile returns the p-th percentile of sorted values.
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(float64(len(sorted)-1) * p)
	return sorted[idx]
}

// MetricsStore persists metrics to Memgraph.
type MetricsStore struct {
	db        graph.Driver
	sessionID string
}

// NewMetricsStore creates a metrics store.
func NewMetricsStore(db graph.Driver, sessionID string) *MetricsStore {
	return &MetricsStore{db: db, sessionID: sessionID}
}

// SaveStats persists metric statistics.
func (s *MetricsStore) SaveStats(ctx context.Context, stats *MetricStats) error {
	query := `
		MERGE (sess:Session {session_id: $session_id})
		CREATE (m:MetricStats {
			type: $type,
			category: $category,
			operation: $operation,
			count: $count,
			sum: $sum,
			mean: $mean,
			min: $min,
			max: $max,
			std_dev: $std_dev,
			p50: $p50,
			p95: $p95,
			p99: $p99,
			recorded_at: $recorded_at
		})
		CREATE (sess)-[:HAS_METRICS]->(m)
	`

	return s.db.ExecuteWrite(ctx, query, map[string]any{
		"session_id":  s.sessionID,
		"type":        string(stats.Type),
		"category":    string(stats.Category),
		"operation":   stats.Operation,
		"count":       stats.Count,
		"sum":         stats.Sum,
		"mean":        stats.Mean,
		"min":         stats.Min,
		"max":         stats.Max,
		"std_dev":     stats.StdDev,
		"p50":         stats.P50,
		"p95":         stats.P95,
		"p99":         stats.P99,
		"recorded_at": time.Now().UTC().Format(time.RFC3339),
	})
}

// GetHistoricalStats retrieves historical stats for baseline comparison.
func (s *MetricsStore) GetHistoricalStats(ctx context.Context, metricType MetricType, category Category, limit int) ([]MetricStats, error) {
	if limit <= 0 {
		limit = 100
	}

	query := `
		MATCH (sess:Session)-[:HAS_METRICS]->(m:MetricStats)
		WHERE m.type = $type AND m.category = $category
		RETURN m.type as type,
		       m.category as category,
		       m.operation as operation,
		       m.count as count,
		       m.sum as sum,
		       m.mean as mean,
		       m.min as min,
		       m.max as max,
		       m.std_dev as std_dev,
		       m.p50 as p50,
		       m.p95 as p95,
		       m.p99 as p99
		ORDER BY m.recorded_at DESC
		LIMIT $limit
	`

	records, err := s.db.Execute(ctx, query, map[string]any{
		"type":     string(metricType),
		"category": string(category),
		"limit":    limit,
	})
	if err != nil {
		return nil, err
	}

	var stats []MetricStats
	for _, r := range records {
		stats = append(stats, MetricStats{
			Type:      MetricType(graph.GetString(r, "type")),
			Category:  Category(graph.GetString(r, "category")),
			Operation: graph.GetString(r, "operation"),
			Count:     graph.GetInt(r, "count"),
			Sum:       graph.GetFloat(r, "sum"),
			Mean:      graph.GetFloat(r, "mean"),
			Min:       graph.GetFloat(r, "min"),
			Max:       graph.GetFloat(r, "max"),
			StdDev:    graph.GetFloat(r, "std_dev"),
			P50:       graph.GetFloat(r, "p50"),
			P95:       graph.GetFloat(r, "p95"),
			P99:       graph.GetFloat(r, "p99"),
		})
	}

	return stats, nil
}
