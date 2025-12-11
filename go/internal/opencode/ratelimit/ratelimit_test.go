// Package ratelimit tests for rate limit detection and provider switching
package ratelimit

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/joss/urp/internal/opencode/domain"
	"github.com/joss/urp/pkg/llm"
)

// MockProvider implements llm.Provider for testing
type MockProvider struct {
	id       string
	failWith string
	delay    time.Duration
}

func (m *MockProvider) ID() string { return m.id }
func (m *MockProvider) Name() string { return m.id }
func (m *MockProvider) Models() []domain.Model { return []domain.Model{} }

func (m *MockProvider) Chat(ctx context.Context, req *llm.ChatRequest) (<-chan domain.StreamEvent, error) {
	if m.delay > 0 {
		time.Sleep(m.delay)
	}

	events := make(chan domain.StreamEvent, 1)

	if m.failWith != "" {
		close(events)
		return nil, errors.New(m.failWith)
	}

	// Send a simple response
	go func() {
		defer close(events)
		events <- domain.StreamEvent{
			Type:  domain.StreamEventDone,
			Done:  true,
		}
	}()

	return events, nil
}

func TestRateLimitDetector_Identification(t *testing.T) {
	detector := NewDetector(&MockProvider{id: "primary"}, &MockProvider{id: "alternative"})
	
	testCases := []struct {
		name          string
		errorMsg      string
		expectRateLimit bool
	}{
		{
			name:          "429 status code",
			errorMsg:      "OpenAI API error 429: Rate limit exceeded",
			expectRateLimit: true,
		},
		{
			name:          "rate limit in message",
			errorMsg:      "Request limit exceeded for tokens per hour",
			expectRateLimit: true,
		},
		{
			name:          "quota exceeded",
			errorMsg:      "Usage quota exceeded for current month",
			expectRateLimit: true,
		},
		{
			name:          "normal error",
			errorMsg:      "Invalid API key",
			expectRateLimit: false,
		},
		{
			name:          "too many requests",
			errorMsg:      "Too many requests, please try again later",
			expectRateLimit: true,
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, isRateLimit := detector.IsRateLimitError(errors.New(tc.errorMsg))
			if isRateLimit != tc.expectRateLimit {
				t.Errorf("Expected rate limit: %v, got: %v", tc.expectRateLimit, isRateLimit)
			}
		})
	}
}

func TestProviderManager_SwitchAndRestore(t *testing.T) {
	primary := &MockProvider{id: "primary", failWith: "OpenAI API error 429: Rate limit exceeded. Try again in 1 minute"}
	alternative := &MockProvider{id: "alternative"}
	
	manager := NewProviderManager(primary, alternative)
	
	// Initially should use primary
	current, isUsingAlternative, _ := manager.GetStatus()
	if current.ID() != "primary" || isUsingAlternative {
		t.Error("Should start with primary provider")
	}
	
	// Simulate rate limit error
	provider, isRateLimit := manager.CheckAndHandleError(errors.New("OpenAI API error 429: Rate limit exceeded. Try again in 1 minute"))
	if !isRateLimit {
		t.Error("Should detect rate limit")
	}
	if provider.ID() != "alternative" {
		t.Errorf("Should switch to alternative, got %s", provider.ID())
	}
	
	// Check status after switching
	current, isUsingAlternative, resetTime := manager.GetStatus()
	if current.ID() != "alternative" || !isUsingAlternative {
		t.Error("Should be using alternative after switch")
	}
	if resetTime.IsZero() {
		t.Error("Should have a reset time")
	}
	
	manager.Close()
}

func TestExtractResetTime(t *testing.T) {
	detector := NewDetector(&MockProvider{id: "primary"}, &MockProvider{id: "alternative"})
	
	testCases := []struct {
		name     string
		errorMsg string
		hasReset bool
	}{
		{
			name:     "has reset time",
			errorMsg: "Rate limit exceeded. Try again in 5 minutes",
			hasReset: true,
		},
		{
			name:     "no reset time",
			errorMsg: "Rate limit exceeded",
			hasReset: false,
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err, isRateLimit := detector.IsRateLimitError(errors.New(tc.errorMsg))
			if !isRateLimit && tc.hasReset {
				t.Errorf("Should detect rate limit")
			}
			if isRateLimit && tc.hasReset && err.ResetTime.IsZero() {
				t.Errorf("Should extract reset time from: %s", tc.errorMsg)
			}
		})
	}
}