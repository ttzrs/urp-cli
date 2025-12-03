// Package memory provides session memory management.
package memory

import (
	"context"
	"fmt"
	"time"

	"github.com/joss/urp/internal/graph"
)

// MemoryEntry represents a session memory item.
type MemoryEntry struct {
	MemoryID   string   `json:"memory_id"`
	Kind       string   `json:"kind"` // note, summary, decision, result, observation
	Text       string   `json:"text"`
	Importance int      `json:"importance"` // 1-5
	SessionID  string   `json:"session_id"`
	InstanceID string   `json:"instance_id"`
	CreatedAt  string   `json:"created_at"`
	Tags       []string `json:"tags"`
	Similarity float64  `json:"similarity,omitempty"`
}

// SessionMemory provides session-scoped memory operations.
type SessionMemory struct {
	db  graph.Driver
	ctx *Context
}

// NewSessionMemory creates a new session memory service.
func NewSessionMemory(db graph.Driver, ctx *Context) *SessionMemory {
	return &SessionMemory{db: db, ctx: ctx}
}

// Add stores a memory in the current session.
func (s *SessionMemory) Add(ctx context.Context, text, kind string, importance int, tags []string) (string, error) {
	if text == "" {
		return "", fmt.Errorf("text cannot be empty")
	}

	// Clamp importance
	if importance < 1 {
		importance = 1
	}
	if importance > 5 {
		importance = 5
	}

	// Resolve session ID (handle name collisions)
	sessionID, err := s.resolveSessionID(ctx)
	if err != nil {
		return "", err
	}

	memoryID := fmt.Sprintf("m-%d", time.Now().UnixNano())
	now := time.Now().UTC().Format(time.RFC3339)

	allTags := append(s.ctx.Tags, tags...)

	query := `
		MERGE (i:Instance {instance_id: $instance_id})
		  ON CREATE SET i.created_at = $now
		MERGE (sess:Session {session_id: $session_id})
		  ON CREATE SET sess.instance_id = $instance_id,
		                sess.user_id = $user_id,
		                sess.started_at = $now,
		                sess.context_signature = $ctx_sig,
		                sess.path = $path
		MERGE (i)-[:HAS_SESSION]->(sess)
		CREATE (m:Memo {
			memory_id: $memory_id,
			kind: $kind,
			text: $text,
			importance: $importance,
			created_at: $now,
			tags: $tags
		})
		CREATE (sess)-[:HAS_MEMORY {at: $now}]->(m)
	`

	err = s.db.ExecuteWrite(ctx, query, map[string]any{
		"instance_id": s.ctx.InstanceID,
		"session_id":  sessionID,
		"user_id":     s.ctx.UserID,
		"ctx_sig":     s.ctx.ContextSignature,
		"path":        s.ctx.Path,
		"memory_id":   memoryID,
		"kind":        kind,
		"text":        truncate(text, 500),
		"importance":  importance,
		"tags":        allTags,
		"now":         now,
	})

	if err != nil {
		return "", err
	}

	return memoryID, nil
}

// Recall searches memories using keyword matching.
func (s *SessionMemory) Recall(ctx context.Context, queryText string, limit int, kind string, minImportance int) ([]MemoryEntry, error) {
	sessionID := s.getResolvedSessionID(ctx)

	whereClause := ""
	if kind != "" {
		whereClause = "AND m.kind = $kind"
	}

	query := fmt.Sprintf(`
		MATCH (sess:Session {session_id: $session_id})-[:HAS_MEMORY]->(m:Memo)
		WHERE m.importance >= $min_importance %s
		RETURN m.memory_id as memory_id,
		       m.kind as kind,
		       m.text as text,
		       m.importance as importance,
		       m.created_at as created_at,
		       m.tags as tags
		ORDER BY m.created_at DESC
		LIMIT $limit
	`, whereClause)

	params := map[string]any{
		"session_id":     sessionID,
		"min_importance": minImportance,
		"limit":          limit,
	}
	if kind != "" {
		params["kind"] = kind
	}

	records, err := s.db.Execute(ctx, query, params)
	if err != nil {
		return nil, err
	}

	// Filter by keyword similarity if query provided
	queryWords := tokenize(queryText)
	var results []MemoryEntry

	for _, r := range records {
		entry := MemoryEntry{
			MemoryID:   getString(r, "memory_id"),
			Kind:       getString(r, "kind"),
			Text:       getString(r, "text"),
			Importance: getInt(r, "importance"),
			SessionID:  s.ctx.SessionID,
			CreatedAt:  getString(r, "created_at"),
		}

		if len(queryWords) > 0 {
			textWords := tokenize(entry.Text)
			sim := jaccardSimilarity(queryWords, textWords)
			if sim < 0.1 {
				continue
			}
			entry.Similarity = sim
		}

		results = append(results, entry)
	}

	return results, nil
}

// List returns all memories for the session.
func (s *SessionMemory) List(ctx context.Context) ([]MemoryEntry, error) {
	sessionID := s.getResolvedSessionID(ctx)

	query := `
		MATCH (sess:Session {session_id: $session_id})-[:HAS_MEMORY]->(m:Memo)
		RETURN m.memory_id as memory_id,
		       m.kind as kind,
		       m.text as text,
		       m.importance as importance,
		       m.created_at as created_at
		ORDER BY m.created_at DESC
	`

	records, err := s.db.Execute(ctx, query, map[string]any{
		"session_id": sessionID,
	})
	if err != nil {
		return nil, err
	}

	var results []MemoryEntry
	for _, r := range records {
		results = append(results, MemoryEntry{
			MemoryID:   getString(r, "memory_id"),
			Kind:       getString(r, "kind"),
			Text:       getString(r, "text"),
			Importance: getInt(r, "importance"),
			SessionID:  sessionID,
			CreatedAt:  getString(r, "created_at"),
		})
	}

	return results, nil
}

// Delete removes a specific memory.
func (s *SessionMemory) Delete(ctx context.Context, memoryID string) error {
	sessionID := s.getResolvedSessionID(ctx)

	query := `
		MATCH (sess:Session {session_id: $session_id})-[r:HAS_MEMORY]->(m:Memo {memory_id: $memory_id})
		DELETE r, m
	`

	return s.db.ExecuteWrite(ctx, query, map[string]any{
		"session_id": sessionID,
		"memory_id":  memoryID,
	})
}

// Clear removes all memories for the session.
func (s *SessionMemory) Clear(ctx context.Context) (int, error) {
	sessionID := s.getResolvedSessionID(ctx)

	// Count first
	countQuery := `
		MATCH (sess:Session {session_id: $session_id})-[:HAS_MEMORY]->(m:Memo)
		RETURN count(m) as count
	`

	records, err := s.db.Execute(ctx, countQuery, map[string]any{
		"session_id": sessionID,
	})
	if err != nil {
		return 0, err
	}

	count := 0
	if len(records) > 0 {
		count = getInt(records[0], "count")
	}

	// Delete all
	deleteQuery := `
		MATCH (sess:Session {session_id: $session_id})-[r:HAS_MEMORY]->(m:Memo)
		DELETE r, m
	`

	err = s.db.ExecuteWrite(ctx, deleteQuery, map[string]any{
		"session_id": sessionID,
	})

	return count, err
}

// Stats returns statistics about session memory.
func (s *SessionMemory) Stats(ctx context.Context) (map[string]any, error) {
	sessionID := s.getResolvedSessionID(ctx)

	query := `
		MATCH (sess:Session {session_id: $session_id})-[:HAS_MEMORY]->(m:Memo)
		RETURN m.kind as kind, count(*) as count
	`

	records, err := s.db.Execute(ctx, query, map[string]any{
		"session_id": sessionID,
	})
	if err != nil {
		return nil, err
	}

	byKind := make(map[string]int)
	total := 0
	for _, r := range records {
		kind := getString(r, "kind")
		count := getInt(r, "count")
		byKind[kind] = count
		total += count
	}

	return map[string]any{
		"session_id": sessionID,
		"total":      total,
		"by_kind":    byKind,
	}, nil
}

// resolveSessionID checks for session name collisions and returns the correct ID.
// If a session with the same project name exists but different path, use project-hash(path).
func (s *SessionMemory) resolveSessionID(ctx context.Context) (string, error) {
	// If session_id was explicitly set via env, use it directly
	if s.ctx.SessionID != s.ctx.Project {
		return s.ctx.SessionID, nil
	}

	// Check if session with this project name exists
	query := `
		MATCH (sess:Session {session_id: $session_id})
		RETURN sess.path as path
		LIMIT 1
	`

	records, err := s.db.Execute(ctx, query, map[string]any{
		"session_id": s.ctx.SessionID,
	})
	if err != nil {
		return "", err
	}

	// No existing session - use simple project name
	if len(records) == 0 {
		return s.ctx.SessionID, nil
	}

	// Session exists - check path
	existingPath := getString(records[0], "path")
	if existingPath == s.ctx.Path || existingPath == "" {
		// Same path or no path recorded - use existing session
		return s.ctx.SessionID, nil
	}

	// Different path - collision! Use project-hash(path)
	return sessionIDWithPath(s.ctx.Project, s.ctx.Path), nil
}

// getResolvedSessionID returns the session ID to use for queries.
// Call this before any read operation.
func (s *SessionMemory) getResolvedSessionID(ctx context.Context) string {
	resolved, err := s.resolveSessionID(ctx)
	if err != nil {
		return s.ctx.SessionID
	}
	return resolved
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

func getString(r graph.Record, key string) string {
	if v, ok := r[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func getInt(r graph.Record, key string) int {
	if v, ok := r[key]; ok {
		switch n := v.(type) {
		case int64:
			return int(n)
		case int:
			return n
		case float64:
			return int(n)
		}
	}
	return 0
}

// tokenize splits text into lowercase words.
func tokenize(text string) map[string]bool {
	words := make(map[string]bool)
	current := ""

	for _, r := range text {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			current += string(r)
		} else {
			if len(current) > 2 {
				words[toLowerCase(current)] = true
			}
			current = ""
		}
	}
	if len(current) > 2 {
		words[toLowerCase(current)] = true
	}

	return words
}

func toLowerCase(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 32
		}
		result[i] = c
	}
	return string(result)
}

// jaccardSimilarity calculates Jaccard index.
func jaccardSimilarity(a, b map[string]bool) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 0
	}

	intersection := 0
	for word := range a {
		if b[word] {
			intersection++
		}
	}

	union := len(a)
	for word := range b {
		if !a[word] {
			union++
		}
	}

	if union == 0 {
		return 0
	}

	return float64(intersection) / float64(union)
}
