package graphstore

import (
	"context"
	"testing"
	"time"

	"github.com/joss/urp/internal/graph"
	"github.com/joss/urp/internal/opencode/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockDriver implements graph.Driver for testing.
type mockDriver struct {
	records     []graph.Record
	executeErr  error
	writeErr    error
	lastQuery   string
	lastParams  map[string]any
	writeCalled bool
}

func (m *mockDriver) Execute(ctx context.Context, query string, params map[string]any) ([]graph.Record, error) {
	m.lastQuery = query
	m.lastParams = params
	return m.records, m.executeErr
}

func (m *mockDriver) ExecuteWrite(ctx context.Context, query string, params map[string]any) error {
	m.writeCalled = true
	m.lastQuery = query
	m.lastParams = params
	return m.writeErr
}

func (m *mockDriver) Close() error { return nil }

func (m *mockDriver) Ping(ctx context.Context) error { return nil }

// --- Session Tests ---

func TestCreateSession(t *testing.T) {
	mock := &mockDriver{}
	store := New(mock)

	sess := &domain.Session{
		ID:        "sess-123",
		ProjectID: "proj-1",
		Directory: "/path/to/project",
		Title:     "Test Session",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err := store.CreateSession(context.Background(), sess)
	require.NoError(t, err)

	assert.True(t, mock.writeCalled)
	assert.Contains(t, mock.lastQuery, "CREATE")
	assert.Equal(t, "sess-123", mock.lastParams["id"])
	assert.Equal(t, "proj-1", mock.lastParams["projectID"])
	assert.Equal(t, "/path/to/project", mock.lastParams["workdir"])
}

func TestCreateSessionWithSummary(t *testing.T) {
	mock := &mockDriver{}
	store := New(mock)

	sess := &domain.Session{
		ID:        "sess-456",
		ProjectID: "proj-1",
		Summary: &domain.Summary{
			Additions: 100,
			Deletions: 50,
			Files:     []string{"file1.go", "file2.go"},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err := store.CreateSession(context.Background(), sess)
	require.NoError(t, err)

	// Summary should be JSON encoded
	summaryJSON, ok := mock.lastParams["summary"].(string)
	assert.True(t, ok)
	assert.Contains(t, summaryJSON, "additions")
}

func TestGetSession(t *testing.T) {
	mock := &mockDriver{
		records: []graph.Record{
			{
				"id":        "sess-123",
				"projectID": "proj-1",
				"workdir":   "/workspace",
				"parentID":  "",
				"title":     "Test",
				"version":   "1",
				"createdAt": int64(1700000000),
				"updatedAt": int64(1700000100),
				"summary":   "",
			},
		},
	}
	store := New(mock)

	sess, err := store.GetSession(context.Background(), "sess-123")
	require.NoError(t, err)
	require.NotNil(t, sess)

	assert.Equal(t, "sess-123", sess.ID)
	assert.Equal(t, "proj-1", sess.ProjectID)
	assert.Equal(t, "/workspace", sess.Directory)
}

func TestGetSessionNotFound(t *testing.T) {
	mock := &mockDriver{records: []graph.Record{}}
	store := New(mock)

	_, err := store.GetSession(context.Background(), "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestListSessions(t *testing.T) {
	mock := &mockDriver{
		records: []graph.Record{
			{
				"id":        "sess-1",
				"projectID": "proj-1",
				"workdir":   "/workspace",
				"parentID":  "",
				"title":     "Session 1",
				"version":   "1",
				"createdAt": int64(1700000000),
				"updatedAt": int64(1700000100),
				"summary":   "",
			},
			{
				"id":        "sess-2",
				"projectID": "proj-1",
				"workdir":   "/workspace",
				"parentID":  "",
				"title":     "Session 2",
				"version":   "1",
				"createdAt": int64(1700000200),
				"updatedAt": int64(1700000300),
				"summary":   "",
			},
		},
	}
	store := New(mock)

	sessions, err := store.ListSessions(context.Background(), "proj-1", 10)
	require.NoError(t, err)
	assert.Len(t, sessions, 2)
	assert.Equal(t, "sess-1", sessions[0].ID)
	assert.Equal(t, "sess-2", sessions[1].ID)
}

func TestUpdateSession(t *testing.T) {
	mock := &mockDriver{}
	store := New(mock)

	sess := &domain.Session{
		ID:        "sess-123",
		Title:     "Updated Title",
		UpdatedAt: time.Now(),
	}

	err := store.UpdateSession(context.Background(), sess)
	require.NoError(t, err)

	assert.True(t, mock.writeCalled)
	assert.Contains(t, mock.lastQuery, "SET")
	assert.Equal(t, "Updated Title", mock.lastParams["title"])
}

func TestDeleteSession(t *testing.T) {
	mock := &mockDriver{}
	store := New(mock)

	err := store.DeleteSession(context.Background(), "sess-123")
	require.NoError(t, err)

	assert.True(t, mock.writeCalled)
	assert.Contains(t, mock.lastQuery, "DETACH DELETE")
}

// --- Message Tests ---

func TestCreateMessage(t *testing.T) {
	mock := &mockDriver{}
	store := New(mock)

	msg := &domain.Message{
		ID:        "msg-123",
		SessionID: "sess-123",
		Role:      domain.RoleUser,
		Parts:     []domain.Part{domain.TextPart{Text: "Hello"}},
		Timestamp: time.Now(),
	}

	err := store.CreateMessage(context.Background(), msg)
	require.NoError(t, err)

	assert.True(t, mock.writeCalled)
	assert.Contains(t, mock.lastQuery, "CREATE")
	assert.Equal(t, "msg-123", mock.lastParams["id"])
	assert.Equal(t, "sess-123", mock.lastParams["sessionID"])
	assert.Equal(t, "user", mock.lastParams["role"])
}

func TestGetMessages(t *testing.T) {
	mock := &mockDriver{
		records: []graph.Record{
			{
				"id":        "msg-1",
				"sessionID": "sess-123",
				"role":      "user",
				"parts":     `[{"type":"text","text":"Hello"}]`,
				"timestamp": int64(1700000000),
			},
			{
				"id":        "msg-2",
				"sessionID": "sess-123",
				"role":      "assistant",
				"parts":     `[{"type":"text","text":"Hi there"}]`,
				"timestamp": int64(1700000100),
			},
		},
	}
	store := New(mock)

	messages, err := store.GetMessages(context.Background(), "sess-123")
	require.NoError(t, err)
	assert.Len(t, messages, 2)
	assert.Equal(t, domain.RoleUser, messages[0].Role)
	assert.Equal(t, domain.RoleAssistant, messages[1].Role)
}

func TestDeleteMessage(t *testing.T) {
	mock := &mockDriver{}
	store := New(mock)

	err := store.DeleteMessage(context.Background(), "msg-123")
	require.NoError(t, err)

	assert.True(t, mock.writeCalled)
	assert.Contains(t, mock.lastQuery, "DELETE")
}

// --- Usage Tests ---

func TestGetUsageEmpty(t *testing.T) {
	mock := &mockDriver{records: []graph.Record{}}
	store := New(mock)

	usage, err := store.GetUsage(context.Background(), "sess-123")
	require.NoError(t, err)
	require.NotNil(t, usage)
	assert.Equal(t, "sess-123", usage.SessionID)
}

func TestGetUsage(t *testing.T) {
	mock := &mockDriver{
		records: []graph.Record{
			{
				"sessionID":    "sess-123",
				"providerID":   "anthropic",
				"modelID":      "claude-3",
				"inputTokens":  int64(1000),
				"outputTokens": int64(500),
				"cacheRead":    int64(100),
				"cacheWrite":   int64(50),
				"inputCost":    0.01,
				"outputCost":   0.02,
				"totalCost":    0.03,
				"messageCount": int64(10),
				"toolCalls":    int64(5),
				"updatedAt":    int64(1700000000),
			},
		},
	}
	store := New(mock)

	usage, err := store.GetUsage(context.Background(), "sess-123")
	require.NoError(t, err)

	assert.Equal(t, "anthropic", usage.ProviderID)
	assert.Equal(t, 1000, usage.Usage.InputTokens)
	assert.Equal(t, 500, usage.Usage.OutputTokens)
	assert.Equal(t, 10, usage.MessageCount)
}

func TestUpdateUsage(t *testing.T) {
	mock := &mockDriver{}
	store := New(mock)

	usage := &domain.SessionUsage{
		SessionID:    "sess-123",
		ProviderID:   "anthropic",
		ModelID:      "claude-3",
		MessageCount: 10,
		ToolCalls:    5,
		UpdatedAt:    time.Now(),
		Usage: domain.Usage{
			InputTokens:  1000,
			OutputTokens: 500,
			TotalCost:    0.03,
		},
	}

	err := store.UpdateUsage(context.Background(), usage)
	require.NoError(t, err)

	assert.True(t, mock.writeCalled)
	assert.Contains(t, mock.lastQuery, "MERGE")
}

func TestGetTotalUsage(t *testing.T) {
	mock := &mockDriver{
		records: []graph.Record{
			{
				"inputTokens":  int64(5000),
				"outputTokens": int64(2500),
				"cacheRead":    int64(500),
				"cacheWrite":   int64(250),
				"inputCost":    0.05,
				"outputCost":   0.10,
				"totalCost":    0.15,
			},
		},
	}
	store := New(mock)

	usage, err := store.GetTotalUsage(context.Background())
	require.NoError(t, err)

	assert.Equal(t, 5000, usage.InputTokens)
	assert.Equal(t, 2500, usage.OutputTokens)
	assert.Equal(t, 0.15, usage.TotalCost)
}

// --- Helper Tests ---

func TestRecordToSessionWithSummary(t *testing.T) {
	r := graph.Record{
		"id":        "sess-123",
		"projectID": "proj-1",
		"workdir":   "/workspace",
		"parentID":  "",
		"title":     "Test",
		"version":   "1",
		"createdAt": int64(1700000000),
		"updatedAt": int64(1700000100),
		"summary":   `{"additions":100,"deletions":50,"files":["file1.go"]}`,
	}

	sess, err := recordToSession(r)
	require.NoError(t, err)
	require.NotNil(t, sess.Summary)
	assert.Equal(t, 100, sess.Summary.Additions)
	assert.Equal(t, 50, sess.Summary.Deletions)
}
