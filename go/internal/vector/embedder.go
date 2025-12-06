// Package vector provides embedding generation.
package vector

import (
	"context"
	"fmt"
	"hash/fnv"
	"math"
	"os"
	"regexp"
	"strings"
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

// LocalEmbedder generates embeddings locally using TF-IDF-like approach.
// This is a fallback when no external embedding service is available.
// For production, use OpenAI/Anthropic embeddings or local models.
type LocalEmbedder struct {
	dims     int
	vocab    map[string]int
	idf      map[string]float32
	docCount int
}

// NewLocalEmbedder creates a new local embedder.
func NewLocalEmbedder(dims int) *LocalEmbedder {
	if dims <= 0 {
		dims = 384 // Default dimension
	}
	return &LocalEmbedder{
		dims:  dims,
		vocab: make(map[string]int),
		idf:   make(map[string]float32),
	}
}

// Embed generates an embedding using a hash-based approach.
// This provides consistent embeddings without external dependencies.
func (e *LocalEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	tokens := tokenize(text)
	if len(tokens) == 0 {
		return make([]float32, e.dims), nil
	}

	// Create embedding using feature hashing
	embedding := make([]float32, e.dims)

	// Term frequency
	tf := make(map[string]int)
	for _, token := range tokens {
		tf[token]++
	}

	// Hash each token to embedding dimensions
	for token, count := range tf {
		// Use multiple hash functions for better distribution
		h1 := hashString(token, 0)
		h2 := hashString(token, 1)
		h3 := hashString(token, 2)

		// Primary position
		pos1 := int(h1 % uint64(e.dims))
		pos2 := int(h2 % uint64(e.dims))
		pos3 := int(h3 % uint64(e.dims))

		// Weight by term frequency (log-scaled)
		weight := float32(1.0 + math.Log(float64(count)))

		// Add to embedding (can be positive or negative based on hash)
		if h1&1 == 0 {
			embedding[pos1] += weight
		} else {
			embedding[pos1] -= weight
		}
		if h2&1 == 0 {
			embedding[pos2] += weight * 0.5
		} else {
			embedding[pos2] -= weight * 0.5
		}
		if h3&1 == 0 {
			embedding[pos3] += weight * 0.25
		} else {
			embedding[pos3] -= weight * 0.25
		}

		// Add n-gram features (bigrams)
		if len(token) > 3 {
			for i := 0; i < len(token)-1; i++ {
				bigram := token[i : i+2]
				bh := hashString(bigram, 3)
				bpos := int(bh % uint64(e.dims))
				if bh&1 == 0 {
					embedding[bpos] += 0.1
				} else {
					embedding[bpos] -= 0.1
				}
			}
		}
	}

	// Normalize to unit vector
	normalize(embedding)

	return embedding, nil
}

// EmbedBatch generates embeddings for multiple texts.
func (e *LocalEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	results := make([][]float32, len(texts))
	for i, text := range texts {
		emb, err := e.Embed(ctx, text)
		if err != nil {
			return nil, fmt.Errorf("embed text %d: %w", i, err)
		}
		results[i] = emb
	}
	return results, nil
}

// Dimensions returns the embedding dimension.
func (e *LocalEmbedder) Dimensions() int {
	return e.dims
}

// tokenize splits text into tokens.
func tokenize(text string) []string {
	// Convert to lowercase
	text = strings.ToLower(text)

	// Split on non-alphanumeric characters
	re := regexp.MustCompile(`[a-z0-9]+`)
	matches := re.FindAllString(text, -1)

	// Filter short tokens
	var tokens []string
	for _, m := range matches {
		if len(m) >= 2 {
			tokens = append(tokens, m)
		}
	}

	return tokens
}

// hashString generates a hash for a string with a seed.
func hashString(s string, seed uint64) uint64 {
	h := fnv.New64a()
	h.Write([]byte{byte(seed), byte(seed >> 8)})
	h.Write([]byte(s))
	return h.Sum64()
}

// normalize converts a vector to unit length.
func normalize(v []float32) {
	var sum float32
	for _, x := range v {
		sum += x * x
	}
	if sum == 0 {
		return
	}
	norm := float32(math.Sqrt(float64(sum)))
	for i := range v {
		v[i] /= norm
	}
}

// DefaultEmbedder is the global embedder instance.
var (
	defaultEmbedder     Embedder
	defaultEmbedderOnce = new(struct{})
)

// DefaultEmbedder returns the default embedder.
func GetDefaultEmbedder() Embedder {
	if defaultEmbedder == nil {
		provider := os.Getenv("URP_EMBEDDING_PROVIDER")
		apiKey := os.Getenv("OPENAI_API_KEY")

		if provider == "tei" {
			teiURL := os.Getenv("TEI_URL")
			defaultEmbedder = NewTeiEmbedder(teiURL)
		} else if provider == "local" {
			defaultEmbedder = NewLocalEmbedder(384)
		} else if apiKey != "" {
			defaultEmbedder = NewOpenAIEmbedder(apiKey)
		} else {
			// Fallback: Check if TEI is reachable (auto-discovery) or use local
			defaultEmbedder = NewLocalEmbedder(384)
		}
	}
	return defaultEmbedder
}

// SetDefaultEmbedder sets the default embedder.
func SetDefaultEmbedder(e Embedder) {
	defaultEmbedder = e
}
