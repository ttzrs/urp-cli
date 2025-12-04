package tool

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/joss/urp/internal/opencode/domain"
)

// ScreenCapture tool for capturing the screen or a region
type ScreenCapture struct{}

func NewScreenCapture() *ScreenCapture {
	return &ScreenCapture{}
}

func (s *ScreenCapture) Info() domain.Tool {
	return domain.Tool{
		Name:        "screen_capture",
		Description: "Capture the screen or a specific region. Returns the image for visual analysis.",
		Parameters: domain.JSONSchema{
			"type": "object",
			"properties": map[string]any{
				"region": map[string]any{
					"type":        "string",
					"description": "Optional region to capture: 'full' (default), 'selection' (user selects), or geometry like '100,100,800,600' (x,y,width,height)",
				},
				"output": map[string]any{
					"type":        "string",
					"description": "Optional output file path. If not specified, uses a temp file.",
				},
				"delay": map[string]any{
					"type":        "integer",
					"description": "Delay in seconds before capture (default: 0)",
				},
			},
		},
	}
}

func (s *ScreenCapture) Execute(ctx context.Context, args map[string]any) (*Result, error) {
	region, _ := args["region"].(string)
	if region == "" {
		region = "full"
	}

	outputPath, _ := args["output"].(string)
	if outputPath == "" {
		tmpDir := os.TempDir()
		outputPath = filepath.Join(tmpDir, fmt.Sprintf("screenshot_%d.png", os.Getpid()))
	}

	delay := 0
	if d, ok := args["delay"].(float64); ok {
		delay = int(d)
	}

	// Detect and execute the appropriate capture method
	capturer := s.detectCapturer()
	if capturer == nil {
		return &Result{
			Output: "No screen capture tool available. Install: grim (Wayland), scrot/import (X11), or screencapture (macOS)",
		}, nil
	}

	err := capturer.capture(ctx, outputPath, region, delay)
	if err != nil {
		return &Result{
			Output: fmt.Sprintf("Screen capture failed: %v", err),
		}, nil
	}

	// Read and encode the captured image
	data, err := os.ReadFile(outputPath)
	if err != nil {
		return &Result{
			Output: fmt.Sprintf("Failed to read captured image: %v", err),
		}, nil
	}

	// Clean up temp file if we created it
	if args["output"] == nil {
		defer os.Remove(outputPath)
	}

	encoded := base64.StdEncoding.EncodeToString(data)

	return &Result{
		Title:  "Screen Capture",
		Output: fmt.Sprintf("[Screen captured: %d bytes, region: %s]", len(data), region),
		Images: []domain.ImagePart{
			{
				Base64:    encoded,
				MediaType: "image/png",
			},
		},
	}, nil
}

// capturer interface for different screen capture implementations
type capturer interface {
	capture(ctx context.Context, output, region string, delay int) error
}

// detectCapturer finds the best available screen capture method
func (s *ScreenCapture) detectCapturer() capturer {
	switch runtime.GOOS {
	case "darwin":
		return &macOSCapturer{}
	case "linux":
		// Check for Wayland
		if os.Getenv("WAYLAND_DISPLAY") != "" {
			if _, err := exec.LookPath("grim"); err == nil {
				return &grimCapturer{}
			}
			if _, err := exec.LookPath("gnome-screenshot"); err == nil {
				return &gnomeScreenshotCapturer{}
			}
		}
		// X11 fallbacks
		if _, err := exec.LookPath("scrot"); err == nil {
			return &scrotCapturer{}
		}
		if _, err := exec.LookPath("import"); err == nil {
			return &importCapturer{}
		}
		if _, err := exec.LookPath("gnome-screenshot"); err == nil {
			return &gnomeScreenshotCapturer{}
		}
	case "windows":
		// Windows would need PowerShell or a library
		return &windowsCapturer{}
	}
	return nil
}

// macOS screencapture
type macOSCapturer struct{}

func (c *macOSCapturer) capture(ctx context.Context, output, region string, delay int) error {
	args := []string{"-x"} // silent

	if delay > 0 {
		args = append(args, "-T", fmt.Sprintf("%d", delay))
	}

	switch region {
	case "selection":
		args = append(args, "-i") // interactive selection
	case "full":
		// default full screen
	default:
		// Parse geometry x,y,width,height
		args = append(args, "-R", region)
	}

	args = append(args, output)
	cmd := exec.CommandContext(ctx, "screencapture", args...)
	return cmd.Run()
}

// grim for Wayland
type grimCapturer struct{}

func (c *grimCapturer) capture(ctx context.Context, output, region string, delay int) error {
	if delay > 0 {
		exec.CommandContext(ctx, "sleep", fmt.Sprintf("%d", delay)).Run()
	}

	args := []string{}
	switch region {
	case "selection":
		// grim with slurp for selection
		slurp := exec.CommandContext(ctx, "slurp")
		geometry, err := slurp.Output()
		if err != nil {
			return fmt.Errorf("slurp selection failed: %w", err)
		}
		args = append(args, "-g", strings.TrimSpace(string(geometry)))
	case "full":
		// default
	default:
		args = append(args, "-g", region)
	}

	args = append(args, output)
	cmd := exec.CommandContext(ctx, "grim", args...)
	return cmd.Run()
}

// scrot for X11
type scrotCapturer struct{}

func (c *scrotCapturer) capture(ctx context.Context, output, region string, delay int) error {
	args := []string{}

	if delay > 0 {
		args = append(args, "-d", fmt.Sprintf("%d", delay))
	}

	switch region {
	case "selection":
		args = append(args, "-s") // select
	case "full":
		// default
	default:
		// scrot doesn't support geometry directly, use full
	}

	args = append(args, output)
	cmd := exec.CommandContext(ctx, "scrot", args...)
	return cmd.Run()
}

// ImageMagick import for X11
type importCapturer struct{}

func (c *importCapturer) capture(ctx context.Context, output, region string, delay int) error {
	if delay > 0 {
		exec.CommandContext(ctx, "sleep", fmt.Sprintf("%d", delay)).Run()
	}

	args := []string{}
	switch region {
	case "selection":
		// import without -window lets user select
	case "full":
		args = append(args, "-window", "root")
	default:
		args = append(args, "-window", "root", "-crop", region)
	}

	args = append(args, output)
	cmd := exec.CommandContext(ctx, "import", args...)
	return cmd.Run()
}

// gnome-screenshot
type gnomeScreenshotCapturer struct{}

func (c *gnomeScreenshotCapturer) capture(ctx context.Context, output, region string, delay int) error {
	args := []string{"-f", output}

	if delay > 0 {
		args = append(args, "-d", fmt.Sprintf("%d", delay))
	}

	switch region {
	case "selection":
		args = append(args, "-a") // area selection
	case "full":
		// default
	}

	cmd := exec.CommandContext(ctx, "gnome-screenshot", args...)
	return cmd.Run()
}

// Windows PowerShell capture
type windowsCapturer struct{}

func (c *windowsCapturer) capture(ctx context.Context, output, region string, delay int) error {
	if delay > 0 {
		exec.CommandContext(ctx, "timeout", "/t", fmt.Sprintf("%d", delay)).Run()
	}

	// PowerShell script to capture screen
	script := fmt.Sprintf(`
Add-Type -AssemblyName System.Windows.Forms
$screen = [System.Windows.Forms.Screen]::PrimaryScreen.Bounds
$bitmap = New-Object System.Drawing.Bitmap($screen.Width, $screen.Height)
$graphics = [System.Drawing.Graphics]::FromImage($bitmap)
$graphics.CopyFromScreen($screen.Location, [System.Drawing.Point]::Empty, $screen.Size)
$bitmap.Save('%s')
`, output)

	cmd := exec.CommandContext(ctx, "powershell", "-Command", script)
	return cmd.Run()
}

var _ Executor = (*ScreenCapture)(nil)
