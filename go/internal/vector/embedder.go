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

// GetDefaultEmbedder returns the default embedder.
// Panics if URP_EMBEDDING_PROVIDER is not configured.
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
			panic("URP_EMBEDDING_PROVIDER=tei but TEI_URL not set. Set TEI_URL=http://urp-tei:80")
		}
		defaultEmbedder = NewTeiEmbedder(teiURL)
	case "openai":
		apiKey := os.Getenv("OPENAI_API_KEY")
		if apiKey == "" {
			panic("URP_EMBEDDING_PROVIDER=openai but OPENAI_API_KEY not set")
		}
		defaultEmbedder = NewOpenAIEmbedder(apiKey)
	case "":
		panic(fmt.Sprintf("URP_EMBEDDING_PROVIDER not set. Set to 'tei' or 'openai'.\n" +
			"For local development: URP_EMBEDDING_PROVIDER=tei TEI_URL=http://localhost:8080\n" +
			"Start TEI with: docker compose up -d tei"))
	default:
		panic(fmt.Sprintf("Unknown URP_EMBEDDING_PROVIDER=%q. Supported: tei, openai", provider))
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
