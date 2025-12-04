package tool

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/joss/urp/internal/opencode/domain"
)

// Computer tool for mouse/keyboard interaction (like Claude Computer Use)
// Actions that modify state (click, type, key) require explicit permission
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

// IsDangerous returns true for actions that modify state
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

	// Detect the interaction backend
	backend := c.detectBackend()
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

// Backend interface for different platforms
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

// ClickEvent represents a captured click event
type ClickEvent struct {
	X         int
	Y         int
	Button    string // "left", "right", "middle"
	Timestamp int64  // Unix timestamp in milliseconds
}

func (c *Computer) detectBackend() computerBackend {
	switch runtime.GOOS {
	case "linux":
		// Check for Wayland
		if os.Getenv("WAYLAND_DISPLAY") != "" {
			if _, err := exec.LookPath("ydotool"); err == nil {
				return &ydotoolBackend{}
			}
			if _, err := exec.LookPath("wtype"); err == nil {
				return &wtypeBackend{}
			}
		}
		// X11
		if _, err := exec.LookPath("xdotool"); err == nil {
			return &xdotoolBackend{}
		}
	case "darwin":
		if _, err := exec.LookPath("cliclick"); err == nil {
			return &cliclickBackend{}
		}
	case "windows":
		// PowerShell-based
		return &windowsBackend{}
	}
	return nil
}

func (c *Computer) getMousePosition(ctx context.Context, backend computerBackend) (*Result, error) {
	x, y, err := backend.getMousePosition(ctx)
	if err != nil {
		return &Result{Output: fmt.Sprintf("Failed to get mouse position: %v", err)}, nil
	}
	return &Result{
		Output: fmt.Sprintf("Mouse position: x=%d, y=%d", x, y),
		Metadata: map[string]any{
			"x": x,
			"y": y,
		},
	}, nil
}

func (c *Computer) screenshotAtCursor(ctx context.Context, args map[string]any) (*Result, error) {
	// Use screen_capture tool logic
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

	return &Result{
		Output: fmt.Sprintf("Clicked %s button %dx at (%d, %d)", button, count, x, y),
	}, nil
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

	// Get current position as start
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

	// Don't echo the full text for security
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

// watchClicks monitors click events and captures screenshots for each click
func (c *Computer) watchClicks(ctx context.Context, backend computerBackend, args map[string]any) (*Result, error) {
	timeout := 10
	if t, ok := args["timeout"].(float64); ok {
		timeout = int(t)
		if timeout > 60 {
			timeout = 60 // Max 60 seconds
		}
	}

	maxEvents := 5
	if m, ok := args["max_events"].(float64); ok {
		maxEvents = int(m)
		if maxEvents > 20 {
			maxEvents = 20 // Safety limit
		}
	}

	// Create context with timeout
	watchCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	// Watch for click events
	events, err := backend.watchClicks(watchCtx, maxEvents)
	if err != nil {
		return &Result{Output: fmt.Sprintf("Watch clicks failed: %v", err)}, nil
	}

	if len(events) == 0 {
		return &Result{
			Output: fmt.Sprintf("No clicks detected within %d seconds", timeout),
		}, nil
	}

	// Capture screenshots for each click event
	var images []domain.ImagePart
	var eventSummary strings.Builder

	eventSummary.WriteString(fmt.Sprintf("Captured %d click events:\n", len(events)))

	for i, evt := range events {
		eventSummary.WriteString(fmt.Sprintf("  %d. %s click at (%d, %d) @ %s\n",
			i+1, evt.Button, evt.X, evt.Y,
			time.UnixMilli(evt.Timestamp).Format("15:04:05.000")))
	}

	// Capture final screenshot with last click position
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

// ============ xdotool backend (X11 Linux) ============

type xdotoolBackend struct{}

func (b *xdotoolBackend) getMousePosition(ctx context.Context) (int, int, error) {
	out, err := exec.CommandContext(ctx, "xdotool", "getmouselocation").Output()
	if err != nil {
		return 0, 0, err
	}
	// Parse: x:1234 y:567 screen:0 window:12345678
	re := regexp.MustCompile(`x:(\d+)\s+y:(\d+)`)
	matches := re.FindStringSubmatch(string(out))
	if len(matches) < 3 {
		return 0, 0, fmt.Errorf("failed to parse mouse position")
	}
	x, _ := strconv.Atoi(matches[1])
	y, _ := strconv.Atoi(matches[2])
	return x, y, nil
}

func (b *xdotoolBackend) click(ctx context.Context, x, y, count int, button string) error {
	btn := "1"
	switch button {
	case "right":
		btn = "3"
	case "middle":
		btn = "2"
	}

	args := []string{"mousemove", strconv.Itoa(x), strconv.Itoa(y)}
	if err := exec.CommandContext(ctx, "xdotool", args...).Run(); err != nil {
		return err
	}

	for i := 0; i < count; i++ {
		if err := exec.CommandContext(ctx, "xdotool", "click", btn).Run(); err != nil {
			return err
		}
	}
	return nil
}

func (b *xdotoolBackend) moveMouse(ctx context.Context, x, y int) error {
	return exec.CommandContext(ctx, "xdotool", "mousemove", strconv.Itoa(x), strconv.Itoa(y)).Run()
}

func (b *xdotoolBackend) drag(ctx context.Context, fromX, fromY, toX, toY int) error {
	// Move to start, press, move to end, release
	exec.CommandContext(ctx, "xdotool", "mousemove", strconv.Itoa(fromX), strconv.Itoa(fromY)).Run()
	exec.CommandContext(ctx, "xdotool", "mousedown", "1").Run()
	exec.CommandContext(ctx, "xdotool", "mousemove", strconv.Itoa(toX), strconv.Itoa(toY)).Run()
	return exec.CommandContext(ctx, "xdotool", "mouseup", "1").Run()
}

func (b *xdotoolBackend) typeText(ctx context.Context, text string) error {
	return exec.CommandContext(ctx, "xdotool", "type", "--", text).Run()
}

func (b *xdotoolBackend) pressKey(ctx context.Context, key string) error {
	// Convert common key names to xdotool format
	key = strings.ReplaceAll(key, "ctrl", "ctrl")
	key = strings.ReplaceAll(key, "alt", "alt")
	key = strings.ReplaceAll(key, "shift", "shift")
	key = strings.ReplaceAll(key, "super", "super")
	key = strings.ReplaceAll(key, "+", "+")
	return exec.CommandContext(ctx, "xdotool", "key", key).Run()
}

func (b *xdotoolBackend) scroll(ctx context.Context, direction string, amount int) error {
	btn := "5" // down
	switch direction {
	case "up":
		btn = "4"
	case "left":
		btn = "6"
	case "right":
		btn = "7"
	}
	for i := 0; i < amount; i++ {
		if err := exec.CommandContext(ctx, "xdotool", "click", btn).Run(); err != nil {
			return err
		}
	}
	return nil
}

func (b *xdotoolBackend) watchClicks(ctx context.Context, maxEvents int) ([]ClickEvent, error) {
	// Use xinput to monitor button events
	// First, find the pointer device
	cmd := exec.CommandContext(ctx, "xinput", "test-xi2", "--root")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("xinput not available: %w", err)
	}

	var events []ClickEvent
	scanner := bufio.NewScanner(stdout)

	// Regex to match button press events
	// Example: EVENT type 15 (ButtonPress)
	buttonPressRe := regexp.MustCompile(`EVENT type \d+ \(ButtonPress\)`)
	// Match detail (button number) and root coordinates
	detailRe := regexp.MustCompile(`detail:\s*(\d+)`)
	rootRe := regexp.MustCompile(`root:\s*(\d+\.?\d*)/(\d+\.?\d*)`)

	var inButtonPress bool
	var currentEvent ClickEvent

	for scanner.Scan() {
		line := scanner.Text()

		if buttonPressRe.MatchString(line) {
			inButtonPress = true
			currentEvent = ClickEvent{
				Timestamp: time.Now().UnixMilli(),
			}
			continue
		}

		if inButtonPress {
			if matches := detailRe.FindStringSubmatch(line); len(matches) > 1 {
				btn, _ := strconv.Atoi(matches[1])
				switch btn {
				case 1:
					currentEvent.Button = "left"
				case 2:
					currentEvent.Button = "middle"
				case 3:
					currentEvent.Button = "right"
				default:
					// Scroll or other button, skip
					inButtonPress = false
					continue
				}
			}

			if matches := rootRe.FindStringSubmatch(line); len(matches) > 2 {
				x, _ := strconv.ParseFloat(matches[1], 64)
				y, _ := strconv.ParseFloat(matches[2], 64)
				currentEvent.X = int(x)
				currentEvent.Y = int(y)

				// Got complete event
				events = append(events, currentEvent)
				inButtonPress = false

				if len(events) >= maxEvents {
					break
				}
			}
		}

		select {
		case <-ctx.Done():
			break
		default:
		}
	}

	cmd.Process.Kill()
	cmd.Wait()

	return events, nil
}

// ============ ydotool backend (Wayland Linux) ============

type ydotoolBackend struct{}

func (b *ydotoolBackend) getMousePosition(ctx context.Context) (int, int, error) {
	// ydotool doesn't have a get position command, need alternative
	return 0, 0, fmt.Errorf("ydotool cannot get mouse position")
}

func (b *ydotoolBackend) click(ctx context.Context, x, y, count int, button string) error {
	// Move first
	exec.CommandContext(ctx, "ydotool", "mousemove", "-a", strconv.Itoa(x), strconv.Itoa(y)).Run()

	btn := "0xC0" // left click (down+up)
	switch button {
	case "right":
		btn = "0xC1"
	case "middle":
		btn = "0xC2"
	}

	for i := 0; i < count; i++ {
		if err := exec.CommandContext(ctx, "ydotool", "click", btn).Run(); err != nil {
			return err
		}
	}
	return nil
}

func (b *ydotoolBackend) moveMouse(ctx context.Context, x, y int) error {
	return exec.CommandContext(ctx, "ydotool", "mousemove", "-a", strconv.Itoa(x), strconv.Itoa(y)).Run()
}

func (b *ydotoolBackend) drag(ctx context.Context, fromX, fromY, toX, toY int) error {
	exec.CommandContext(ctx, "ydotool", "mousemove", "-a", strconv.Itoa(fromX), strconv.Itoa(fromY)).Run()
	exec.CommandContext(ctx, "ydotool", "click", "0x40").Run() // down
	exec.CommandContext(ctx, "ydotool", "mousemove", "-a", strconv.Itoa(toX), strconv.Itoa(toY)).Run()
	return exec.CommandContext(ctx, "ydotool", "click", "0x80").Run() // up
}

func (b *ydotoolBackend) typeText(ctx context.Context, text string) error {
	return exec.CommandContext(ctx, "ydotool", "type", "--", text).Run()
}

func (b *ydotoolBackend) pressKey(ctx context.Context, key string) error {
	return exec.CommandContext(ctx, "ydotool", "key", key).Run()
}

func (b *ydotoolBackend) scroll(ctx context.Context, direction string, amount int) error {
	// ydotool uses different scroll syntax
	return fmt.Errorf("scroll not implemented for ydotool")
}

func (b *ydotoolBackend) watchClicks(ctx context.Context, maxEvents int) ([]ClickEvent, error) {
	// libinput debug-events requires root, try evtest or read from /dev/input
	// For now, return error - Wayland click monitoring is complex
	return nil, fmt.Errorf("click monitoring on Wayland requires libinput debug-events (needs root)")
}

// ============ wtype backend (Wayland typing only) ============

type wtypeBackend struct {
	ydotoolBackend // Embed for mouse operations
}

func (b *wtypeBackend) typeText(ctx context.Context, text string) error {
	return exec.CommandContext(ctx, "wtype", text).Run()
}

// ============ cliclick backend (macOS) ============

type cliclickBackend struct{}

func (b *cliclickBackend) getMousePosition(ctx context.Context) (int, int, error) {
	out, err := exec.CommandContext(ctx, "cliclick", "p:.").Output()
	if err != nil {
		return 0, 0, err
	}
	// Parse: x,y
	parts := strings.Split(strings.TrimSpace(string(out)), ",")
	if len(parts) < 2 {
		return 0, 0, fmt.Errorf("failed to parse position")
	}
	x, _ := strconv.Atoi(parts[0])
	y, _ := strconv.Atoi(parts[1])
	return x, y, nil
}

func (b *cliclickBackend) click(ctx context.Context, x, y, count int, button string) error {
	cmd := "c" // click
	if button == "right" {
		cmd = "rc"
	}
	coord := fmt.Sprintf("%s:%d,%d", cmd, x, y)

	for i := 0; i < count; i++ {
		if err := exec.CommandContext(ctx, "cliclick", coord).Run(); err != nil {
			return err
		}
	}
	return nil
}

func (b *cliclickBackend) moveMouse(ctx context.Context, x, y int) error {
	return exec.CommandContext(ctx, "cliclick", fmt.Sprintf("m:%d,%d", x, y)).Run()
}

func (b *cliclickBackend) drag(ctx context.Context, fromX, fromY, toX, toY int) error {
	return exec.CommandContext(ctx, "cliclick",
		fmt.Sprintf("dd:%d,%d", fromX, fromY),
		fmt.Sprintf("du:%d,%d", toX, toY)).Run()
}

func (b *cliclickBackend) typeText(ctx context.Context, text string) error {
	return exec.CommandContext(ctx, "cliclick", fmt.Sprintf("t:%s", text)).Run()
}

func (b *cliclickBackend) pressKey(ctx context.Context, key string) error {
	return exec.CommandContext(ctx, "cliclick", fmt.Sprintf("kp:%s", key)).Run()
}

func (b *cliclickBackend) scroll(ctx context.Context, direction string, amount int) error {
	// cliclick doesn't support scroll directly
	return fmt.Errorf("scroll not supported on macOS via cliclick")
}

func (b *cliclickBackend) watchClicks(ctx context.Context, maxEvents int) ([]ClickEvent, error) {
	// macOS doesn't have a simple CLI tool for monitoring clicks
	// Would require Accessibility permissions and a custom tool
	return nil, fmt.Errorf("click monitoring on macOS requires Accessibility permissions (not supported via CLI)")
}

// ============ Windows backend (PowerShell) ============

type windowsBackend struct{}

func (b *windowsBackend) getMousePosition(ctx context.Context) (int, int, error) {
	script := `Add-Type -AssemblyName System.Windows.Forms; [System.Windows.Forms.Cursor]::Position | ForEach-Object { "$($_.X),$($_.Y)" }`
	out, err := exec.CommandContext(ctx, "powershell", "-Command", script).Output()
	if err != nil {
		return 0, 0, err
	}
	parts := strings.Split(strings.TrimSpace(string(out)), ",")
	if len(parts) < 2 {
		return 0, 0, fmt.Errorf("failed to parse position")
	}
	x, _ := strconv.Atoi(parts[0])
	y, _ := strconv.Atoi(parts[1])
	return x, y, nil
}

func (b *windowsBackend) click(ctx context.Context, x, y, count int, button string) error {
	script := fmt.Sprintf(`
Add-Type -AssemblyName System.Windows.Forms
[System.Windows.Forms.Cursor]::Position = New-Object System.Drawing.Point(%d, %d)
Add-Type @"
using System;
using System.Runtime.InteropServices;
public class Mouse {
    [DllImport("user32.dll")]
    public static extern void mouse_event(int dwFlags, int dx, int dy, int dwData, int dwExtraInfo);
}
"@
`, x, y)

	flags := "0x02, 0x04" // left down, left up
	if button == "right" {
		flags = "0x08, 0x10"
	}

	for i := 0; i < count; i++ {
		script += fmt.Sprintf("[Mouse]::mouse_event(%s, 0, 0, 0, 0)\n", flags)
	}

	return exec.CommandContext(ctx, "powershell", "-Command", script).Run()
}

func (b *windowsBackend) moveMouse(ctx context.Context, x, y int) error {
	script := fmt.Sprintf(`Add-Type -AssemblyName System.Windows.Forms; [System.Windows.Forms.Cursor]::Position = New-Object System.Drawing.Point(%d, %d)`, x, y)
	return exec.CommandContext(ctx, "powershell", "-Command", script).Run()
}

func (b *windowsBackend) drag(ctx context.Context, fromX, fromY, toX, toY int) error {
	return fmt.Errorf("drag not implemented for Windows")
}

func (b *windowsBackend) typeText(ctx context.Context, text string) error {
	script := fmt.Sprintf(`Add-Type -AssemblyName System.Windows.Forms; [System.Windows.Forms.SendKeys]::SendWait('%s')`,
		strings.ReplaceAll(text, "'", "''"))
	return exec.CommandContext(ctx, "powershell", "-Command", script).Run()
}

func (b *windowsBackend) pressKey(ctx context.Context, key string) error {
	// Convert to SendKeys format
	keyMap := map[string]string{
		"Return": "{ENTER}", "Enter": "{ENTER}",
		"Tab": "{TAB}", "Escape": "{ESC}",
		"Backspace": "{BACKSPACE}", "Delete": "{DELETE}",
		"ctrl+c": "^c", "ctrl+v": "^v", "ctrl+a": "^a",
		"alt+Tab": "%{TAB}",
	}
	if mapped, ok := keyMap[key]; ok {
		key = mapped
	}
	script := fmt.Sprintf(`Add-Type -AssemblyName System.Windows.Forms; [System.Windows.Forms.SendKeys]::SendWait('%s')`, key)
	return exec.CommandContext(ctx, "powershell", "-Command", script).Run()
}

func (b *windowsBackend) scroll(ctx context.Context, direction string, amount int) error {
	return fmt.Errorf("scroll not implemented for Windows")
}

func (b *windowsBackend) watchClicks(ctx context.Context, maxEvents int) ([]ClickEvent, error) {
	// Would require a Windows hook - complex to do via PowerShell
	return nil, fmt.Errorf("click monitoring on Windows requires native hooks (not supported via PowerShell)")
}

// ScreenshotWithCursor captures the screen and overlays cursor position info
func (c *Computer) ScreenshotWithCursor(ctx context.Context) (*Result, error) {
	backend := c.detectBackend()

	// Get mouse position
	var posInfo string
	if backend != nil {
		x, y, err := backend.getMousePosition(ctx)
		if err == nil {
			posInfo = fmt.Sprintf(" (cursor at %d,%d)", x, y)
		}
	}

	// Capture screen
	sc := NewScreenCapture()
	result, err := sc.Execute(ctx, map[string]any{"region": "full"})
	if err != nil {
		return nil, err
	}

	// Add cursor info to output
	if result != nil && posInfo != "" {
		result.Output += posInfo
	}

	return result, nil
}

var _ Executor = (*Computer)(nil)
