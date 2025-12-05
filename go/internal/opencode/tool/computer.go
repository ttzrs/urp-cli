package tool

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/joss/urp/internal/opencode/domain"
)

// Computer tool for mouse/keyboard interaction (like Claude Computer Use)
// Actions that modify state (click, type, key) require explicit permission.
// Backend implementations are in separate files: computer_xdotool.go, etc.
type Computer struct{}

func NewComputer() *Computer {
	return &Computer{}
}

func (c *Computer) Info() domain.Tool {
	return domain.Tool{
		Name: "computer",
		Description: `Interact with the computer screen, mouse, and keyboard.
Safe actions (no confirmation): mouse_position, screenshot
Dangerous actions (REQUIRE confirmation): click, type, key, move, drag, scroll

Use this tool to automate GUI interactions when command-line tools are insufficient.`,
		Parameters: domain.JSONSchema{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]any{
					"type": "string",
					"enum": []string{
						"mouse_position", // Get current mouse position
						"screenshot",     // Capture screen (optionally at cursor)
						"click",          // Click at position (DANGEROUS)
						"double_click",   // Double click (DANGEROUS)
						"right_click",    // Right click (DANGEROUS)
						"move",           // Move mouse (DANGEROUS)
						"drag",           // Drag from current to target (DANGEROUS)
						"type",           // Type text (DANGEROUS)
						"key",            // Press key combo (DANGEROUS)
						"scroll",         // Scroll (DANGEROUS)
						"watch_clicks",   // Monitor clicks and capture screenshots
					},
					"description": "Action to perform",
				},
				"x": map[string]any{
					"type":        "integer",
					"description": "X coordinate for click/move actions",
				},
				"y": map[string]any{
					"type":        "integer",
					"description": "Y coordinate for click/move actions",
				},
				"text": map[string]any{
					"type":        "string",
					"description": "Text to type (for 'type' action) or key combo (for 'key' action, e.g. 'ctrl+c', 'Return')",
				},
				"direction": map[string]any{
					"type":        "string",
					"enum":        []string{"up", "down", "left", "right"},
					"description": "Scroll direction",
				},
				"amount": map[string]any{
					"type":        "integer",
					"description": "Scroll amount (default: 3)",
				},
				"timeout": map[string]any{
					"type":        "integer",
					"description": "Timeout in seconds for watch_clicks (default: 10, max: 60)",
				},
				"max_events": map[string]any{
					"type":        "integer",
					"description": "Maximum click events to capture (default: 5)",
				},
			},
			"required": []string{"action"},
		},
	}
}

// IsDangerous returns true for actions that modify state.
func (c *Computer) IsDangerous(action string) bool {
	switch action {
	case "click", "double_click", "right_click", "move", "drag", "type", "key", "scroll":
		return true
	}
	return false
}

func (c *Computer) Execute(ctx context.Context, args map[string]any) (*Result, error) {
	action, _ := args["action"].(string)
	if action == "" {
		return nil, ErrInvalidArgs
	}

	backend := detectBackend()
	if backend == nil {
		return &Result{
			Output: "No computer interaction tool available. Install: xdotool (X11), ydotool (Wayland), or cliclick (macOS)",
		}, nil
	}

	switch action {
	case "mouse_position":
		return c.getMousePosition(ctx, backend)
	case "screenshot":
		return c.screenshotAtCursor(ctx, args)
	case "click":
		return c.click(ctx, backend, args, 1, "left")
	case "double_click":
		return c.click(ctx, backend, args, 2, "left")
	case "right_click":
		return c.click(ctx, backend, args, 1, "right")
	case "move":
		return c.moveMouse(ctx, backend, args)
	case "drag":
		return c.drag(ctx, backend, args)
	case "type":
		return c.typeText(ctx, backend, args)
	case "key":
		return c.pressKey(ctx, backend, args)
	case "scroll":
		return c.scroll(ctx, backend, args)
	case "watch_clicks":
		return c.watchClicks(ctx, backend, args)
	default:
		return &Result{Output: fmt.Sprintf("Unknown action: %s", action)}, nil
	}
}

// computerBackend interface for platform-specific implementations.
// Implementations: xdotoolBackend, ydotoolBackend, cliclickBackend, windowsBackend
type computerBackend interface {
	getMousePosition(ctx context.Context) (x, y int, err error)
	click(ctx context.Context, x, y, count int, button string) error
	moveMouse(ctx context.Context, x, y int) error
	drag(ctx context.Context, fromX, fromY, toX, toY int) error
	typeText(ctx context.Context, text string) error
	pressKey(ctx context.Context, key string) error
	scroll(ctx context.Context, direction string, amount int) error
	watchClicks(ctx context.Context, maxEvents int) ([]ClickEvent, error)
}

// ClickEvent represents a captured click event.
type ClickEvent struct {
	X         int
	Y         int
	Button    string // "left", "right", "middle"
	Timestamp int64  // Unix timestamp in milliseconds
}

// detectBackend is defined in platform-specific files:
// computer_backend_linux.go, computer_backend_darwin.go,
// computer_backend_windows.go, computer_backend_other.go

func (c *Computer) getMousePosition(ctx context.Context, backend computerBackend) (*Result, error) {
	x, y, err := backend.getMousePosition(ctx)
	if err != nil {
		return &Result{Output: fmt.Sprintf("Failed to get mouse position: %v", err)}, nil
	}
	return &Result{
		Output:   fmt.Sprintf("Mouse position: x=%d, y=%d", x, y),
		Metadata: map[string]any{"x": x, "y": y},
	}, nil
}

func (c *Computer) screenshotAtCursor(ctx context.Context, args map[string]any) (*Result, error) {
	sc := NewScreenCapture()
	return sc.Execute(ctx, map[string]any{"region": "full"})
}

func (c *Computer) click(ctx context.Context, backend computerBackend, args map[string]any, count int, button string) (*Result, error) {
	x, y, err := c.getCoords(args)
	if err != nil {
		return &Result{Output: err.Error()}, nil
	}

	if err := backend.click(ctx, x, y, count, button); err != nil {
		return &Result{Output: fmt.Sprintf("Click failed: %v", err)}, nil
	}

	return &Result{Output: fmt.Sprintf("Clicked %s button %dx at (%d, %d)", button, count, x, y)}, nil
}

func (c *Computer) moveMouse(ctx context.Context, backend computerBackend, args map[string]any) (*Result, error) {
	x, y, err := c.getCoords(args)
	if err != nil {
		return &Result{Output: err.Error()}, nil
	}

	if err := backend.moveMouse(ctx, x, y); err != nil {
		return &Result{Output: fmt.Sprintf("Move failed: %v", err)}, nil
	}

	return &Result{Output: fmt.Sprintf("Moved mouse to (%d, %d)", x, y)}, nil
}

func (c *Computer) drag(ctx context.Context, backend computerBackend, args map[string]any) (*Result, error) {
	x, y, err := c.getCoords(args)
	if err != nil {
		return &Result{Output: err.Error()}, nil
	}

	startX, startY, err := backend.getMousePosition(ctx)
	if err != nil {
		return &Result{Output: fmt.Sprintf("Failed to get start position: %v", err)}, nil
	}

	if err := backend.drag(ctx, startX, startY, x, y); err != nil {
		return &Result{Output: fmt.Sprintf("Drag failed: %v", err)}, nil
	}

	return &Result{Output: fmt.Sprintf("Dragged from (%d, %d) to (%d, %d)", startX, startY, x, y)}, nil
}

func (c *Computer) typeText(ctx context.Context, backend computerBackend, args map[string]any) (*Result, error) {
	text, _ := args["text"].(string)
	if text == "" {
		return &Result{Output: "text parameter required for type action"}, nil
	}

	if err := backend.typeText(ctx, text); err != nil {
		return &Result{Output: fmt.Sprintf("Type failed: %v", err)}, nil
	}

	preview := text
	if len(preview) > 20 {
		preview = preview[:20] + "..."
	}
	return &Result{Output: fmt.Sprintf("Typed %d characters: %q", len(text), preview)}, nil
}

func (c *Computer) pressKey(ctx context.Context, backend computerBackend, args map[string]any) (*Result, error) {
	key, _ := args["text"].(string)
	if key == "" {
		return &Result{Output: "text parameter required for key action (e.g. 'ctrl+c', 'Return')"}, nil
	}

	if err := backend.pressKey(ctx, key); err != nil {
		return &Result{Output: fmt.Sprintf("Key press failed: %v", err)}, nil
	}

	return &Result{Output: fmt.Sprintf("Pressed key: %s", key)}, nil
}

func (c *Computer) scroll(ctx context.Context, backend computerBackend, args map[string]any) (*Result, error) {
	direction, _ := args["direction"].(string)
	if direction == "" {
		direction = "down"
	}

	amount := 3
	if a, ok := args["amount"].(float64); ok {
		amount = int(a)
	}

	if err := backend.scroll(ctx, direction, amount); err != nil {
		return &Result{Output: fmt.Sprintf("Scroll failed: %v", err)}, nil
	}

	return &Result{Output: fmt.Sprintf("Scrolled %s by %d", direction, amount)}, nil
}

func (c *Computer) watchClicks(ctx context.Context, backend computerBackend, args map[string]any) (*Result, error) {
	timeout := 10
	if t, ok := args["timeout"].(float64); ok {
		timeout = int(t)
		if timeout > 60 {
			timeout = 60
		}
	}

	maxEvents := 5
	if m, ok := args["max_events"].(float64); ok {
		maxEvents = int(m)
		if maxEvents > 20 {
			maxEvents = 20
		}
	}

	watchCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	events, err := backend.watchClicks(watchCtx, maxEvents)
	if err != nil {
		return &Result{Output: fmt.Sprintf("Watch clicks failed: %v", err)}, nil
	}

	if len(events) == 0 {
		return &Result{Output: fmt.Sprintf("No clicks detected within %d seconds", timeout)}, nil
	}

	var images []domain.ImagePart
	var eventSummary strings.Builder
	eventSummary.WriteString(fmt.Sprintf("Captured %d click events:\n", len(events)))

	for i, evt := range events {
		eventSummary.WriteString(fmt.Sprintf("  %d. %s click at (%d, %d) @ %s\n",
			i+1, evt.Button, evt.X, evt.Y,
			time.UnixMilli(evt.Timestamp).Format("15:04:05.000")))
	}

	sc := NewScreenCapture()
	result, err := sc.Execute(ctx, map[string]any{"region": "full"})
	if err == nil && result != nil && len(result.Images) > 0 {
		images = append(images, result.Images...)
	}

	return &Result{
		Title:  "Click Watch",
		Output: eventSummary.String(),
		Images: images,
		Metadata: map[string]any{
			"events":      events,
			"event_count": len(events),
		},
	}, nil
}

func (c *Computer) getCoords(args map[string]any) (int, int, error) {
	x, ok1 := args["x"].(float64)
	y, ok2 := args["y"].(float64)
	if !ok1 || !ok2 {
		return 0, 0, fmt.Errorf("x and y coordinates required")
	}
	return int(x), int(y), nil
}

// ScreenshotWithCursor captures the screen and overlays cursor position info.
func (c *Computer) ScreenshotWithCursor(ctx context.Context) (*Result, error) {
	backend := detectBackend()

	var posInfo string
	if backend != nil {
		x, y, err := backend.getMousePosition(ctx)
		if err == nil {
			posInfo = fmt.Sprintf(" (cursor at %d,%d)", x, y)
		}
	}

	sc := NewScreenCapture()
	result, err := sc.Execute(ctx, map[string]any{"region": "full"})
	if err != nil {
		return nil, err
	}

	if result != nil && posInfo != "" {
		result.Output += posInfo
	}

	return result, nil
}

var _ Executor = (*Computer)(nil)
