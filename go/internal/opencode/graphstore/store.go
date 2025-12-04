// Package graphstore implements opencode's domain.Store using urp's graph database.
// This enables opencode sessions/messages to persist in Memgraph instead of SQLite.
package graphstore

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/joss/urp/internal/graph"
	"github.com/joss/urp/internal/opencode/domain"
)

// Store implements domain.Store backed by Memgraph.
type Store struct {
	db graph.Driver
}

// New creates a graph-backed store.
func New(db graph.Driver) *Store {
	return &Store{db: db}
}

// --- SessionStore ---

func (s *Store) CreateSession(ctx context.Context, sess *domain.Session) error {
	summaryJSON := ""
	if sess.Summary != nil {
		b, _ := json.Marshal(sess.Summary)
		summaryJSON = string(b)
	}

	query := `
		CREATE (s:Session:OpenCode {
			id: $id,
			projectID: $projectID,
			workdir: $workdir,
			parentID: $parentID,
			title: $title,
			version: $version,
			createdAt: $createdAt,
			updatedAt: $updatedAt,
			summary: $summary
		})
	`
	return s.db.ExecuteWrite(ctx, query, map[string]any{
		"id":        sess.ID,
		"projectID": sess.ProjectID,
		"workdir":   sess.Directory,
		"parentID":  sess.ParentID,
		"title":     sess.Title,
		"version":   sess.Version,
		"createdAt": sess.CreatedAt.Unix(),
		"updatedAt": sess.UpdatedAt.Unix(),
		"summary":   summaryJSON,
	})
}

func (s *Store) GetSession(ctx context.Context, id string) (*domain.Session, error) {
	query := `
		MATCH (s:Session:OpenCode {id: $id})
		RETURN s.id as id, s.projectID as projectID, s.workdir as workdir,
		       s.parentID as parentID, s.title as title, s.version as version,
		       s.createdAt as createdAt, s.updatedAt as updatedAt, s.summary as summary
	`
	records, err := s.db.Execute(ctx, query, map[string]any{"id": id})
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("session not found: %s", id)
	}
	return recordToSession(records[0])
}

func (s *Store) ListSessions(ctx context.Context, projectID string, limit int) ([]*domain.Session, error) {
	query := `
		MATCH (s:Session:OpenCode)
		WHERE $projectID = '' OR s.projectID = $projectID
		RETURN s.id as id, s.projectID as projectID, s.workdir as workdir,
		       s.parentID as parentID, s.title as title, s.version as version,
		       s.createdAt as createdAt, s.updatedAt as updatedAt, s.summary as summary
		ORDER BY s.updatedAt DESC
		LIMIT $limit
	`
	records, err := s.db.Execute(ctx, query, map[string]any{
		"projectID": projectID,
		"limit":     limit,
	})
	if err != nil {
		return nil, err
	}

	var sessions []*domain.Session
	for _, r := range records {
		sess, err := recordToSession(r)
		if err != nil {
			continue
		}
		sessions = append(sessions, sess)
	}
	return sessions, nil
}

func (s *Store) UpdateSession(ctx context.Context, sess *domain.Session) error {
	summaryJSON := ""
	if sess.Summary != nil {
		b, _ := json.Marshal(sess.Summary)
		summaryJSON = string(b)
	}

	query := `
		MATCH (s:Session:OpenCode {id: $id})
		SET s.title = $title,
		    s.updatedAt = $updatedAt,
		    s.summary = $summary
	`
	return s.db.ExecuteWrite(ctx, query, map[string]any{
		"id":        sess.ID,
		"title":     sess.Title,
		"updatedAt": sess.UpdatedAt.Unix(),
		"summary":   summaryJSON,
	})
}

func (s *Store) DeleteSession(ctx context.Context, id string) error {
	// Delete messages first
	if err := s.db.ExecuteWrite(ctx, `
		MATCH (m:Message:OpenCode {sessionID: $id})
		DETACH DELETE m
	`, map[string]any{"id": id}); err != nil {
		return err
	}

	// Delete usage
	if err := s.db.ExecuteWrite(ctx, `
		MATCH (u:Usage:OpenCode {sessionID: $id})
		DETACH DELETE u
	`, map[string]any{"id": id}); err != nil {
		return err
	}

	// Delete session
	return s.db.ExecuteWrite(ctx, `
		MATCH (s:Session:OpenCode {id: $id})
		DETACH DELETE s
	`, map[string]any{"id": id})
}

// --- MessageStore ---

func (s *Store) CreateMessage(ctx context.Context, msg *domain.Message) error {
	partsJSON, err := domain.MarshalParts(msg.Parts)
	if err != nil {
		return fmt.Errorf("marshal parts: %w", err)
	}

	query := `
		MATCH (sess:Session:OpenCode {id: $sessionID})
		CREATE (m:Message:OpenCode {
			id: $id,
			sessionID: $sessionID,
			role: $role,
			parts: $parts,
			timestamp: $timestamp
		})
		CREATE (sess)-[:HAS_MESSAGE]->(m)
	`
	return s.db.ExecuteWrite(ctx, query, map[string]any{
		"id":        msg.ID,
		"sessionID": msg.SessionID,
		"role":      string(msg.Role),
		"parts":     string(partsJSON),
		"timestamp": msg.Timestamp.Unix(),
	})
}

func (s *Store) GetMessages(ctx context.Context, sessionID string) ([]*domain.Message, error) {
	query := `
		MATCH (m:Message:OpenCode {sessionID: $sessionID})
		RETURN m.id as id, m.sessionID as sessionID, m.role as role,
		       m.parts as parts, m.timestamp as timestamp
		ORDER BY m.timestamp ASC
	`
	records, err := s.db.Execute(ctx, query, map[string]any{"sessionID": sessionID})
	if err != nil {
		return nil, err
	}

	var messages []*domain.Message
	for _, r := range records {
		msg, err := recordToMessage(r)
		if err != nil {
			continue
		}
		messages = append(messages, msg)
	}
	return messages, nil
}

func (s *Store) UpdateMessage(ctx context.Context, msg *domain.Message) error {
	partsJSON, err := domain.MarshalParts(msg.Parts)
	if err != nil {
		return fmt.Errorf("marshal parts: %w", err)
	}

	return s.db.ExecuteWrite(ctx, `
		MATCH (m:Message:OpenCode {id: $id})
		SET m.parts = $parts
	`, map[string]any{
		"id":    msg.ID,
		"parts": string(partsJSON),
	})
}

func (s *Store) DeleteMessage(ctx context.Context, id string) error {
	return s.db.ExecuteWrite(ctx, `
		MATCH (m:Message:OpenCode {id: $id})
		DETACH DELETE m
	`, map[string]any{"id": id})
}

// --- UsageStore ---

func (s *Store) GetUsage(ctx context.Context, sessionID string) (*domain.SessionUsage, error) {
	query := `
		MATCH (u:Usage:OpenCode {sessionID: $sessionID})
		RETURN u.sessionID as sessionID, u.providerID as providerID,
		       u.modelID as modelID, u.inputTokens as inputTokens,
		       u.outputTokens as outputTokens, u.cacheRead as cacheRead,
		       u.cacheWrite as cacheWrite, u.inputCost as inputCost,
		       u.outputCost as outputCost, u.totalCost as totalCost,
		       u.messageCount as messageCount, u.toolCalls as toolCalls,
		       u.updatedAt as updatedAt
	`
	records, err := s.db.Execute(ctx, query, map[string]any{"sessionID": sessionID})
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return &domain.SessionUsage{SessionID: sessionID, UpdatedAt: time.Now()}, nil
	}
	return recordToUsage(records[0]), nil
}

func (s *Store) UpdateUsage(ctx context.Context, usage *domain.SessionUsage) error {
	query := `
		MERGE (u:Usage:OpenCode {sessionID: $sessionID})
		SET u.providerID = $providerID,
		    u.modelID = $modelID,
		    u.inputTokens = $inputTokens,
		    u.outputTokens = $outputTokens,
		    u.cacheRead = $cacheRead,
		    u.cacheWrite = $cacheWrite,
		    u.inputCost = $inputCost,
		    u.outputCost = $outputCost,
		    u.totalCost = $totalCost,
		    u.messageCount = $messageCount,
		    u.toolCalls = $toolCalls,
		    u.updatedAt = $updatedAt
	`
	return s.db.ExecuteWrite(ctx, query, map[string]any{
		"sessionID":    usage.SessionID,
		"providerID":   usage.ProviderID,
		"modelID":      usage.ModelID,
		"inputTokens":  usage.Usage.InputTokens,
		"outputTokens": usage.Usage.OutputTokens,
		"cacheRead":    usage.Usage.CacheRead,
		"cacheWrite":   usage.Usage.CacheWrite,
		"inputCost":    usage.Usage.InputCost,
		"outputCost":   usage.Usage.OutputCost,
		"totalCost":    usage.Usage.TotalCost,
		"messageCount": usage.MessageCount,
		"toolCalls":    usage.ToolCalls,
		"updatedAt":    usage.UpdatedAt.Unix(),
	})
}

func (s *Store) GetTotalUsage(ctx context.Context) (*domain.Usage, error) {
	query := `
		MATCH (u:Usage:OpenCode)
		RETURN sum(u.inputTokens) as inputTokens,
		       sum(u.outputTokens) as outputTokens,
		       sum(u.cacheRead) as cacheRead,
		       sum(u.cacheWrite) as cacheWrite,
		       sum(u.inputCost) as inputCost,
		       sum(u.outputCost) as outputCost,
		       sum(u.totalCost) as totalCost
	`
	records, err := s.db.Execute(ctx, query, nil)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return &domain.Usage{}, nil
	}

	r := records[0]
	return &domain.Usage{
		InputTokens:  getInt(r, "inputTokens"),
		OutputTokens: getInt(r, "outputTokens"),
		CacheRead:    getInt(r, "cacheRead"),
		CacheWrite:   getInt(r, "cacheWrite"),
		InputCost:    getFloat(r, "inputCost"),
		OutputCost:   getFloat(r, "outputCost"),
		TotalCost:    getFloat(r, "totalCost"),
	}, nil
}

// --- Helpers ---

func recordToSession(r graph.Record) (*domain.Session, error) {
	sess := &domain.Session{
		ID:        getString(r, "id"),
		ProjectID: getString(r, "projectID"),
		Directory: getString(r, "workdir"),
		ParentID:  getString(r, "parentID"),
		Title:     getString(r, "title"),
		Version:   getString(r, "version"),
		CreatedAt: time.Unix(int64(getInt(r, "createdAt")), 0),
		UpdatedAt: time.Unix(int64(getInt(r, "updatedAt")), 0),
	}

	if summaryStr := getString(r, "summary"); summaryStr != "" {
		var summary domain.Summary
		if json.Unmarshal([]byte(summaryStr), &summary) == nil {
			sess.Summary = &summary
		}
	}
	return sess, nil
}

func recordToMessage(r graph.Record) (*domain.Message, error) {
	msg := &domain.Message{
		ID:        getString(r, "id"),
		SessionID: getString(r, "sessionID"),
		Role:      domain.Role(getString(r, "role")),
		Timestamp: time.Unix(int64(getInt(r, "timestamp")), 0),
	}

	if partsStr := getString(r, "parts"); partsStr != "" {
		parts, err := domain.UnmarshalParts([]byte(partsStr))
		if err == nil {
			msg.Parts = parts
		}
	}
	return msg, nil
}

func recordToUsage(r graph.Record) *domain.SessionUsage {
	return &domain.SessionUsage{
		SessionID:    getString(r, "sessionID"),
		ProviderID:   getString(r, "providerID"),
		ModelID:      getString(r, "modelID"),
		MessageCount: getInt(r, "messageCount"),
		ToolCalls:    getInt(r, "toolCalls"),
		UpdatedAt:    time.Unix(int64(getInt(r, "updatedAt")), 0),
		Usage: domain.Usage{
			InputTokens:  getInt(r, "inputTokens"),
			OutputTokens: getInt(r, "outputTokens"),
			CacheRead:    getInt(r, "cacheRead"),
			CacheWrite:   getInt(r, "cacheWrite"),
			InputCost:    getFloat(r, "inputCost"),
			OutputCost:   getFloat(r, "outputCost"),
			TotalCost:    getFloat(r, "totalCost"),
		},
	}
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
		case int:
			return n
		case int64:
			return int(n)
		case float64:
			return int(n)
		}
	}
	return 0
}

func getFloat(r graph.Record, key string) float64 {
	if v, ok := r[key]; ok {
		switch n := v.(type) {
		case float64:
			return n
		case int64:
			return float64(n)
		case int:
			return float64(n)
		}
	}
	return 0
}
