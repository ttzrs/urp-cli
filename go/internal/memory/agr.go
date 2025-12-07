// Package memory provides Adaptive Graph-Guided Retrieval (AGR).
package memory

import (
	"context"
	"fmt"

	"github.com/joss/urp/internal/graph"
)

// AGRConfig configures the AGR algorithm.
type AGRConfig struct {
	MaxHops             int     // Maximum traversal depth (default: 3)
	ConfidenceThreshold float64 // Minimum score to include in results (default: 0.3)
	ExpansionThreshold  float64 // Minimum score to expand neighbors (default: 0.15)
	Limit               int     // Maximum results to return (default: 10)
}

// DefaultAGRConfig returns sensible defaults.
func DefaultAGRConfig() AGRConfig {
	return AGRConfig{
		MaxHops:             3,
		ConfidenceThreshold: 0.3,
		ExpansionThreshold:  0.15,
		Limit:               10,
	}
}

// AGRResult represents a single result with traversal metadata.
type AGRResult struct {
	Entry    KnowledgeEntry `json:"entry"`
	Score    float64        `json:"score"`
	HopCount int            `json:"hop_count"`
	Path     []string       `json:"path,omitempty"` // IDs of traversed nodes
}

// AdaptiveRetriever implements graph-guided retrieval.
type AdaptiveRetriever struct {
	db     graph.Driver
	config AGRConfig
}

// NewAdaptiveRetriever creates a new AGR instance.
func NewAdaptiveRetriever(db graph.Driver, config AGRConfig) *AdaptiveRetriever {
	if config.MaxHops <= 0 {
		config.MaxHops = 3
	}
	if config.ConfidenceThreshold <= 0 {
		config.ConfidenceThreshold = 0.3
	}
	if config.ExpansionThreshold <= 0 {
		config.ExpansionThreshold = 0.15
	}
	if config.Limit <= 0 {
		config.Limit = 10
	}
	return &AdaptiveRetriever{db: db, config: config}
}

// Search performs adaptive graph-guided retrieval.
// Starts from seed nodes and expands based on confidence.
func (a *AdaptiveRetriever) Search(ctx context.Context, queryText string, seedIDs []string) ([]AGRResult, error) {
	if len(seedIDs) == 0 {
		return nil, nil
	}

	queryWords := tokenize(queryText)
	if len(queryWords) == 0 {
		return nil, nil
	}

	visited := make(map[string]bool)
	var results []AGRResult

	// Initialize frontier with seeds
	frontier := make([]frontierNode, 0, len(seedIDs))
	for _, id := range seedIDs {
		frontier = append(frontier, frontierNode{id: id, hop: 0, path: []string{}})
	}

	for hop := 0; hop < a.config.MaxHops && len(frontier) > 0; hop++ {
		var nextFrontier []frontierNode

		for _, node := range frontier {
			if visited[node.id] {
				continue
			}
			visited[node.id] = true

			// Fetch node data
			entry, err := a.fetchKnowledge(ctx, node.id)
			if err != nil || entry == nil {
				continue
			}

			// Calculate similarity
			entryWords := tokenize(entry.Text)
			score := jaccardSimilarity(queryWords, entryWords)

			// Include if above confidence threshold
			if score >= a.config.ConfidenceThreshold {
				path := append(node.path, node.id)
				results = append(results, AGRResult{
					Entry:    *entry,
					Score:    score,
					HopCount: hop,
					Path:     path,
				})

				// Early termination if we have enough results
				if len(results) >= a.config.Limit*2 {
					break
				}
			}

			// Expand if above expansion threshold
			if score >= a.config.ExpansionThreshold {
				neighbors, err := a.getNeighbors(ctx, node.id)
				if err == nil {
					for _, neighborID := range neighbors {
						if !visited[neighborID] {
							nextFrontier = append(nextFrontier, frontierNode{
								id:   neighborID,
								hop:  hop + 1,
								path: append(node.path, node.id),
							})
						}
					}
				}
			}
		}

		frontier = nextFrontier

		// Stop if we have enough high-quality results
		if len(results) >= a.config.Limit {
			break
		}
	}

	// Sort by score and limit
	sortAGRResults(results)
	if len(results) > a.config.Limit {
		results = results[:a.config.Limit]
	}

	return results, nil
}

// SearchFromQuery finds seeds automatically and performs AGR.
func (a *AdaptiveRetriever) SearchFromQuery(ctx context.Context, queryText, kind string) ([]AGRResult, error) {
	// Find initial seeds using keyword match
	seeds, err := a.findSeeds(ctx, queryText, kind, 5)
	if err != nil {
		return nil, err
	}

	return a.Search(ctx, queryText, seeds)
}

type frontierNode struct {
	id   string
	hop  int
	path []string
}

func (a *AdaptiveRetriever) fetchKnowledge(ctx context.Context, knowledgeID string) (*KnowledgeEntry, error) {
	query := `
		MATCH (k:Knowledge {knowledge_id: $id})
		RETURN k.knowledge_id as knowledge_id,
		       k.kind as kind,
		       k.scope as scope,
		       k.text as text,
		       k.context_signature as context_signature,
		       k.created_at as created_at
	`

	records, err := a.db.Execute(ctx, query, map[string]any{"id": knowledgeID})
	if err != nil || len(records) == 0 {
		return nil, err
	}

	return &KnowledgeEntry{
		KnowledgeID:      graph.GetString(records[0], "knowledge_id"),
		Kind:             graph.GetString(records[0], "kind"),
		Scope:            graph.GetString(records[0], "scope"),
		Text:             graph.GetString(records[0], "text"),
		ContextSignature: graph.GetString(records[0], "context_signature"),
		CreatedAt:        graph.GetString(records[0], "created_at"),
	}, nil
}

func (a *AdaptiveRetriever) getNeighbors(ctx context.Context, knowledgeID string) ([]string, error) {
	// Get related knowledge through various relationships
	query := `
		MATCH (k:Knowledge {knowledge_id: $id})
		OPTIONAL MATCH (k)-[:SIMILAR_TO|RELATED_TO|RESOLVES|PRECEDES]-(neighbor:Knowledge)
		OPTIONAL MATCH (k)<-[:CREATED]-(s:Session)-[:CREATED]->(sibling:Knowledge)
		WHERE sibling.knowledge_id <> k.knowledge_id
		WITH collect(DISTINCT neighbor.knowledge_id) + collect(DISTINCT sibling.knowledge_id) as neighbors
		UNWIND neighbors as n
		RETURN DISTINCT n as neighbor_id
		LIMIT 20
	`

	records, err := a.db.Execute(ctx, query, map[string]any{"id": knowledgeID})
	if err != nil {
		return nil, err
	}

	neighbors := make([]string, 0, len(records))
	for _, r := range records {
		if id := graph.GetString(r, "neighbor_id"); id != "" {
			neighbors = append(neighbors, id)
		}
	}

	return neighbors, nil
}

func (a *AdaptiveRetriever) findSeeds(ctx context.Context, queryText, kind string, limit int) ([]string, error) {
	// Find initial knowledge entries using full-text-like matching
	whereClause := "k.text IS NOT NULL"
	params := map[string]any{"limit": limit}

	if kind != "" {
		whereClause += " AND k.kind = $kind"
		params["kind"] = kind
	}

	query := fmt.Sprintf(`
		MATCH (k:Knowledge)
		WHERE %s
		RETURN k.knowledge_id as id, k.text as text
		ORDER BY k.created_at DESC
		LIMIT 50
	`, whereClause)

	records, err := a.db.Execute(ctx, query, params)
	if err != nil {
		return nil, err
	}

	// Score and rank locally
	queryWords := tokenize(queryText)
	type scored struct {
		id    string
		score float64
	}
	var scored_list []scored

	for _, r := range records {
		text := graph.GetString(r, "text")
		id := graph.GetString(r, "id")
		if id == "" {
			continue
		}

		textWords := tokenize(text)
		score := jaccardSimilarity(queryWords, textWords)
		if score > 0.05 { // Low threshold for seeds
			scored_list = append(scored_list, scored{id: id, score: score})
		}
	}

	// Sort by score
	for i := 0; i < len(scored_list); i++ {
		for j := i + 1; j < len(scored_list); j++ {
			if scored_list[j].score > scored_list[i].score {
				scored_list[i], scored_list[j] = scored_list[j], scored_list[i]
			}
		}
	}

	// Return top N
	result := make([]string, 0, limit)
	for i := 0; i < len(scored_list) && i < limit; i++ {
		result = append(result, scored_list[i].id)
	}

	return result, nil
}

func sortAGRResults(results []AGRResult) {
	// Sort by score descending, then by hop count ascending
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			// Primary: higher score first
			if results[j].Score > results[i].Score {
				results[i], results[j] = results[j], results[i]
			} else if results[j].Score == results[i].Score {
				// Secondary: fewer hops first
				if results[j].HopCount < results[i].HopCount {
					results[i], results[j] = results[j], results[i]
				}
			}
		}
	}
}

// CreateSimilarityEdge creates a SIMILAR_TO relationship between knowledge entries.
func (a *AdaptiveRetriever) CreateSimilarityEdge(ctx context.Context, id1, id2 string, weight float64) error {
	query := `
		MATCH (k1:Knowledge {knowledge_id: $id1})
		MATCH (k2:Knowledge {knowledge_id: $id2})
		MERGE (k1)-[r:SIMILAR_TO]-(k2)
		ON CREATE SET r.weight = $weight, r.created_at = timestamp()
		ON MATCH SET r.weight = $weight
	`

	return a.db.ExecuteWrite(ctx, query, map[string]any{
		"id1":    id1,
		"id2":    id2,
		"weight": weight,
	})
}

// BuildSimilarityGraph creates SIMILAR_TO edges for all knowledge entries.
// Call this periodically to enable AGR graph traversal.
func (a *AdaptiveRetriever) BuildSimilarityGraph(ctx context.Context, minSimilarity float64) (int, error) {
	// Get all knowledge
	query := `
		MATCH (k:Knowledge)
		RETURN k.knowledge_id as id, k.text as text
	`

	records, err := a.db.Execute(ctx, query, nil)
	if err != nil {
		return 0, err
	}

	type entry struct {
		id    string
		words map[string]bool
	}

	entries := make([]entry, 0, len(records))
	for _, r := range records {
		id := graph.GetString(r, "id")
		text := graph.GetString(r, "text")
		if id == "" {
			continue
		}

		words := tokenize(text)
		entries = append(entries, entry{id: id, words: words})
	}

	// Compare all pairs and create edges
	edgeCount := 0
	for i := 0; i < len(entries); i++ {
		for j := i + 1; j < len(entries); j++ {
			sim := jaccardFromSets(entries[i].words, entries[j].words)
			if sim >= minSimilarity {
				err := a.CreateSimilarityEdge(ctx, entries[i].id, entries[j].id, sim)
				if err == nil {
					edgeCount++
				}
			}
		}
	}

	return edgeCount, nil
}

func jaccardFromSets(a, b map[string]bool) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}

	intersection := 0
	for k := range a {
		if b[k] {
			intersection++
		}
	}

	union := len(a) + len(b) - intersection
	if union == 0 {
		return 0
	}

	return float64(intersection) / float64(union)
}
