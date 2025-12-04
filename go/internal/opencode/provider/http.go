package provider

import "net/http"

// HTTPClient interface for HTTP requests (enables testing)
type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

// Verify http.Client implements HTTPClient
var _ HTTPClient = (*http.Client)(nil)
