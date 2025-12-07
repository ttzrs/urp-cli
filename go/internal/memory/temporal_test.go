package memory

import (
	"testing"
	"time"
)

func TestNewTemporalTracker(t *testing.T) {
	tracker := NewTemporalTracker(nil, 0)
	if tracker.windowSize != 1000 {
		t.Errorf("Expected default window size 1000, got %d", tracker.windowSize)
	}

	tracker = NewTemporalTracker(nil, 500)
	if tracker.windowSize != 500 {
		t.Errorf("Expected window size 500, got %d", tracker.windowSize)
	}
}

func TestRecordAccess(t *testing.T) {
	tracker := NewTemporalTracker(nil, 100)

	tracker.RecordAccess("k1", "test query", "session1")
	tracker.RecordAccess("k1", "another query", "session1")
	tracker.RecordAccess("k2", "different", "session1")

	pattern := tracker.GetPattern("k1")
	if pattern == nil {
		t.Fatal("Expected pattern for k1")
	}
	if pattern.AccessCount != 2 {
		t.Errorf("Expected access count 2, got %d", pattern.AccessCount)
	}

	pattern = tracker.GetPattern("k2")
	if pattern.AccessCount != 1 {
		t.Errorf("Expected access count 1, got %d", pattern.AccessCount)
	}
}

func TestHourlyHeat(t *testing.T) {
	tracker := NewTemporalTracker(nil, 100)

	// Record multiple accesses
	for i := 0; i < 5; i++ {
		tracker.RecordAccess("k1", "query", "session")
	}

	pattern := tracker.GetPattern("k1")
	currentHour := time.Now().Hour()

	if pattern.HourlyHeat[currentHour] != 5 {
		t.Errorf("Expected 5 accesses at hour %d, got %d", currentHour, pattern.HourlyHeat[currentHour])
	}
}

func TestWeekdayHeat(t *testing.T) {
	tracker := NewTemporalTracker(nil, 100)

	tracker.RecordAccess("k1", "query", "session")
	tracker.RecordAccess("k1", "query", "session")

	pattern := tracker.GetPattern("k1")
	currentDay := int(time.Now().Weekday())

	if pattern.WeekdayHeat[currentDay] != 2 {
		t.Errorf("Expected 2 accesses on weekday %d, got %d", currentDay, pattern.WeekdayHeat[currentDay])
	}
}

func TestGetHotKnowledge(t *testing.T) {
	tracker := NewTemporalTracker(nil, 100)

	// Manually set patterns with different velocities
	tracker.mu.Lock()
	tracker.patterns["k1"] = &TemporalPattern{
		KnowledgeID: "k1",
		AccessCount: 100,
		LastAccess:  time.Now(),
		Velocity:    10.0, // Highest
	}
	tracker.patterns["k2"] = &TemporalPattern{
		KnowledgeID: "k2",
		AccessCount: 50,
		LastAccess:  time.Now(),
		Velocity:    5.0,
	}
	tracker.patterns["k3"] = &TemporalPattern{
		KnowledgeID: "k3",
		AccessCount: 10,
		LastAccess:  time.Now(),
		Velocity:    1.0,
	}
	tracker.mu.Unlock()

	hot := tracker.GetHotKnowledge(3)

	if len(hot) != 3 {
		t.Errorf("Expected 3 hot items, got %d", len(hot))
	}

	// k1 should be first (highest velocity)
	if hot[0] != "k1" {
		t.Errorf("Expected k1 to be hottest, got %s", hot[0])
	}
	// k2 should be second
	if hot[1] != "k2" {
		t.Errorf("Expected k2 to be second, got %s", hot[1])
	}
}

func TestGetDecayedKnowledge(t *testing.T) {
	tracker := NewTemporalTracker(nil, 100)

	// Create a pattern that looks old
	tracker.mu.Lock()
	tracker.patterns["old1"] = &TemporalPattern{
		KnowledgeID: "old1",
		LastAccess:  time.Now().Add(-48 * time.Hour),
		AccessCount: 5,
	}
	tracker.patterns["old2"] = &TemporalPattern{
		KnowledgeID: "old2",
		LastAccess:  time.Now().Add(-72 * time.Hour),
		AccessCount: 3,
	}
	tracker.mu.Unlock()

	// Record fresh access
	tracker.RecordAccess("fresh", "query", "session")

	decayed := tracker.GetDecayedKnowledge(24*time.Hour, 10)

	// Should have old1 and old2
	if len(decayed) != 2 {
		t.Errorf("Expected 2 decayed items, got %d", len(decayed))
	}

	// Fresh should not be in decayed list
	for _, id := range decayed {
		if id == "fresh" {
			t.Error("Fresh item should not be in decayed list")
		}
	}
}

func TestPredictNextAccess(t *testing.T) {
	tracker := NewTemporalTracker(nil, 100)

	// Need at least 3 accesses for prediction
	tracker.RecordAccess("k1", "query", "session")
	pred := tracker.PredictNextAccess("k1")
	if pred != nil {
		t.Error("Should not predict with only 1 access")
	}

	tracker.RecordAccess("k1", "query", "session")
	pred = tracker.PredictNextAccess("k1")
	if pred != nil {
		t.Error("Should not predict with only 2 accesses")
	}

	tracker.RecordAccess("k1", "query", "session")
	pred = tracker.PredictNextAccess("k1")
	if pred == nil {
		t.Error("Should predict with 3+ accesses")
	}
}

func TestGetPeakHours(t *testing.T) {
	tracker := NewTemporalTracker(nil, 100)

	// Simulate accesses at different hours manually
	tracker.mu.Lock()
	tracker.patterns["k1"] = &TemporalPattern{
		KnowledgeID: "k1",
		HourlyHeat:  [24]int{0, 0, 0, 0, 0, 0, 0, 0, 0, 10, 5, 2, 0, 0, 3, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		// Hour 9 = 10, Hour 10 = 5, Hour 14 = 3, Hour 11 = 2
	}
	tracker.mu.Unlock()

	peaks := tracker.GetPeakHours("k1")

	if len(peaks) != 3 {
		t.Errorf("Expected 3 peak hours, got %d", len(peaks))
	}

	// First peak should be hour 9 (highest count)
	if peaks[0] != 9 {
		t.Errorf("Expected hour 9 as top peak, got %d", peaks[0])
	}
}

func TestCoAccess(t *testing.T) {
	tracker := NewTemporalTracker(nil, 100)

	// Simulate items accessed together
	tracker.RecordAccess("k1", "query", "session")
	tracker.RecordAccess("k2", "query", "session") // k1 and k2 co-accessed
	tracker.RecordAccess("k1", "query", "session") // k1 again
	tracker.RecordAccess("k2", "query", "session") // k2 again
	tracker.RecordAccess("k1", "query", "session") // k1 again

	pattern := tracker.GetPattern("k1")

	// k2 should be in co-accessed list since they were accessed together multiple times
	found := false
	for _, id := range pattern.CoAccessedIDs {
		if id == "k2" {
			found = true
			break
		}
	}

	if !found {
		t.Error("Expected k2 in co-accessed list for k1")
	}
}

func TestStats(t *testing.T) {
	tracker := NewTemporalTracker(nil, 100)

	tracker.RecordAccess("k1", "query", "session")
	tracker.RecordAccess("k1", "query", "session")
	tracker.RecordAccess("k2", "query", "session")

	stats := tracker.Stats()

	if stats["tracked_items"].(int) != 2 {
		t.Errorf("Expected 2 tracked items, got %v", stats["tracked_items"])
	}

	if stats["total_accesses"].(int) != 3 {
		t.Errorf("Expected 3 total accesses, got %v", stats["total_accesses"])
	}

	if stats["window_used"].(int) != 3 {
		t.Errorf("Expected window_used 3, got %v", stats["window_used"])
	}
}

func TestVelocity(t *testing.T) {
	tracker := NewTemporalTracker(nil, 100)

	// Record accesses
	for i := 0; i < 10; i++ {
		tracker.RecordAccess("k1", "query", "session")
	}

	pattern := tracker.GetPattern("k1")

	// Velocity should be positive (accesses/hour)
	if pattern.Velocity <= 0 {
		t.Errorf("Expected positive velocity, got %f", pattern.Velocity)
	}
}

func TestWindowEviction(t *testing.T) {
	tracker := NewTemporalTracker(nil, 3) // Very small window

	tracker.RecordAccess("k1", "query", "session")
	tracker.RecordAccess("k2", "query", "session")
	tracker.RecordAccess("k3", "query", "session")
	tracker.RecordAccess("k4", "query", "session") // Should evict k1's event

	stats := tracker.Stats()
	if stats["window_used"].(int) != 3 {
		t.Errorf("Expected window to be limited to 3, got %v", stats["window_used"])
	}
}

func BenchmarkRecordAccess(b *testing.B) {
	tracker := NewTemporalTracker(nil, 10000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tracker.RecordAccess("k1", "benchmark query", "session")
	}
}

func BenchmarkGetHotKnowledge(b *testing.B) {
	tracker := NewTemporalTracker(nil, 10000)

	// Populate with data
	for i := 0; i < 1000; i++ {
		id := "k" + string(rune(i%100))
		tracker.RecordAccess(id, "query", "session")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tracker.GetHotKnowledge(10)
	}
}
