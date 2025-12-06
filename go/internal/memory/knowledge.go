// Package memory provides knowledge store management.
package memory

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/joss/urp/internal/graph"
	urpstrings "github.com/joss/urp/internal/strings"
)

// KnowledgeEntry represents a knowledge item.
type KnowledgeEntry struct {
	KnowledgeID      string  `json:"knowledge_id"`
	Kind             string  `json:"kind"` // error, fix, rule, pattern, plan, insight
	Scope            string  `json:"scope"` // session, instance, global
	Text             string  `json:"text"`
	SessionID        string  `json:"session_id"`
	InstanceID       string  `json:"instance_id"`
	ContextSignature string  `json:"context_signature"`
	CreatedAt        string  `json:"created_at"`
	Similarity       float64 `json:"similarity,omitempty"`
	SourceLevel      string  `json:"source_level,omitempty"`
}

// KnowledgeStore provides shared knowledge operations.
type KnowledgeStore struct {
	db  graph.Driver
	ctx *Context
}

// NewKnowledgeStore creates a new knowledge store.
func NewKnowledgeStore(db graph.Driver, ctx *Context) *KnowledgeStore {
	return &KnowledgeStore{db: db, ctx: ctx}
}

// Store saves knowledge to the graph.
func (k *KnowledgeStore) Store(ctx context.Context, text, kind, scope string) (string, error) {
	if text == "" {
		return "", fmt.Errorf("text cannot be empty")
	}

	knowledgeID := fmt.Sprintf("k-%d", time.Now().UnixNano())
	now := time.Now().UTC().Format(time.RFC3339)

	query := `
		MERGE (i:Instance {instance_id: $instance_id})
		  ON CREATE SET i.created_at = $now
		MERGE (sess:Session {session_id: $session_id})
		  ON CREATE SET sess.instance_id = $instance_id,
		                sess.user_id = $user_id,
		                sess.started_at = $now,
		                sess.context_signature = $ctx_sig
		MERGE (i)-[:HAS_SESSION]->(sess)
		MERGE (k:Knowledge {knowledge_id: $knowledge_id})
		  ON CREATE SET k.kind = $kind,
		                k.text = $text,
		                k.scope = $scope,
		                k.created_at = $now,
		                k.context_signature = $ctx_sig
		  ON MATCH SET k.text = $text,
		               k.scope = $scope
		MERGE (sess)-[:CREATED {at: $now}]->(k)
	`

	err := k.db.ExecuteWrite(ctx, query, map[string]any{
		"instance_id":  k.ctx.InstanceID,
		"session_id":   k.ctx.SessionID,
		"user_id":      k.ctx.UserID,
		"ctx_sig":      k.ctx.ContextSignature,
		"knowledge_id": knowledgeID,
		"kind":         kind,
		"text":         urpstrings.TruncateNoEllipsis(text, 1000),
		"scope":        scope,
		"now":          now,
	})

	if err != nil {
		return "", err
	}

	return knowledgeID, nil
}

// Reject marks knowledge as not applicable for this session.
func (k *KnowledgeStore) Reject(ctx context.Context, knowledgeID, reason string) error {
	query := `
		MATCH (sess:Session {session_id: $session_id})
		MATCH (k:Knowledge {knowledge_id: $knowledge_id})
		MERGE (sess)-[r:REJECTED]->(k)
		  ON CREATE SET r.at = timestamp(), r.reason = $reason
		  ON MATCH SET r.reason = $reason
	`

	return k.db.ExecuteWrite(ctx, query, map[string]any{
		"session_id":   k.ctx.SessionID,
		"knowledge_id": knowledgeID,
		"reason":       reason,
	})
}

// GetRejected returns IDs rejected by this session.
func (k *KnowledgeStore) GetRejected(ctx context.Context) (map[string]bool, error) {
	query := `
		MATCH (sess:Session {session_id: $session_id})-[:REJECTED]->(k:Knowledge)
		RETURN k.knowledge_id as id
	`

	records, err := k.db.Execute(ctx, query, map[string]any{
		"session_id": k.ctx.SessionID,
	})
	if err != nil {
		return nil, err
	}

	rejected := make(map[string]bool)
	for _, r := range records {
		if id := graph.GetString(r, "id"); id != "" {
			rejected[id] = true
		}
	}

	return rejected, nil
}

// MarkUsed records that this session used a piece of knowledge.
func (k *KnowledgeStore) MarkUsed(ctx context.Context, knowledgeID string, similarity float64) error {
	query := `
		MATCH (sess:Session {session_id: $session_id})
		MATCH (k:Knowledge {knowledge_id: $knowledge_id})
		MERGE (sess)-[r:USED]->(k)
		  ON CREATE SET r.at = timestamp(), r.similarity = $similarity
	`

	return k.db.ExecuteWrite(ctx, query, map[string]any{
		"session_id":   k.ctx.SessionID,
		"knowledge_id": knowledgeID,
		"similarity":   similarity,
	})
}

// Query searches knowledge with multi-level strategy.
func (k *KnowledgeStore) Query(ctx context.Context, queryText string, limit int, level, kind string) ([]KnowledgeEntry, error) {
	rejected, _ := k.GetRejected(ctx)
	queryWords := tokenize(queryText)

	var results []KnowledgeEntry
	seen := make(map[string]bool)

	// Level 1: Session-scoped knowledge
	if level == "session" || level == "all" || level == "" {
		sessionResults, _ := k.queryLevel(ctx, "session", k.ctx.SessionID, "", limit)
		for _, entry := range sessionResults {
			if rejected[entry.KnowledgeID] || seen[entry.KnowledgeID] {
				continue
			}
			if kind != "" && entry.Kind != kind {
				continue
			}

			entry.SourceLevel = "session"
			if len(queryWords) > 0 {
				textWords := tokenize(entry.Text)
				entry.Similarity = jaccardSimilarity(queryWords, textWords)
				if entry.Similarity < 0.1 {
					continue
				}
			}
			seen[entry.KnowledgeID] = true
			results = append(results, entry)
		}

		if level == "session" {
			return sortBySimilarity(results, limit), nil
		}
	}

	// Level 2: Instance-scoped knowledge
	if level == "instance" || level == "all" || level == "" {
		instanceResults, _ := k.queryLevel(ctx, "instance", "", k.ctx.InstanceID, limit)
		for _, entry := range instanceResults {
			if rejected[entry.KnowledgeID] || seen[entry.KnowledgeID] {
				continue
			}
			if kind != "" && entry.Kind != kind {
				continue
			}
			if !IsCompatible(entry.ContextSignature, k.ctx.ContextSignature, false) {
				continue
			}

			entry.SourceLevel = "instance"
			if len(queryWords) > 0 {
				textWords := tokenize(entry.Text)
				entry.Similarity = jaccardSimilarity(queryWords, textWords)
				if entry.Similarity < 0.1 {
					continue
				}
			}
			seen[entry.KnowledgeID] = true
			results = append(results, entry)
		}

		if level == "instance" {
			return sortBySimilarity(results, limit), nil
		}
	}

	// Level 3: Global knowledge
	if level == "global" || level == "all" || level == "" {
		globalResults, _ := k.queryLevel(ctx, "global", "", "", limit)
		for _, entry := range globalResults {
			if rejected[entry.KnowledgeID] || seen[entry.KnowledgeID] {
				continue
			}
			if kind != "" && entry.Kind != kind {
				continue
			}

			entry.SourceLevel = "global"
			if len(queryWords) > 0 {
				textWords := tokenize(entry.Text)
				entry.Similarity = jaccardSimilarity(queryWords, textWords)
				if entry.Similarity < 0.1 {
					continue
				}
			}
			seen[entry.KnowledgeID] = true
			results = append(results, entry)
		}
	}

	return sortBySimilarity(results, limit), nil
}

func (k *KnowledgeStore) queryLevel(ctx context.Context, scope, sessionID, instanceID string, limit int) ([]KnowledgeEntry, error) {
	whereClause := "k.scope = $scope"
	params := map[string]any{
		"scope": scope,
		"limit": limit,
	}

	if sessionID != "" {
		whereClause += " AND sess.session_id = $session_id"
		params["session_id"] = sessionID
	}
	if instanceID != "" {
		whereClause += " AND k.instance_id = $instance_id"
		params["instance_id"] = instanceID
	}

	query := fmt.Sprintf(`
		MATCH (sess:Session)-[:CREATED]->(k:Knowledge)
		WHERE %s
		RETURN k.knowledge_id as knowledge_id,
		       k.kind as kind,
		       k.scope as scope,
		       k.text as text,
		       k.context_signature as context_signature,
		       k.created_at as created_at,
		       sess.session_id as session_id
		ORDER BY k.created_at DESC
		LIMIT $limit
	`, whereClause)

	records, err := k.db.Execute(ctx, query, params)
	if err != nil {
		return nil, err
	}

	var results []KnowledgeEntry
	for _, r := range records {
		results = append(results, KnowledgeEntry{
			KnowledgeID:      graph.GetString(r, "knowledge_id"),
			Kind:             graph.GetString(r, "kind"),
			Scope:            graph.GetString(r, "scope"),
			Text:             graph.GetString(r, "text"),
			ContextSignature: graph.GetString(r, "context_signature"),
			CreatedAt:        graph.GetString(r, "created_at"),
			SessionID:        graph.GetString(r, "session_id"),
		})
	}

	return results, nil
}

// ExportMemory promotes a session memory to knowledge.
func (k *KnowledgeStore) ExportMemory(ctx context.Context, memoryID, kind, scope string) (string, error) {
	// Get memory
	query := `
		MATCH (sess:Session {session_id: $session_id})-[:HAS_MEMORY]->(m:Memory {memory_id: $memory_id})
		RETURN m.text as text, m.tags as tags
	`

	records, err := k.db.Execute(ctx, query, map[string]any{
		"session_id": k.ctx.SessionID,
		"memory_id":  memoryID,
	})
	if err != nil {
		return "", err
	}

	if len(records) == 0 {
		return "", fmt.Errorf("memory %s not found in session", memoryID)
	}

	text := graph.GetString(records[0], "text")

	// Store as knowledge
	knowledgeID, err := k.Store(ctx, text, kind, scope)
	if err != nil {
		return "", err
	}

	// Record export relationship
	exportQuery := `
		MATCH (sess:Session {session_id: $session_id})
		MATCH (k:Knowledge {knowledge_id: $knowledge_id})
		MERGE (sess)-[:EXPORTED {at: timestamp()}]->(k)
	`

	k.db.ExecuteWrite(ctx, exportQuery, map[string]any{
		"session_id":   k.ctx.SessionID,
		"knowledge_id": knowledgeID,
	})

	return knowledgeID, nil
}

// Promote changes knowledge scope to global.
func (k *KnowledgeStore) Promote(ctx context.Context, knowledgeID string) error {
	query := `
		MATCH (k:Knowledge {knowledge_id: $knowledge_id})
		SET k.scope = 'global', k.promoted_at = timestamp()
	`

	return k.db.ExecuteWrite(ctx, query, map[string]any{
		"knowledge_id": knowledgeID,
	})
}

// List returns all knowledge with optional filtering.
func (k *KnowledgeStore) List(ctx context.Context, kind, scope string, limit int) ([]KnowledgeEntry, error) {
	params := map[string]any{"limit": limit}

	var conditions []string
	if kind != "" {
		conditions = append(conditions, "k.kind = $kind")
		params["kind"] = kind
	}
	if scope != "" {
		conditions = append(conditions, "k.scope = $scope")
		params["scope"] = scope
	}

	var query string
	if len(conditions) > 0 {
		whereClause := strings.Join(conditions, " AND ")
		query = fmt.Sprintf(`
		MATCH (k:Knowledge)
		WHERE %s
		RETURN k.knowledge_id as knowledge_id,`, whereClause)
	} else {
		query = `
		MATCH (k:Knowledge)
		RETURN k.knowledge_id as knowledge_id,`
	}

	query += `
		       k.kind as kind,
		       k.scope as scope,
		       k.text as text,
		       k.created_at as created_at
		ORDER BY k.created_at DESC
		LIMIT $limit
	`

	records, err := k.db.Execute(ctx, query, params)
	if err != nil {
		return nil, err
	}

	var results []KnowledgeEntry
	for _, r := range records {
		results = append(results, KnowledgeEntry{
			KnowledgeID: graph.GetString(r, "knowledge_id"),
			Kind:        graph.GetString(r, "kind"),
			Scope:       graph.GetString(r, "scope"),
			Text:        graph.GetString(r, "text"),
			CreatedAt:   graph.GetString(r, "created_at"),
		})
	}

	return results, nil
}

// Stats returns knowledge store statistics.
func (k *KnowledgeStore) Stats(ctx context.Context) (map[string]any, error) {
	query := `
		MATCH (k:Knowledge)
		RETURN k.scope as scope, count(*) as count
	`

	records, err := k.db.Execute(ctx, query, nil)
	if err != nil {
		return nil, err
	}

	byScope := make(map[string]int)
	total := 0
	for _, r := range records {
		scope := graph.GetString(r, "scope")
		count := graph.GetInt(r, "count")
		byScope[scope] = count
		total += count
	}

	return map[string]any{
		"total":    total,
		"by_scope": byScope,
	}, nil
}

// Provenance returns who created/used/rejected knowledge.
func (k *KnowledgeStore) Provenance(ctx context.Context, knowledgeID string) (map[string]any, error) {
	query := `
		MATCH (k:Knowledge {knowledge_id: $knowledge_id})
		OPTIONAL MATCH (creator:Session)-[c:CREATED]->(k)
		OPTIONAL MATCH (user:Session)-[u:USED]->(k)
		OPTIONAL MATCH (rejector:Session)-[r:REJECTED]->(k)
		RETURN k.kind as kind,
		       k.scope as scope,
		       k.created_at as created_at,
		       k.context_signature as context,
		       collect(DISTINCT creator.session_id) as created_by,
		       collect(DISTINCT user.session_id) as used_by,
		       collect(DISTINCT {session: rejector.session_id, reason: r.reason}) as rejected_by
	`

	records, err := k.db.Execute(ctx, query, map[string]any{
		"knowledge_id": knowledgeID,
	})
	if err != nil {
		return nil, err
	}

	if len(records) == 0 {
		return nil, fmt.Errorf("knowledge not found")
	}

	return map[string]any{
		"knowledge_id": knowledgeID,
		"kind":         graph.GetString(records[0], "kind"),
		"scope":        graph.GetString(records[0], "scope"),
		"created_at":   graph.GetString(records[0], "created_at"),
		"context":      graph.GetString(records[0], "context"),
		"created_by":   records[0]["created_by"],
		"used_by":      records[0]["used_by"],
		"rejected_by":  records[0]["rejected_by"],
	}, nil
}

func sortBySimilarity(entries []KnowledgeEntry, limit int) []KnowledgeEntry {
	// Simple bubble sort for small slices
	for i := 0; i < len(entries); i++ {
		for j := i + 1; j < len(entries); j++ {
			if entries[j].Similarity > entries[i].Similarity {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}

	if len(entries) > limit {
		return entries[:limit]
	}
	return entries
}

// Ping verifies the database connection is alive.
func (k *KnowledgeStore) Ping(ctx context.Context) error {
	return k.db.Ping(ctx)
}

// Close releases any resources held by the store.
func (k *KnowledgeStore) Close() error {
	return nil // Connection managed externally
}
