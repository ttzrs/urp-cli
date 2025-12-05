// Package cognitive implements system signals injection
// Micro-messages injected into LLM context based on system state
package cognitive

import (
	"fmt"
	"strings"

	urpstrings "github.com/joss/urp/internal/strings"
)

// SignalType represents the type of system signal
type SignalType int

const (
	SignalError        SignalType = iota // Tool/command failed
	SignalMemoryLow                      // Context window filling up
	SignalMemoryCrit                     // Critical - must compact
	SignalDockerError                    // Container/orchestration issue
	SignalGraphOffline                   // Memgraph unavailable
	SignalRetry                          // Autocorrection in progress
	SignalTimeout                        // Operation timed out
	SignalPermDenied                     // Permission denied
	SignalNetworkError                   // Network connectivity issue
)

// Signal represents a system notification for the LLM
type Signal struct {
	Type    SignalType
	Message string
	Context map[string]string // Additional context
	Urgent  bool              // If true, inject immediately
}

// AgentProfile defines how signals are formatted for different agent types
type AgentProfile int

const (
	ProfileBuild   AgentProfile = iota // Code-focused agent
	ProfileExplore                     // Research/exploration agent
	ProfileRefactor                    // Cleanup/optimization agent
	ProfileDebug                       // Debugging specialist
	ProfileGeneric                     // Generic assistant
)

// SignalInjector manages context signal injection
type SignalInjector struct {
	profile       AgentProfile
	signals       []Signal
	tokenUsage    float64 // 0.0 - 1.0
	maxSignals    int
	dockerHealthy bool
	graphHealthy  bool
}

// NewSignalInjector creates a new signal injector
func NewSignalInjector(profile AgentProfile) *SignalInjector {
	return &SignalInjector{
		profile:       profile,
		signals:       make([]Signal, 0),
		maxSignals:    5, // Max signals to inject at once
		dockerHealthy: true,
		graphHealthy:  true,
	}
}

// SetTokenUsage updates current token usage (0.0 - 1.0)
func (s *SignalInjector) SetTokenUsage(usage float64) {
	s.tokenUsage = usage

	// Auto-generate memory signals
	if usage > 0.9 {
		s.AddSignal(Signal{
			Type:    SignalMemoryCrit,
			Message: fmt.Sprintf("%.0f%% context used", usage*100),
			Urgent:  true,
		})
	} else if usage > 0.7 {
		s.AddSignal(Signal{
			Type:    SignalMemoryLow,
			Message: fmt.Sprintf("%.0f%% context used", usage*100),
			Urgent:  false,
		})
	}
}

// SetDockerHealth updates Docker/container health status
func (s *SignalInjector) SetDockerHealth(healthy bool, err string) {
	if s.dockerHealthy && !healthy {
		s.AddSignal(Signal{
			Type:    SignalDockerError,
			Message: err,
			Urgent:  true,
		})
	}
	s.dockerHealthy = healthy
}

// SetGraphHealth updates Memgraph connection status
func (s *SignalInjector) SetGraphHealth(healthy bool) {
	if s.graphHealthy && !healthy {
		s.AddSignal(Signal{
			Type:    SignalGraphOffline,
			Message: "Graph database unavailable",
			Urgent:  false,
		})
	}
	s.graphHealthy = healthy
}

// AddSignal adds a signal to the queue
func (s *SignalInjector) AddSignal(sig Signal) {
	// Deduplicate by type
	for i, existing := range s.signals {
		if existing.Type == sig.Type {
			s.signals[i] = sig // Replace with newer
			return
		}
	}

	s.signals = append(s.signals, sig)

	// Trim to max
	if len(s.signals) > s.maxSignals {
		s.signals = s.signals[len(s.signals)-s.maxSignals:]
	}
}

// AddError adds an error signal
func (s *SignalInjector) AddError(toolName, errorMsg string) {
	s.AddSignal(Signal{
		Type:    SignalError,
		Message: errorMsg,
		Context: map[string]string{"tool": toolName},
		Urgent:  true,
	})
}

// AddRetry adds a retry signal
func (s *SignalInjector) AddRetry(attempt, maxAttempts int) {
	s.AddSignal(Signal{
		Type:    SignalRetry,
		Message: fmt.Sprintf("Attempt %d/%d", attempt, maxAttempts),
		Urgent:  true,
	})
}

// Clear removes all signals
func (s *SignalInjector) Clear() {
	s.signals = s.signals[:0]
}

// ClearType removes signals of a specific type
func (s *SignalInjector) ClearType(t SignalType) {
	filtered := make([]Signal, 0, len(s.signals))
	for _, sig := range s.signals {
		if sig.Type != t {
			filtered = append(filtered, sig)
		}
	}
	s.signals = filtered
}

// HasSignals returns true if there are pending signals
func (s *SignalInjector) HasSignals() bool {
	return len(s.signals) > 0
}

// HasUrgent returns true if there are urgent signals
func (s *SignalInjector) HasUrgent() bool {
	for _, sig := range s.signals {
		if sig.Urgent {
			return true
		}
	}
	return false
}

// BuildContextBlock builds the signal block for injection into LLM context
// Returns empty string if no relevant signals
func (s *SignalInjector) BuildContextBlock() string {
	if len(s.signals) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("\n<system-signals>\n")

	for _, sig := range s.signals {
		formatted := s.formatSignal(sig)
		if formatted != "" {
			b.WriteString(formatted)
			b.WriteString("\n")
		}
	}

	b.WriteString("</system-signals>\n")
	return b.String()
}

// formatSignal formats a signal based on agent profile
func (s *SignalInjector) formatSignal(sig Signal) string {
	// Get icon and base message
	icon, action := s.getSignalMeta(sig)

	switch s.profile {
	case ProfileBuild:
		return s.formatBuildProfile(icon, sig, action)
	case ProfileDebug:
		return s.formatDebugProfile(icon, sig, action)
	case ProfileRefactor:
		return s.formatRefactorProfile(icon, sig, action)
	default:
		return s.formatGenericProfile(icon, sig, action)
	}
}

// getSignalMeta returns icon and suggested action for a signal type
func (s *SignalInjector) getSignalMeta(sig Signal) (icon, action string) {
	switch sig.Type {
	case SignalError:
		return "⚡", "Analyze and fix the error before continuing"
	case SignalMemoryLow:
		return "📊", "Consider summarizing completed work"
	case SignalMemoryCrit:
		return "🚨", "CRITICAL: Summarize now or context will be lost"
	case SignalDockerError:
		return "🐳", "Container issue - check docker status"
	case SignalGraphOffline:
		return "🔌", "Graph DB offline - context enrichment unavailable"
	case SignalRetry:
		return "🔄", "Previous attempt failed - try alternative approach"
	case SignalTimeout:
		return "⏱️", "Operation timed out - check if service is responsive"
	case SignalPermDenied:
		return "🔒", "Permission denied - may need different approach"
	case SignalNetworkError:
		return "🌐", "Network error - check connectivity"
	default:
		return "ℹ️", ""
	}
}

// Profile-specific formatters

func (s *SignalInjector) formatBuildProfile(icon string, sig Signal, action string) string {
	switch sig.Type {
	case SignalError:
		tool := sig.Context["tool"]
		if tool == "bash" || tool == "execute" {
			return fmt.Sprintf("%s BUILD ERROR: %s\n   → Run tests to verify fix: `go test ./...`", icon, urpstrings.Truncate(sig.Message, 100))
		}
		return fmt.Sprintf("%s ERROR in %s: %s", icon, tool, urpstrings.Truncate(sig.Message, 100))

	case SignalMemoryCrit:
		return fmt.Sprintf("%s MEMORY CRITICAL (%s)\n   → Complete current task, then summarize progress", icon, sig.Message)

	case SignalRetry:
		return fmt.Sprintf("%s AUTOCORRECT %s\n   → Previous fix failed. Try: read error, check imports, verify types", icon, sig.Message)

	default:
		return fmt.Sprintf("%s %s", icon, sig.Message)
	}
}

func (s *SignalInjector) formatDebugProfile(icon string, sig Signal, action string) string {
	switch sig.Type {
	case SignalError:
		return fmt.Sprintf("%s DEBUG REQUIRED: %s\n   → Trace: check stack, variables, state", icon, urpstrings.Truncate(sig.Message, 150))

	case SignalMemoryCrit:
		return fmt.Sprintf("%s MEMORY CRITICAL\n   → Capture findings before context loss", icon)

	case SignalRetry:
		return fmt.Sprintf("%s RETRY %s\n   → Different debugging strategy needed", icon, sig.Message)

	default:
		return fmt.Sprintf("%s %s", icon, sig.Message)
	}
}

func (s *SignalInjector) formatRefactorProfile(icon string, sig Signal, action string) string {
	switch sig.Type {
	case SignalError:
		return fmt.Sprintf("%s REFACTOR BROKE SOMETHING: %s\n   → Revert partial changes, refactor in smaller steps", icon, urpstrings.Truncate(sig.Message, 100))

	case SignalMemoryCrit:
		return fmt.Sprintf("%s MEMORY CRITICAL\n   → Commit current refactoring before proceeding", icon)

	default:
		return fmt.Sprintf("%s %s", icon, sig.Message)
	}
}

func (s *SignalInjector) formatGenericProfile(icon string, sig Signal, action string) string {
	base := fmt.Sprintf("%s %s", icon, sig.Message)
	if action != "" {
		return base + "\n   → " + action
	}
	return base
}

// SignalStats returns statistics about current signals
func (s *SignalInjector) SignalStats() map[string]int {
	stats := map[string]int{
		"total":  len(s.signals),
		"urgent": 0,
	}

	for _, sig := range s.signals {
		if sig.Urgent {
			stats["urgent"]++
		}
	}

	return stats
}

// InjectionPolicy controls when and how signals are injected
type InjectionPolicy int

const (
	PolicyNever      InjectionPolicy = iota // Never inject (disabled)
	PolicyOnTrigger                         // Only when triggered (default)
	PolicyWithNormal                        // Bundle with next normal message
	PolicyImmediate                         // Inject immediately (for urgent)
)

// InjectionDecision represents the result of ShouldInject
type InjectionDecision struct {
	Inject         bool            // Should we inject signals?
	Policy         InjectionPolicy // How to inject
	Block          string          // Pre-built signal block
	ReplacesNormal bool            // True if signals replace normal content
	TokenBudget    int             // Tokens used by signals
}

// ShouldInject determines if and how signals should be injected
// This is the key logic: signals don't always go, only when relevant
func (s *SignalInjector) ShouldInject(normalMsgTokens int, totalBudget int) InjectionDecision {
	decision := InjectionDecision{
		Inject: false,
		Policy: PolicyNever,
	}

	if len(s.signals) == 0 {
		return decision
	}

	// Calculate signal block tokens
	block := s.BuildContextBlock()
	signalTokens := len(block) / 4 // Rough estimate

	decision.Block = block
	decision.TokenBudget = signalTokens

	// CASE 1: Urgent signals - always inject, may replace normal content
	if s.HasUrgent() {
		decision.Inject = true
		decision.Policy = PolicyImmediate

		// If we're tight on budget, signals can replace some normal content
		remainingBudget := totalBudget - normalMsgTokens
		if remainingBudget < signalTokens {
			decision.ReplacesNormal = true
		}
		return decision
	}

	// CASE 2: Memory pressure > 70% - bundle with normal message
	if s.tokenUsage > 0.7 {
		decision.Inject = true
		decision.Policy = PolicyWithNormal
		return decision
	}

	// CASE 3: Has error signals - inject with next tool result
	for _, sig := range s.signals {
		if sig.Type == SignalError || sig.Type == SignalRetry {
			decision.Inject = true
			decision.Policy = PolicyOnTrigger
			return decision
		}
	}

	// CASE 4: Non-urgent, low pressure - skip until next trigger
	return decision
}

// ConsumeSignals returns and clears signals that should be sent
// Only call this when actually sending to LLM
func (s *SignalInjector) ConsumeSignals(policy InjectionPolicy) string {
	if len(s.signals) == 0 {
		return ""
	}

	// For immediate policy, send all
	if policy == PolicyImmediate {
		block := s.BuildContextBlock()
		s.Clear()
		return block
	}

	// For normal bundling, only send non-urgent
	if policy == PolicyWithNormal {
		var toSend []Signal
		var toKeep []Signal

		for _, sig := range s.signals {
			if sig.Urgent {
				toSend = append(toSend, sig)
			} else {
				toKeep = append(toKeep, sig)
			}
		}

		// Build block from toSend only if any
		if len(toSend) == 0 {
			return ""
		}

		oldSignals := s.signals
		s.signals = toSend
		block := s.BuildContextBlock()
		s.signals = toKeep

		// If we sent urgent, and there were non-urgent, restore
		if len(toKeep) == 0 {
			s.signals = nil
		}

		_ = oldSignals
		return block
	}

	// For trigger policy, send matching signals
	if policy == PolicyOnTrigger {
		var toSend []Signal
		var toKeep []Signal

		for _, sig := range s.signals {
			if sig.Type == SignalError || sig.Type == SignalRetry || sig.Urgent {
				toSend = append(toSend, sig)
			} else {
				toKeep = append(toKeep, sig)
			}
		}

		if len(toSend) == 0 {
			return ""
		}

		s.signals = toSend
		block := s.BuildContextBlock()
		s.signals = toKeep
		return block
	}

	return ""
}

// InjectIntoPrompt injects signals into a prompt if appropriate
// Returns modified prompt and whether injection happened
func (s *SignalInjector) InjectIntoPrompt(prompt string, totalBudget int) (string, bool) {
	promptTokens := len(prompt) / 4

	decision := s.ShouldInject(promptTokens, totalBudget)
	if !decision.Inject {
		return prompt, false
	}

	block := s.ConsumeSignals(decision.Policy)
	if block == "" {
		return prompt, false
	}

	// Prepend signals to prompt
	return block + "\n" + prompt, true
}
