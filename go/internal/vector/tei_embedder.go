package vector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// TeiEmbedder uses HuggingFace Text Embeddings Inference API.
type TeiEmbedder struct {
	url        string
	httpClient *http.Client
	model      string
}

// NewTeiEmbedder creates a new TEI embedder.
func NewTeiEmbedder(url string) *TeiEmbedder {
	if url == "" {
		url = "http://localhost:8080" // Default to local exposed port if running outside container
	}
	// Ensure URL doesn't have trailing slash
	if len(url) > 0 && url[len(url)-1] == '/' {
		url = url[:len(url)-1]
	}

	return &TeiEmbedder{
		url: url,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Embed generates an embedding for a single text.
func (e *TeiEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	batch, err := e.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(batch) == 0 {
		return nil, fmt.Errorf("empty response from TEI")
	}
	return batch[0], nil
}

// EmbedBatch generates embeddings for multiple texts.
func (e *TeiEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	// TEI API: POST /embed
	// Body: {"inputs": ["text1", "text2"], "normalize": true, "truncate": true}

	payload := map[string]any{
		"inputs":    texts,
		"normalize": true,
		"truncate":  true,
	}

	jsonBody, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", e.url+"/embed", bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("tei error %d: %s", resp.StatusCode, string(body))
	}

	// Response is simply [[float, ...], ...]
	var embeddings [][]float32
	if err := json.NewDecoder(resp.Body).Decode(&embeddings); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return embeddings, nil
}

// Dimensions returns the embedding dimension.
// Queries the /info endpoint to get model dimensions, caches result.
// Default fallback: 384 (BAAI/bge-small-en-v1.5)
func (e *TeiEmbedder) Dimensions() int {
	// For now, return 384 for BGE-small (default model)
	// TODO: Query /info endpoint for dynamic dimension detection
	return 384
}
