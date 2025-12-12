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
	ApiKey    string
	BaseURL   string
	ModelName string
	Client    *http.Client
}

// NewOpenAIClient creates a client from environment variables.
// Looks for URP_GATE_MODEL_ID, API_KEY/OPENAI_API_KEY, etc.
func NewOpenAIClient(modelOverride string) *OpenAIClient {
	// Model priority: override > URP_GATE_MODEL_ID > default
	model := modelOverride
	if model == "" {
		model = os.Getenv("URP_GATE_MODEL_ID")
	}
	if model == "" {
		model = config.GetEnvOrDefault("URP_DEFAULT_GATE_MODEL", "glm-4.6")
	}

	// API Key priority: URP_GATE_MODEL_API_KEY > PROXY_API_KEY > API_KEY > OPENAI_API_KEY
	apiKey := os.Getenv("URP_GATE_MODEL_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("PROXY_API_KEY")
	}
	if apiKey == "" {
		apiKey = os.Getenv("API_KEY")
	}
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}

	// Base URL priority: URP_GATE_MODEL_URL > PROXY_BASE_URL > BASE_URL > OPENAI_BASE_URL > default
	baseURL := os.Getenv("URP_GATE_MODEL_URL")
	if baseURL == "" {
		baseURL = os.Getenv("PROXY_BASE_URL")
	}
	if baseURL == "" {
		baseURL = os.Getenv("BASE_URL")
	}
	if baseURL == "" {
		baseURL = os.Getenv("OPENAI_BASE_URL")
	}
	if baseURL == "" {
		baseURL = config.GetEnvOrDefault("URP_DEFAULT_OPENAI_BASE_URL", "https://api.openai.com/v1")
	}

	return &OpenAIClient{
		ApiKey:    apiKey,
		BaseURL:   baseURL,
		ModelName: model,
		Client:    &http.Client{Timeout: 30 * time.Second},
	}
}

// IsConfigured returns true if the client has an API key configured.
func (c *OpenAIClient) IsConfigured() bool {
	return c.ApiKey != ""
}

// Model returns the configured model name.
func (c *OpenAIClient) Model() string {
	return c.ModelName
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
	Usage *usageInfo `json:"usage,omitempty"`
}

type usageInfo struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// FilterNoise implements the GateClient interface.
// It uses the configured model (e.g., gpt-4o-mini) to filter logs.
func (c *OpenAIClient) FilterNoise(ctx context.Context, goal string, rawInput string) (string, error) {
	result, err := c.FilterNoiseWithUsage(ctx, goal, rawInput)
	if err != nil {
		return "", err
	}
	return result.FilteredText, nil
}

// FilterNoiseWithUsage filters noise and returns detailed usage info.
func (c *OpenAIClient) FilterNoiseWithUsage(ctx context.Context, goal string, rawInput string) (*GateResult, error) {
	// Check if configured
	if !c.IsConfigured() {
		// Return empty result without error - Gate is optional
		return &GateResult{
			Model:    c.ModelName,
			Filtered: false,
		}, nil
	}

	// 1. Construct the Gating Prompt
	systemPrompt := `You are a CONTEXT COMPILER. Your goal is to filter noise from logs/data.
Input: Raw Logs/Text.
Instruction: 
1. Identify if this input contains specific errors, warnings, or data CRITICAL to the user's Goal.
2. If YES: Extract ONLY the relevant lines exactly. Do not summarize.
3. If NO (it's just info/debug noise): Return the string "NO_SIGNAL".`

	userPrompt := fmt.Sprintf("GOAL: %s\n\nRAW INPUT:\n%s", goal, rawInput)

	// 2. Prepare Request
	reqBody := chatRequest{
		Model: c.ModelName,
		Messages: []message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Stream: false,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshalling request: %w", err)
	}

	url := fmt.Sprintf("%s/chat/completions", strings.TrimRight(c.BaseURL, "/"))

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.ApiKey))

	// 3. Execute
	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("api request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("api error (status %d): %s", resp.StatusCode, string(body))
	}

	// 4. Parse Response
	var result chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	gateResult := &GateResult{
		Model:    c.ModelName,
		Filtered: false,
	}

	// Extract usage info
	if result.Usage != nil {
		gateResult.InputTokens = result.Usage.PromptTokens
		gateResult.OutputTokens = result.Usage.CompletionTokens
	}

	if len(result.Choices) == 0 {
		return gateResult, nil
	}

	content := strings.TrimSpace(result.Choices[0].Message.Content)

	// 5. Apply Sparsity Logic
	if content == "NO_SIGNAL" {
		return gateResult, nil // Sparsity: Return empty filtered text
	}

	gateResult.FilteredText = content
	gateResult.Filtered = true
	return gateResult, nil
}
