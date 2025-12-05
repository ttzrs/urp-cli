package cognitive

import (
	"context"
	"testing"

	"github.com/joss/urp/internal/opencode/domain"
)

func TestEngine_Creation(t *testing.T) {
	cfg := DefaultEngineConfig()
	engine := NewEngine(cfg)

	if engine == nil {
		t.Fatal("engine should not be nil")
	}

	if engine.tokenBudget != cfg.TokenBudget {
		t.Errorf("token budget mismatch: got %d, want %d", engine.tokenBudget, cfg.TokenBudget)
	}
}

func TestEngine_StateUpdates(t *testing.T) {
	engine := NewEngine(DefaultEngineConfig())

	// First file edit
	engine.SetCurrentEdit("/app/main.go", "main")
	if !engine.isNewFile {
		t.Error("first file should be marked as new")
	}

	// Same file edit
	engine.SetCurrentEdit("/app/main.go", "helper")
	if engine.editCount != 1 {
		t.Errorf("edit count should be 1, got %d", engine.editCount)
	}

	// Different file
	engine.SetCurrentEdit("/app/other.go", "foo")
	if engine.editCount != 0 {
		t.Error("edit count should reset for new file")
	}
}

func TestEngine_OptimizeMode(t *testing.T) {
	engine := NewEngine(DefaultEngineConfig())

	// Default mode
	mode, _ := engine.OptimizeMode()
	if mode != ModeFocused {
		t.Error("default mode should be focused")
	}

	// With error
	engine.SetError("undefined: FooBar")
	mode, reason := engine.OptimizeMode()
	if mode != ModeFull {
		t.Error("error should trigger full mode")
	}
	if reason == "" {
		t.Error("should have reason")
	}

	// Clear error, many edits
	engine.ClearError()
	engine.SetCurrentEdit("/app/main.go", "main")
	for i := 0; i < 10; i++ {
		engine.SetCurrentEdit("/app/main.go", "main")
	}
	mode, _ = engine.OptimizeMode()
	if mode != ModeMinimal {
		t.Errorf("many edits should trigger minimal mode, got %d", mode)
	}
}

func TestEngine_SignalInjection(t *testing.T) {
	engine := NewEngine(DefaultEngineConfig())

	prompt := "Please fix the bug"

	// No signals
	result, injected := engine.InjectSignals(prompt)
	if injected {
		t.Error("should not inject without signals")
	}
	if result != prompt {
		t.Error("prompt should be unchanged")
	}

	// With error signal
	engine.SetError("undefined: FooBar")
	result, injected = engine.InjectSignals(prompt)
	if !injected {
		t.Error("should inject with error signal")
	}
	if !containsString(result, prompt) {
		t.Error("should contain original prompt")
	}
}

func TestEngine_MemoryHygiene(t *testing.T) {
	engine := NewEngine(DefaultEngineConfig())

	messages := []domain.Message{
		{
			ID:   "1",
			Role: domain.RoleUser,
			Parts: []domain.Part{
				domain.TextPart{Text: "Run tests"},
			},
		},
		{
			ID:   "2",
			Role: domain.RoleAssistant,
			Parts: []domain.Part{
				domain.ToolCallPart{
					Name:   "bash",
					Result: "FAIL: TestFoo",
					Error:  "exit 1",
				},
			},
		},
	}

	// Task not solved - keep all
	cleaned := engine.CleanMessages(messages, false)
	if len(cleaned) < len(messages) {
		t.Error("should keep messages when task not solved")
	}

	// Task solved - can clean up
	cleaned = engine.CleanMessages(messages, true)
	// Should at least keep user message
	if len(cleaned) == 0 {
		t.Error("should keep some messages")
	}
}

func TestEngine_HandleError(t *testing.T) {
	engine := NewEngine(DefaultEngineConfig())
	ctx := context.Background()

	// Without reflex (basic mode)
	tc, err := engine.HandleError(ctx, "undefined: FooBar in main.go:42", "/app/main.go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tc.Error == "" {
		t.Error("should have error")
	}

	// Error state should be set
	if !engine.hasError {
		t.Error("hasError should be true")
	}

	// Format error context
	formatted := engine.FormatErrorContext(tc)
	if !containsString(formatted, "FooBar") {
		t.Error("formatted context should contain error")
	}
}

func TestEngine_Stats(t *testing.T) {
	engine := NewEngine(DefaultEngineConfig())

	engine.SetCurrentEdit("/app/main.go", "main")
	engine.SetError("test error")

	stats := engine.Stats()

	// Check structure
	if _, ok := stats["context"]; !ok {
		t.Error("should have context stats")
	}
	if _, ok := stats["signals"]; !ok {
		t.Error("should have signal stats")
	}
	if _, ok := stats["state"]; !ok {
		t.Error("should have state")
	}

	// Check state values
	state := stats["state"].(map[string]interface{})
	if state["currentFile"] != "/app/main.go" {
		t.Error("should have current file")
	}
	if state["hasError"] != true {
		t.Error("should have error flag set")
	}
}

func TestEngine_ContextBuilding(t *testing.T) {
	engine := NewEngine(EngineConfig{
		TokenBudget: 10000,
		Profile:     ProfileBuild,
		Hygiene:     DefaultHygieneConfig(),
	})

	engine.SetCurrentEdit("/app/main.go", "main")

	// Add context items
	engine.AddContextItem(ContextItem{
		Type:     "file",
		Path:     "/app/main.go",
		Content:  "func main() {}",
		Priority: PriorityEssential,
	})

	engine.AddContextItem(ContextItem{
		Type:     "file",
		Path:     "/app/utils.go",
		Content:  "func helper() {}",
		Priority: PriorityMedium,
	})

	context := engine.BuildContext()

	if !containsString(context, "main.go") {
		t.Error("context should contain main.go")
	}
	if !containsString(context, "[CONTEXT:") {
		t.Error("context should contain mode hint")
	}

	// Clear and verify
	engine.ClearContext()
	context = engine.BuildContext()
	if containsString(context, "main.go") {
		t.Error("context should be cleared")
	}
}

func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
