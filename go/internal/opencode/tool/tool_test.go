package tool

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBashExecute(t *testing.T) {
	bash := NewBash("/tmp")

	tests := []struct {
		name    string
		args    map[string]any
		wantErr bool
	}{
		{
			name:    "echo command",
			args:    map[string]any{"command": "echo hello"},
			wantErr: false,
		},
		{
			name:    "empty command",
			args:    map[string]any{"command": ""},
			wantErr: true,
		},
		{
			name:    "missing command",
			args:    map[string]any{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := bash.Execute(context.Background(), tt.args)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.NotEmpty(t, result.Output, "expected output for successful command")
		})
	}
}

func TestReadExecute(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("line1\nline2\nline3"), 0644))

	read := NewRead()

	tests := []struct {
		name       string
		args       map[string]any
		wantErr    bool
		wantOutput string
	}{
		{
			name:       "read file",
			args:       map[string]any{"file_path": testFile},
			wantOutput: "line1",
		},
		{
			name:    "missing path",
			args:    map[string]any{},
			wantErr: true,
		},
		{
			name: "nonexistent file",
			args: map[string]any{"file_path": "/nonexistent/file.txt"},
			// Returns error in result, not error return
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := read.Execute(context.Background(), tt.args)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			if tt.wantOutput != "" {
				assert.Contains(t, result.Output, tt.wantOutput)
			}
		})
	}
}

func TestWriteExecute(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "write_test.txt")

	write := NewWrite()

	tests := []struct {
		name    string
		args    map[string]any
		wantErr bool
	}{
		{
			name:    "write file",
			args:    map[string]any{"file_path": testFile, "content": "test content"},
			wantErr: false,
		},
		{
			name:    "missing path",
			args:    map[string]any{"content": "test"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := write.Execute(context.Background(), tt.args)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}

	// Verify file was written
	content, err := os.ReadFile(testFile)
	require.NoError(t, err)
	assert.Equal(t, "test content", string(content))
}

func TestEditExecute(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "edit_test.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("hello world"), 0644))

	edit := NewEdit()

	tests := []struct {
		name    string
		args    map[string]any
		wantErr bool
	}{
		{
			name: "edit file",
			args: map[string]any{
				"file_path":  testFile,
				"old_string": "world",
				"new_string": "golang",
			},
		},
		{
			name: "nonexistent string",
			args: map[string]any{
				"file_path":  testFile,
				"old_string": "notfound",
				"new_string": "replacement",
			},
			// Error returned in result, not error return
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := edit.Execute(context.Background(), tt.args)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}

	// Verify edit was applied
	content, _ := os.ReadFile(testFile)
	assert.Equal(t, "hello golang", string(content))
}

func TestGlobExecute(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test files
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "file1.go"), []byte(""), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "file2.go"), []byte(""), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "file.txt"), []byte(""), 0644))

	glob := NewGlob(tmpDir)

	tests := []struct {
		name      string
		pattern   string
		wantCount int
	}{
		{"find go files", "*.go", 2},
		{"find all files", "*", 3},
		{"find txt files", "*.txt", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := glob.Execute(context.Background(), map[string]any{"pattern": tt.pattern})

			require.NoError(t, err)
			assert.Equal(t, tt.wantCount, result.Metadata["count"].(int))
		})
	}
}

func TestRegistry(t *testing.T) {
	r := NewRegistry()
	r.Register(NewBash("/tmp"))
	r.Register(NewRead())

	t.Run("Get", func(t *testing.T) {
		tool, ok := r.Get("bash")
		require.True(t, ok, "expected to find bash tool")
		assert.Equal(t, "bash", tool.Info().Name)
	})

	t.Run("All", func(t *testing.T) {
		all := r.All()
		assert.Len(t, all, 2)
	})

	t.Run("Execute", func(t *testing.T) {
		result, err := r.Execute(context.Background(), "bash", map[string]any{"command": "echo test"})
		require.NoError(t, err)
		assert.Contains(t, result.Output, "test")
	})

	t.Run("NotFound", func(t *testing.T) {
		_, err := r.Execute(context.Background(), "notfound", nil)
		assert.ErrorIs(t, err, ErrToolNotFound)
	})
}

func TestScreenshotExecute(t *testing.T) {
	tmpDir := t.TempDir()

	// Minimal PNG file (1x1 transparent pixel)
	pngData := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
		0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1F, 0x15, 0xC4,
		0x89, 0x00, 0x00, 0x00, 0x0A, 0x49, 0x44, 0x41,
		0x54, 0x78, 0x9C, 0x63, 0x00, 0x01, 0x00, 0x00,
		0x05, 0x00, 0x01, 0x0D, 0x0A, 0x2D, 0xB4, 0x00,
		0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, 0x44, 0xAE,
		0x42, 0x60, 0x82,
	}

	pngFile := filepath.Join(tmpDir, "test.png")
	jpgFile := filepath.Join(tmpDir, "test.jpg")
	txtFile := filepath.Join(tmpDir, "test.txt")

	require.NoError(t, os.WriteFile(pngFile, pngData, 0644))
	require.NoError(t, os.WriteFile(jpgFile, []byte("fake jpeg"), 0644))
	require.NoError(t, os.WriteFile(txtFile, []byte("not an image"), 0644))

	screenshot := NewScreenshot()

	tests := []struct {
		name       string
		args       map[string]any
		wantErr    bool
		wantImages int
		wantMedia  string
	}{
		{
			name:       "read PNG image",
			args:       map[string]any{"path": pngFile},
			wantImages: 1,
			wantMedia:  "image/png",
		},
		{
			name:       "read JPEG image",
			args:       map[string]any{"path": jpgFile},
			wantImages: 1,
			wantMedia:  "image/jpeg",
		},
		{
			name:       "unsupported format",
			args:       map[string]any{"path": txtFile},
			wantImages: 0,
		},
		{
			name:    "missing path",
			args:    map[string]any{},
			wantErr: true,
		},
		{
			name:       "nonexistent file",
			args:       map[string]any{"path": "/nonexistent/image.png"},
			wantImages: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := screenshot.Execute(context.Background(), tt.args)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Len(t, result.Images, tt.wantImages)

			if tt.wantImages > 0 {
				assert.Equal(t, tt.wantMedia, result.Images[0].MediaType)
				assert.NotEmpty(t, result.Images[0].Base64)
			}
		})
	}
}

func TestScreenshotInfo(t *testing.T) {
	screenshot := NewScreenshot()
	info := screenshot.Info()

	assert.Equal(t, "screenshot", info.Name)
	assert.NotEmpty(t, info.Description)
	assert.NotNil(t, info.Parameters)
}

func TestScreenCaptureInfo(t *testing.T) {
	sc := NewScreenCapture()
	info := sc.Info()

	assert.Equal(t, "screen_capture", info.Name)
	assert.NotEmpty(t, info.Description)
	assert.NotNil(t, info.Parameters)
}

func TestScreenCaptureDetectCapturer(t *testing.T) {
	sc := NewScreenCapture()
	capturer := sc.detectCapturer()
	t.Logf("Detected capturer: %T", capturer)
}

func TestScreenCaptureExecuteNoTool(t *testing.T) {
	sc := NewScreenCapture()

	result, err := sc.Execute(context.Background(), map[string]any{
		"output": "/nonexistent/path/that/should/fail.png",
	})

	require.NoError(t, err, "Execute should not return error")
	require.NotNil(t, result)
	t.Logf("Result: %s", result.Output)
}

func TestDefaultRegistryTools(t *testing.T) {
	registry := DefaultRegistry("/tmp")

	expectedTools := []string{"screen_capture", "screenshot", "computer", "browser"}
	for _, name := range expectedTools {
		t.Run(name, func(t *testing.T) {
			_, ok := registry.Get(name)
			assert.True(t, ok, "%s tool should be registered", name)
		})
	}
}

func TestComputerInfo(t *testing.T) {
	c := NewComputer()
	info := c.Info()

	assert.Equal(t, "computer", info.Name)
	assert.NotEmpty(t, info.Description)
}

func TestComputerIsDangerous(t *testing.T) {
	c := NewComputer()

	tests := []struct {
		action    string
		dangerous bool
	}{
		{"mouse_position", false},
		{"screenshot", false},
		{"click", true},
		{"double_click", true},
		{"right_click", true},
		{"move", true},
		{"drag", true},
		{"type", true},
		{"key", true},
		{"scroll", true},
	}

	for _, tt := range tests {
		t.Run(tt.action, func(t *testing.T) {
			assert.Equal(t, tt.dangerous, c.IsDangerous(tt.action))
		})
	}
}

func TestComputerDetectBackend(t *testing.T) {
	c := NewComputer()
	backend := c.detectBackend()
	t.Logf("Detected backend: %T", backend)
}

func TestComputerExecuteActions(t *testing.T) {
	c := NewComputer()

	tests := []struct {
		name    string
		args    map[string]any
		wantErr error
	}{
		{
			name: "mouse_position",
			args: map[string]any{"action": "mouse_position"},
		},
		{
			name: "invalid_action",
			args: map[string]any{"action": "invalid_action"},
		},
		{
			name:    "missing_action",
			args:    map[string]any{},
			wantErr: ErrInvalidArgs,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := c.Execute(context.Background(), tt.args)

			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)
			t.Logf("Result: %s", result.Output)
		})
	}
}

func TestComputerWatchClicksAction(t *testing.T) {
	c := NewComputer()

	result, err := c.Execute(context.Background(), map[string]any{
		"action":     "watch_clicks",
		"timeout":    1,
		"max_events": 1,
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	t.Logf("Result: %s", result.Output)
}

func TestClickEventStruct(t *testing.T) {
	event := ClickEvent{
		X:         100,
		Y:         200,
		Button:    "left",
		Timestamp: 1234567890000,
	}

	assert.Equal(t, 100, event.X)
	assert.Equal(t, 200, event.Y)
	assert.Equal(t, "left", event.Button)
}
