package cognitive

import (
	"testing"

	"github.com/joss/urp/internal/strings"
)

func TestSignalInjector_BasicSignals(t *testing.T) {
	si := NewSignalInjector(ProfileBuild)

	// No signals initially
	if si.HasSignals() {
		t.Error("should have no signals initially")
	}

	// Add error signal
	si.AddError("bash", "exit status 1: undefined: FooBar")

	if !si.HasSignals() {
		t.Error("should have signals after adding")
	}

	if !si.HasUrgent() {
		t.Error("error signals should be urgent")
	}

	block := si.BuildContextBlock()
	if !containsStr(block, "⚡") {
		t.Error("error block should contain error icon")
	}
	if !containsStr(block, "FooBar") {
		t.Error("error block should contain error message")
	}
}

func TestSignalInjector_MemoryPressure(t *testing.T) {
	si := NewSignalInjector(ProfileGeneric)

	// Below threshold - no signal
	si.SetTokenUsage(0.5)
	if si.HasSignals() {
		t.Error("50% usage should not trigger signal")
	}

	// Warning threshold
	si.SetTokenUsage(0.75)
	if !si.HasSignals() {
		t.Error("75% usage should trigger warning")
	}

	block := si.BuildContextBlock()
	if !containsStr(block, "75%") {
		t.Errorf("should show usage percentage, got: %s", block)
	}

	// Critical threshold
	si.Clear()
	si.SetTokenUsage(0.95)
	if !si.HasUrgent() {
		t.Error("95% usage should be urgent")
	}

	block = si.BuildContextBlock()
	if !containsStr(block, "CRITICAL") {
		t.Error("95% should show CRITICAL")
	}
}

func TestSignalInjector_Deduplication(t *testing.T) {
	si := NewSignalInjector(ProfileBuild)

	// Add same type multiple times
	si.AddError("bash", "error 1")
	si.AddError("bash", "error 2")
	si.AddError("bash", "error 3")

	stats := si.SignalStats()
	if stats["total"] != 1 {
		t.Errorf("should deduplicate same type, got %d", stats["total"])
	}

	// Should have latest error
	block := si.BuildContextBlock()
	if !containsStr(block, "error 3") {
		t.Error("should have latest error message")
	}
}

func TestSignalInjector_ProfileFormatting(t *testing.T) {
	tests := []struct {
		profile  AgentProfile
		contains string
	}{
		{ProfileBuild, "go test"},
		{ProfileDebug, "Trace"},
		{ProfileRefactor, "smaller steps"},
	}

	for _, tt := range tests {
		si := NewSignalInjector(tt.profile)
		si.AddError("bash", "some error")

		block := si.BuildContextBlock()
		if !containsStr(block, tt.contains) {
			t.Errorf("profile %d should contain %q, got: %s", tt.profile, tt.contains, block)
		}
	}
}

func TestSignalInjector_DockerSignals(t *testing.T) {
	si := NewSignalInjector(ProfileBuild)

	// Initially healthy
	si.SetDockerHealth(true, "")
	if si.HasSignals() {
		t.Error("healthy docker should not trigger signal")
	}

	// Becomes unhealthy
	si.SetDockerHealth(false, "container exited")
	if !si.HasSignals() {
		t.Error("unhealthy docker should trigger signal")
	}

	block := si.BuildContextBlock()
	if !containsStr(block, "🐳") {
		t.Error("should have docker icon")
	}
}

func TestSignalInjector_RetrySignals(t *testing.T) {
	si := NewSignalInjector(ProfileBuild)

	si.AddRetry(2, 3)

	block := si.BuildContextBlock()
	if !containsStr(block, "2/3") {
		t.Error("should show attempt count")
	}
	if !containsStr(block, "AUTOCORRECT") {
		t.Error("build profile should mention autocorrect")
	}
}

func TestSignalInjector_ClearType(t *testing.T) {
	si := NewSignalInjector(ProfileGeneric)

	si.AddError("bash", "error")
	si.SetTokenUsage(0.8)
	si.SetDockerHealth(false, "down")

	// Clear only errors
	si.ClearType(SignalError)

	block := si.BuildContextBlock()
	if containsStr(block, "⚡") {
		t.Error("error signals should be cleared")
	}
	if !containsStr(block, "📊") {
		t.Error("memory signals should remain")
	}
}

func TestSignalInjector_MaxSignals(t *testing.T) {
	si := NewSignalInjector(ProfileGeneric)
	si.maxSignals = 3

	// Add more than max (different types)
	si.AddSignal(Signal{Type: SignalError, Message: "e1"})
	si.AddSignal(Signal{Type: SignalTimeout, Message: "t1"})
	si.AddSignal(Signal{Type: SignalNetworkError, Message: "n1"})
	si.AddSignal(Signal{Type: SignalPermDenied, Message: "p1"})

	stats := si.SignalStats()
	if stats["total"] > 3 {
		t.Errorf("should cap at max signals, got %d", stats["total"])
	}
}

func TestTruncate(t *testing.T) {
	short := "hello"
	if strings.Truncate(short, 10) != short {
		t.Error("should not truncate short string")
	}

	long := "this is a very long error message"
	truncated := strings.Truncate(long, 15)
	if len(truncated) > 15 {
		t.Errorf("truncated too long: %d", len(truncated))
	}
	if !containsStr(truncated, "...") {
		t.Error("should end with ...")
	}
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// ---- Injection Policy Tests ----

func TestShouldInject_NoSignals(t *testing.T) {
	si := NewSignalInjector(ProfileBuild)

	decision := si.ShouldInject(1000, 200000)
	if decision.Inject {
		t.Error("should not inject when no signals")
	}
}

func TestShouldInject_UrgentAlwaysInjects(t *testing.T) {
	si := NewSignalInjector(ProfileBuild)

	// Add urgent signal
	si.AddError("bash", "critical error")

	decision := si.ShouldInject(1000, 200000)
	if !decision.Inject {
		t.Error("urgent signals should always inject")
	}
	if decision.Policy != PolicyImmediate {
		t.Error("urgent signals should use immediate policy")
	}
}

func TestShouldInject_MemoryPressureBundles(t *testing.T) {
	si := NewSignalInjector(ProfileBuild)

	// Non-urgent, low memory
	si.SetGraphHealth(false) // Not urgent
	si.SetTokenUsage(0.5)

	decision := si.ShouldInject(1000, 200000)
	if decision.Inject {
		t.Error("non-urgent at 50% should not inject")
	}

	// Non-urgent, high memory
	si.SetTokenUsage(0.75)
	decision = si.ShouldInject(1000, 200000)
	if !decision.Inject {
		t.Error("high memory should trigger injection")
	}
	if decision.Policy != PolicyWithNormal {
		t.Errorf("high memory should bundle with normal, got %d", decision.Policy)
	}
}

func TestShouldInject_ErrorTriggersOnTrigger(t *testing.T) {
	si := NewSignalInjector(ProfileBuild)

	// Add non-urgent error (low memory)
	si.AddSignal(Signal{
		Type:    SignalError,
		Message: "minor error",
		Urgent:  false,
	})
	si.SetTokenUsage(0.3) // Low memory

	decision := si.ShouldInject(1000, 200000)
	if !decision.Inject {
		t.Error("error signals should trigger injection")
	}
	if decision.Policy != PolicyOnTrigger {
		t.Error("error should use on-trigger policy")
	}
}

func TestConsumeSignals_ClearsAfterConsume(t *testing.T) {
	si := NewSignalInjector(ProfileBuild)
	si.AddError("bash", "error 1")

	block := si.ConsumeSignals(PolicyImmediate)
	if block == "" {
		t.Error("should have produced block")
	}

	// Signals should be cleared
	if si.HasSignals() {
		t.Error("signals should be cleared after consume")
	}

	// Second consume should be empty
	block2 := si.ConsumeSignals(PolicyImmediate)
	if block2 != "" {
		t.Error("second consume should be empty")
	}
}

func TestInjectIntoPrompt(t *testing.T) {
	si := NewSignalInjector(ProfileBuild)

	prompt := "Please fix the bug"

	// No signals - no injection
	result, injected := si.InjectIntoPrompt(prompt, 200000)
	if injected {
		t.Error("should not inject without signals")
	}
	if result != prompt {
		t.Error("prompt should be unchanged")
	}

	// Add urgent signal
	si.AddError("bash", "undefined: FooBar")

	result, injected = si.InjectIntoPrompt(prompt, 200000)
	if !injected {
		t.Error("should inject with urgent signal")
	}
	if !containsStr(result, "<system-signals>") {
		t.Error("should contain signal block")
	}
	if !containsStr(result, prompt) {
		t.Error("should still contain original prompt")
	}

	// Signals consumed - next inject should not happen
	_, injected = si.InjectIntoPrompt(prompt, 200000)
	if injected {
		t.Error("signals should be consumed after inject")
	}
}

func TestShouldInject_ReplacesNormalWhenTight(t *testing.T) {
	si := NewSignalInjector(ProfileBuild)
	si.AddError("bash", "critical error that needs attention now please fix immediately")

	// Signal block is roughly 200-300 chars = ~75 tokens
	// Budget: 100 tokens, normal msg uses 90 → only 10 left for signals
	// So signals should replace normal content
	decision := si.ShouldInject(90, 100)

	if !decision.ReplacesNormal {
		t.Errorf("should indicate replacement when budget is tight (signal tokens: %d)", decision.TokenBudget)
	}
}
