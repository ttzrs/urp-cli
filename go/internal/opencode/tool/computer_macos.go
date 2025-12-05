//go:build darwin

package tool

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// cliclickBackend implements computerBackend for macOS via cliclick.
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
	return fmt.Errorf("scroll not supported on macOS via cliclick")
}

func (b *cliclickBackend) watchClicks(ctx context.Context, maxEvents int) ([]ClickEvent, error) {
	return nil, fmt.Errorf("click monitoring on macOS requires Accessibility permissions (not supported via CLI)")
}
