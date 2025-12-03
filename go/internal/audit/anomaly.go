// Package audit anomaly detection for operations.
package audit

import (
	"context"
	"math"
	"time"

	"github.com/joss/urp/internal/graph"
)

// AnomalyLevel indicates severity of detected anomaly.
type AnomalyLevel string

const (
	AnomalyNone     AnomalyLevel = "none"
	AnomalyLow      AnomalyLevel = "low"      // 1-2 sigma
	AnomalyMedium   AnomalyLevel = "medium"   // 2-3 sigma
	AnomalyHigh     AnomalyLevel = "high"     // 3+ sigma
	AnomalyCritical AnomalyLevel = "critical" // 4+ sigma or threshold breach
)

// Anomaly represents a detected anomaly.
type Anomaly struct {
	ID          string       `json:"id"`
	Level       AnomalyLevel `json:"level"`
	Type        MetricType   `json:"type"`
	Category    Category     `json:"category"`
	Operation   string       `json:"operation"`
	Value       float64      `json:"value"`
	Expected    float64      `json:"expected"`
	StdDev      float64      `json:"std_dev"`
	ZScore      float64      `json:"z_score"`
	Description string       `json:"description"`
	DetectedAt  time.Time    `json:"detected_at"`
	SessionID   string       `json:"session_id"`
}

// Baseline represents expected behavior for an operation.
type Baseline struct {
	Type      MetricType `json:"type"`
	Category  Category   `json:"category"`
	Operation string     `json:"operation"`
	Mean      float64    `json:"mean"`
	StdDev    float64    `json:"std_dev"`
	Min       float64    `json:"min"`
	Max       float64    `json:"max"`
	SampleSize int       `json:"sample_size"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// Thresholds for anomaly detection.
type Thresholds struct {
	LatencyP99Ms     float64 // Max acceptable P99 latency
	ErrorRatePercent float64 // Max acceptable error rate
	OutputSizeMB     float64 // Max acceptable output size
}

// DefaultThresholds provides sensible defaults.
var DefaultThresholds = Thresholds{
	LatencyP99Ms:     5000,  // 5 seconds
	ErrorRatePercent: 10,    // 10%
	OutputSizeMB:     100,   // 100MB
}

// AnomalyDetector finds anomalies in metrics.
type AnomalyDetector struct {
	baselines  map[string]*Baseline
	thresholds Thresholds
	collector  *MetricsCollector
}

// NewAnomalyDetector creates an anomaly detector.
func NewAnomalyDetector(collector *MetricsCollector, thresholds Thresholds) *AnomalyDetector {
	return &AnomalyDetector{
		baselines:  make(map[string]*Baseline),
		thresholds: thresholds,
		collector:  collector,
	}
}

// SetBaseline sets the expected baseline for a metric.
func (d *AnomalyDetector) SetBaseline(b *Baseline) {
	key := baselineKey(b.Type, b.Category, b.Operation)
	d.baselines[key] = b
}

// GetBaseline returns the baseline for a metric.
func (d *AnomalyDetector) GetBaseline(metricType MetricType, category Category, operation string) *Baseline {
	key := baselineKey(metricType, category, operation)
	return d.baselines[key]
}

// ComputeBaseline calculates baseline from current metrics.
func (d *AnomalyDetector) ComputeBaseline(metricType MetricType, category Category, operation string) *Baseline {
	stats := d.collector.GetStats(metricType, category, operation)
	if stats == nil || stats.Count < 10 {
		return nil // Not enough data
	}

	baseline := &Baseline{
		Type:       metricType,
		Category:   category,
		Operation:  operation,
		Mean:       stats.Mean,
		StdDev:     stats.StdDev,
		Min:        stats.Min,
		Max:        stats.Max,
		SampleSize: stats.Count,
		UpdatedAt:  time.Now(),
	}

	d.SetBaseline(baseline)
	return baseline
}

// Check analyzes an audit event for anomalies.
func (d *AnomalyDetector) Check(event *AuditEvent) []Anomaly {
	var anomalies []Anomaly

	// Check latency
	if event.DurationMs > 0 {
		if a := d.checkMetric(MetricLatency, event.Category, event.Operation,
			float64(event.DurationMs), event.SessionID); a != nil {
			anomalies = append(anomalies, *a)
		}
	}

	// Check output size
	if event.OutputSize > 0 {
		if a := d.checkMetric(MetricOutputSize, event.Category, event.Operation,
			float64(event.OutputSize), event.SessionID); a != nil {
			anomalies = append(anomalies, *a)
		}
	}

	// Check threshold breaches
	anomalies = append(anomalies, d.checkThresholds(event)...)

	return anomalies
}

// checkMetric checks a single metric against baseline.
func (d *AnomalyDetector) checkMetric(metricType MetricType, category Category, operation string,
	value float64, sessionID string) *Anomaly {

	baseline := d.GetBaseline(metricType, category, operation)
	if baseline == nil {
		// Try category-level baseline
		baseline = d.GetBaseline(metricType, category, "")
	}
	if baseline == nil || baseline.StdDev == 0 {
		return nil
	}

	zScore := (value - baseline.Mean) / baseline.StdDev
	level := zScoreToLevel(zScore)

	if level == AnomalyNone {
		return nil
	}

	return &Anomaly{
		ID:          generateAnomalyID(),
		Level:       level,
		Type:        metricType,
		Category:    category,
		Operation:   operation,
		Value:       value,
		Expected:    baseline.Mean,
		StdDev:      baseline.StdDev,
		ZScore:      zScore,
		Description: describeAnomaly(metricType, value, baseline.Mean, zScore),
		DetectedAt:  time.Now(),
		SessionID:   sessionID,
	}
}

// checkThresholds checks hard threshold breaches.
func (d *AnomalyDetector) checkThresholds(event *AuditEvent) []Anomaly {
	var anomalies []Anomaly

	// Latency threshold
	if float64(event.DurationMs) > d.thresholds.LatencyP99Ms {
		anomalies = append(anomalies, Anomaly{
			ID:          generateAnomalyID(),
			Level:       AnomalyCritical,
			Type:        MetricLatency,
			Category:    event.Category,
			Operation:   event.Operation,
			Value:       float64(event.DurationMs),
			Expected:    d.thresholds.LatencyP99Ms,
			Description: "Latency exceeded threshold",
			DetectedAt:  time.Now(),
			SessionID:   event.SessionID,
		})
	}

	// Output size threshold (convert to MB)
	outputMB := float64(event.OutputSize) / (1024 * 1024)
	if outputMB > d.thresholds.OutputSizeMB {
		anomalies = append(anomalies, Anomaly{
			ID:          generateAnomalyID(),
			Level:       AnomalyHigh,
			Type:        MetricOutputSize,
			Category:    event.Category,
			Operation:   event.Operation,
			Value:       outputMB,
			Expected:    d.thresholds.OutputSizeMB,
			Description: "Output size exceeded threshold",
			DetectedAt:  time.Now(),
			SessionID:   event.SessionID,
		})
	}

	return anomalies
}

// DetectAll scans all current metrics for anomalies.
func (d *AnomalyDetector) DetectAll() []Anomaly {
	var anomalies []Anomaly

	allStats := d.collector.GetAllStats()
	for _, stats := range allStats {
		baseline := d.GetBaseline(stats.Type, stats.Category, stats.Operation)
		if baseline == nil {
			continue
		}

		// Check if current mean deviates from baseline
		if baseline.StdDev > 0 {
			zScore := (stats.Mean - baseline.Mean) / baseline.StdDev
			level := zScoreToLevel(zScore)

			if level != AnomalyNone {
				anomalies = append(anomalies, Anomaly{
					ID:          generateAnomalyID(),
					Level:       level,
					Type:        stats.Type,
					Category:    stats.Category,
					Operation:   stats.Operation,
					Value:       stats.Mean,
					Expected:    baseline.Mean,
					StdDev:      baseline.StdDev,
					ZScore:      zScore,
					Description: describeAnomaly(stats.Type, stats.Mean, baseline.Mean, zScore),
					DetectedAt:  time.Now(),
				})
			}
		}

		// Check P99 spike
		if stats.P99 > baseline.Max*2 {
			anomalies = append(anomalies, Anomaly{
				ID:          generateAnomalyID(),
				Level:       AnomalyHigh,
				Type:        stats.Type,
				Category:    stats.Category,
				Operation:   stats.Operation,
				Value:       stats.P99,
				Expected:    baseline.Max,
				Description: "P99 significantly higher than historical max",
				DetectedAt:  time.Now(),
			})
		}
	}

	return anomalies
}

// Helper functions

func baselineKey(t MetricType, c Category, o string) string {
	return string(t) + ":" + string(c) + ":" + o
}

func zScoreToLevel(z float64) AnomalyLevel {
	absZ := math.Abs(z)
	switch {
	case absZ >= 4:
		return AnomalyCritical
	case absZ >= 3:
		return AnomalyHigh
	case absZ >= 2:
		return AnomalyMedium
	case absZ >= 1.5:
		return AnomalyLow
	default:
		return AnomalyNone
	}
}

func describeAnomaly(t MetricType, value, expected, zScore float64) string {
	direction := "higher"
	if zScore < 0 {
		direction = "lower"
	}

	switch t {
	case MetricLatency:
		return formatFloat(value) + "ms " + direction + " than expected " + formatFloat(expected) + "ms"
	case MetricErrorRate:
		return formatFloat(value*100) + "% error rate, " + direction + " than baseline"
	case MetricOutputSize:
		return formatFloat(value/1024) + "KB " + direction + " than expected"
	default:
		return formatFloat(value) + " " + direction + " than expected " + formatFloat(expected)
	}
}

func formatFloat(f float64) string {
	if f == float64(int64(f)) {
		return string(rune(int64(f) + '0'))
	}
	// Simple formatting
	s := ""
	i := int64(f)
	frac := int64((f - float64(i)) * 100)
	if frac < 0 {
		frac = -frac
	}
	s = intToStr(i) + "." + intToStr(frac)
	return s
}

func intToStr(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if neg {
		digits = append([]byte{'-'}, digits...)
	}
	return string(digits)
}

var anomalyCounter int64

func generateAnomalyID() string {
	anomalyCounter++
	return "anom-" + intToStr(anomalyCounter)
}

// AnomalyStore persists anomalies to Memgraph.
type AnomalyStore struct {
	db        graph.Driver
	sessionID string
}

// NewAnomalyStore creates an anomaly store.
func NewAnomalyStore(db graph.Driver, sessionID string) *AnomalyStore {
	return &AnomalyStore{db: db, sessionID: sessionID}
}

// Save persists an anomaly.
func (s *AnomalyStore) Save(ctx context.Context, a *Anomaly) error {
	query := `
		MERGE (sess:Session {session_id: $session_id})
		CREATE (anom:Anomaly {
			anomaly_id: $anomaly_id,
			level: $level,
			type: $type,
			category: $category,
			operation: $operation,
			value: $value,
			expected: $expected,
			std_dev: $std_dev,
			z_score: $z_score,
			description: $description,
			detected_at: $detected_at
		})
		CREATE (sess)-[:DETECTED]->(anom)
	`

	return s.db.ExecuteWrite(ctx, query, map[string]any{
		"session_id":  s.sessionID,
		"anomaly_id":  a.ID,
		"level":       string(a.Level),
		"type":        string(a.Type),
		"category":    string(a.Category),
		"operation":   a.Operation,
		"value":       a.Value,
		"expected":    a.Expected,
		"std_dev":     a.StdDev,
		"z_score":     a.ZScore,
		"description": a.Description,
		"detected_at": a.DetectedAt.UTC().Format(time.RFC3339),
	})
}

// GetRecent retrieves recent anomalies.
func (s *AnomalyStore) GetRecent(ctx context.Context, limit int) ([]Anomaly, error) {
	if limit <= 0 {
		limit = 50
	}

	query := `
		MATCH (sess:Session {session_id: $session_id})-[:DETECTED]->(a:Anomaly)
		RETURN a.anomaly_id as anomaly_id,
		       a.level as level,
		       a.type as type,
		       a.category as category,
		       a.operation as operation,
		       a.value as value,
		       a.expected as expected,
		       a.std_dev as std_dev,
		       a.z_score as z_score,
		       a.description as description,
		       a.detected_at as detected_at
		ORDER BY a.detected_at DESC
		LIMIT $limit
	`

	records, err := s.db.Execute(ctx, query, map[string]any{
		"session_id": s.sessionID,
		"limit":      limit,
	})
	if err != nil {
		return nil, err
	}

	var anomalies []Anomaly
	for _, r := range records {
		a := Anomaly{
			ID:          getString(r, "anomaly_id"),
			Level:       AnomalyLevel(getString(r, "level")),
			Type:        MetricType(getString(r, "type")),
			Category:    Category(getString(r, "category")),
			Operation:   getString(r, "operation"),
			Value:       getFloat(r, "value"),
			Expected:    getFloat(r, "expected"),
			StdDev:      getFloat(r, "std_dev"),
			ZScore:      getFloat(r, "z_score"),
			Description: getString(r, "description"),
		}

		if detected := getString(r, "detected_at"); detected != "" {
			if t, err := time.Parse(time.RFC3339, detected); err == nil {
				a.DetectedAt = t
			}
		}

		anomalies = append(anomalies, a)
	}

	return anomalies, nil
}

// GetByLevel retrieves anomalies of a specific level.
func (s *AnomalyStore) GetByLevel(ctx context.Context, level AnomalyLevel, limit int) ([]Anomaly, error) {
	if limit <= 0 {
		limit = 50
	}

	query := `
		MATCH (sess:Session {session_id: $session_id})-[:DETECTED]->(a:Anomaly)
		WHERE a.level = $level
		RETURN a.anomaly_id as anomaly_id,
		       a.level as level,
		       a.type as type,
		       a.category as category,
		       a.operation as operation,
		       a.value as value,
		       a.expected as expected,
		       a.description as description,
		       a.detected_at as detected_at
		ORDER BY a.detected_at DESC
		LIMIT $limit
	`

	records, err := s.db.Execute(ctx, query, map[string]any{
		"session_id": s.sessionID,
		"level":      string(level),
		"limit":      limit,
	})
	if err != nil {
		return nil, err
	}

	var anomalies []Anomaly
	for _, r := range records {
		a := Anomaly{
			ID:          getString(r, "anomaly_id"),
			Level:       AnomalyLevel(getString(r, "level")),
			Type:        MetricType(getString(r, "type")),
			Category:    Category(getString(r, "category")),
			Operation:   getString(r, "operation"),
			Value:       getFloat(r, "value"),
			Expected:    getFloat(r, "expected"),
			Description: getString(r, "description"),
		}

		if detected := getString(r, "detected_at"); detected != "" {
			if t, err := time.Parse(time.RFC3339, detected); err == nil {
				a.DetectedAt = t
			}
		}

		anomalies = append(anomalies, a)
	}

	return anomalies, nil
}
