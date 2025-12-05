// Package lsp provides a minimal LSP client for diagnostics and hover info
package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"
)

// Client is a minimal LSP client
type Client struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	reqID  atomic.Int64
	mu     sync.Mutex

	// Response handlers
	pending map[int64]chan json.RawMessage
	pendMu  sync.Mutex
}

// Position represents a position in a file
type Position struct {
	Line      int `json:"line"`      // 0-indexed
	Character int `json:"character"` // 0-indexed
}

// Range represents a range in a file
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// Location represents a location in a file
type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

// Diagnostic represents a diagnostic message
type Diagnostic struct {
	Range    Range  `json:"range"`
	Severity int    `json:"severity"` // 1=Error, 2=Warning, 3=Info, 4=Hint
	Source   string `json:"source,omitempty"`
	Message  string `json:"message"`
}

// Hover represents hover information
type Hover struct {
	Contents MarkupContent `json:"contents"`
	Range    *Range        `json:"range,omitempty"`
}

// MarkupContent represents markdown or plaintext content
type MarkupContent struct {
	Kind  string `json:"kind"` // "plaintext" or "markdown"
	Value string `json:"value"`
}

// NewClient creates a new LSP client by starting the server
func NewClient(ctx context.Context, serverCmd string, args ...string) (*Client, error) {
	cmd := exec.CommandContext(ctx, serverCmd, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		stdin.Close()
		return nil, fmt.Errorf("start server: %w", err)
	}

	c := &Client{
		cmd:     cmd,
		stdin:   stdin,
		stdout:  bufio.NewReader(stdout),
		pending: make(map[int64]chan json.RawMessage),
	}

	// Start reader goroutine
	go c.readLoop()

	return c, nil
}

// Close shuts down the LSP server
func (c *Client) Close() error {
	c.stdin.Close()
	return c.cmd.Wait()
}

// Initialize sends the initialize request
func (c *Client) Initialize(ctx context.Context, rootURI string) error {
	params := map[string]any{
		"processId":    nil,
		"rootUri":      rootURI,
		"capabilities": map[string]any{},
	}

	_, err := c.call(ctx, "initialize", params)
	if err != nil {
		return err
	}

	// Send initialized notification
	return c.notify("initialized", map[string]any{})
}

// Hover requests hover information at a position
func (c *Client) Hover(ctx context.Context, uri string, pos Position) (*Hover, error) {
	params := map[string]any{
		"textDocument": map[string]string{"uri": uri},
		"position":     pos,
	}

	result, err := c.call(ctx, "textDocument/hover", params)
	if err != nil {
		return nil, err
	}

	if result == nil || string(result) == "null" {
		return nil, nil
	}

	var hover Hover
	if err := json.Unmarshal(result, &hover); err != nil {
		return nil, fmt.Errorf("unmarshal hover: %w", err)
	}

	return &hover, nil
}

// Diagnostics requests diagnostics for a file (via publishDiagnostics notification)
// Note: Most LSP servers push diagnostics via notifications, not requests
func (c *Client) OpenFile(ctx context.Context, uri, text, languageID string) error {
	params := map[string]any{
		"textDocument": map[string]any{
			"uri":        uri,
			"languageId": languageID,
			"version":    1,
			"text":       text,
		},
	}

	return c.notify("textDocument/didOpen", params)
}

// call sends a request and waits for response
func (c *Client) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := c.reqID.Add(1)

	req := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	}

	// Register response handler
	respChan := make(chan json.RawMessage, 1)
	c.pendMu.Lock()
	c.pending[id] = respChan
	c.pendMu.Unlock()

	defer func() {
		c.pendMu.Lock()
		delete(c.pending, id)
		c.pendMu.Unlock()
	}()

	// Send request
	if err := c.send(req); err != nil {
		return nil, err
	}

	// Wait for response
	select {
	case result := <-respChan:
		return result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// notify sends a notification (no response expected)
func (c *Client) notify(method string, params any) error {
	req := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	}
	return c.send(req)
}

// send writes a JSON-RPC message
func (c *Client) send(msg any) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
	if _, err := c.stdin.Write([]byte(header)); err != nil {
		return err
	}
	_, err = c.stdin.Write(data)
	return err
}

// readLoop reads responses from the server
func (c *Client) readLoop() {
	for {
		// Read Content-Length header
		var contentLen int
		for {
			line, err := c.stdout.ReadString('\n')
			if err != nil {
				return
			}
			if line == "\r\n" {
				break
			}
			if _, err := fmt.Sscanf(line, "Content-Length: %d", &contentLen); err == nil {
				// Got content length
			}
		}

		if contentLen == 0 {
			continue
		}

		// Read body
		body := make([]byte, contentLen)
		if _, err := io.ReadFull(c.stdout, body); err != nil {
			return
		}

		// Parse response
		var msg struct {
			ID     *int64          `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}

		if err := json.Unmarshal(body, &msg); err != nil {
			continue
		}

		// Handle response
		if msg.ID != nil {
			c.pendMu.Lock()
			if ch, ok := c.pending[*msg.ID]; ok {
				ch <- msg.Result
			}
			c.pendMu.Unlock()
		}
	}
}
