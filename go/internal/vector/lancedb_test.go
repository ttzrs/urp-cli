package vector

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLanceStore(t *testing.T) {
	// Use temp dir for tests
	tmpDir, err := os.MkdirTemp("", "vector_test")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := NewLanceStore(tmpDir)
	if err != nil {
		t.Fatalf("NewLanceStore: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Test Add
	entry := VectorEntry{
		ID:     "test-1",
		Text:   "ModuleNotFoundError: No module named 'foo'",
		Vector: []float32{0.1, 0.2, 0.3, 0.4, 0.5},
		Kind:   "error",
		Metadata: map[string]string{
			"command": "python test.py",
			"project": "myproject",
		},
	}

	err = store.Add(ctx, entry)
	if err != nil {
		t.Errorf("Add: %v", err)
	}

	// Test Count
	count, err := store.Count(ctx)
	if err != nil {
		t.Errorf("Count: %v", err)
	}
	if count != 1 {
		t.Errorf("Count = %d, want 1", count)
	}

	// Test Search
	results, err := store.Search(ctx, []float32{0.1, 0.2, 0.3, 0.4, 0.5}, 10, "error")
	if err != nil {
		t.Errorf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("Search results = %d, want 1", len(results))
	}
	if results[0].Score < 0.99 {
		t.Errorf("Score = %f, want >= 0.99", results[0].Score)
	}

	// Test Delete
	err = store.Delete(ctx, "test-1")
	if err != nil {
		t.Errorf("Delete: %v", err)
	}

	count, _ = store.Count(ctx)
	if count != 0 {
		t.Errorf("Count after delete = %d, want 0", count)
	}
}

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name string
		a    []float32
		b    []float32
		want float32
	}{
		{
			name: "identical",
			a:    []float32{1, 0, 0},
			b:    []float32{1, 0, 0},
			want: 1.0,
		},
		{
			name: "orthogonal",
			a:    []float32{1, 0, 0},
			b:    []float32{0, 1, 0},
			want: 0.0,
		},
		{
			name: "opposite",
			a:    []float32{1, 0, 0},
			b:    []float32{-1, 0, 0},
			want: -1.0,
		},
		{
			name: "similar",
			a:    []float32{1, 1, 0},
			b:    []float32{1, 0, 0},
			want: 0.707, // 1/sqrt(2)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cosineSimilarity(tt.a, tt.b)
			diff := got - tt.want
			if diff < -0.01 || diff > 0.01 {
				t.Errorf("cosineSimilarity = %f, want %f", got, tt.want)
			}
		})
	}
}

func TestGenerateID(t *testing.T) {
	id1 := generateID("hello world")
	id2 := generateID("hello world")
	id3 := generateID("different text")

	if id1 != id2 {
		t.Error("Same text should produce same ID")
	}
	if id1 == id3 {
		t.Error("Different text should produce different ID")
	}
	if len(id1) != 16 {
		t.Errorf("ID length = %d, want 16", len(id1))
	}
}

func TestPersistence(t *testing.T) {
	// Use temp dir for tests
	tmpDir, err := os.MkdirTemp("", "vector_persist_test")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	ctx := context.Background()

	// Create store and add entry
	store1, err := NewLanceStore(tmpDir)
	if err != nil {
		t.Fatalf("NewLanceStore: %v", err)
	}

	entry := VectorEntry{
		ID:     "persist-test",
		Text:   "Test persistence",
		Vector: []float32{0.1, 0.2, 0.3, 0.4, 0.5},
		Kind:   "test",
		Metadata: map[string]string{
			"key": "value",
		},
	}

	err = store1.Add(ctx, entry)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	store1.Close()

	// Check files exist
	metaFile := filepath.Join(tmpDir, "index.json")
	vecFile := filepath.Join(tmpDir, "vectors.bin")

	if _, err := os.Stat(metaFile); os.IsNotExist(err) {
		t.Error("index.json should exist")
	}
	if _, err := os.Stat(vecFile); os.IsNotExist(err) {
		t.Error("vectors.bin should exist")
	}

	// Reopen store and verify data
	store2, err := NewLanceStore(tmpDir)
	if err != nil {
		t.Fatalf("NewLanceStore reopen: %v", err)
	}
	defer store2.Close()

	count, _ := store2.Count(ctx)
	if count != 1 {
		t.Errorf("Count after reload = %d, want 1", count)
	}

	results, err := store2.Search(ctx, []float32{0.1, 0.2, 0.3, 0.4, 0.5}, 10, "test")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("Results = %d, want 1", len(results))
	}
	if results[0].Entry.ID != "persist-test" {
		t.Errorf("Entry ID = %s, want persist-test", results[0].Entry.ID)
	}
	if results[0].Entry.Metadata["key"] != "value" {
		t.Error("Metadata not preserved")
	}
}

func TestFloatConversion(t *testing.T) {
	tests := []float32{0, 1.0, -1.0, 0.5, 3.14159, -0.001}

	for _, f := range tests {
		bits := floatBits(f)
		back := floatFromBits(bits)
		if back != f {
			t.Errorf("floatFromBits(floatBits(%f)) = %f", f, back)
		}
	}
}
