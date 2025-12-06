package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// --- EntityType Tests ---

func TestEntityTypeGraphLabel(t *testing.T) {
	tests := []struct {
		entityType EntityType
		want       string
	}{
		{EntityFile, "File"},
		{EntityFunction, "Function"},
		{EntityMethod, "Method"},
		{EntityStruct, "Struct"},
		{EntityInterface, "Interface"},
		{EntityClass, "Class"},
		{EntityType("Unknown"), "Entity"}, // fallback
	}

	for _, tt := range tests {
		t.Run(string(tt.entityType), func(t *testing.T) {
			got := tt.entityType.GraphLabel()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestEntityTypeStatKey(t *testing.T) {
	tests := []struct {
		entityType EntityType
		want       string
	}{
		{EntityFile, "files"},
		{EntityFunction, "functions"},
		{EntityMethod, "functions"}, // Methods count as functions
		{EntityStruct, "structs"},
		{EntityInterface, "interfaces"},
		{EntityClass, "classes"},
		{EntityType("Unknown"), "other"}, // fallback
	}

	for _, tt := range tests {
		t.Run(string(tt.entityType), func(t *testing.T) {
			got := tt.entityType.StatKey()
			assert.Equal(t, tt.want, got)
		})
	}
}

// --- Entity Tests ---

func TestEntity(t *testing.T) {
	e := Entity{
		ID:        "path/to/file.go::MyFunc",
		Type:      EntityFunction,
		Name:      "MyFunc",
		Path:      "path/to/file.go",
		Signature: "MyFunc(x int) error",
		StartLine: 10,
		EndLine:   25,
		Metadata:  map[string]string{"package": "main"},
	}

	assert.Equal(t, "path/to/file.go::MyFunc", e.ID)
	assert.Equal(t, EntityFunction, e.Type)
	assert.Equal(t, "MyFunc", e.Name)
	assert.Equal(t, "path/to/file.go", e.Path)
	assert.Contains(t, e.Signature, "error")
	assert.Equal(t, 10, e.StartLine)
	assert.Equal(t, 25, e.EndLine)
	assert.Equal(t, "main", e.Metadata["package"])
}

func TestEntityTypes(t *testing.T) {
	// All defined types should be non-empty
	types := []EntityType{
		EntityFile,
		EntityFunction,
		EntityClass,
		EntityStruct,
		EntityInterface,
		EntityMethod,
	}

	for _, et := range types {
		assert.NotEmpty(t, string(et))
		assert.NotEmpty(t, et.GraphLabel())
		assert.NotEmpty(t, et.StatKey())
	}
}

// --- Relationship Tests ---

func TestRelationship(t *testing.T) {
	r := Relationship{
		From: "file.go::FuncA",
		To:   "file.go::FuncB",
		Type: "CALLS",
	}

	assert.Equal(t, "file.go::FuncA", r.From)
	assert.Equal(t, "file.go::FuncB", r.To)
	assert.Equal(t, "CALLS", r.Type)
}

// --- Commit Tests ---

func TestCommit(t *testing.T) {
	ts := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

	c := Commit{
		Hash:      "abc123def456",
		Author:    "John Doe",
		Email:     "john@example.com",
		Message:   "Fix: resolve null pointer exception",
		Timestamp: ts,
		Files:     []string{"file1.go", "file2.go"},
	}

	assert.Equal(t, "abc123def456", c.Hash)
	assert.Equal(t, "John Doe", c.Author)
	assert.Equal(t, "john@example.com", c.Email)
	assert.Contains(t, c.Message, "null pointer")
	assert.Equal(t, ts, c.Timestamp)
	assert.Len(t, c.Files, 2)
}

// --- Author Tests ---

func TestAuthor(t *testing.T) {
	a := Author{
		Name:       "Jane Doe",
		Email:      "jane@example.com",
		Commits:    42,
		LinesAdded: 1500,
	}

	assert.Equal(t, "Jane Doe", a.Name)
	assert.Equal(t, "jane@example.com", a.Email)
	assert.Equal(t, 42, a.Commits)
	assert.Equal(t, 1500, a.LinesAdded)
}
