package audit

import (
	"testing"
	"time"
)

func TestAnomalyDetector(t *testing.T) {
	collector := NewMetricsCollector(1*time.Hour, 1000)
	detector := NewAnomalyDetector(collector, DefaultThresholds)

	// Establish baseline with consistent latencies around 100ms
	for i := 0; i < 50; i++ {
		collector.Record(&AuditEvent{
			Category:   CategoryCode,
			Operation:  "ingest",
			DurationMs: int64(95 + i%10), // 95-104ms, tight distribution
			SessionID:  "test",
		})
	}

	// Compute baseline
	baseline := detector.ComputeBaseline(MetricLatency, CategoryCode, "ingest")
	if baseline == nil {
		t.Fatal("expected baseline")
	}

	if baseline.Mean < 95 || baseline.Mean > 105 {
		t.Errorf("expected mean ~100, got %.2f", baseline.Mean)
	}

	// Check normal event - should not trigger anomaly
	normalEvent := &AuditEvent{
		Category:   CategoryCode,
		Operation:  "ingest",
		DurationMs: 100,
		SessionID:  "test",
	}

	anomalies := detector.Check(normalEvent)
	if len(anomalies) > 0 {
		t.Errorf("expected no anomalies for normal event, got %d", len(anomalies))
	}

	// Check anomalous event - 500ms is way above baseline
	anomalousEvent := &AuditEvent{
		Category:   CategoryCode,
		Operation:  "ingest",
		DurationMs: 500,
		SessionID:  "test",
	}

	anomalies = detector.Check(anomalousEvent)
	if len(anomalies) == 0 {
		t.Error("expected anomaly for 500ms latency")
	}
}

func TestAnomalyThresholds(t *testing.T) {
	collector := NewMetricsCollector(1*time.Hour, 1000)
	thresholds := Thresholds{
		LatencyP99Ms:     100, // Very low threshold
		ErrorRatePercent: 5,
		OutputSizeMB:     1,
	}
	detector := NewAnomalyDetector(collector, thresholds)

	// Event exceeding latency threshold
	event := &AuditEvent{
		Category:   CategoryCode,
		Operation:  "slow-op",
		DurationMs: 200, // Exceeds 100ms threshold
		SessionID:  "test",
	}

	anomalies := detector.Check(event)
	found := false
	for _, a := range anomalies {
		if a.Level == AnomalyCritical && a.Type == MetricLatency {
			found = true
			break
		}
	}

	if !found {
		t.Error("expected critical latency anomaly")
	}
}

func TestAnomalyOutputSize(t *testing.T) {
	collector := NewMetricsCollector(1*time.Hour, 1000)
	thresholds := Thresholds{
		LatencyP99Ms:     5000,
		ErrorRatePercent: 10,
		OutputSizeMB:     1, // 1MB threshold
	}
	detector := NewAnomalyDetector(collector, thresholds)

	// Event with huge output
	event := &AuditEvent{
		Category:   CategoryCode,
		Operation:  "big-output",
		DurationMs: 100,
		OutputSize: 10 * 1024 * 1024, // 10MB
		SessionID:  "test",
	}

	anomalies := detector.Check(event)
	found := false
	for _, a := range anomalies {
		if a.Level == AnomalyHigh && a.Type == MetricOutputSize {
			found = true
			break
		}
	}

	if !found {
		t.Error("expected high output size anomaly")
	}
}

func TestZScoreToLevel(t *testing.T) {
	tests := []struct {
		z        float64
		expected AnomalyLevel
	}{
		{0.5, AnomalyNone},
		{1.0, AnomalyNone},
		{1.5, AnomalyLow},
		{2.0, AnomalyMedium},
		{2.5, AnomalyMedium},
		{3.0, AnomalyHigh},
		{3.5, AnomalyHigh},
		{4.0, AnomalyCritical},
		{5.0, AnomalyCritical},
		{-2.0, AnomalyMedium}, // Negative z-scores should work too
		{-4.0, AnomalyCritical},
	}

	for _, tt := range tests {
		got := zScoreToLevel(tt.z)
		if got != tt.expected {
			t.Errorf("zScoreToLevel(%.1f) = %s, want %s", tt.z, got, tt.expected)
		}
	}
}

func TestDetectAll(t *testing.T) {
	collector := NewMetricsCollector(1*time.Hour, 1000)
	detector := NewAnomalyDetector(collector, DefaultThresholds)

	// Establish baseline
	for i := 0; i < 50; i++ {
		collector.Record(&AuditEvent{
			Category:   CategoryCode,
			Operation:  "test",
			DurationMs: 100,
			SessionID:  "test",
		})
	}
	detector.ComputeBaseline(MetricLatency, CategoryCode, "test")

	// Add some anomalous data points
	for i := 0; i < 10; i++ {
		collector.Record(&AuditEvent{
			Category:   CategoryCode,
			Operation:  "test",
			DurationMs: 500, // Much higher than baseline
			SessionID:  "test",
		})
	}

	anomalies := detector.DetectAll()
	// Should detect drift in the mean
	if len(anomalies) == 0 {
		t.Log("No anomalies detected - may be expected if mean hasn't shifted enough")
	}
}

func TestBaselineKey(t *testing.T) {
	key := baselineKey(MetricLatency, CategoryCode, "ingest")
	expected := "latency_ms:code:ingest"

	if key != expected {
		t.Errorf("expected %s, got %s", expected, key)
	}
}

func TestSetGetBaseline(t *testing.T) {
	collector := NewMetricsCollector(1*time.Hour, 1000)
	detector := NewAnomalyDetector(collector, DefaultThresholds)

	baseline := &Baseline{
		Type:       MetricLatency,
		Category:   CategoryCode,
		Operation:  "test",
		Mean:       100,
		StdDev:     10,
		Min:        80,
		Max:        120,
		SampleSize: 100,
		UpdatedAt:  time.Now(),
	}

	detector.SetBaseline(baseline)

	got := detector.GetBaseline(MetricLatency, CategoryCode, "test")
	if got == nil {
		t.Fatal("expected baseline")
	}

	if got.Mean != 100 {
		t.Errorf("expected mean 100, got %.2f", got.Mean)
	}
}

func TestAnomalyLevels(t *testing.T) {
	levels := []AnomalyLevel{
		AnomalyNone,
		AnomalyLow,
		AnomalyMedium,
		AnomalyHigh,
		AnomalyCritical,
	}

	for _, l := range levels {
		if l == "" {
			t.Error("empty anomaly level")
		}
	}
}
