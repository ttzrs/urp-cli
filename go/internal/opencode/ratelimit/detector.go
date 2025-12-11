// Package ratelimit provides rate limit detection and provider switching functionality.
package ratelimit

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/joss/urp/pkg/llm"
)

// RateLimitError represents a rate limit error with reset information
type RateLimitError struct {
	ProviderID string
	ErrorMsg   string
	ResetTime  time.Time
	ResetIn    time.Duration
}

func (r *RateLimitError) Error() string {
	return fmt.Sprintf("rate limit exceeded for %s: %s (resets at %s)", r.ProviderID, r.ErrorMsg, r.ResetTime.Format(time.RFC3339))
}

// Detector detects rate limit errors and manages provider switching
type Detector struct {
	primaryProvider   llm.Provider
	alternativeProvider llm.Provider
	switchCallback    func(newProvider llm.Provider)
	restoreCallback   func(originalProvider llm.Provider)
}

// NewDetector creates a new rate limit detector
func NewDetector(primary llm.Provider, alternative llm.Provider) *Detector {
	return &Detector{
		primaryProvider:     primary,
		alternativeProvider: alternative,
	}
}

// WithSwitchCallback sets a callback for when provider switching occurs
func (d *Detector) WithSwitchCallback(cb func(newProvider llm.Provider)) *Detector {
	d.switchCallback = cb
	return d
}

// WithRestoreCallback sets a callback for when the original provider is restored
func (d *Detector) WithRestoreCallback(cb func(originalProvider llm.Provider)) *Detector {
	d.restoreCallback = cb
	return d
}

// IsRateLimitError checks if an error indicates a rate limit has been reached
func (d *Detector) IsRateLimitError(err error) (*RateLimitError, bool) {
	if err == nil {
		return nil, false
	}

	errStr := err.Error()
	providerID := "unknown"

	// Get provider ID if possible
	if d.primaryProvider != nil {
		providerID = d.primaryProvider.ID()
	}

	// Check for common rate limit indicators
	if d.containsRateLimitIndicators(errStr) {
		// Try to extract reset time from error message or headers
		resetTime := d.extractResetTime(errStr, nil)
		if resetTime.IsZero() {
			// Default reset time if not found in error
			resetTime = time.Now().Add(1 * time.Hour) // Default to 1 hour
		}
		return &RateLimitError{
			ProviderID: providerID,
			ErrorMsg:   errStr,
			ResetTime:  resetTime,
			ResetIn:    time.Until(resetTime),
		}, true
	}

	return nil, false
}

// containsRateLimitIndicators checks if error message contains rate limit indicators
func (d *Detector) containsRateLimitIndicators(errMsg string) bool {
	lowerErr := strings.ToLower(errMsg)

	// Common rate limit patterns
	patterns := []string{
		"rate limit",
		"429",
		"too many requests",
		"exceeded",
		"quota",
		"requests per minute",
		"requests per hour",
		"requests per day",
		"api limit",
		"usage limit",
		"over quota",
		"usage quota",
		"limit has been reached",
		"try again later",
		"try again in",
	}

	for _, pattern := range patterns {
		if strings.Contains(lowerErr, pattern) {
			return true
		}
	}

	// Regex patterns for more complex matching
	regexPatterns := []string{
		`requests.*limit`,
		`quota.*exceeded`,
		`limit.*reached`,
	}

	for _, pattern := range regexPatterns {
		matched, _ := regexp.MatchString(pattern, lowerErr)
		if matched {
			return true
		}
	}

	return false
}

// extractResetTime attempts to extract reset time from error message or response headers
func (d *Detector) extractResetTime(errMsg string, headers http.Header) time.Time {
	// Try to parse reset time from common header values
	if headers != nil {
		// Check common rate limit headers
		if resetStr := headers.Get("X-RateLimit-Reset"); resetStr != "" {
			if resetUnix, err := parseUnixTimestamp(resetStr); err == nil {
				return resetUnix
			}
		}

		if resetStr := headers.Get("X-Ratelimit-Reset"); resetStr != "" {
			if resetUnix, err := parseUnixTimestamp(resetStr); err == nil {
				return resetUnix
			}
		}

		if resetStr := headers.Get("Retry-After"); resetStr != "" {
			if retryAfter, err := parseRetryAfter(resetStr); err == nil {
				return time.Now().Add(time.Duration(retryAfter) * time.Second)
			}
		}
	}

	// Try to parse reset time from error message
	if resetTime := d.parseResetFromError(errMsg); !resetTime.IsZero() {
		return resetTime
	}

	return time.Time{}
}

// parseResetFromError extracts reset time from error message
func (d *Detector) parseResetFromError(errMsg string) time.Time {
	// Look for time patterns in the error message
	patterns := []string{
		// Patterns like "Rate limit exceeded. Try again in 1 hour"
		`try again in (\d+)\s*(hour|hours)`,
		`try again in (\d+)\s*(minute|minutes)`,
		`try again in (\d+)\s*(second|seconds)`,

		// Patterns like "Please try again in 1h30m"
		`try again in ([0-9hms]+)`,

		// Look for reset times
		`resets at (\d{1,2}:\d{2})`,
		`reset time: (.+)`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(errMsg)
		if len(matches) > 0 {
			if len(matches) > 2 { // Hours/minutes/seconds pattern
				if count, err := parseInt(matches[1]); err == nil {
					var duration time.Duration
					switch matches[2] {
					case "hour", "hours":
						duration = time.Duration(count) * time.Hour
					case "minute", "minutes":
						duration = time.Duration(count) * time.Minute
					case "second", "seconds":
						duration = time.Duration(count) * time.Second
					}
					if duration > 0 {
						return time.Now().Add(duration)
					}
				}
			} else if len(matches) > 1 { // Simple time or duration pattern
				if match := matches[1]; match != "" {
					// Try to parse as duration (e.g., "1h30m")
					if duration, err := time.ParseDuration(match); err == nil {
						return time.Now().Add(duration)
					}
					// Try to parse as time string (e.g., "14:30")
					if strings.Contains(match, ":") {
						now := time.Now()
						var resetTime time.Time
						if parsed, err := time.Parse("15:04", match); err == nil {
							resetTime = time.Date(now.Year(), now.Month(), now.Day(), parsed.Hour(), parsed.Minute(), 0, 0, now.Location())
							if resetTime.Before(now) {
								resetTime = resetTime.Add(24 * time.Hour) // Tomorrow
							}
						}
						if !resetTime.IsZero() && resetTime.After(now) {
							return resetTime
						}
					}
				}
			}
		}
	}

	return time.Time{}
}

// SwitchToAlternative switches to the alternative provider
func (d *Detector) SwitchToAlternative() llm.Provider {
	if d.alternativeProvider != nil && d.switchCallback != nil {
		d.switchCallback(d.alternativeProvider)
	}
	return d.alternativeProvider
}

// RestorePrimary restores the primary provider
func (d *Detector) RestorePrimary() llm.Provider {
	if d.primaryProvider != nil && d.restoreCallback != nil {
		d.restoreCallback(d.primaryProvider)
	}
	return d.primaryProvider
}

// parseUnixTimestamp parses a Unix timestamp string
func parseUnixTimestamp(ts string) (time.Time, error) {
	if ts == "" {
		return time.Time{}, fmt.Errorf("empty timestamp")
	}

	var unix int64
	if _, err := fmt.Sscanf(ts, "%d", &unix); err != nil {
		return time.Time{}, err
	}

	return time.Unix(unix, 0), nil
}

// parseRetryAfter parses Retry-After header (can be seconds or date)
func parseRetryAfter(retryAfter string) (int64, error) {
	if retryAfter == "" {
		return 0, fmt.Errorf("empty retry-after")
	}

	// Try parsing as seconds first
	if seconds, err := parseInt(retryAfter); err == nil {
		return int64(seconds), nil
	}

	// Try parsing as date (RFC1123 format)
	if date, err := time.Parse(http.TimeFormat, retryAfter); err == nil {
		return int64(time.Until(date).Seconds()), nil
	}

	return 0, fmt.Errorf("unable to parse retry-after: %s", retryAfter)
}

// Helper functions
func parseInt(s string) (int, error) {
	var i int
	_, err := fmt.Sscanf(s, "%d", &i)
	return i, err
}