package session

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/joss/urp/internal/opencode/domain"
)

// Manager handles session lifecycle
type Manager struct {
	store domain.Store // Uses all store operations (sessions, messages, usage)
}

// NewManager creates a Manager with the given storage (accepts interface)
func NewManager(s domain.Store) *Manager {
	return &Manager{store: s}
}

// Create creates a new session for the given directory
func (m *Manager) Create(ctx context.Context, dir string) (*domain.Session, error) {
	projectID := projectIDFromDir(dir)

	sess := &domain.Session{
		ID:        ulid.Make().String(),
		ProjectID: projectID,
		Directory: dir,
		Title:     "New Session",
		Version:   "1.0.0",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := m.store.CreateSession(ctx, sess); err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	return sess, nil
}

// Get retrieves a session by ID
func (m *Manager) Get(ctx context.Context, id string) (*domain.Session, error) {
	return m.store.GetSession(ctx, id)
}

// GetLatest gets the most recent session for a directory
func (m *Manager) GetLatest(ctx context.Context, dir string) (*domain.Session, error) {
	projectID := projectIDFromDir(dir)
	sessions, err := m.store.ListSessions(ctx, projectID, 1)
	if err != nil {
		return nil, err
	}
	if len(sessions) == 0 {
		return nil, nil
	}
	return sessions[0], nil
}

// GetOrCreate gets the latest session or creates a new one
func (m *Manager) GetOrCreate(ctx context.Context, dir string) (*domain.Session, error) {
	sess, err := m.GetLatest(ctx, dir)
	if err != nil {
		return nil, err
	}
	if sess != nil {
		return sess, nil
	}
	return m.Create(ctx, dir)
}

// List returns sessions for a directory
func (m *Manager) List(ctx context.Context, dir string, limit int) ([]*domain.Session, error) {
	projectID := projectIDFromDir(dir)
	return m.store.ListSessions(ctx, projectID, limit)
}

// Update updates session metadata
func (m *Manager) Update(ctx context.Context, sess *domain.Session) error {
	return m.store.UpdateSession(ctx, sess)
}

// Delete removes a session
func (m *Manager) Delete(ctx context.Context, id string) error {
	return m.store.DeleteSession(ctx, id)
}

// Fork creates a child session from an existing one
func (m *Manager) Fork(ctx context.Context, parentID string) (*domain.Session, error) {
	parent, err := m.Get(ctx, parentID)
	if err != nil {
		return nil, fmt.Errorf("get parent: %w", err)
	}

	sess := &domain.Session{
		ID:        ulid.Make().String(),
		ProjectID: parent.ProjectID,
		Directory: parent.Directory,
		ParentID:  parent.ID,
		Title:     parent.Title + " (fork)",
		Version:   parent.Version,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := m.store.CreateSession(ctx, sess); err != nil {
		return nil, fmt.Errorf("create fork: %w", err)
	}

	// Copy messages from parent
	messages, err := m.store.GetMessages(ctx, parentID)
	if err != nil {
		return nil, fmt.Errorf("get messages: %w", err)
	}

	for _, msg := range messages {
		newMsg := *msg
		newMsg.ID = ulid.Make().String()
		newMsg.SessionID = sess.ID
		if err := m.store.CreateMessage(ctx, &newMsg); err != nil {
			return nil, fmt.Errorf("copy message: %w", err)
		}
	}

	return sess, nil
}

// AddMessage adds a message to a session
func (m *Manager) AddMessage(ctx context.Context, msg *domain.Message) error {
	return m.store.CreateMessage(ctx, msg)
}

// GetMessages retrieves all messages for a session
func (m *Manager) GetMessages(ctx context.Context, sessionID string) ([]*domain.Message, error) {
	return m.store.GetMessages(ctx, sessionID)
}

// UpdateMessage updates an existing message
func (m *Manager) UpdateMessage(ctx context.Context, msg *domain.Message) error {
	return m.store.UpdateMessage(ctx, msg)
}

// GetUsage retrieves usage stats for a session
func (m *Manager) GetUsage(ctx context.Context, sessionID string) (*domain.SessionUsage, error) {
	return m.store.GetUsage(ctx, sessionID)
}

// RecordUsage records usage from a stream event
func (m *Manager) RecordUsage(ctx context.Context, sessionID, providerID, modelID string, usage *domain.Usage) error {
	if usage == nil {
		return nil
	}
	su := &domain.SessionUsage{
		SessionID:  sessionID,
		ProviderID: providerID,
		ModelID:    modelID,
		Usage:      *usage,
	}
	return m.store.UpdateUsage(ctx, su)
}

// GetTotalUsage retrieves aggregate usage across all sessions
func (m *Manager) GetTotalUsage(ctx context.Context) (*domain.Usage, error) {
	return m.store.GetTotalUsage(ctx)
}

func projectIDFromDir(dir string) string {
	absDir, _ := filepath.Abs(dir)
	if absDir == "" {
		absDir = dir
	}

	// Try to find git root
	gitRoot := findGitRoot(absDir)
	if gitRoot != "" {
		absDir = gitRoot
	}

	hash := sha256.Sum256([]byte(absDir))
	return hex.EncodeToString(hash[:8])
}

func findGitRoot(dir string) string {
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
