package gate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/joss/urp/internal/config"
)

// OpenAIClient is a generic client for OpenAI-compatible APIs (Cerebras, Groq, Proxies, etc.)
type OpenAIClient struct {
	ApiKey  string
	BaseURL string
	Model   string
	Client  *http.Client
}

// NewOpenAIClient creates a client with the specified model.
// If model is empty, it attempts to load from env or defaults.
func NewOpenAIClient(modelEnvVar string) *OpenAIClient {
	model := os.Getenv(modelEnvVar)
	if model == "" {
		model = os.Getenv("URP_GATE_MODEL_ID") // Use URP_GATE_MODEL_ID env var
	}
	if model == "" {
		model = config.GetEnvOrDefault("URP_DEFAULT_GATE_MODEL", "gpt-4o-mini") // Use config for default
	}

	// Check if specific gate model API key is provided, otherwise use general keys
	apiKey := os.Getenv("URP_GATE_MODEL_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("API_KEY")
	}
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}

	// Check if specific gate model URL is provided, otherwise use general URLs
	baseURL := os.Getenv("URP_GATE_MODEL_URL")
	if baseURL == "" {
		baseURL = os.Getenv("BASE_URL")
	}
	if baseURL == "" {
		baseURL = os.Getenv("OPENAI_BASE_URL")
	}
	if baseURL == "" {
		baseURL = config.GetEnvOrDefault("URP_DEFAULT_OPENAI_BASE_URL", "https://api.openai.com/v1") // Use config for default
	}

	return &OpenAIClient{
		ApiKey:  apiKey,
		BaseURL: baseURL,
		Model:   model,
		Client:  &http.Client{Timeout: 30 * time.Second},
	}
}

// Request/Response structs for OpenAI-compatible API
type chatRequest struct {
	Model    string    `json:"model"`
	Messages []message `json:"messages"`
	Stream   bool      `json:"stream"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message message `json:"message"`
	} `json:"choices"`
}

// FilterNoise implements the GateClient interface.
// It uses the configured model (e.g., qwen3-coder-flash) to filter logs.
func (c *OpenAIClient) FilterNoise(ctx context.Context, goal string, rawInput string) (string, error) {
	// 1. Construct the Gating Prompt
	systemPrompt := `
		You are a CONTEXT COMPILER. Your goal is to filter noise from logs/data.
		Input: Raw Logs/Text.
		Instruction: 
		1. Identify if this input contains specific errors, warnings, or data CRITICAL to the user's Goal.
		2. If YES: Extract ONLY the relevant lines exactly. Do not summarize.
		3. If NO (it's just info/debug noise): Return the string "NO_SIGNAL".
	`
	userPrompt := fmt.Sprintf("GOAL: %s\n\nRAW INPUT:\n%s", goal, rawInput)

	// 2. Prepare Request
	reqBody := chatRequest{
		Model: c.Model,
		Messages: []message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Stream: false,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshalling request: %w", err)
	}

	url := fmt.Sprintf("%s/chat/completions", strings.TrimRight(c.BaseURL, "/"))
	// Handle /v1 duplication if present
	if strings.HasSuffix(c.BaseURL, "/v1") && strings.HasSuffix(url, "/v1/chat/completions") {
		// If baseurl ends in /v1, the Append above makes it /v1/chat/completions correctly
	} else if !strings.Contains(url, "/chat/completions") {
		// Just ensure it's correct
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.ApiKey))

	// 3. Execute
	resp, err := c.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("api request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("api error (status %d): %s", resp.StatusCode, string(body))
	}

	// 4. Parse Response
	var result chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decoding response: %w", err)
	}

	if len(result.Choices) == 0 {
		return "", nil
	}

	content := strings.TrimSpace(result.Choices[0].Message.Content)

	// 5. Apply Sparsity Logic
	if content == "NO_SIGNAL" {
		return "", nil // Sparsity: Return empty string
	}

	return content, nil
}
