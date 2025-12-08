package vector

import (
	"context"
	"os"
	"testing"
)

func TestGetDefaultEmbedderFallsBackToNull(t *testing.T) {
	// Save and clear environment
	origProvider := os.Getenv("URP_EMBEDDING_PROVIDER")
	origTEI := os.Getenv("TEI_URL")
	origOpenAI := os.Getenv("OPENAI_API_KEY")

	os.Unsetenv("URP_EMBEDDING_PROVIDER")
	os.Unsetenv("TEI_URL")
	os.Unsetenv("OPENAI_API_KEY")

	// Reset default embedder
	ResetDefaultEmbedder()

	defer func() {
		// Restore environment
		if origProvider != "" {
			os.Setenv("URP_EMBEDDING_PROVIDER", origProvider)
		}
		if origTEI != "" {
			os.Setenv("TEI_URL", origTEI)
		}
		if origOpenAI != "" {
			os.Setenv("OPENAI_API_KEY", origOpenAI)
		}
		ResetDefaultEmbedder()
	}()

	// Should return NullEmbedder (dimensions=0) without configuration
	embedder := GetDefaultEmbedder()
	if embedder == nil {
		t.Error("GetDefaultEmbedder should return NullEmbedder, not nil")
	}
	if embedder.Dimensions() != 0 {
		t.Errorf("NullEmbedder should have 0 dimensions, got %d", embedder.Dimensions())
	}
}

func TestGetDefaultEmbedderTEI(t *testing.T) {
	// Save and clear environment
	origProvider := os.Getenv("URP_EMBEDDING_PROVIDER")
	origTEI := os.Getenv("TEI_URL")

	os.Setenv("URP_EMBEDDING_PROVIDER", "tei")
	os.Setenv("TEI_URL", "http://localhost:8080")

	// Reset default embedder
	ResetDefaultEmbedder()

	defer func() {
		// Restore environment
		if origProvider != "" {
			os.Setenv("URP_EMBEDDING_PROVIDER", origProvider)
		} else {
			os.Unsetenv("URP_EMBEDDING_PROVIDER")
		}
		if origTEI != "" {
			os.Setenv("TEI_URL", origTEI)
		} else {
			os.Unsetenv("TEI_URL")
		}
		ResetDefaultEmbedder()
	}()

	// Should not panic with valid config
	embedder := GetDefaultEmbedder()
	if embedder == nil {
		t.Error("GetDefaultEmbedder returned nil with valid TEI config")
	}
}

func TestGetDefaultEmbedderTEIFallsBackWithoutURL(t *testing.T) {
	// Save and clear environment
	origProvider := os.Getenv("URP_EMBEDDING_PROVIDER")
	origTEI := os.Getenv("TEI_URL")

	os.Setenv("URP_EMBEDDING_PROVIDER", "tei")
	os.Unsetenv("TEI_URL")

	// Reset default embedder
	ResetDefaultEmbedder()

	defer func() {
		// Restore environment
		if origProvider != "" {
			os.Setenv("URP_EMBEDDING_PROVIDER", origProvider)
		} else {
			os.Unsetenv("URP_EMBEDDING_PROVIDER")
		}
		if origTEI != "" {
			os.Setenv("TEI_URL", origTEI)
		}
		ResetDefaultEmbedder()
	}()

	// Should return NullEmbedder when TEI_URL is missing
	embedder := GetDefaultEmbedder()
	if embedder == nil {
		t.Error("GetDefaultEmbedder should return NullEmbedder, not nil")
	}
	if embedder.Dimensions() != 0 {
		t.Errorf("Should fallback to NullEmbedder (dims=0), got %d", embedder.Dimensions())
	}
}

func TestGetDefaultEmbedderOpenAIFallsBackWithoutKey(t *testing.T) {
	// Save and clear environment
	origProvider := os.Getenv("URP_EMBEDDING_PROVIDER")
	origOpenAI := os.Getenv("OPENAI_API_KEY")

	os.Setenv("URP_EMBEDDING_PROVIDER", "openai")
	os.Unsetenv("OPENAI_API_KEY")

	// Reset default embedder
	ResetDefaultEmbedder()

	defer func() {
		// Restore environment
		if origProvider != "" {
			os.Setenv("URP_EMBEDDING_PROVIDER", origProvider)
		} else {
			os.Unsetenv("URP_EMBEDDING_PROVIDER")
		}
		if origOpenAI != "" {
			os.Setenv("OPENAI_API_KEY", origOpenAI)
		}
		ResetDefaultEmbedder()
	}()

	// Should return NullEmbedder when OPENAI_API_KEY is missing
	embedder := GetDefaultEmbedder()
	if embedder == nil {
		t.Error("GetDefaultEmbedder should return NullEmbedder, not nil")
	}
	if embedder.Dimensions() != 0 {
		t.Errorf("Should fallback to NullEmbedder (dims=0), got %d", embedder.Dimensions())
	}
}

func TestGetDefaultEmbedderUnknownProvider(t *testing.T) {
	// Save and clear environment
	origProvider := os.Getenv("URP_EMBEDDING_PROVIDER")

	os.Setenv("URP_EMBEDDING_PROVIDER", "unknown-provider")

	// Reset default embedder
	ResetDefaultEmbedder()

	defer func() {
		// Restore environment
		if origProvider != "" {
			os.Setenv("URP_EMBEDDING_PROVIDER", origProvider)
		} else {
			os.Unsetenv("URP_EMBEDDING_PROVIDER")
		}
		ResetDefaultEmbedder()
	}()

	// Should return NullEmbedder for unknown provider (graceful fallback)
	embedder := GetDefaultEmbedder()
	if embedder == nil {
		t.Error("GetDefaultEmbedder should return NullEmbedder, not nil")
	}
	if embedder.Dimensions() != 0 {
		t.Errorf("Should fallback to NullEmbedder (dims=0), got %d", embedder.Dimensions())
	}
}

func TestSetDefaultEmbedder(t *testing.T) {
	// Reset first
	ResetDefaultEmbedder()

	// Create a mock embedder
	mock := &mockEmbedder{dims: 384}
	SetDefaultEmbedder(mock)

	// Should return the mock
	embedder := GetDefaultEmbedder()
	if embedder == nil {
		t.Error("GetDefaultEmbedder should return the set embedder")
	}

	// Cleanup
	ResetDefaultEmbedder()
}

// mockEmbedder for testing
type mockEmbedder struct {
	dims int
}

func (m *mockEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	return make([]float32, m.dims), nil
}

func (m *mockEmbedder) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	results := make([][]float32, len(texts))
	for i := range texts {
		results[i] = make([]float32, m.dims)
	}
	return results, nil
}

func (m *mockEmbedder) Dimensions() int {
	return m.dims
}
