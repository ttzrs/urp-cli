package vector

import (
	"context"
	"encoding/json"
	"time"

	"github.com/joss/urp/internal/graph"
)

// MemgraphStore implements Store using Memgraph database.
type MemgraphStore struct {
	db graph.Driver
}

// NewMemgraphStore creates a new Memgraph vector store.
func NewMemgraphStore(db graph.Driver) *MemgraphStore {
	return &MemgraphStore{
		db: db,
	}
}

// Add stores a vector entry.
func (s *MemgraphStore) Add(ctx context.Context, entry VectorEntry) error {
	if entry.ID == "" {
		entry.ID = generateID(entry.Text)
	}
	if entry.CreatedAt == 0 {
		entry.CreatedAt = time.Now().Unix()
	}

	// Serialize metadata
	metaJSON, err := json.Marshal(entry.Metadata)
	if err != nil {
		metaJSON = []byte("{}")
	}

	query := `
		MERGE (v:VectorEntry {id: $id})
		SET v.text = $text,
			v.kind = $kind,
			v.metadata = $metadata,
			v.created_at = $created_at,
			v.vector = $vector
	`

	params := map[string]any{
		"id":         entry.ID,
		"text":       entry.Text,
		"kind":       entry.Kind,
		"metadata":   string(metaJSON),
		"created_at": entry.CreatedAt,
		"vector":     entry.Vector,
	}

	return s.db.ExecuteWrite(ctx, query, params)
}

// Search finds similar vectors using cosine similarity in Cypher.
func (s *MemgraphStore) Search(ctx context.Context, vector []float32, limit int, kind string) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 10
	}

	// Optimized Cypher query for cosine similarity
	// Note: We use list comprehension to compute dot product and norms
	query := `
		MATCH (v:VectorEntry)
		WHERE ($kind = "" OR v.kind = $kind) AND v.vector IS NOT NULL
		WITH v, $vector AS target
		WITH v, 
			 reduce(dot = 0.0, i IN range(0, size(v.vector)-1) | dot + v.vector[i] * target[i]) AS dot_product,
			 reduce(ss_a = 0.0, x IN v.vector | ss_a + x*x) AS norm_a_sq,
			 reduce(ss_b = 0.0, y IN target | ss_b + y*y) AS norm_b_sq
		WITH v, dot_product, sqrt(norm_a_sq) * sqrt(norm_b_sq) AS denominator
		WITH v, CASE WHEN denominator = 0 THEN 0 ELSE dot_product / denominator END AS score
		ORDER BY score DESC
		LIMIT $limit
		RETURN v.id, v.text, v.kind, v.metadata, v.created_at, v.vector, score
	`

	params := map[string]any{
		"vector": vector,
		"kind":   kind,
		"limit":  limit,
	}

	records, err := s.db.Execute(ctx, query, params)
	if err != nil {
		return nil, err
	}

	var results []SearchResult
	for _, rec := range records {
		// Parse metadata
		var meta map[string]string
		if metaStr, ok := rec["v.metadata"].(string); ok {
			_ = json.Unmarshal([]byte(metaStr), &meta)
		}

		// Parse vector
		var vec []float32
		if vecInterface, ok := rec["v.vector"].([]interface{}); ok {
			vec = make([]float32, len(vecInterface))
			for i, v := range vecInterface {
				if f, ok := v.(float64); ok {
					vec[i] = float32(f)
				}
			}
		}

		entry := VectorEntry{
			ID:        rec["v.id"].(string),
			Text:      rec["v.text"].(string),
			Kind:      rec["v.kind"].(string),
			Metadata:  meta,
			CreatedAt: rec["v.created_at"].(int64), // Assuming int64 coming back from driver
			Vector:    vec,
		}
		
		// Handle int/float conversion for CreatedAt if needed
		if t, ok := rec["v.created_at"].(float64); ok {
			entry.CreatedAt = int64(t)
		}

		score := float32(0)
		if s, ok := rec["score"].(float64); ok {
			score = float32(s)
		}

		results = append(results, SearchResult{
			Entry:    entry,
			Score:    score,
			Distance: 1 - score,
		})
	}

	return results, nil
}

// Delete removes an entry by ID.
func (s *MemgraphStore) Delete(ctx context.Context, id string) error {
	query := `MATCH (v:VectorEntry {id: $id}) DETACH DELETE v`
	return s.db.ExecuteWrite(ctx, query, map[string]any{"id": id})
}

// Count returns total entries.
func (s *MemgraphStore) Count(ctx context.Context) (int, error) {
	query := `MATCH (v:VectorEntry) RETURN count(v) as count`
	records, err := s.db.Execute(ctx, query, nil)
	if err != nil {
		return 0, err
	}
	if len(records) == 0 {
		return 0, nil
	}
	if c, ok := records[0]["count"].(int64); ok {
		return int(c), nil
	}
	return 0, nil
}

// Close closes the store (no-op for Memgraph store as connection is managed externally).
func (s *MemgraphStore) Close() error {
	return nil
}

// Ping verifies the database connection is alive.
func (s *MemgraphStore) Ping(ctx context.Context) error {
	return s.db.Ping(ctx)
}
