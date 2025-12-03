package audit

import (
	"testing"
	"time"
)

func TestMetricsCollector(t *testing.T) {
	collector := NewMetricsCollector(1*time.Hour, 1000)

	// Record some events
	for i := 0; i < 20; i++ {
		event := &AuditEvent{
			Category:   CategoryCode,
			Operation:  "ingest",
			DurationMs: int64(100 + i*10), // 100-290ms
			OutputSize: 1024 * (i + 1),
			SessionID:  "test-sess",
		}
		collector.Record(event)
	}

	// Get stats
	stats := collector.GetStats(MetricLatency, CategoryCode, "ingest")
	if stats == nil {
		t.Fatal("expected stats, got nil")
	}

	if stats.Count != 20 {
		t.Errorf("expected 20 samples, got %d", stats.Count)
	}

	if stats.Min != 100 {
		t.Errorf("expected min 100, got %.0f", stats.Min)
	}

	if stats.Max != 290 {
		t.Errorf("expected max 290, got %.0f", stats.Max)
	}

	// Mean should be around 195
	if stats.Mean < 190 || stats.Mean > 200 {
		t.Errorf("expected mean ~195, got %.0f", stats.Mean)
	}

	if stats.StdDev <= 0 {
		t.Error("expected positive std dev")
	}
}

func TestMetricsCollectorAllStats(t *testing.T) {
	collector := NewMetricsCollector(1*time.Hour, 1000)

	// Record events for different categories
	categories := []Category{CategoryCode, CategoryGit, CategorySystem}
	for _, cat := range categories {
		for i := 0; i < 5; i++ {
			collector.Record(&AuditEvent{
				Category:   cat,
				Operation:  "test-op",
				DurationMs: int64(100),
				SessionID:  "test",
			})
		}
	}

	allStats := collector.GetAllStats()
	if len(allStats) < 3 {
		t.Errorf("expected at least 3 stat groups, got %d", len(allStats))
	}
}

func TestMetricsCollectorPrune(t *testing.T) {
	// Small window for testing
	collector := NewMetricsCollector(1*time.Millisecond, 100)

	// Record event
	collector.Record(&AuditEvent{
		Category:   CategoryCode,
		Operation:  "test",
		DurationMs: 100,
		SessionID:  "test",
	})

	// Wait for window to expire
	time.Sleep(5 * time.Millisecond)

	// Record another to trigger prune
	collector.Record(&AuditEvent{
		Category:   CategoryCode,
		Operation:  "test",
		DurationMs: 200,
		SessionID:  "test",
	})

	stats := collector.GetStats(MetricLatency, CategoryCode, "test")
	if stats == nil {
		t.Fatal("expected stats")
	}

	// Should only have the recent event
	if stats.Count != 1 {
		t.Errorf("expected 1 after prune, got %d", stats.Count)
	}
}

func TestMetricsCollectorMaxSize(t *testing.T) {
	collector := NewMetricsCollector(1*time.Hour, 10) // Max 10

	// Record more than max
	for i := 0; i < 20; i++ {
		collector.Record(&AuditEvent{
			Category:   CategoryCode,
			Operation:  "test",
			DurationMs: int64(i),
			SessionID:  "test",
		})
	}

	stats := collector.GetStats(MetricLatency, CategoryCode, "test")
	if stats == nil {
		t.Fatal("expected stats")
	}

	if stats.Count > 10 {
		t.Errorf("expected max 10 samples, got %d", stats.Count)
	}
}

func TestPercentile(t *testing.T) {
	values := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}

	p50 := percentile(values, 0.50)
	// Index-based: 0.50 * 9 = 4.5 → index 4 → value 5
	if p50 != 5 {
		t.Errorf("expected p50=5, got %.0f", p50)
	}

	p90 := percentile(values, 0.90)
	// Index-based: 0.90 * 9 = 8.1 → index 8 → value 9
	if p90 != 9 {
		t.Errorf("expected p90=9, got %.0f", p90)
	}

	// For p99 with 10 elements: 0.99 * 9 = 8.91 → index 8 → value 9
	p99 := percentile(values, 0.99)
	if p99 != 9 {
		t.Errorf("expected p99=9, got %.0f", p99)
	}

	// Test with more elements for actual p99
	moreValues := make([]float64, 100)
	for i := range moreValues {
		moreValues[i] = float64(i + 1)
	}
	p99Large := percentile(moreValues, 0.99)
	// 0.99 * 99 = 98.01 → index 98 → value 99
	if p99Large != 99 {
		t.Errorf("expected p99=99 for 100 elements, got %.0f", p99Large)
	}
}

func TestSortFloat64(t *testing.T) {
	values := []float64{5, 2, 8, 1, 9, 3}
	sortFloat64(values)

	expected := []float64{1, 2, 3, 5, 8, 9}
	for i, v := range values {
		if v != expected[i] {
			t.Errorf("at %d: expected %.0f, got %.0f", i, expected[i], v)
		}
	}
}
