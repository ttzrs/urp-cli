package session

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/joss/urp/internal/opencode/domain"
)

// ─────────────────────────────────────────────────────────────────────────────
// Mock Store for Testing (DIP in action)
// ─────────────────────────────────────────────────────────────────────────────

type MockStore struct {
	sessions map[string]*domain.Session
	messages map[string][]*domain.Message
	usage    map[string]*domain.SessionUsage

	// Error injection
	CreateSessionErr error
	GetSessionErr    error
	ListSessionsErr  error
	CreateMessageErr error
	GetMessagesErr   error
	DeleteMessageErr error

	// Call tracking
	CreateSessionCalls int
	GetSessionCalls    int
	ListSessionsCalls  int
	CreateMessageCalls int
	GetMessagesCalls   int
	DeleteMessageCalls int
}

func NewMockStore() *MockStore {
	return &MockStore{
		sessions: make(map[string]*domain.Session),
		messages: make(map[string][]*domain.Message),
		usage:    make(map[string]*domain.SessionUsage),
	}
}

// Session operations
func (m *MockStore) CreateSession(ctx context.Context, sess *domain.Session) error {
	m.CreateSessionCalls++
	if m.CreateSessionErr != nil {
		return m.CreateSessionErr
	}
	m.sessions[sess.ID] = sess
	return nil
}

func (m *MockStore) GetSession(ctx context.Context, id string) (*domain.Session, error) {
	m.GetSessionCalls++
	if m.GetSessionErr != nil {
		return nil, m.GetSessionErr
	}
	return m.sessions[id], nil
}

func (m *MockStore) ListSessions(ctx context.Context, projectID string, limit int) ([]*domain.Session, error) {
	m.ListSessionsCalls++
	if m.ListSessionsErr != nil {
		return nil, m.ListSessionsErr
	}
	var result []*domain.Session
	for _, sess := range m.sessions {
		if sess.ProjectID == projectID {
			result = append(result, sess)
		}
	}
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (m *MockStore) UpdateSession(ctx context.Context, sess *domain.Session) error {
	m.sessions[sess.ID] = sess
	return nil
}

func (m *MockStore) DeleteSession(ctx context.Context, id string) error {
	delete(m.sessions, id)
	return nil
}

// Message operations
func (m *MockStore) CreateMessage(ctx context.Context, msg *domain.Message) error {
	m.CreateMessageCalls++
	if m.CreateMessageErr != nil {
		return m.CreateMessageErr
	}
	m.messages[msg.SessionID] = append(m.messages[msg.SessionID], msg)
	return nil
}

func (m *MockStore) GetMessages(ctx context.Context, sessionID string) ([]*domain.Message, error) {
	m.GetMessagesCalls++
	if m.GetMessagesErr != nil {
		return nil, m.GetMessagesErr
	}
	return m.messages[sessionID], nil
}

func (m *MockStore) UpdateMessage(ctx context.Context, msg *domain.Message) error {
	msgs := m.messages[msg.SessionID]
	for i, m := range msgs {
		if m.ID == msg.ID {
			msgs[i] = msg
			break
		}
	}
	return nil
}

func (m *MockStore) DeleteMessage(ctx context.Context, id string) error {
	m.DeleteMessageCalls++
	if m.DeleteMessageErr != nil {
		return m.DeleteMessageErr
	}
	for sessionID, msgs := range m.messages {
		for i, msg := range msgs {
			if msg.ID == id {
				m.messages[sessionID] = append(msgs[:i], msgs[i+1:]...)
				return nil
			}
		}
	}
	return nil
}

// Usage operations
func (m *MockStore) GetUsage(ctx context.Context, sessionID string) (*domain.SessionUsage, error) {
	return m.usage[sessionID], nil
}

func (m *MockStore) UpdateUsage(ctx context.Context, u *domain.SessionUsage) error {
	m.usage[u.SessionID] = u
	return nil
}

func (m *MockStore) GetTotalUsage(ctx context.Context) (*domain.Usage, error) {
	var total domain.Usage
	for _, u := range m.usage {
		total.InputTokens += u.Usage.InputTokens
		total.OutputTokens += u.Usage.OutputTokens
	}
	return &total, nil
}

// Verify MockStore implements domain.Store
var _ domain.Store = (*MockStore)(nil)

// ─────────────────────────────────────────────────────────────────────────────
// Manager Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestNewManager(t *testing.T) {
	store := NewMockStore()
	mgr := NewManager(store)
	require.NotNil(t, mgr)
}

func TestManager_Create(t *testing.T) {
	store := NewMockStore()
	mgr := NewManager(store)
	ctx := context.Background()

	sess, err := mgr.Create(ctx, "/test/project")

	require.NoError(t, err)
	require.NotNil(t, sess)
	assert.NotEmpty(t, sess.ID)
	assert.Equal(t, "/test/project", sess.Directory)
	assert.Equal(t, "New Session", sess.Title)
	assert.Equal(t, 1, store.CreateSessionCalls)
}

func TestManager_Get(t *testing.T) {
	store := NewMockStore()
	mgr := NewManager(store)
	ctx := context.Background()

	// Create a session first
	sess, _ := mgr.Create(ctx, "/test/project")

	// Get it back
	retrieved, err := mgr.Get(ctx, sess.ID)

	require.NoError(t, err)
	require.NotNil(t, retrieved)
	assert.Equal(t, sess.ID, retrieved.ID)
}

func TestManager_GetLatest(t *testing.T) {
	store := NewMockStore()
	mgr := NewManager(store)
	ctx := context.Background()

	// No sessions yet
	latest, err := mgr.GetLatest(ctx, "/test/project")
	require.NoError(t, err)
	assert.Nil(t, latest)

	// Create a session
	sess, _ := mgr.Create(ctx, "/test/project")

	// Now GetLatest should return it
	latest, err = mgr.GetLatest(ctx, "/test/project")
	require.NoError(t, err)
	require.NotNil(t, latest)
	assert.Equal(t, sess.ID, latest.ID)
}

func TestManager_GetOrCreate(t *testing.T) {
	store := NewMockStore()
	mgr := NewManager(store)
	ctx := context.Background()

	// First call creates
	sess1, err := mgr.GetOrCreate(ctx, "/test/project")
	require.NoError(t, err)
	require.NotNil(t, sess1)
	assert.Equal(t, 1, store.CreateSessionCalls)

	// Second call returns existing
	sess2, err := mgr.GetOrCreate(ctx, "/test/project")
	require.NoError(t, err)
	require.NotNil(t, sess2)
	assert.Equal(t, sess1.ID, sess2.ID)
	assert.Equal(t, 1, store.CreateSessionCalls) // Not called again
}

func TestManager_List(t *testing.T) {
	store := NewMockStore()
	mgr := NewManager(store)
	ctx := context.Background()

	// Create multiple sessions
	mgr.Create(ctx, "/test/project")
	mgr.Create(ctx, "/test/project")
	mgr.Create(ctx, "/other/project")

	// List for first project
	sessions, err := mgr.List(ctx, "/test/project", 10)
	require.NoError(t, err)
	assert.Len(t, sessions, 2)
}

func TestManager_Delete(t *testing.T) {
	store := NewMockStore()
	mgr := NewManager(store)
	ctx := context.Background()

	sess, _ := mgr.Create(ctx, "/test/project")
	err := mgr.Delete(ctx, sess.ID)
	require.NoError(t, err)

	// Should be gone
	retrieved, err := mgr.Get(ctx, sess.ID)
	require.NoError(t, err)
	assert.Nil(t, retrieved)
}

func TestManager_Fork(t *testing.T) {
	store := NewMockStore()
	mgr := NewManager(store)
	ctx := context.Background()

	// Create parent session
	parent, _ := mgr.Create(ctx, "/test/project")
	parent.Title = "Original Session"
	mgr.Update(ctx, parent)

	// Add some messages
	mgr.AddMessage(ctx, &domain.Message{
		ID:        "msg1",
		SessionID: parent.ID,
		Role:      domain.RoleUser,
		Parts:     []domain.Part{domain.TextPart{Text: "Hello"}},
	})
	mgr.AddMessage(ctx, &domain.Message{
		ID:        "msg2",
		SessionID: parent.ID,
		Role:      domain.RoleAssistant,
		Parts:     []domain.Part{domain.TextPart{Text: "Hi there"}},
	})

	// Fork
	forked, err := mgr.Fork(ctx, parent.ID)

	require.NoError(t, err)
	require.NotNil(t, forked)
	assert.NotEqual(t, parent.ID, forked.ID)
	assert.Equal(t, parent.ID, forked.ParentID)
	assert.Equal(t, "Original Session (fork)", forked.Title)

	// Forked session should have copies of messages
	msgs, _ := mgr.GetMessages(ctx, forked.ID)
	assert.Len(t, msgs, 2)
}

func TestManager_Messages(t *testing.T) {
	store := NewMockStore()
	mgr := NewManager(store)
	ctx := context.Background()

	sess, _ := mgr.Create(ctx, "/test/project")

	// Add messages
	msg := &domain.Message{
		ID:        "test-msg",
		SessionID: sess.ID,
		Role:      domain.RoleUser,
		Parts:     []domain.Part{domain.TextPart{Text: "Test message"}},
		Timestamp: time.Now(),
	}

	err := mgr.AddMessage(ctx, msg)
	require.NoError(t, err)

	// Retrieve messages
	msgs, err := mgr.GetMessages(ctx, sess.ID)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, "test-msg", msgs[0].ID)
}

func TestManager_Usage(t *testing.T) {
	store := NewMockStore()
	mgr := NewManager(store)
	ctx := context.Background()

	sess, _ := mgr.Create(ctx, "/test/project")

	// Record usage
	usage := &domain.Usage{
		InputTokens:  100,
		OutputTokens: 50,
	}
	err := mgr.RecordUsage(ctx, sess.ID, "anthropic", "claude-3", usage)
	require.NoError(t, err)

	// Get usage
	sessionUsage, err := mgr.GetUsage(ctx, sess.ID)
	require.NoError(t, err)
	require.NotNil(t, sessionUsage)
	assert.Equal(t, 100, sessionUsage.Usage.InputTokens)
	assert.Equal(t, 50, sessionUsage.Usage.OutputTokens)
}

func TestManager_RecordUsage_NilUsage(t *testing.T) {
	store := NewMockStore()
	mgr := NewManager(store)
	ctx := context.Background()

	// Should not error with nil usage
	err := mgr.RecordUsage(ctx, "sess-id", "provider", "model", nil)
	require.NoError(t, err)
}

func TestManager_GetTotalUsage(t *testing.T) {
	store := NewMockStore()
	mgr := NewManager(store)
	ctx := context.Background()

	// Create sessions with usage
	sess1, _ := mgr.Create(ctx, "/test/project1")
	sess2, _ := mgr.Create(ctx, "/test/project2")

	mgr.RecordUsage(ctx, sess1.ID, "anthropic", "claude-3", &domain.Usage{InputTokens: 100, OutputTokens: 50})
	mgr.RecordUsage(ctx, sess2.ID, "anthropic", "claude-3", &domain.Usage{InputTokens: 200, OutputTokens: 100})

	total, err := mgr.GetTotalUsage(ctx)
	require.NoError(t, err)
	assert.Equal(t, 300, total.InputTokens)
	assert.Equal(t, 150, total.OutputTokens)
}

// ─────────────────────────────────────────────────────────────────────────────
// Helper Function Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestProjectIDFromDir(t *testing.T) {
	// Same directory should produce same ID
	id1 := projectIDFromDir("/test/project")
	id2 := projectIDFromDir("/test/project")
	assert.Equal(t, id1, id2)

	// Different directories should produce different IDs
	id3 := projectIDFromDir("/other/project")
	assert.NotEqual(t, id1, id3)

	// ID should be hex string
	assert.Len(t, id1, 16) // 8 bytes = 16 hex chars
}

// ─────────────────────────────────────────────────────────────────────────────
// Title Generator Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestGenerateSimple(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
	}{
		{
			name:   "short message",
			input:  "Fix the login bug",
			maxLen: 50,
		},
		{
			name:   "long message truncated",
			input:  "This is a very long message that should be truncated because it exceeds the maximum allowed length for a title",
			maxLen: 50,
		},
		{
			name:   "with sentence ending",
			input:  "Fix the bug. Then deploy.",
			maxLen: 50,
		},
		{
			name:   "with newlines",
			input:  "Fix\nthe\nbug",
			maxLen: 50,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateSimple(tt.input)
			assert.LessOrEqual(t, len(result), tt.maxLen)
			assert.NotEmpty(t, result)
		})
	}
}

func TestGenerateSimple_EdgeCases(t *testing.T) {
	// Very short
	assert.Equal(t, "Hi", GenerateSimple("Hi"))

	// Exactly 50 chars
	msg50 := "12345678901234567890123456789012345678901234567890"
	result := GenerateSimple(msg50)
	assert.LessOrEqual(t, len(result), 50)

	// Empty spaces
	assert.NotEmpty(t, GenerateSimple("  hello  world  "))
}

// ─────────────────────────────────────────────────────────────────────────────
// Compact Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestEstimateTokens(t *testing.T) {
	messages := []*domain.Message{
		{
			Parts: []domain.Part{
				domain.TextPart{Text: "Hello world"}, // 11 chars / 4 = 2
			},
		},
		{
			Parts: []domain.Part{
				domain.TextPart{Text: "This is a test"}, // 14 chars / 4 = 3
			},
		},
	}

	tokens := EstimateTokens(messages)
	assert.Equal(t, 5, tokens) // 2 + 3 = 5
}

func TestEstimateTokens_WithToolCalls(t *testing.T) {
	messages := []*domain.Message{
		{
			Parts: []domain.Part{
				domain.ToolCallPart{
					Name:   "bash",
					Result: "output text here", // ~4 tokens
				},
			},
		},
	}

	tokens := EstimateTokens(messages)
	assert.Equal(t, 4, tokens) // 16 chars / 4
}

func TestEstimateTokens_Empty(t *testing.T) {
	tokens := EstimateTokens(nil)
	assert.Equal(t, 0, tokens)

	tokens = EstimateTokens([]*domain.Message{})
	assert.Equal(t, 0, tokens)
}

// ─────────────────────────────────────────────────────────────────────────────
// CompactResult Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestCompactResult(t *testing.T) {
	result := CompactResult{
		OriginalCount:   10,
		SummarizedCount: 7,
		NewCount:        4,
		Summary:         "Test summary",
		Skipped:         false,
	}

	assert.Equal(t, 10, result.OriginalCount)
	assert.Equal(t, 7, result.SummarizedCount)
	assert.Equal(t, 4, result.NewCount)
	assert.Equal(t, "Test summary", result.Summary)
	assert.False(t, result.Skipped)
}
