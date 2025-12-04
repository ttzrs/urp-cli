package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/joss/urp/internal/opencode/domain"
)

// WebFetch fetches content from URLs
type WebFetch struct {
	client *http.Client
}

func NewWebFetch() *WebFetch {
	return &WebFetch{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (w *WebFetch) Info() domain.Tool {
	return domain.Tool{
		ID:          "webfetch",
		Name:        "webfetch",
		Description: "Fetch content from a URL. Returns the page content as text.",
		Parameters: domain.JSONSchema{
			"type": "object",
			"properties": map[string]any{
				"url": map[string]any{
					"type":        "string",
					"description": "The URL to fetch",
				},
			},
			"required": []string{"url"},
		},
	}
}

func (w *WebFetch) Execute(ctx context.Context, args map[string]any) (*Result, error) {
	rawURL, ok := args["url"].(string)
	if !ok || rawURL == "" {
		return &Result{Error: ErrInvalidArgs}, ErrInvalidArgs
	}

	// Validate URL
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return &Result{
			Title:  "WebFetch",
			Output: fmt.Sprintf("Invalid URL: %s", err),
			Error:  err,
		}, nil
	}

	// Upgrade HTTP to HTTPS
	if parsedURL.Scheme == "http" {
		parsedURL.Scheme = "https"
	}

	req, err := http.NewRequestWithContext(ctx, "GET", parsedURL.String(), nil)
	if err != nil {
		return &Result{
			Title:  "WebFetch",
			Output: fmt.Sprintf("Failed to create request: %s", err),
			Error:  err,
		}, nil
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; OpenCode/1.0)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := w.client.Do(req)
	if err != nil {
		return &Result{
			Title:  "WebFetch",
			Output: fmt.Sprintf("Failed to fetch: %s", err),
			Error:  err,
		}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &Result{
			Title:  "WebFetch",
			Output: fmt.Sprintf("HTTP %d: %s", resp.StatusCode, resp.Status),
			Error:  fmt.Errorf("HTTP %d", resp.StatusCode),
		}, nil
	}

	// Read body with limit
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024)) // 1MB limit
	if err != nil {
		return &Result{
			Title:  "WebFetch",
			Output: fmt.Sprintf("Failed to read response: %s", err),
			Error:  err,
		}, nil
	}

	content := string(body)

	// Simple HTML to text conversion
	content = htmlToText(content)

	// Truncate if too long
	if len(content) > 50000 {
		content = content[:50000] + "\n... (content truncated)"
	}

	return &Result{
		Title:  fmt.Sprintf("Fetched %s", parsedURL.Host),
		Output: content,
		Metadata: map[string]any{
			"url":         parsedURL.String(),
			"status":      resp.StatusCode,
			"contentType": resp.Header.Get("Content-Type"),
		},
	}, nil
}

// WebSearch performs web searches
type WebSearch struct {
	client *http.Client
}

func NewWebSearch() *WebSearch {
	return &WebSearch{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (w *WebSearch) Info() domain.Tool {
	return domain.Tool{
		ID:          "websearch",
		Name:        "websearch",
		Description: "Search the web using DuckDuckGo. Returns search results.",
		Parameters: domain.JSONSchema{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "The search query",
				},
				"max_results": map[string]any{
					"type":        "number",
					"description": "Maximum number of results (default: 10)",
				},
			},
			"required": []string{"query"},
		},
	}
}

func (w *WebSearch) Execute(ctx context.Context, args map[string]any) (*Result, error) {
	query, ok := args["query"].(string)
	if !ok || query == "" {
		return &Result{Error: ErrInvalidArgs}, ErrInvalidArgs
	}

	maxResults := 10
	if mr, ok := args["max_results"].(float64); ok && mr > 0 {
		maxResults = int(mr)
		if maxResults > 20 {
			maxResults = 20
		}
	}

	// Use DuckDuckGo HTML search (no API key needed)
	results, err := w.duckDuckGoSearch(ctx, query, maxResults)
	if err != nil {
		return &Result{
			Title:  "WebSearch",
			Output: fmt.Sprintf("Search failed: %s", err),
			Error:  err,
		}, nil
	}

	var sb strings.Builder
	for i, r := range results {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, r.Title))
		sb.WriteString(fmt.Sprintf("   %s\n", r.URL))
		if r.Snippet != "" {
			sb.WriteString(fmt.Sprintf("   %s\n", r.Snippet))
		}
		sb.WriteString("\n")
	}

	if sb.Len() == 0 {
		sb.WriteString("No results found")
	}

	return &Result{
		Title:  fmt.Sprintf("Search: %s", query),
		Output: sb.String(),
		Metadata: map[string]any{
			"query":   query,
			"results": len(results),
		},
	}, nil
}

type searchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

func (w *WebSearch) duckDuckGoSearch(ctx context.Context, query string, maxResults int) ([]searchResult, error) {
	// DuckDuckGo Instant Answer API
	apiURL := fmt.Sprintf("https://api.duckduckgo.com/?q=%s&format=json&no_html=1&skip_disambig=1",
		url.QueryEscape(query))

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "OpenCode/1.0")

	resp, err := w.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var ddgResp struct {
		Abstract       string `json:"Abstract"`
		AbstractURL    string `json:"AbstractURL"`
		AbstractSource string `json:"AbstractSource"`
		RelatedTopics  []struct {
			Text     string `json:"Text"`
			FirstURL string `json:"FirstURL"`
		} `json:"RelatedTopics"`
		Results []struct {
			Text     string `json:"Text"`
			FirstURL string `json:"FirstURL"`
		} `json:"Results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&ddgResp); err != nil {
		return nil, err
	}

	var results []searchResult

	// Add abstract if available
	if ddgResp.Abstract != "" && ddgResp.AbstractURL != "" {
		results = append(results, searchResult{
			Title:   ddgResp.AbstractSource,
			URL:     ddgResp.AbstractURL,
			Snippet: ddgResp.Abstract,
		})
	}

	// Add direct results
	for _, r := range ddgResp.Results {
		if len(results) >= maxResults {
			break
		}
		results = append(results, searchResult{
			Title:   extractTitle(r.Text),
			URL:     r.FirstURL,
			Snippet: r.Text,
		})
	}

	// Add related topics
	for _, r := range ddgResp.RelatedTopics {
		if len(results) >= maxResults {
			break
		}
		if r.FirstURL != "" {
			results = append(results, searchResult{
				Title:   extractTitle(r.Text),
				URL:     r.FirstURL,
				Snippet: r.Text,
			})
		}
	}

	return results, nil
}

func extractTitle(text string) string {
	// DuckDuckGo often returns "Title - Description"
	if idx := strings.Index(text, " - "); idx > 0 {
		return text[:idx]
	}
	if len(text) > 60 {
		return text[:60] + "..."
	}
	return text
}

// Simple HTML to text converter
func htmlToText(html string) string {
	// Remove scripts and styles
	scriptRe := regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	html = scriptRe.ReplaceAllString(html, "")

	styleRe := regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	html = styleRe.ReplaceAllString(html, "")

	// Remove HTML comments
	commentRe := regexp.MustCompile(`<!--.*?-->`)
	html = commentRe.ReplaceAllString(html, "")

	// Convert common elements
	html = regexp.MustCompile(`(?i)<br\s*/?>|</p>|</div>|</li>|</tr>`).ReplaceAllString(html, "\n")
	html = regexp.MustCompile(`(?i)<li[^>]*>`).ReplaceAllString(html, "• ")
	html = regexp.MustCompile(`(?i)<h[1-6][^>]*>`).ReplaceAllString(html, "\n## ")
	html = regexp.MustCompile(`(?i)</h[1-6]>`).ReplaceAllString(html, "\n")

	// Remove all remaining tags
	tagRe := regexp.MustCompile(`<[^>]+>`)
	html = tagRe.ReplaceAllString(html, "")

	// Decode HTML entities
	html = strings.ReplaceAll(html, "&nbsp;", " ")
	html = strings.ReplaceAll(html, "&amp;", "&")
	html = strings.ReplaceAll(html, "&lt;", "<")
	html = strings.ReplaceAll(html, "&gt;", ">")
	html = strings.ReplaceAll(html, "&quot;", "\"")
	html = strings.ReplaceAll(html, "&#39;", "'")

	// Clean up whitespace
	html = regexp.MustCompile(`[ \t]+`).ReplaceAllString(html, " ")
	html = regexp.MustCompile(`\n{3,}`).ReplaceAllString(html, "\n\n")
	html = strings.TrimSpace(html)

	return html
}
