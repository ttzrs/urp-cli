//go:build windows

package tool

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// windowsBackend implements computerBackend for Windows via PowerShell.
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
	return nil, fmt.Errorf("click monitoring on Windows requires native hooks (not supported via PowerShell)")
}
