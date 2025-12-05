package cognitive

import (
	"strings"
	"testing"
)

func TestContextOptimizer_Modes(t *testing.T) {
	tests := []struct {
		mode       ContextMode
		budgetPct  float64
		name       string
	}{
		{ModeFull, 0.90, "full"},
		{ModeFocused, 0.50, "focused"},
		{ModeMinimal, 0.20, "minimal"},
		{ModeDelta, 0.15, "delta"},
		{ModeMemory, 0.10, "memory"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opt := NewContextOptimizer(100000)
			opt.SetMode(tt.mode)

			if opt.getBudgetPercentage() != tt.budgetPct {
				t.Errorf("mode %s: got %.2f, want %.2f", tt.name, opt.getBudgetPercentage(), tt.budgetPct)
			}
		})
	}
}

func TestContextOptimizer_Priority(t *testing.T) {
	opt := NewContextOptimizer(100000)
	opt.SetCurrentEdit("/app/handler.go", "HandleRequest")

	// Current file should be essential
	item1 := ContextItem{
		Type:    "file",
		Path:    "/app/handler.go",
		Content: "package app",
	}
	if opt.inferPriority(item1) != PriorityEssential {
		t.Error("current file should be essential")
	}

	// Same directory should be medium
	item2 := ContextItem{
		Type:    "file",
		Path:    "/app/utils.go",
		Content: "package app",
	}
	if opt.inferPriority(item2) != PriorityMedium {
		t.Error("same directory should be medium")
	}

	// Different directory should be low
	item3 := ContextItem{
		Type:    "file",
		Path:    "/other/something.go",
		Content: "package other",
	}
	if opt.inferPriority(item3) != PriorityLow {
		t.Error("different directory should be low")
	}
}

func TestContextOptimizer_Build(t *testing.T) {
	opt := NewContextOptimizer(1000) // Small budget for testing
	opt.SetMode(ModeFocused)
	opt.SetCurrentEdit("/app/main.go", "main")

	// Add items with different priorities
	opt.AddItem(ContextItem{
		Type:     "file",
		Path:     "/app/main.go",
		Content:  "func main() { /* code */ }",
		Priority: PriorityEssential,
	})

	opt.AddItem(ContextItem{
		Type:     "file",
		Path:     "/app/utils.go",
		Content:  "func helper() {}",
		Priority: PriorityMedium,
	})

	opt.AddItem(ContextItem{
		Type:     "file",
		Path:     "/docs/readme.md",
		Content:  strings.Repeat("documentation ", 1000), // Large, low priority
		Priority: PriorityLow,
	})

	result := opt.Build()

	// Essential item should be included
	if !strings.Contains(result, "main.go") {
		t.Error("essential item should be included")
	}

	// Low priority large item should be skipped due to budget
	if strings.Contains(result, "documentation") {
		t.Error("large low-priority item should be skipped")
	}

	// Should have mode hint
	if !strings.Contains(result, "[CONTEXT:") {
		t.Error("should include mode hint")
	}
}

func TestContextOptimizer_ErrorContext(t *testing.T) {
	opt := NewContextOptimizer(10000)
	opt.SetError("undefined: FooBar")

	result := opt.Build()

	if !strings.Contains(result, "<current-error>") {
		t.Error("should include error block")
	}
	if !strings.Contains(result, "FooBar") {
		t.Error("should include error message")
	}
}

func TestContextOptimizer_MemoryMode(t *testing.T) {
	opt := NewContextOptimizer(10000)
	opt.SetMode(ModeMemory)
	opt.SetMemoryState("Working on auth module. Already implemented JWT signing.")

	result := opt.Build()

	if !strings.Contains(result, "<memory-state>") {
		t.Error("memory mode should include memory state")
	}
	if !strings.Contains(result, "JWT signing") {
		t.Error("should include memory content")
	}
}

func TestContextOptimizer_AutoSelectMode(t *testing.T) {
	opt := NewContextOptimizer(100000)

	// Error → Full
	mode := opt.AutoSelectMode(true, false, 0)
	if mode != ModeFull {
		t.Error("error should trigger full mode")
	}

	// New file → Full
	mode = opt.AutoSelectMode(false, true, 0)
	if mode != ModeFull {
		t.Error("new file should trigger full mode")
	}

	// Many edits → Minimal
	mode = opt.AutoSelectMode(false, false, 10)
	if mode != ModeMinimal {
		t.Error("many edits should trigger minimal mode")
	}

	// Few edits → Focused
	mode = opt.AutoSelectMode(false, false, 3)
	if mode != ModeFocused {
		t.Error("few edits should trigger focused mode")
	}
}

func TestContextOptimizer_TruncateItem(t *testing.T) {
	opt := NewContextOptimizer(100000)

	longContent := strings.Repeat("x", 10000)
	item := ContextItem{
		Type:    "code",
		Path:    "/app/big.go",
		Content: longContent,
		Tokens:  2500,
	}

	truncated := opt.truncateItem(item, 100)

	if truncated.Tokens > 100 {
		t.Errorf("truncated tokens should be <= 100, got %d", truncated.Tokens)
	}

	if !strings.Contains(truncated.Content, "[truncated]") {
		t.Error("truncated content should indicate truncation")
	}
}

func TestContextOptimizer_EditHistory(t *testing.T) {
	opt := NewContextOptimizer(100000)

	// Edit several files
	for i := 0; i < 15; i++ {
		opt.SetCurrentEdit("/app/file"+string(rune('a'+i))+".go", "func")
	}

	// History should be capped at 10
	if len(opt.editHistory) > 10 {
		t.Errorf("edit history should be capped at 10, got %d", len(opt.editHistory))
	}

	// Recent files should have higher priority (but not current file)
	// After 15 edits (a-o), history keeps last 10: f,g,h,i,j,k,l,m,n,o
	// Current file is "o", so test with "n" which is in history but not current
	item := ContextItem{
		Type:    "file",
		Path:    "/app/filen.go", // In history, but not current
		Content: "code",
	}
	priority := opt.inferPriority(item)
	if priority != PriorityHigh {
		t.Errorf("recently edited file should be high priority, got %d", priority)
	}

	// Current file should be essential
	currentItem := ContextItem{
		Type:    "file",
		Path:    "/app/fileo.go", // This is current
		Content: "code",
	}
	if opt.inferPriority(currentItem) != PriorityEssential {
		t.Error("current file should be essential priority")
	}
}

func TestContextOptimizer_Stats(t *testing.T) {
	opt := NewContextOptimizer(100000)
	opt.SetMode(ModeFocused)
	opt.NextTurn()
	opt.NextTurn()

	stats := opt.Stats()

	if stats["budget"] != 100000 {
		t.Error("budget should be 100000")
	}
	if stats["turn"] != 2 {
		t.Error("turn should be 2")
	}
	if stats["mode"] != int(ModeFocused) {
		t.Error("mode should be ModeFocused")
	}
}

func TestGetRecommendedMode(t *testing.T) {
	opt := NewContextOptimizer(100000)

	mode, reason := opt.GetRecommendedMode(true, false, 0)
	if mode != ModeFull {
		t.Error("error should recommend full mode")
	}
	if !strings.Contains(reason, "Error") {
		t.Error("reason should mention error")
	}

	mode, reason = opt.GetRecommendedMode(false, false, 10)
	if mode != ModeMinimal {
		t.Error("many edits should recommend minimal")
	}
	if !strings.Contains(reason, "10 edits") {
		t.Error("reason should mention edit count")
	}
}
