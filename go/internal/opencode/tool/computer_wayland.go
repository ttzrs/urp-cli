//go:build linux

package tool

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
)

// ydotoolBackend implements computerBackend for Wayland Linux via ydotool.
type ydotoolBackend struct{}

func (b *ydotoolBackend) getMousePosition(ctx context.Context) (int, int, error) {
	return 0, 0, fmt.Errorf("ydotool cannot get mouse position")
}

func (b *ydotoolBackend) click(ctx context.Context, x, y, count int, button string) error {
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
	return fmt.Errorf("scroll not implemented for ydotool")
}

func (b *ydotoolBackend) watchClicks(ctx context.Context, maxEvents int) ([]ClickEvent, error) {
	return nil, fmt.Errorf("click monitoring on Wayland requires libinput debug-events (needs root)")
}

// wtypeBackend extends ydotool with wtype for better text input.
type wtypeBackend struct {
	ydotoolBackend
}

func (b *wtypeBackend) typeText(ctx context.Context, text string) error {
	return exec.CommandContext(ctx, "wtype", text).Run()
}
