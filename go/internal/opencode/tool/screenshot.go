package tool

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/joss/urp/internal/opencode/domain"
)

// Screenshot tool for reading and encoding images
type Screenshot struct{}

func NewScreenshot() *Screenshot {
	return &Screenshot{}
}

func (s *Screenshot) Info() domain.Tool {
	return domain.Tool{
		Name:        "screenshot",
		Description: "Read an image file and return it for visual analysis. Supports PNG, JPEG, GIF, WebP.",
		Parameters: domain.JSONSchema{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Path to the image file",
				},
			},
			"required": []string{"path"},
		},
	}
}

func (s *Screenshot) Execute(ctx context.Context, args map[string]any) (*Result, error) {
	path, ok := args["path"].(string)
	if !ok || path == "" {
		return nil, ErrInvalidArgs
	}

	// Expand home directory
	if strings.HasPrefix(path, "~") {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, path[1:])
	}

	// Read file
	data, err := os.ReadFile(path)
	if err != nil {
		return &Result{
			Output: fmt.Sprintf("Error reading file: %v", err),
			Error:  err,
		}, nil
	}

	// Detect media type from extension
	ext := strings.ToLower(filepath.Ext(path))
	mediaType := "image/png"
	switch ext {
	case ".jpg", ".jpeg":
		mediaType = "image/jpeg"
	case ".gif":
		mediaType = "image/gif"
	case ".webp":
		mediaType = "image/webp"
	case ".png":
		mediaType = "image/png"
	default:
		return &Result{
			Output: fmt.Sprintf("Unsupported image format: %s", ext),
		}, nil
	}

	// Encode to base64
	encoded := base64.StdEncoding.EncodeToString(data)

	// Return image for vision analysis
	return &Result{
		Title:  fmt.Sprintf("Image: %s", filepath.Base(path)),
		Output: fmt.Sprintf("[Image loaded: %s, %d bytes, %s]", filepath.Base(path), len(data), mediaType),
		Images: []domain.ImagePart{
			{
				Base64:    encoded,
				MediaType: mediaType,
			},
		},
	}, nil
}

var _ Executor = (*Screenshot)(nil)
