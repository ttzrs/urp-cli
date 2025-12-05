package tool

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/joss/urp/internal/opencode/domain"
	"github.com/joss/urp/internal/opencode/lsp"
)

// LSPHover provides hover information via LSP
type LSPHover struct {
	workDir string
	clients map[string]*lsp.Client // language -> client
}

// NewLSPHover creates a new LSPHover tool
func NewLSPHover(workDir string) *LSPHover {
	return &LSPHover{
		workDir: workDir,
		clients: make(map[string]*lsp.Client),
	}
}

func (h *LSPHover) Info() domain.Tool {
	return domain.Tool{
		Name:        "lsp_hover",
		Description: "Get type and documentation info at a position (via LSP)",
		Parameters: domain.JSONSchema{
			"type": "object",
			"properties": map[string]any{
				"file_path": map[string]any{
					"type":        "string",
					"description": "Path to the file",
				},
				"line": map[string]any{
					"type":        "integer",
					"description": "Line number (1-indexed)",
				},
				"character": map[string]any{
					"type":        "integer",
					"description": "Character position (0-indexed)",
				},
			},
			"required": []string{"file_path", "line", "character"},
		},
	}
}

func (h *LSPHover) Execute(ctx context.Context, args map[string]any) (*Result, error) {
	filePath, _ := args["file_path"].(string)
	line, _ := args["line"].(float64)
	char, _ := args["character"].(float64)

	if filePath == "" {
		return &Result{Error: fmt.Errorf("file_path is required")}, nil
	}

	// Resolve path
	if !filepath.IsAbs(filePath) {
		filePath = filepath.Join(h.workDir, filePath)
	}

	// Check file exists
	if _, err := os.Stat(filePath); err != nil {
		return &Result{Error: fmt.Errorf("file not found: %s", filePath)}, nil
	}

	// Determine language and get client
	lang := detectLanguage(filePath)
	client, err := h.getClient(ctx, lang)
	if err != nil {
		return &Result{
			Output: fmt.Sprintf("LSP not available for %s: %v\n\nUse the read tool to inspect the file manually.", lang, err),
		}, nil
	}

	// Convert to 0-indexed line
	pos := lsp.Position{
		Line:      int(line) - 1,
		Character: int(char),
	}

	uri := "file://" + filePath
	hover, err := client.Hover(ctx, uri, pos)
	if err != nil {
		return &Result{Error: fmt.Errorf("hover request failed: %w", err)}, nil
	}

	if hover == nil {
		return &Result{Output: "No hover information available at this position"}, nil
	}

	return &Result{
		Title:  fmt.Sprintf("Hover: %s:%d:%d", filepath.Base(filePath), int(line), int(char)),
		Output: hover.Contents.Value,
	}, nil
}

// getClient returns or creates an LSP client for the language
func (h *LSPHover) getClient(ctx context.Context, lang string) (*lsp.Client, error) {
	if client, ok := h.clients[lang]; ok {
		return client, nil
	}

	// Get server command for language
	serverCmd, serverArgs := getServerCommand(lang)
	if serverCmd == "" {
		return nil, fmt.Errorf("no LSP server configured for %s", lang)
	}

	client, err := lsp.NewClient(ctx, serverCmd, serverArgs...)
	if err != nil {
		return nil, err
	}

	// Initialize
	rootURI := "file://" + h.workDir
	if err := client.Initialize(ctx, rootURI); err != nil {
		client.Close()
		return nil, err
	}

	h.clients[lang] = client
	return client, nil
}

// Close closes all LSP clients
func (h *LSPHover) Close() {
	for _, client := range h.clients {
		client.Close()
	}
}

// detectLanguage is defined in diagnostics.go

func getServerCommand(lang string) (string, []string) {
	switch lang {
	case "go":
		return "gopls", []string{}
	case "typescript", "javascript":
		return "typescript-language-server", []string{"--stdio"}
	case "python":
		return "pyright-langserver", []string{"--stdio"}
	case "rust":
		return "rust-analyzer", []string{}
	default:
		return "", nil
	}
}
