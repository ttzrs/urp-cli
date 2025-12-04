package tool

import (
	"context"
	"testing"
)

func TestBrowserInfo(t *testing.T) {
	b := NewBrowser()
	info := b.Info()

	if info.Name != "browser" {
		t.Errorf("expected name 'browser', got %q", info.Name)
	}

	if info.Description == "" {
		t.Error("description should not be empty")
	}

	props, ok := info.Parameters["properties"].(map[string]any)
	if !ok {
		t.Fatal("parameters should have properties")
	}

	requiredParams := []string{"action", "url", "selector", "text", "script"}
	for _, p := range requiredParams {
		if _, exists := props[p]; !exists {
			t.Errorf("missing parameter: %s", p)
		}
	}
}

func TestBrowserExecuteNoAction(t *testing.T) {
	b := NewBrowser()
	ctx := context.Background()

	_, err := b.Execute(ctx, map[string]any{})
	if err != ErrInvalidArgs {
		t.Errorf("expected ErrInvalidArgs for missing action, got %v", err)
	}
}

func TestBrowserExecuteUnknownAction(t *testing.T) {
	b := NewBrowser()
	ctx := context.Background()

	result, err := b.Execute(ctx, map[string]any{"action": "invalid"})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil || result.Output == "" {
		t.Error("should return result with output for unknown action")
	}
}

func TestBrowserLaunchNoBrowser(t *testing.T) {
	// Skip if no browser available (CI environment)
	t.Skip("requires browser, skip in CI")

	b := NewBrowser()
	ctx := context.Background()
	defer b.Cleanup()

	result, err := b.Execute(ctx, map[string]any{
		"action":   "launch",
		"headless": true,
	})

	if err != nil {
		t.Fatalf("launch failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected result")
	}
	if result.Error != nil {
		t.Logf("Browser launch error (may be expected in CI): %v", result.Error)
	}
}

func TestBrowserNavigateRequiresURL(t *testing.T) {
	b := NewBrowser()
	ctx := context.Background()

	result, err := b.Execute(ctx, map[string]any{"action": "navigate"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || result.Output == "" {
		t.Error("should return result indicating url required")
	}
}

func TestBrowserClickRequiresSelector(t *testing.T) {
	b := NewBrowser()
	ctx := context.Background()

	result, err := b.Execute(ctx, map[string]any{"action": "click"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || result.Output == "" {
		t.Error("should return result indicating selector required")
	}
}

func TestBrowserTypeRequiresBoth(t *testing.T) {
	b := NewBrowser()
	ctx := context.Background()

	// Missing selector
	result, err := b.Execute(ctx, map[string]any{
		"action": "type",
		"text":   "hello",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || result.Output == "" {
		t.Error("should return result indicating selector required")
	}

	// Missing text
	result, err = b.Execute(ctx, map[string]any{
		"action":   "type",
		"selector": "#input",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || result.Output == "" {
		t.Error("should return result indicating text required")
	}
}

func TestBrowserEvaluateRequiresScript(t *testing.T) {
	b := NewBrowser()
	ctx := context.Background()

	result, err := b.Execute(ctx, map[string]any{"action": "evaluate"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || result.Output == "" {
		t.Error("should return result indicating script required")
	}
}

func TestBrowserElementsRequiresSelector(t *testing.T) {
	b := NewBrowser()
	ctx := context.Background()

	result, err := b.Execute(ctx, map[string]any{"action": "elements"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || result.Output == "" {
		t.Error("should return result indicating selector required")
	}
}

func TestBrowserCloseNoRunning(t *testing.T) {
	b := NewBrowser()
	ctx := context.Background()

	result, err := b.Execute(ctx, map[string]any{"action": "close"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || result.Output == "" {
		t.Error("should return result")
	}
}

func TestBrowserImplementsExecutor(t *testing.T) {
	var _ Executor = (*Browser)(nil)
}

func TestDefaultRegistryIncludesBrowser(t *testing.T) {
	r := DefaultRegistry("/tmp")

	if _, ok := r.Get("browser"); !ok {
		t.Error("DefaultRegistry should include browser tool")
	}
}
