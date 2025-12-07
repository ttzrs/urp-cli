package memory

import (
	"testing"
	"time"
)

func TestQueryCache_GetSet(t *testing.T) {
	cache := NewQueryCache(10, time.Minute)

	entries := []KnowledgeEntry{
		{KnowledgeID: "k-1", Text: "test1"},
		{KnowledgeID: "k-2", Text: "test2"},
	}

	// Should miss on first get
	_, ok := cache.Get("query", "all", "", 10)
	if ok {
		t.Error("Expected cache miss on empty cache")
	}

	// Set and get
	cache.Set("query", "all", "", 10, entries)
	result, ok := cache.Get("query", "all", "", 10)
	if !ok {
		t.Error("Expected cache hit after set")
	}
	if len(result) != 2 {
		t.Errorf("Expected 2 entries, got %d", len(result))
	}

	// Different params should miss
	_, ok = cache.Get("query", "session", "", 10)
	if ok {
		t.Error("Expected cache miss for different params")
	}
}

func TestQueryCache_TTL(t *testing.T) {
	cache := NewQueryCache(10, 50*time.Millisecond)

	entries := []KnowledgeEntry{{KnowledgeID: "k-1"}}
	cache.Set("query", "all", "", 10, entries)

	// Should hit immediately
	_, ok := cache.Get("query", "all", "", 10)
	if !ok {
		t.Error("Expected cache hit")
	}

	// Wait for TTL
	time.Sleep(60 * time.Millisecond)

	// Should miss after TTL
	_, ok = cache.Get("query", "all", "", 10)
	if ok {
		t.Error("Expected cache miss after TTL")
	}
}

func TestQueryCache_LRU(t *testing.T) {
	cache := NewQueryCache(3, time.Minute)

	// Fill cache
	cache.Set("q1", "all", "", 10, []KnowledgeEntry{{KnowledgeID: "k-1"}})
	cache.Set("q2", "all", "", 10, []KnowledgeEntry{{KnowledgeID: "k-2"}})
	cache.Set("q3", "all", "", 10, []KnowledgeEntry{{KnowledgeID: "k-3"}})

	// Access q1 to make it recently used
	cache.Get("q1", "all", "", 10)

	// Add q4, should evict q2 (oldest)
	cache.Set("q4", "all", "", 10, []KnowledgeEntry{{KnowledgeID: "k-4"}})

	// q2 should be evicted
	_, ok := cache.Get("q2", "all", "", 10)
	if ok {
		t.Error("Expected q2 to be evicted")
	}

	// q1 should still be present
	_, ok = cache.Get("q1", "all", "", 10)
	if !ok {
		t.Error("Expected q1 to still be present")
	}
}

func TestQueryCache_Stats(t *testing.T) {
	cache := NewQueryCache(10, time.Minute)

	cache.Set("q1", "all", "", 10, []KnowledgeEntry{{KnowledgeID: "k-1"}})

	// 2 hits
	cache.Get("q1", "all", "", 10)
	cache.Get("q1", "all", "", 10)

	// 1 miss
	cache.Get("q2", "all", "", 10)

	stats := cache.Stats()
	if stats.Hits != 2 {
		t.Errorf("Expected 2 hits, got %d", stats.Hits)
	}
	if stats.Misses != 1 {
		t.Errorf("Expected 1 miss, got %d", stats.Misses)
	}
	if stats.Size != 1 {
		t.Errorf("Expected size 1, got %d", stats.Size)
	}

	// Hit rate should be 2/3 = 0.666...
	if stats.HitRate < 0.65 || stats.HitRate > 0.68 {
		t.Errorf("Expected hit rate ~0.67, got %f", stats.HitRate)
	}
}

func TestQueryCache_Invalidate(t *testing.T) {
	cache := NewQueryCache(10, time.Minute)

	cache.Set("q1", "all", "", 10, []KnowledgeEntry{{KnowledgeID: "k-1"}})
	cache.Set("q2", "all", "", 10, []KnowledgeEntry{{KnowledgeID: "k-2"}})

	cache.InvalidateAll()

	_, ok := cache.Get("q1", "all", "", 10)
	if ok {
		t.Error("Expected cache miss after invalidate")
	}

	stats := cache.Stats()
	if stats.Size != 0 {
		t.Errorf("Expected size 0 after invalidate, got %d", stats.Size)
	}
}

func TestQueryCache_CopiesResults(t *testing.T) {
	cache := NewQueryCache(10, time.Minute)

	original := []KnowledgeEntry{{KnowledgeID: "k-1", Text: "original"}}
	cache.Set("q1", "all", "", 10, original)

	// Modify original
	original[0].Text = "modified"

	// Get should return copy with original value
	result, _ := cache.Get("q1", "all", "", 10)
	if result[0].Text != "original" {
		t.Error("Cache should return copy, not reference")
	}
}

func BenchmarkQueryCache_Get(b *testing.B) {
	cache := NewQueryCache(1000, time.Minute)

	entries := []KnowledgeEntry{
		{KnowledgeID: "k-1", Text: "test knowledge entry"},
	}
	cache.Set("benchmark query text", "all", "fix", 10, entries)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Get("benchmark query text", "all", "fix", 10)
	}
}

func BenchmarkQueryCache_Set(b *testing.B) {
	cache := NewQueryCache(1000, time.Minute)

	entries := []KnowledgeEntry{
		{KnowledgeID: "k-1", Text: "test knowledge entry"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Set("query", "all", "fix", 10, entries)
	}
}
