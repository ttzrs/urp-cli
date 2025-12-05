//go:build linux

package tool

import (
	"os"
	"os/exec"
)

func detectBackend() computerBackend {
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		if _, err := exec.LookPath("ydotool"); err == nil {
			return &ydotoolBackend{}
		}
		if _, err := exec.LookPath("wtype"); err == nil {
			return &wtypeBackend{}
		}
	}
	if _, err := exec.LookPath("xdotool"); err == nil {
		return &xdotoolBackend{}
	}
	return nil
}
