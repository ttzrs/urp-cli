// Package bridge provides adapters between urp graph and opencode-go domain.
// This enables opencode's session/message/usage system to use Memgraph as backend.
package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/joss/urp/internal/graph"
)

// Session mirrors opencode's domain.Session for graph storage.
type Session struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"projectID"`
	Directory string    `json:"directory"`
	ParentID  string    `json:"parentID,omitempty"`
	Title     string    `json:"title"`
	Version   string    `json:"version"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	Summary   *Summary  `json:"summary,omitempty"`
}

type Summary struct {
	Additions int      `json:"additions"`
	Deletions int      `json:"deletions"`
	Files     []string `json:"files"`
}

// Message mirrors opencode's domain.Message.
type Message struct {
	ID        string    `json:"id"`
	SessionID string    `json:"sessionID"`
	Role      string    `json:"role"` // user, assistant, system
	Parts     []Part    `json:"parts"`
	Timestamp time.Time `json:"timestamp"`
}

// Part types match opencode's polymorphic parts.
type Part struct {
	Type     string         `json:"type"` // text, reasoning, tool_call, file, image
	Text     string         `json:"text,omitempty"`
	ToolID   string         `json:"toolID,omitempty"`
	Name     string         `json:"name,omitempty"`
	Args     map[string]any `json:"args,omitempty"`
	Result   string         `json:"result,omitempty"`
	Error    string         `json:"error,omitempty"`
	Duration int64          `json:"duration,omitempty"` // nanoseconds
	Path     string         `json:"path,omitempty"`
	Content  string         `json:"content,omitempty"`
	Language string         `json:"language,omitempty"`
	Base64   string         `json:"base64,omitempty"`
	Media    string         `json:"mediaType,omitempty"`
}

// Usage tracks token consumption.
type Usage struct {
	InputTokens  int     `json:"inputTokens"`
	OutputTokens int     `json:"outputTokens"`
	CacheRead    int     `json:"cacheRead"`
	CacheWrite   int     `json:"cacheWrite"`
	InputCost    float64 `json:"inputCost"`
	OutputCost   float64 `json:"outputCost"`
	TotalCost    float64 `json:"totalCost"`
}

// SessionUsage tracks per-session usage.
type SessionUsage struct {
	SessionID    string    `json:"sessionID"`
	ProviderID   string    `json:"providerID"`
	ModelID      string    `json:"modelID"`
	Usage        Usage     `json:"usage"`
	MessageCount int       `json:"messageCount"`
	ToolCalls    int       `json:"toolCalls"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// GraphStore implements storage using urp's graph database.
// This allows opencode-go to use Memgraph as its backend.
type GraphStore struct {
	db graph.Driver
}

// NewGraphStore creates a store backed by the graph database.
func NewGraphStore(db graph.Driver) *GraphStore {
	return &GraphStore{db: db}
}

// --- SessionStore interface ---

// CreateSession stores a new session in the graph.
func (s *GraphStore) CreateSession(ctx context.Context, sess *Session) error {
	summaryJSON := ""
	if sess.Summary != nil {
		b, _ := json.Marshal(sess.Summary)
		summaryJSON = string(b)
	}

	query := `
		CREATE (s:Session:OpenCode {
			id: $id,
			projectID: $projectID,
			directory: $directory,
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
		"directory": sess.Directory,
		"parentID":  sess.ParentID,
		"title":     sess.Title,
		"version":   sess.Version,
		"createdAt": sess.CreatedAt.Unix(),
		"updatedAt": sess.UpdatedAt.Unix(),
		"summary":   summaryJSON,
	})
}

// GetSession retrieves a session by ID.
func (s *GraphStore) GetSession(ctx context.Context, id string) (*Session, error) {
	query := `
		MATCH (s:Session:OpenCode {id: $id})
		RETURN s.id as id, s.projectID as projectID, s.directory as directory,
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

// ListSessions returns sessions for a project.
func (s *GraphStore) ListSessions(ctx context.Context, projectID string, limit int) ([]*Session, error) {
	query := `
		MATCH (s:Session:OpenCode)
		WHERE $projectID = '' OR s.projectID = $projectID
		RETURN s.id as id, s.projectID as projectID, s.directory as directory,
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

	var sessions []*Session
	for _, r := range records {
		sess, err := recordToSession(r)
		if err != nil {
			continue
		}
		sessions = append(sessions, sess)
	}
	return sessions, nil
}

// UpdateSession updates an existing session.
func (s *GraphStore) UpdateSession(ctx context.Context, sess *Session) error {
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

// DeleteSession removes a session and its messages.
func (s *GraphStore) DeleteSession(ctx context.Context, id string) error {
	// Delete messages first
	query := `
		MATCH (m:Message:OpenCode {sessionID: $id})
		DETACH DELETE m
	`
	if err := s.db.ExecuteWrite(ctx, query, map[string]any{"id": id}); err != nil {
		return err
	}

	// Delete session
	query = `
		MATCH (s:Session:OpenCode {id: $id})
		DETACH DELETE s
	`
	return s.db.ExecuteWrite(ctx, query, map[string]any{"id": id})
}

// --- MessageStore interface ---

// CreateMessage stores a new message.
func (s *GraphStore) CreateMessage(ctx context.Context, msg *Message) error {
	partsJSON, _ := json.Marshal(msg.Parts)

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
		"role":      msg.Role,
		"parts":     string(partsJSON),
		"timestamp": msg.Timestamp.Unix(),
	})
}

// GetMessages retrieves all messages for a session.
func (s *GraphStore) GetMessages(ctx context.Context, sessionID string) ([]*Message, error) {
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

	var messages []*Message
	for _, r := range records {
		msg, err := recordToMessage(r)
		if err != nil {
			continue
		}
		messages = append(messages, msg)
	}
	return messages, nil
}

// UpdateMessage updates an existing message.
func (s *GraphStore) UpdateMessage(ctx context.Context, msg *Message) error {
	partsJSON, _ := json.Marshal(msg.Parts)

	query := `
		MATCH (m:Message:OpenCode {id: $id})
		SET m.parts = $parts
	`
	return s.db.ExecuteWrite(ctx, query, map[string]any{
		"id":    msg.ID,
		"parts": string(partsJSON),
	})
}

// DeleteMessage removes a message.
func (s *GraphStore) DeleteMessage(ctx context.Context, id string) error {
	query := `
		MATCH (m:Message:OpenCode {id: $id})
		DETACH DELETE m
	`
	return s.db.ExecuteWrite(ctx, query, map[string]any{"id": id})
}

// --- UsageStore interface ---

// GetUsage retrieves usage for a session.
func (s *GraphStore) GetUsage(ctx context.Context, sessionID string) (*SessionUsage, error) {
	query := `
		MATCH (u:Usage:OpenCode {sessionID: $sessionID})
		RETURN u.sessionID as sessionID, u.providerID as providerID,
		       u.modelID as modelID, u.usage as usage,
		       u.messageCount as messageCount, u.toolCalls as toolCalls,
		       u.updatedAt as updatedAt
	`
	records, err := s.db.Execute(ctx, query, map[string]any{"sessionID": sessionID})
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		// Return empty usage
		return &SessionUsage{SessionID: sessionID, UpdatedAt: time.Now()}, nil
	}

	return recordToUsage(records[0])
}

// UpdateUsage creates or updates session usage.
func (s *GraphStore) UpdateUsage(ctx context.Context, usage *SessionUsage) error {
	usageJSON, _ := json.Marshal(usage.Usage)

	query := `
		MERGE (u:Usage:OpenCode {sessionID: $sessionID})
		SET u.providerID = $providerID,
		    u.modelID = $modelID,
		    u.usage = $usage,
		    u.messageCount = $messageCount,
		    u.toolCalls = $toolCalls,
		    u.updatedAt = $updatedAt
	`
	return s.db.ExecuteWrite(ctx, query, map[string]any{
		"sessionID":    usage.SessionID,
		"providerID":   usage.ProviderID,
		"modelID":      usage.ModelID,
		"usage":        string(usageJSON),
		"messageCount": usage.MessageCount,
		"toolCalls":    usage.ToolCalls,
		"updatedAt":    usage.UpdatedAt.Unix(),
	})
}

// GetTotalUsage sums all usage across sessions.
func (s *GraphStore) GetTotalUsage(ctx context.Context) (*Usage, error) {
	query := `
		MATCH (u:Usage:OpenCode)
		RETURN u.usage as usage
	`
	records, err := s.db.Execute(ctx, query, nil)
	if err != nil {
		return nil, err
	}

	total := &Usage{}
	for _, r := range records {
		if usageStr, ok := r["usage"].(string); ok {
			var u Usage
			if json.Unmarshal([]byte(usageStr), &u) == nil {
				total.InputTokens += u.InputTokens
				total.OutputTokens += u.OutputTokens
				total.CacheRead += u.CacheRead
				total.CacheWrite += u.CacheWrite
				total.InputCost += u.InputCost
				total.OutputCost += u.OutputCost
				total.TotalCost += u.TotalCost
			}
		}
	}
	return total, nil
}

// --- Helpers ---

func recordToSession(r graph.Record) (*Session, error) {
	sess := &Session{
		ID:        getString(r, "id"),
		ProjectID: getString(r, "projectID"),
		Directory: getString(r, "directory"),
		ParentID:  getString(r, "parentID"),
		Title:     getString(r, "title"),
		Version:   getString(r, "version"),
		CreatedAt: time.Unix(getInt64(r, "createdAt"), 0),
		UpdatedAt: time.Unix(getInt64(r, "updatedAt"), 0),
	}

	if summaryStr := getString(r, "summary"); summaryStr != "" {
		var summary Summary
		if json.Unmarshal([]byte(summaryStr), &summary) == nil {
			sess.Summary = &summary
		}
	}

	return sess, nil
}

func recordToMessage(r graph.Record) (*Message, error) {
	msg := &Message{
		ID:        getString(r, "id"),
		SessionID: getString(r, "sessionID"),
		Role:      getString(r, "role"),
		Timestamp: time.Unix(getInt64(r, "timestamp"), 0),
	}

	if partsStr := getString(r, "parts"); partsStr != "" {
		var parts []Part
		if json.Unmarshal([]byte(partsStr), &parts) == nil {
			msg.Parts = parts
		}
	}

	return msg, nil
}

func recordToUsage(r graph.Record) (*SessionUsage, error) {
	su := &SessionUsage{
		SessionID:    getString(r, "sessionID"),
		ProviderID:   getString(r, "providerID"),
		ModelID:      getString(r, "modelID"),
		MessageCount: int(getInt64(r, "messageCount")),
		ToolCalls:    int(getInt64(r, "toolCalls")),
		UpdatedAt:    time.Unix(getInt64(r, "updatedAt"), 0),
	}

	if usageStr := getString(r, "usage"); usageStr != "" {
		json.Unmarshal([]byte(usageStr), &su.Usage)
	}

	return su, nil
}

func getString(r graph.Record, key string) string {
	if v, ok := r[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func getInt64(r graph.Record, key string) int64 {
	if v, ok := r[key]; ok {
		switch n := v.(type) {
		case int64:
			return n
		case int:
			return int64(n)
		case float64:
			return int64(n)
		}
	}
	return 0
}
