package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/joss/urp/internal/graph"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- KnowledgeType Tests ---

func TestKnowledgeTypes(t *testing.T) {
	types := []KnowledgeType{
		TypeSolutions,
		TypeMemories,
		TypeKnowledge,
		TypeSkills,
		TypeSessions,
		TypeVectors,
		TypeAll,
	}

	for _, kt := range types {
		assert.NotEmpty(t, string(kt))
	}
}

// --- BackupMetadata Tests ---

func TestBackupMetadata(t *testing.T) {
	meta := &BackupMetadata{
		Version:     "1.0",
		CreatedAt:   time.Now(),
		Project:     "test-project",
		Description: "Test backup",
		Types:       []KnowledgeType{TypeSolutions, TypeMemories},
		Counts:      map[string]int{"solutions": 10, "memories": 5},
		Checksums:   map[string]string{"solutions.json": "abc123"},
	}

	assert.Equal(t, "1.0", meta.Version)
	assert.Equal(t, "test-project", meta.Project)
	assert.Len(t, meta.Types, 2)
	assert.Equal(t, 10, meta.Counts["solutions"])
}

func TestBackupMetadataJSON(t *testing.T) {
	meta := &BackupMetadata{
		Version:     "1.0",
		CreatedAt:   time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
		Project:     "test",
		Description: "desc",
		Types:       []KnowledgeType{TypeSolutions},
		Counts:      map[string]int{"solutions": 5},
	}

	data, err := json.Marshal(meta)
	require.NoError(t, err)

	var parsed BackupMetadata
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)

	assert.Equal(t, meta.Version, parsed.Version)
	assert.Equal(t, meta.Project, parsed.Project)
}

// --- Helper Function Tests ---

func TestContains(t *testing.T) {
	types := []KnowledgeType{TypeSolutions, TypeMemories}

	assert.True(t, contains(types, TypeSolutions))
	assert.True(t, contains(types, TypeMemories))
	assert.False(t, contains(types, TypeSkills))
	assert.False(t, contains(types, TypeAll))
}

func TestContainsEmpty(t *testing.T) {
	var types []KnowledgeType
	assert.False(t, contains(types, TypeSolutions))
}

func TestAddToTar(t *testing.T) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	data := []byte(`{"test": "data"}`)
	err := addToTar(tw, "test.json", data)
	require.NoError(t, err)

	tw.Close()

	// Read back
	tr := tar.NewReader(&buf)
	header, err := tr.Next()
	require.NoError(t, err)

	assert.Equal(t, "test.json", header.Name)
	assert.Equal(t, int64(len(data)), header.Size)
}

// --- BackupManager Tests (with mock) ---

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

func TestNewBackupManager(t *testing.T) {
	mock := &mockDriver{}
	mgr := NewBackupManager(mock, "/data")

	assert.NotNil(t, mgr)
	assert.Equal(t, "/data", mgr.dataDir)
}

func TestExportType(t *testing.T) {
	mock := &mockDriver{
		records: []graph.Record{
			{"s": map[string]any{"id": "sol-1", "description": "Test"}},
		},
	}
	mgr := NewBackupManager(mock, "/data")

	data, count, err := mgr.exportType(context.Background(), TypeSolutions)
	require.NoError(t, err)

	assert.Equal(t, 1, count)
	assert.Contains(t, string(data), "sol-1")
	assert.Contains(t, mock.lastQuery, "Solution")
}

func TestExportTypeUnknown(t *testing.T) {
	mock := &mockDriver{}
	mgr := NewBackupManager(mock, "/data")

	_, _, err := mgr.exportType(context.Background(), "unknown")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown type")
}

func TestClearType(t *testing.T) {
	mock := &mockDriver{}
	mgr := NewBackupManager(mock, "/data")

	err := mgr.clearType(context.Background(), TypeSolutions)
	require.NoError(t, err)

	assert.True(t, mock.writeCalled)
	assert.Contains(t, mock.lastQuery, "DELETE")
}

func TestClearTypeUnknown(t *testing.T) {
	mock := &mockDriver{}
	mgr := NewBackupManager(mock, "/data")

	err := mgr.clearType(context.Background(), "unknown")
	assert.NoError(t, err) // Unknown types are silently ignored
}

// --- Integration-like Tests ---

func TestExportImportRoundtrip(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()
	backupPath := filepath.Join(tmpDir, "test.tar.gz")

	// Mock with some data
	mock := &mockDriver{
		records: []graph.Record{
			{"s": map[string]any{"id": "sol-1", "desc": "Solution 1"}},
			{"s": map[string]any{"id": "sol-2", "desc": "Solution 2"}},
		},
	}
	mgr := NewBackupManager(mock, tmpDir)

	// Export
	meta, err := mgr.Export(
		context.Background(),
		[]KnowledgeType{TypeSolutions},
		backupPath,
		"Test backup",
	)
	require.NoError(t, err)
	require.NotNil(t, meta)

	assert.Equal(t, "1.0", meta.Version)
	assert.Equal(t, "Test backup", meta.Description)
	assert.Equal(t, 2, meta.Counts["solutions"])

	// Verify file was created
	_, err = os.Stat(backupPath)
	require.NoError(t, err)

	// List backup
	listMeta, err := mgr.List(backupPath)
	require.NoError(t, err)
	assert.Equal(t, meta.Description, listMeta.Description)
}

func TestListInvalidFile(t *testing.T) {
	mock := &mockDriver{}
	mgr := NewBackupManager(mock, "/data")

	_, err := mgr.List("/nonexistent/file.tar.gz")
	assert.Error(t, err)
}

func TestListInvalidGzip(t *testing.T) {
	tmpDir := t.TempDir()
	invalidPath := filepath.Join(tmpDir, "invalid.tar.gz")

	// Write non-gzip data
	err := os.WriteFile(invalidPath, []byte("not gzip data"), 0644)
	require.NoError(t, err)

	mock := &mockDriver{}
	mgr := NewBackupManager(mock, tmpDir)

	_, err = mgr.List(invalidPath)
	assert.Error(t, err)
}

func TestImportMissingMetadata(t *testing.T) {
	tmpDir := t.TempDir()
	backupPath := filepath.Join(tmpDir, "no-meta.tar.gz")

	// Create a valid tar.gz without metadata
	file, err := os.Create(backupPath)
	require.NoError(t, err)

	gzw := gzip.NewWriter(file)
	tw := tar.NewWriter(gzw)

	// Add a data file but no metadata
	addToTar(tw, "solutions.json", []byte("[]"))

	tw.Close()
	gzw.Close()
	file.Close()

	mock := &mockDriver{}
	mgr := NewBackupManager(mock, tmpDir)

	_, err = mgr.Import(context.Background(), backupPath, nil, false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "metadata")
}

func TestExportAllTypes(t *testing.T) {
	tmpDir := t.TempDir()
	backupPath := filepath.Join(tmpDir, "all.tar.gz")

	mock := &mockDriver{records: []graph.Record{}}
	mgr := NewBackupManager(mock, tmpDir)

	meta, err := mgr.Export(
		context.Background(),
		[]KnowledgeType{TypeAll},
		backupPath,
		"All types",
	)
	require.NoError(t, err)

	// When TypeAll is requested, it expands to all types
	assert.Equal(t, []KnowledgeType{TypeAll}, meta.Types) // Original request is preserved
}
