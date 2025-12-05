// Package tokens provides token counting using tiktoken-go.
// Used for session compaction and context management.
package tokens

import (
	"strings"
	"sync"

	"github.com/pkoukk/tiktoken-go"

	"github.com/joss/urp/internal/opencode/domain"
)

// Counter provides token counting for messages and text.
// Uses cl100k_base encoding (used by Claude and GPT-4).
type Counter struct {
	enc  *tiktoken.Tiktoken
	once sync.Once
	err  error
}

// Global counter instance
var defaultCounter = &Counter{}

// Count returns the number of tokens in the given text.
func Count(text string) int {
	return defaultCounter.Count(text)
}

// CountMessages returns total tokens for a slice of messages.
func CountMessages(msgs []domain.Message) int {
	return defaultCounter.CountMessages(msgs)
}

// CountMessage returns tokens for a single message.
func CountMessage(msg domain.Message) int {
	return defaultCounter.CountMessage(msg)
}

// Count returns the number of tokens in the given text.
func (c *Counter) Count(text string) int {
	c.init()
	if c.err != nil || c.enc == nil {
		// Fallback: rough estimate (4 chars per token)
		return len(text) / 4
	}
	return len(c.enc.Encode(text, nil, nil))
}

// CountMessages returns total tokens for a slice of messages.
func (c *Counter) CountMessages(msgs []domain.Message) int {
	total := 0
	for _, msg := range msgs {
		total += c.CountMessage(msg)
	}
	return total
}

// CountMessage returns tokens for a single message.
func (c *Counter) CountMessage(msg domain.Message) int {
	// Base overhead per message (role, formatting)
	tokens := 4

	for _, part := range msg.Parts {
		tokens += c.countPart(part)
	}

	return tokens
}

func (c *Counter) countPart(part domain.Part) int {
	switch p := part.(type) {
	case domain.TextPart:
		return c.Count(p.Text)
	case domain.ReasoningPart:
		return c.Count(p.Text)
	case domain.ToolCallPart:
		// Tool name + args + result
		tokens := c.Count(p.Name) + 10 // overhead
		for k, v := range p.Args {
			tokens += c.Count(k)
			if s, ok := v.(string); ok {
				tokens += c.Count(s)
			}
		}
		if p.Result != "" {
			tokens += c.Count(p.Result)
		}
		if p.Error != "" {
			tokens += c.Count(p.Error)
		}
		return tokens
	case domain.FilePart:
		// Path + content
		return c.Count(p.Path) + c.Count(p.Content) + 10
	case domain.ImagePart:
		// Images are counted differently by providers
		// Rough estimate: 85 tokens for low-detail, 765 for high-detail
		if len(p.Base64) > 100000 {
			return 765 // High detail
		}
		return 85 // Low detail
	default:
		return 0
	}
}

func (c *Counter) init() {
	c.once.Do(func() {
		// cl100k_base is used by Claude and GPT-4
		c.enc, c.err = tiktoken.GetEncoding("cl100k_base")
	})
}

// Estimate provides quick token estimates without full encoding.
type Estimate struct{}

// Tokens estimates token count (4 chars per token average).
func (Estimate) Tokens(text string) int {
	// Count words and punctuation roughly
	words := len(strings.Fields(text))
	// Average 1.3 tokens per word
	return int(float64(words) * 1.3)
}

// ShouldCompact returns true if messages exceed threshold.
func ShouldCompact(msgs []domain.Message, threshold int) bool {
	return CountMessages(msgs) > threshold
}
