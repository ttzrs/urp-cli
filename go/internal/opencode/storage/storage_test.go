package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/joss/urp/internal/opencode/domain"
)

// ─────────────────────────────────────────────────────────────────────────────
// Test Helpers
// ─────────────────────────────────────────────────────────────────────────────

func setupTestStorage(t *testing.T) (*Storage, func()) {
	t.Helper()

	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "storage_test_*")
	require.NoError(t, err)

	storage, err := New(tmpDir)
	require.NoError(t, err)

	cleanup := func() {
		storage.Close()
		os.RemoveAll(tmpDir)
	}

	return storage, cleanup
}

// ─────────────────────────────────────────────────────────────────────────────
// Constructor Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestNew(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "storage_test_*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	storage, err := New(tmpDir)
	require.NoError(t, err)
	require.NotNil(t, storage)
	defer storage.Close()

	// Database file should exist
	dbPath := filepath.Join(tmpDir, "opencode.db")
	_, err = os.Stat(dbPath)
	assert.NoError(t, err)
}

func TestNew_CreatesDirectory(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "storage_test_*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Use a non-existent subdirectory
	subDir := filepath.Join(tmpDir, "new", "nested", "dir")

	storage, err := New(subDir)
	require.NoError(t, err)
	require.NotNil(t, storage)
	defer storage.Close()

	// Directory should be created
	_, err = os.Stat(subDir)
	assert.NoError(t, err)
}

// ─────────────────────────────────────────────────────────────────────────────
// Session Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestStorage_CreateSession(t *testing.T) {
	storage, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()

	sess := &domain.Session{
		ID:        "test-session-1",
		ProjectID: "project-123",
		Directory: "/test/project",
		Title:     "Test Session",
		Version:   "1.0.0",
		CreatedAt: now,
		UpdatedAt: now,
	}

	err := storage.CreateSession(ctx, sess)
	require.NoError(t, err)
}

func TestStorage_GetSession(t *testing.T) {
	storage, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().Truncate(time.Second) // SQLite truncates to seconds

	sess := &domain.Session{
		ID:        "test-session-1",
		ProjectID: "project-123",
		Directory: "/test/project",
		Title:     "Test Session",
		Version:   "1.0.0",
		CreatedAt: now,
		UpdatedAt: now,
	}

	err := storage.CreateSession(ctx, sess)
	require.NoError(t, err)

	// Retrieve it
	retrieved, err := storage.GetSession(ctx, "test-session-1")
	require.NoError(t, err)
	require.NotNil(t, retrieved)

	assert.Equal(t, sess.ID, retrieved.ID)
	assert.Equal(t, sess.ProjectID, retrieved.ProjectID)
	assert.Equal(t, sess.Directory, retrieved.Directory)
	assert.Equal(t, sess.Title, retrieved.Title)
	assert.Equal(t, sess.Version, retrieved.Version)
}

func TestStorage_GetSession_WithParent(t *testing.T) {
	storage, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()

	sess := &domain.Session{
		ID:        "child-session",
		ProjectID: "project-123",
		ParentID:  "parent-session",
		Directory: "/test/project",
		Title:     "Child Session",
		Version:   "1.0.0",
		CreatedAt: now,
		UpdatedAt: now,
	}

	err := storage.CreateSession(ctx, sess)
	require.NoError(t, err)

	retrieved, err := storage.GetSession(ctx, "child-session")
	require.NoError(t, err)
	assert.Equal(t, "parent-session", retrieved.ParentID)
}

func TestStorage_ListSessions(t *testing.T) {
	storage, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()

	// Create multiple sessions
	for i := 0; i < 5; i++ {
		sess := &domain.Session{
			ID:        "session-" + string(rune('A'+i)),
			ProjectID: "project-123",
			Directory: "/test/project",
			Title:     "Session " + string(rune('A'+i)),
			Version:   "1.0.0",
			CreatedAt: now,
			UpdatedAt: now.Add(time.Duration(i) * time.Minute),
		}
		err := storage.CreateSession(ctx, sess)
		require.NoError(t, err)
	}

	// List with limit
	sessions, err := storage.ListSessions(ctx, "project-123", 3)
	require.NoError(t, err)
	assert.Len(t, sessions, 3)

	// Should be ordered by updated_at DESC
	assert.Equal(t, "session-E", sessions[0].ID)
}

func TestStorage_ListSessions_DifferentProjects(t *testing.T) {
	storage, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()

	// Sessions for project A
	for i := 0; i < 3; i++ {
		sess := &domain.Session{
			ID:        "sess-a-" + string(rune('1'+i)),
			ProjectID: "project-A",
			Directory: "/test/a",
			Title:     "Session A" + string(rune('1'+i)),
			Version:   "1.0.0",
			CreatedAt: now,
			UpdatedAt: now,
		}
		storage.CreateSession(ctx, sess)
	}

	// Sessions for project B
	for i := 0; i < 2; i++ {
		sess := &domain.Session{
			ID:        "sess-b-" + string(rune('1'+i)),
			ProjectID: "project-B",
			Directory: "/test/b",
			Title:     "Session B" + string(rune('1'+i)),
			Version:   "1.0.0",
			CreatedAt: now,
			UpdatedAt: now,
		}
		storage.CreateSession(ctx, sess)
	}

	// List only project A
	sessionsA, err := storage.ListSessions(ctx, "project-A", 10)
	require.NoError(t, err)
	assert.Len(t, sessionsA, 3)

	// List only project B
	sessionsB, err := storage.ListSessions(ctx, "project-B", 10)
	require.NoError(t, err)
	assert.Len(t, sessionsB, 2)
}

func TestStorage_UpdateSession(t *testing.T) {
	storage, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()

	sess := &domain.Session{
		ID:        "test-session",
		ProjectID: "project-123",
		Directory: "/test/project",
		Title:     "Original Title",
		Version:   "1.0.0",
		CreatedAt: now,
		UpdatedAt: now,
	}

	err := storage.CreateSession(ctx, sess)
	require.NoError(t, err)

	// Update
	sess.Title = "Updated Title"
	err = storage.UpdateSession(ctx, sess)
	require.NoError(t, err)

	// Verify
	retrieved, err := storage.GetSession(ctx, "test-session")
	require.NoError(t, err)
	assert.Equal(t, "Updated Title", retrieved.Title)
}

func TestStorage_DeleteSession(t *testing.T) {
	storage, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()

	sess := &domain.Session{
		ID:        "to-delete",
		ProjectID: "project-123",
		Directory: "/test/project",
		Title:     "Delete Me",
		Version:   "1.0.0",
		CreatedAt: now,
		UpdatedAt: now,
	}

	err := storage.CreateSession(ctx, sess)
	require.NoError(t, err)

	// Delete
	err = storage.DeleteSession(ctx, "to-delete")
	require.NoError(t, err)

	// Should be gone
	_, err = storage.GetSession(ctx, "to-delete")
	assert.Error(t, err) // sql.ErrNoRows
}

// ─────────────────────────────────────────────────────────────────────────────
// Message Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestStorage_CreateMessage(t *testing.T) {
	storage, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()

	// Create session first
	sess := &domain.Session{
		ID:        "sess-1",
		ProjectID: "project-123",
		Directory: "/test",
		Title:     "Test",
		Version:   "1.0.0",
		CreatedAt: now,
		UpdatedAt: now,
	}
	storage.CreateSession(ctx, sess)

	msg := &domain.Message{
		ID:        "msg-1",
		SessionID: "sess-1",
		Role:      domain.RoleUser,
		Parts: []domain.Part{
			domain.TextPart{Text: "Hello, world!"},
		},
		Timestamp: now,
	}

	err := storage.CreateMessage(ctx, msg)
	require.NoError(t, err)
}

func TestStorage_GetMessages(t *testing.T) {
	storage, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()

	// Create session
	sess := &domain.Session{
		ID:        "sess-1",
		ProjectID: "project-123",
		Directory: "/test",
		Title:     "Test",
		Version:   "1.0.0",
		CreatedAt: now,
		UpdatedAt: now,
	}
	storage.CreateSession(ctx, sess)

	// Create messages
	for i := 0; i < 3; i++ {
		msg := &domain.Message{
			ID:        "msg-" + string(rune('1'+i)),
			SessionID: "sess-1",
			Role:      domain.RoleUser,
			Parts: []domain.Part{
				domain.TextPart{Text: "Message " + string(rune('1'+i))},
			},
			Timestamp: now.Add(time.Duration(i) * time.Minute),
		}
		storage.CreateMessage(ctx, msg)
	}

	// Retrieve
	messages, err := storage.GetMessages(ctx, "sess-1")
	require.NoError(t, err)
	assert.Len(t, messages, 3)

	// Should be ordered by timestamp ASC
	assert.Equal(t, "msg-1", messages[0].ID)
	assert.Equal(t, "msg-3", messages[2].ID)
}

func TestStorage_GetMessages_WithToolCall(t *testing.T) {
	storage, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()

	// Create session
	sess := &domain.Session{
		ID:        "sess-1",
		ProjectID: "project-123",
		Directory: "/test",
		Title:     "Test",
		Version:   "1.0.0",
		CreatedAt: now,
		UpdatedAt: now,
	}
	storage.CreateSession(ctx, sess)

	msg := &domain.Message{
		ID:        "msg-tool",
		SessionID: "sess-1",
		Role:      domain.RoleAssistant,
		Parts: []domain.Part{
			domain.TextPart{Text: "Let me run that command"},
			domain.ToolCallPart{
				ToolID: "tool-1",
				Name:   "bash",
				Args:   map[string]any{"command": "ls -la"},
				Result: "file1.txt\nfile2.txt",
			},
		},
		Timestamp: now,
	}

	err := storage.CreateMessage(ctx, msg)
	require.NoError(t, err)

	// Retrieve
	messages, err := storage.GetMessages(ctx, "sess-1")
	require.NoError(t, err)
	require.Len(t, messages, 1)
	assert.Len(t, messages[0].Parts, 2)
}

func TestStorage_UpdateMessage(t *testing.T) {
	storage, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()

	// Create session and message
	sess := &domain.Session{
		ID:        "sess-1",
		ProjectID: "project-123",
		Directory: "/test",
		Title:     "Test",
		Version:   "1.0.0",
		CreatedAt: now,
		UpdatedAt: now,
	}
	storage.CreateSession(ctx, sess)

	msg := &domain.Message{
		ID:        "msg-1",
		SessionID: "sess-1",
		Role:      domain.RoleUser,
		Parts: []domain.Part{
			domain.TextPart{Text: "Original text"},
		},
		Timestamp: now,
	}
	storage.CreateMessage(ctx, msg)

	// Update
	msg.Parts = []domain.Part{
		domain.TextPart{Text: "Updated text"},
	}
	err := storage.UpdateMessage(ctx, msg)
	require.NoError(t, err)

	// Verify
	messages, _ := storage.GetMessages(ctx, "sess-1")
	textPart := messages[0].Parts[0].(domain.TextPart)
	assert.Equal(t, "Updated text", textPart.Text)
}

func TestStorage_DeleteMessage(t *testing.T) {
	storage, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()

	// Create session and messages
	sess := &domain.Session{
		ID:        "sess-1",
		ProjectID: "project-123",
		Directory: "/test",
		Title:     "Test",
		Version:   "1.0.0",
		CreatedAt: now,
		UpdatedAt: now,
	}
	storage.CreateSession(ctx, sess)

	storage.CreateMessage(ctx, &domain.Message{
		ID:        "msg-1",
		SessionID: "sess-1",
		Role:      domain.RoleUser,
		Parts:     []domain.Part{domain.TextPart{Text: "Keep"}},
		Timestamp: now,
	})
	storage.CreateMessage(ctx, &domain.Message{
		ID:        "msg-2",
		SessionID: "sess-1",
		Role:      domain.RoleUser,
		Parts:     []domain.Part{domain.TextPart{Text: "Delete"}},
		Timestamp: now,
	})

	// Delete one
	err := storage.DeleteMessage(ctx, "msg-2")
	require.NoError(t, err)

	// Verify
	messages, _ := storage.GetMessages(ctx, "sess-1")
	assert.Len(t, messages, 1)
	assert.Equal(t, "msg-1", messages[0].ID)
}

// ─────────────────────────────────────────────────────────────────────────────
// Config Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestStorage_Config(t *testing.T) {
	storage, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	// Set config
	err := storage.SetConfig(ctx, "test-key", "test-value")
	require.NoError(t, err)

	// Get config
	value, err := storage.GetConfig(ctx, "test-key")
	require.NoError(t, err)
	assert.Equal(t, "test-value", value)

	// Update config (upsert)
	err = storage.SetConfig(ctx, "test-key", "new-value")
	require.NoError(t, err)

	value, err = storage.GetConfig(ctx, "test-key")
	require.NoError(t, err)
	assert.Equal(t, "new-value", value)
}

func TestStorage_GetConfig_NotFound(t *testing.T) {
	storage, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	_, err := storage.GetConfig(ctx, "nonexistent")
	assert.Error(t, err)
}

// ─────────────────────────────────────────────────────────────────────────────
// Usage Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestStorage_Usage(t *testing.T) {
	storage, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()

	// Create session first
	sess := &domain.Session{
		ID:        "sess-1",
		ProjectID: "project-123",
		Directory: "/test",
		Title:     "Test",
		Version:   "1.0.0",
		CreatedAt: now,
		UpdatedAt: now,
	}
	storage.CreateSession(ctx, sess)

	// Create usage
	usage := &domain.SessionUsage{
		SessionID:  "sess-1",
		ProviderID: "anthropic",
		ModelID:    "claude-3-opus",
		Usage: domain.Usage{
			InputTokens:  100,
			OutputTokens: 50,
			TotalCost:    0.015,
		},
		MessageCount: 2,
		ToolCalls:    1,
	}

	err := storage.UpdateUsage(ctx, usage)
	require.NoError(t, err)

	// Get usage
	retrieved, err := storage.GetUsage(ctx, "sess-1")
	require.NoError(t, err)
	require.NotNil(t, retrieved)
	assert.Equal(t, 100, retrieved.Usage.InputTokens)
	assert.Equal(t, 50, retrieved.Usage.OutputTokens)
}

func TestStorage_UpdateUsage_Accumulates(t *testing.T) {
	storage, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()

	// Create session
	sess := &domain.Session{
		ID:        "sess-1",
		ProjectID: "project-123",
		Directory: "/test",
		Title:     "Test",
		Version:   "1.0.0",
		CreatedAt: now,
		UpdatedAt: now,
	}
	storage.CreateSession(ctx, sess)

	// First usage update
	usage1 := &domain.SessionUsage{
		SessionID:  "sess-1",
		ProviderID: "anthropic",
		ModelID:    "claude-3",
		Usage:      domain.Usage{InputTokens: 100, OutputTokens: 50},
	}
	storage.UpdateUsage(ctx, usage1)

	// Second usage update (should accumulate)
	usage2 := &domain.SessionUsage{
		SessionID:  "sess-1",
		ProviderID: "anthropic",
		ModelID:    "claude-3",
		Usage:      domain.Usage{InputTokens: 200, OutputTokens: 100},
	}
	storage.UpdateUsage(ctx, usage2)

	// Verify accumulation
	retrieved, err := storage.GetUsage(ctx, "sess-1")
	require.NoError(t, err)
	assert.Equal(t, 300, retrieved.Usage.InputTokens)  // 100 + 200
	assert.Equal(t, 150, retrieved.Usage.OutputTokens) // 50 + 100
}

func TestStorage_GetUsage_NotFound(t *testing.T) {
	storage, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	usage, err := storage.GetUsage(ctx, "nonexistent")
	require.NoError(t, err) // No error, just nil
	assert.Nil(t, usage)
}

func TestStorage_GetTotalUsage(t *testing.T) {
	storage, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()

	// Create sessions with usage
	for i := 0; i < 3; i++ {
		sess := &domain.Session{
			ID:        "sess-" + string(rune('1'+i)),
			ProjectID: "project-123",
			Directory: "/test",
			Title:     "Test",
			Version:   "1.0.0",
			CreatedAt: now,
			UpdatedAt: now,
		}
		storage.CreateSession(ctx, sess)

		usage := &domain.SessionUsage{
			SessionID:  "sess-" + string(rune('1'+i)),
			ProviderID: "anthropic",
			ModelID:    "claude-3",
			Usage:      domain.Usage{InputTokens: 100, OutputTokens: 50},
		}
		storage.UpdateUsage(ctx, usage)
	}

	total, err := storage.GetTotalUsage(ctx)
	require.NoError(t, err)
	assert.Equal(t, 300, total.InputTokens)  // 3 * 100
	assert.Equal(t, 150, total.OutputTokens) // 3 * 50
}

func TestStorage_GetTotalUsage_Empty(t *testing.T) {
	storage, cleanup := setupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	total, err := storage.GetTotalUsage(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, total.InputTokens)
	assert.Equal(t, 0, total.OutputTokens)
}

// ─────────────────────────────────────────────────────────────────────────────
// Interface Compliance Test
// ─────────────────────────────────────────────────────────────────────────────

func TestStorage_ImplementsStore(t *testing.T) {
	var _ domain.Store = (*Storage)(nil)
}

// ─────────────────────────────────────────────────────────────────────────────
// Close Test
// ─────────────────────────────────────────────────────────────────────────────

func TestStorage_Close(t *testing.T) {
	storage, cleanup := setupTestStorage(t)

	err := storage.Close()
	require.NoError(t, err)

	// Calling cleanup after close should not panic
	cleanup()
}
