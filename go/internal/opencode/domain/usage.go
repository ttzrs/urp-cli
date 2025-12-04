package domain

import (
	"fmt"
	"time"
)

// Usage tracks token usage and costs for a session or message
type Usage struct {
	InputTokens  int     `json:"inputTokens"`
	OutputTokens int     `json:"outputTokens"`
	CacheRead    int     `json:"cacheRead,omitempty"`
	CacheWrite   int     `json:"cacheWrite,omitempty"`
	InputCost    float64 `json:"inputCost"`
	OutputCost   float64 `json:"outputCost"`
	TotalCost    float64 `json:"totalCost"`
}

// Add combines two Usage values
func (u *Usage) Add(other Usage) {
	u.InputTokens += other.InputTokens
	u.OutputTokens += other.OutputTokens
	u.CacheRead += other.CacheRead
	u.CacheWrite += other.CacheWrite
	u.InputCost += other.InputCost
	u.OutputCost += other.OutputCost
	u.TotalCost += other.TotalCost
}

// SessionUsage tracks usage per session
type SessionUsage struct {
	SessionID    string    `json:"sessionID"`
	ProviderID   string    `json:"providerID"`
	ModelID      string    `json:"modelID"`
	Usage        Usage     `json:"usage"`
	MessageCount int       `json:"messageCount"`
	ToolCalls    int       `json:"toolCalls"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// TokenCounter estimates tokens for text
type TokenCounter interface {
	Count(text string) int
	CountMessages(messages []Message) int
}

// SimpleTokenCounter uses character-based estimation (1 token ≈ 4 chars)
type SimpleTokenCounter struct{}

func (c *SimpleTokenCounter) Count(text string) int {
	// Rough approximation: 1 token ≈ 4 characters for English
	// This is a simplification; real tokenizers vary by model
	return (len(text) + 3) / 4
}

func (c *SimpleTokenCounter) CountMessages(messages []Message) int {
	total := 0
	for _, msg := range messages {
		// Per-message overhead (role, formatting)
		total += 4
		for _, part := range msg.Parts {
			switch p := part.(type) {
			case TextPart:
				total += c.Count(p.Text)
			case ReasoningPart:
				total += c.Count(p.Text)
			case ToolCallPart:
				total += c.Count(p.Name)
				total += 50 // average tool call overhead
				total += c.Count(p.Result)
			case FilePart:
				total += c.Count(p.Content)
			}
		}
	}
	return total
}

// CalculateCost computes cost from tokens and model pricing
func CalculateCost(inputTokens, outputTokens int, model Model) Usage {
	inputCost := float64(inputTokens) * model.InputCost / 1_000_000
	outputCost := float64(outputTokens) * model.OutputCost / 1_000_000

	return Usage{
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		InputCost:    inputCost,
		OutputCost:   outputCost,
		TotalCost:    inputCost + outputCost,
	}
}

// FormatCost returns a human-readable cost string
func FormatCost(cost float64) string {
	if cost < 0.01 {
		return "<$0.01"
	}
	return fmt.Sprintf("$%.2f", cost)
}

// FormatTokens returns a human-readable token count
func FormatTokens(tokens int) string {
	if tokens < 1000 {
		return fmt.Sprintf("%d", tokens)
	}
	return fmt.Sprintf("%.1fk", float64(tokens)/1000)
}

// CacheStats holds computed cache statistics
type CacheStats struct {
	TotalOps int     // Total cache operations
	Reads    int     // Cache hits
	Writes   int     // Cache misses/creates
	HitRate  float64 // Percentage of reads (0-100)
	Savings  float64 // Estimated cost savings from caching
}

// GetCacheStats computes cache statistics from usage data
func (u *Usage) GetCacheStats(inputCostPerMillion float64) CacheStats {
	stats := CacheStats{
		Reads:  u.CacheRead,
		Writes: u.CacheWrite,
	}
	stats.TotalOps = stats.Reads + stats.Writes
	if stats.TotalOps > 0 {
		stats.HitRate = float64(stats.Reads) / float64(stats.TotalOps) * 100
		// Savings: cache reads cost 10% of normal input tokens (Anthropic)
		stats.Savings = float64(stats.Reads) * inputCostPerMillion / 1_000_000 * 0.9
	}
	return stats
}

// FormatCacheStats returns a human-readable cache stats string
func FormatCacheStats(stats CacheStats) string {
	if stats.TotalOps == 0 {
		return "no cache"
	}
	return fmt.Sprintf("%.0f%% hit (%s read, %s write)",
		stats.HitRate,
		FormatTokens(stats.Reads),
		FormatTokens(stats.Writes))
}
