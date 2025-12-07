package memory

import (
	"context"
	"testing"

	"github.com/joss/urp/internal/graph"
)

func TestDefaultAGRConfig(t *testing.T) {
	cfg := DefaultAGRConfig()

	if cfg.MaxHops != 3 {
		t.Errorf("Expected MaxHops=3, got %d", cfg.MaxHops)
	}
	if cfg.ConfidenceThreshold != 0.3 {
		t.Errorf("Expected ConfidenceThreshold=0.3, got %f", cfg.ConfidenceThreshold)
	}
	if cfg.ExpansionThreshold != 0.15 {
		t.Errorf("Expected ExpansionThreshold=0.15, got %f", cfg.ExpansionThreshold)
	}
	if cfg.Limit != 10 {
		t.Errorf("Expected Limit=10, got %d", cfg.Limit)
	}
}

func TestNewAdaptiveRetriever_Defaults(t *testing.T) {
	// Zero config should get defaults
	ar := NewAdaptiveRetriever(nil, AGRConfig{})

	if ar.config.MaxHops != 3 {
		t.Errorf("Expected default MaxHops=3, got %d", ar.config.MaxHops)
	}
	if ar.config.ConfidenceThreshold != 0.3 {
		t.Errorf("Expected default ConfidenceThreshold=0.3, got %f", ar.config.ConfidenceThreshold)
	}
}

func TestAGRSearch_EmptySeeds(t *testing.T) {
	ar := NewAdaptiveRetriever(nil, DefaultAGRConfig())

	results, err := ar.Search(context.Background(), "test query", nil)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if results != nil {
		t.Errorf("Expected nil results for empty seeds, got %v", results)
	}
}

func TestAGRSearch_EmptyQuery(t *testing.T) {
	ar := NewAdaptiveRetriever(nil, DefaultAGRConfig())

	results, err := ar.Search(context.Background(), "", []string{"seed1"})
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if results != nil {
		t.Errorf("Expected nil results for empty query, got %v", results)
	}
}

func TestJaccardFromSets(t *testing.T) {
	tests := []struct {
		name     string
		a        map[string]bool
		b        map[string]bool
		expected float64
	}{
		{
			name:     "empty sets",
			a:        map[string]bool{},
			b:        map[string]bool{},
			expected: 0,
		},
		{
			name:     "one empty",
			a:        map[string]bool{"a": true},
			b:        map[string]bool{},
			expected: 0,
		},
		{
			name:     "identical",
			a:        map[string]bool{"a": true, "b": true},
			b:        map[string]bool{"a": true, "b": true},
			expected: 1.0,
		},
		{
			name:     "no overlap",
			a:        map[string]bool{"a": true, "b": true},
			b:        map[string]bool{"c": true, "d": true},
			expected: 0,
		},
		{
			name:     "partial overlap",
			a:        map[string]bool{"a": true, "b": true, "c": true},
			b:        map[string]bool{"b": true, "c": true, "d": true},
			expected: 0.5, // intersection=2, union=4
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := jaccardFromSets(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("Expected %f, got %f", tt.expected, result)
			}
		})
	}
}

func TestSortAGRResults(t *testing.T) {
	results := []AGRResult{
		{Entry: KnowledgeEntry{KnowledgeID: "k1"}, Score: 0.3, HopCount: 2},
		{Entry: KnowledgeEntry{KnowledgeID: "k2"}, Score: 0.8, HopCount: 1},
		{Entry: KnowledgeEntry{KnowledgeID: "k3"}, Score: 0.5, HopCount: 0},
		{Entry: KnowledgeEntry{KnowledgeID: "k4"}, Score: 0.8, HopCount: 0}, // same score as k2, fewer hops
	}

	sortAGRResults(results)

	// Should be sorted by score desc, then hop count asc
	if results[0].Entry.KnowledgeID != "k4" {
		t.Errorf("Expected k4 first (score=0.8, hops=0), got %s", results[0].Entry.KnowledgeID)
	}
	if results[1].Entry.KnowledgeID != "k2" {
		t.Errorf("Expected k2 second (score=0.8, hops=1), got %s", results[1].Entry.KnowledgeID)
	}
	if results[2].Entry.KnowledgeID != "k3" {
		t.Errorf("Expected k3 third (score=0.5), got %s", results[2].Entry.KnowledgeID)
	}
	if results[3].Entry.KnowledgeID != "k1" {
		t.Errorf("Expected k1 last (score=0.3), got %s", results[3].Entry.KnowledgeID)
	}
}

func TestAGRResult_Path(t *testing.T) {
	result := AGRResult{
		Entry:    KnowledgeEntry{KnowledgeID: "k1"},
		Score:    0.5,
		HopCount: 2,
		Path:     []string{"seed", "middle", "k1"},
	}

	if len(result.Path) != 3 {
		t.Errorf("Expected path length 3, got %d", len(result.Path))
	}
	if result.Path[0] != "seed" {
		t.Errorf("Expected path[0]='seed', got %s", result.Path[0])
	}
}

// MockGraphDriver for integration tests
type mockAGRDriver struct {
	nodes     map[string]*KnowledgeEntry
	neighbors map[string][]string
}

func (m *mockAGRDriver) Execute(ctx context.Context, query string, params map[string]any) ([]graph.Record, error) {
	// Simulate fetchKnowledge
	if id, ok := params["id"].(string); ok {
		if entry, exists := m.nodes[id]; exists {
			return []graph.Record{{
				"knowledge_id":      entry.KnowledgeID,
				"kind":              entry.Kind,
				"scope":             entry.Scope,
				"text":              entry.Text,
				"context_signature": entry.ContextSignature,
				"created_at":        entry.CreatedAt,
			}}, nil
		}
	}

	// Simulate findSeeds - return all nodes
	if _, ok := params["limit"]; ok {
		var results []graph.Record
		for id, entry := range m.nodes {
			results = append(results, graph.Record{
				"id":   id,
				"text": entry.Text,
			})
		}
		return results, nil
	}

	return nil, nil
}

func (m *mockAGRDriver) ExecuteWrite(ctx context.Context, query string, params map[string]any) error {
	return nil
}

func (m *mockAGRDriver) Close() error {
	return nil
}

func (m *mockAGRDriver) Ping(ctx context.Context) error {
	return nil
}

func TestAGRSearch_WithMockDB(t *testing.T) {
	mock := &mockAGRDriver{
		nodes: map[string]*KnowledgeEntry{
			"k1": {KnowledgeID: "k1", Text: "error handling in golang"},
			"k2": {KnowledgeID: "k2", Text: "golang error patterns best practices"},
			"k3": {KnowledgeID: "k3", Text: "python async programming"},
		},
		neighbors: map[string][]string{
			"k1": {"k2"},
			"k2": {"k1"},
		},
	}

	ar := NewAdaptiveRetriever(mock, AGRConfig{
		MaxHops:             2,
		ConfidenceThreshold: 0.1,
		ExpansionThreshold:  0.05,
		Limit:               10,
	})

	results, err := ar.Search(context.Background(), "golang error handling", []string{"k1"})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) == 0 {
		t.Error("Expected at least one result")
	}

	// First result should be k1 (exact match)
	if len(results) > 0 && results[0].Entry.KnowledgeID != "k1" {
		t.Errorf("Expected k1 as first result, got %s", results[0].Entry.KnowledgeID)
	}
}

func BenchmarkJaccardFromSets(b *testing.B) {
	a := map[string]bool{"error": true, "handling": true, "golang": true, "best": true, "practices": true}
	c := map[string]bool{"error": true, "handling": true, "python": true, "async": true, "await": true}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		jaccardFromSets(a, c)
	}
}

func BenchmarkSortAGRResults(b *testing.B) {
	results := make([]AGRResult, 100)
	for i := range results {
		results[i] = AGRResult{
			Entry:    KnowledgeEntry{KnowledgeID: "k" + string(rune(i))},
			Score:    float64(i%10) / 10.0,
			HopCount: i % 5,
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Copy to avoid sorting already sorted
		cp := make([]AGRResult, len(results))
		copy(cp, results)
		sortAGRResults(cp)
	}
}
