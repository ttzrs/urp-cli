// Package vector provides embedding generation.
package vector

import (
	"context"
	"fmt"
	"os"
)

// Embedder generates vector embeddings from text.
type Embedder interface {
	// Embed generates an embedding for the given text.
	Embed(ctx context.Context, text string) ([]float32, error)

	// EmbedBatch generates embeddings for multiple texts.
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)

	// Dimensions returns the embedding dimension.
	Dimensions() int
}

// DefaultEmbedder is the global embedder instance.
var defaultEmbedder Embedder

// NullEmbedder is a no-op embedder that returns empty embeddings.
// Used when no embedding provider is configured.
type NullEmbedder struct{}

func (n *NullEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	return nil, nil
}

func (n *NullEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	return make([][]float32, len(texts)), nil
}

func (n *NullEmbedder) Dimensions() int { return 0 }

// GetDefaultEmbedder returns the default embedder.
// Returns NullEmbedder if no provider is configured.
// Supported providers: "tei", "openai"
func GetDefaultEmbedder() Embedder {
	if defaultEmbedder != nil {
		return defaultEmbedder
	}

	provider := os.Getenv("URP_EMBEDDING_PROVIDER")

	switch provider {
	case "tei":
		teiURL := os.Getenv("TEI_URL")
		if teiURL == "" {
			fmt.Fprintf(os.Stderr, "Warning: URP_EMBEDDING_PROVIDER=tei but TEI_URL not set\n")
			defaultEmbedder = &NullEmbedder{}
		} else {
			defaultEmbedder = NewTeiEmbedder(teiURL)
		}
	case "openai":
		apiKey := os.Getenv("OPENAI_API_KEY")
		if apiKey == "" {
			fmt.Fprintf(os.Stderr, "Warning: URP_EMBEDDING_PROVIDER=openai but OPENAI_API_KEY not set\n")
			defaultEmbedder = &NullEmbedder{}
		} else {
			defaultEmbedder = NewOpenAIEmbedder(apiKey)
		}
	case "":
		// No embedder configured - use null embedder (graph-only mode)
		defaultEmbedder = &NullEmbedder{}
	default:
		fmt.Fprintf(os.Stderr, "Warning: Unknown URP_EMBEDDING_PROVIDER=%q, using null embedder\n", provider)
		defaultEmbedder = &NullEmbedder{}
	}

	return defaultEmbedder
}

// SetDefaultEmbedder sets the default embedder.
func SetDefaultEmbedder(e Embedder) {
	defaultEmbedder = e
}

// ResetDefaultEmbedder clears the default embedder (for testing).
func ResetDefaultEmbedder() {
	defaultEmbedder = nil
}
