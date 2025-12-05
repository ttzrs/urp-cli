package session

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/joss/urp/internal/opencode/domain"
)

// ShareFormat represents an exported session
type ShareFormat struct {
	Version   string            `json:"version"`
	ExportedAt time.Time        `json:"exported_at"`
	Session   *domain.Session   `json:"session"`
	Messages  []*domain.Message `json:"messages"`
	Usage     *domain.SessionUsage `json:"usage,omitempty"`
}

// Export exports a session to JSON format
func (m *Manager) Export(ctx context.Context, sessionID string) ([]byte, error) {
	sess, err := m.Get(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}
	if sess == nil {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	messages, err := m.GetMessages(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("get messages: %w", err)
	}

	usage, _ := m.GetUsage(ctx, sessionID) // ignore errors

	share := ShareFormat{
		Version:    "1.0.0",
		ExportedAt: time.Now(),
		Session:    sess,
		Messages:   messages,
		Usage:      usage,
	}

	data, err := json.MarshalIndent(share, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	return data, nil
}

// Import imports a session from JSON format
func (m *Manager) Import(ctx context.Context, data []byte, newDir string) (*domain.Session, error) {
	var share ShareFormat
	if err := json.Unmarshal(data, &share); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}

	if share.Session == nil {
		return nil, fmt.Errorf("invalid share format: no session")
	}

	// Create new session with new ID
	now := time.Now()
	sess := &domain.Session{
		ID:        ulid.Make().String(),
		ProjectID: projectIDFromDir(newDir),
		Directory: newDir,
		ParentID:  share.Session.ID, // Link to original
		Title:     share.Session.Title + " (imported)",
		Version:   share.Version,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := m.store.CreateSession(ctx, sess); err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	// Import messages with new IDs
	for _, msg := range share.Messages {
		newMsg := *msg
		newMsg.ID = ulid.Make().String()
		newMsg.SessionID = sess.ID
		if err := m.store.CreateMessage(ctx, &newMsg); err != nil {
			return nil, fmt.Errorf("create message: %w", err)
		}
	}

	return sess, nil
}

// ExportSummary exports a session summary (no full message content)
func (m *Manager) ExportSummary(ctx context.Context, sessionID string) ([]byte, error) {
	sess, err := m.Get(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}

	messages, err := m.GetMessages(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("get messages: %w", err)
	}

	// Summarize messages (just roles and timestamps)
	type msgSummary struct {
		Role      domain.Role `json:"role"`
		Timestamp time.Time   `json:"timestamp"`
		HasTools  bool        `json:"has_tools,omitempty"`
	}

	summaries := make([]msgSummary, len(messages))
	for i, msg := range messages {
		hasTools := false
		for _, part := range msg.Parts {
			if _, ok := part.(domain.ToolCallPart); ok {
				hasTools = true
				break
			}
		}
		summaries[i] = msgSummary{
			Role:      msg.Role,
			Timestamp: msg.Timestamp,
			HasTools:  hasTools,
		}
	}

	summary := struct {
		Version    string       `json:"version"`
		Session    *domain.Session `json:"session"`
		MessageCount int        `json:"message_count"`
		Messages   []msgSummary `json:"messages"`
	}{
		Version:      "1.0.0",
		Session:      sess,
		MessageCount: len(messages),
		Messages:     summaries,
	}

	return json.MarshalIndent(summary, "", "  ")
}
