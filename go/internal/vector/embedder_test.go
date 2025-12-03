package vector

import (
	"context"
	"testing"
)

func TestLocalEmbedder(t *testing.T) {
	embedder := NewLocalEmbedder(384)

	ctx := context.Background()

	// Test basic embedding
	emb, err := embedder.Embed(ctx, "ModuleNotFoundError: No module named 'foo'")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}

	if len(emb) != 384 {
		t.Errorf("Embedding dimension = %d, want 384", len(emb))
	}

	// Test unit vector (should be normalized)
	var sum float32
	for _, v := range emb {
		sum += v * v
	}
	if sum < 0.99 || sum > 1.01 {
		t.Errorf("Embedding norm = %f, want 1.0", sum)
	}
}

func TestEmbeddingSimilarity(t *testing.T) {
	embedder := NewLocalEmbedder(384)
	ctx := context.Background()

	// Similar texts should have high similarity
	emb1, _ := embedder.Embed(ctx, "ModuleNotFoundError: No module named 'requests'")
	emb2, _ := embedder.Embed(ctx, "ModuleNotFoundError: No module named 'flask'")
	emb3, _ := embedder.Embed(ctx, "Connection refused on port 8080")

	sim12 := cosineSimilarity(emb1, emb2)
	sim13 := cosineSimilarity(emb1, emb3)

	// Similar errors should be more similar than unrelated ones
	if sim12 <= sim13 {
		t.Errorf("Similar errors should have higher similarity: sim12=%f, sim13=%f", sim12, sim13)
	}
}

func TestEmbedBatch(t *testing.T) {
	embedder := NewLocalEmbedder(384)
	ctx := context.Background()

	texts := []string{
		"error one",
		"error two",
		"error three",
	}

	embeddings, err := embedder.EmbedBatch(ctx, texts)
	if err != nil {
		t.Fatalf("EmbedBatch: %v", err)
	}

	if len(embeddings) != 3 {
		t.Errorf("EmbedBatch count = %d, want 3", len(embeddings))
	}

	for i, emb := range embeddings {
		if len(emb) != 384 {
			t.Errorf("Embedding %d dimension = %d, want 384", i, len(emb))
		}
	}
}

func TestTokenize(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"hello world", 2},
		{"ModuleNotFoundError: No module named 'foo'", 5}, // modulenotfounderror, no, module, named, foo
		{"", 0},
		{"a b c", 0},        // all too short
		{"abc def ghi", 3},  // all >= 2 chars
		{"HTTP 404 Error", 3},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			tokens := tokenize(tt.input)
			if len(tokens) != tt.want {
				t.Errorf("tokenize(%q) = %d tokens, want %d (got: %v)",
					tt.input, len(tokens), tt.want, tokens)
			}
		})
	}
}

func TestEmptyText(t *testing.T) {
	embedder := NewLocalEmbedder(384)
	ctx := context.Background()

	emb, err := embedder.Embed(ctx, "")
	if err != nil {
		t.Fatalf("Embed empty: %v", err)
	}

	if len(emb) != 384 {
		t.Errorf("Empty embedding dimension = %d, want 384", len(emb))
	}

	// Empty text should produce zero vector
	var sum float32
	for _, v := range emb {
		sum += v * v
	}
	if sum != 0 {
		t.Errorf("Empty embedding should be zero vector, got norm %f", sum)
	}
}
