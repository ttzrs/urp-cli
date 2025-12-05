//go:build linux

package tool

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// xdotoolBackend implements computerBackend for X11 Linux via xdotool.
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
	exec.CommandContext(ctx, "xdotool", "mousemove", strconv.Itoa(fromX), strconv.Itoa(fromY)).Run()
	exec.CommandContext(ctx, "xdotool", "mousedown", "1").Run()
	exec.CommandContext(ctx, "xdotool", "mousemove", strconv.Itoa(toX), strconv.Itoa(toY)).Run()
	return exec.CommandContext(ctx, "xdotool", "mouseup", "1").Run()
}

func (b *xdotoolBackend) typeText(ctx context.Context, text string) error {
	return exec.CommandContext(ctx, "xdotool", "type", "--", text).Run()
}

func (b *xdotoolBackend) pressKey(ctx context.Context, key string) error {
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

	buttonPressRe := regexp.MustCompile(`EVENT type \d+ \(ButtonPress\)`)
	detailRe := regexp.MustCompile(`detail:\s*(\d+)`)
	rootRe := regexp.MustCompile(`root:\s*(\d+\.?\d*)/(\d+\.?\d*)`)

	var inButtonPress bool
	var currentEvent ClickEvent

	for scanner.Scan() {
		line := scanner.Text()

		if buttonPressRe.MatchString(line) {
			inButtonPress = true
			currentEvent = ClickEvent{Timestamp: time.Now().UnixMilli()}
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
					inButtonPress = false
					continue
				}
			}

			if matches := rootRe.FindStringSubmatch(line); len(matches) > 2 {
				x, _ := strconv.ParseFloat(matches[1], 64)
				y, _ := strconv.ParseFloat(matches[2], 64)
				currentEvent.X = int(x)
				currentEvent.Y = int(y)

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
