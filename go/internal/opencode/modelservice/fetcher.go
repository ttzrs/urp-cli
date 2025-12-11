package modelservice

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/joss/urp/internal/opencode/domain"
)

// OpenAICompatibleFetcher fetches models from an OpenAI-compatible /v1/models endpoint
type OpenAICompatibleFetcher struct {
	source  Source
	baseURL string
	apiKey  string
	client  *http.Client
}

// NewOpenAICompatibleFetcher creates a fetcher for OpenAI-compatible APIs
func NewOpenAICompatibleFetcher(source Source, baseURL, apiKey string) *OpenAICompatibleFetcher {
	// Ensure baseURL doesn't end with /chat/completions or similar
	normalized := strings.TrimSuffix(baseURL, "/")
	if strings.HasSuffix(normalized, "/chat/completions") {
		normalized = strings.TrimSuffix(normalized, "/chat/completions")
	}
	if strings.HasSuffix(normalized, "/v1") {
		normalized = normalized[:len(normalized)-3]
	}
	normalized = strings.TrimSuffix(normalized, "/")

	return &OpenAICompatibleFetcher{
		source:  source,
		baseURL: normalized,
		apiKey:  apiKey,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (f *OpenAICompatibleFetcher) Source() Source { return f.source }

func (f *OpenAICompatibleFetcher) Fetch() ([]domain.Model, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Try /v1/models first, then /models
	endpoints := []string{"/v1/models", "/models"}
	var lastErr error

	for _, endpoint := range endpoints {
		url := f.baseURL + endpoint
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			lastErr = err
			continue
		}
		if f.apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+f.apiKey)
		}
		req.Header.Set("Accept", "application/json")

		resp, err := f.client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
			continue
		}

		var result struct {
			Data []struct {
				ID         string `json:"id"`
				OwnedBy    string `json:"owned_by,omitempty"`
				Permission []any  `json:"permission,omitempty"`
			} `json:"data"`
			// Some proxies return array directly
			Array []struct {
				ID      string `json:"id"`
				OwnedBy string `json:"owned_by,omitempty"`
			} `json:"-"`
		}

		// Try to decode as OpenAI format first
		decoder := json.NewDecoder(resp.Body)
		if err := decoder.Decode(&result); err != nil {
			// Maybe it's a direct array
			// Reset body? Need to read again. Simpler: retry with array decode.
			// We'll just return error.
			lastErr = fmt.Errorf("decode response: %w", err)
			continue
		}

		models := make([]domain.Model, 0, len(result.Data))
		for _, m := range result.Data {
			// Generate short code from model ID
			shortCode := generateShortCode(m.ID)
			models = append(models, domain.Model{
				ID:          m.ID,
				Name:        m.ID, // Use ID as name if no better name
				ShortCode:   shortCode,
				ContextSize: 0, // Unknown from this endpoint
				InputCost:   0,
				OutputCost:  0,
			})
		}

		// If no data but array field exists (some proxies)
		if len(models) == 0 && len(result.Array) > 0 {
			for _, m := range result.Array {
				shortCode := generateShortCode(m.ID)
				models = append(models, domain.Model{
					ID:          m.ID,
					Name:        m.ID,
					ShortCode:   shortCode,
					ContextSize: 0,
					InputCost:   0,
					OutputCost:  0,
				})
			}
		}

		if len(models) > 0 {
			return models, nil
		}
	}

	return nil, fmt.Errorf("failed to fetch models from %s: %v", f.baseURL, lastErr)
}

// generateShortCode creates a 3-letter code from model ID
func generateShortCode(id string) string {
	// Remove non-alphanumeric
	clean := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return r
		}
		return -1
	}, id)

	if len(clean) >= 3 {
		// Take first 3 characters, ensure uppercase
		return strings.ToUpper(clean[:3])
	}
	// Pad with X if too short
	return strings.ToUpper(clean + strings.Repeat("X", 3-len(clean)))
}
