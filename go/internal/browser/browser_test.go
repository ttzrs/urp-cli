package browser

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestExecutor_Basic(t *testing.T) {
	// Skip if no display available (CI environment)
	if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		t.Skip("No display available, skipping browser test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create headless executor
	executor, err := NewExecutor(Config{
		Mode:    ModeExecutor,
		Timeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}
	defer executor.Close()

	// Navigate to a simple page
	err = executor.Navigate(ctx, "https://example.com")
	if err != nil {
		t.Fatalf("Failed to navigate: %v", err)
	}

	// Get page title using GetText on title element
	title, err := executor.Eval(ctx, "() => document.title")
	if err != nil {
		t.Fatalf("Failed to get title: %v", err)
	}

	titleStr, ok := title.(string)
	if !ok || titleStr != "Example Domain" {
		t.Errorf("Expected 'Example Domain', got '%v'", title)
	}

	// Get URL
	url, err := executor.GetURL(ctx)
	if err != nil {
		t.Fatalf("Failed to get URL: %v", err)
	}

	if url != "https://example.com/" {
		t.Errorf("Expected 'https://example.com/', got '%s'", url)
	}

	// Take screenshot
	screenshot, err := executor.Screenshot(ctx, false)
	if err != nil {
		t.Fatalf("Failed to take screenshot: %v", err)
	}

	if len(screenshot) < 1000 {
		t.Errorf("Screenshot too small: %d bytes", len(screenshot))
	}

	t.Logf("Screenshot captured: %d bytes", len(screenshot))
}

func TestExecutor_Actions(t *testing.T) {
	if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		t.Skip("No display available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	executor, err := NewExecutor(Config{Timeout: 10 * time.Second})
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}
	defer executor.Close()

	// Test action flow
	actions := []Action{
		{Type: "navigate", Value: "https://example.com"},
		{Type: "wait", Selector: "h1"},
	}

	err = executor.ExecuteFlow(ctx, actions)
	if err != nil {
		t.Fatalf("Failed to execute flow: %v", err)
	}

	// Verify we're on the page
	text, err := executor.GetText(ctx, "h1")
	if err != nil {
		t.Fatalf("Failed to get text: %v", err)
	}

	if text != "Example Domain" {
		t.Errorf("Expected 'Example Domain', got '%s'", text)
	}
}

func TestBrowser_Modes(t *testing.T) {
	tests := []struct {
		name     string
		headless bool
	}{
		{"Headless", true},
		// Observer mode requires display, skip in CI
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			if tt.headless {
				cfg.Mode = ModeExecutor
			} else {
				cfg.Mode = ModeObserver
			}

			b, err := New(cfg)
			if err != nil {
				t.Fatalf("Failed to create browser: %v", err)
			}
			defer b.Close()

			if b.Mode() != cfg.Mode {
				t.Errorf("Mode mismatch: got %d, want %d", b.Mode(), cfg.Mode)
			}
		})
	}
}
