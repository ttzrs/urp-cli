package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"

	"github.com/joss/urp/internal/opencode/domain"
)

// Client implements the MCP (Model Context Protocol) client
type Client struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  io.ReadCloser
	scanner *bufio.Scanner

	requestID atomic.Int64
	pending   sync.Map // map[int64]chan *Response

	tools     []domain.Tool
	toolsOnce sync.Once

	mu sync.Mutex
}

// Message types for JSON-RPC 2.0
type Request struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func (e *RPCError) Error() string {
	return fmt.Sprintf("RPC error %d: %s", e.Code, e.Message)
}

// NewClient creates a new MCP client for a server
func NewClient(command string, args []string, env map[string]string) (*Client, error) {
	cmd := exec.Command(command, args...)

	// Set environment
	for k, v := range env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start server: %w", err)
	}

	c := &Client{
		cmd:     cmd,
		stdin:   stdin,
		stdout:  stdout,
		scanner: bufio.NewScanner(stdout),
	}

	// Start response reader
	go c.readResponses()

	// Initialize connection
	if err := c.initialize(); err != nil {
		c.Close()
		return nil, fmt.Errorf("initialize: %w", err)
	}

	return c, nil
}

func (c *Client) readResponses() {
	for c.scanner.Scan() {
		line := c.scanner.Text()
		if line == "" {
			continue
		}

		var resp Response
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			continue
		}

		if ch, ok := c.pending.LoadAndDelete(resp.ID); ok {
			ch.(chan *Response) <- &resp
		}
	}
}

func (c *Client) send(req *Request) (*Response, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	ch := make(chan *Response, 1)
	c.pending.Store(req.ID, ch)

	data, err := json.Marshal(req)
	if err != nil {
		c.pending.Delete(req.ID)
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	if _, err := c.stdin.Write(append(data, '\n')); err != nil {
		c.pending.Delete(req.ID)
		return nil, fmt.Errorf("write request: %w", err)
	}

	resp := <-ch
	if resp.Error != nil {
		return nil, resp.Error
	}

	return resp, nil
}

func (c *Client) initialize() error {
	req := &Request{
		JSONRPC: "2.0",
		ID:      c.requestID.Add(1),
		Method:  "initialize",
		Params: map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{},
			"clientInfo": map[string]any{
				"name":    "opencode",
				"version": "1.0.0",
			},
		},
	}

	resp, err := c.send(req)
	if err != nil {
		return err
	}

	// Send initialized notification
	notif := &Request{
		JSONRPC: "2.0",
		ID:      c.requestID.Add(1),
		Method:  "notifications/initialized",
	}

	data, _ := json.Marshal(notif)
	c.stdin.Write(append(data, '\n'))

	_ = resp // We could parse server capabilities here
	return nil
}

// ListTools returns available tools from the MCP server
func (c *Client) ListTools(ctx context.Context) ([]domain.Tool, error) {
	var err error
	c.toolsOnce.Do(func() {
		req := &Request{
			JSONRPC: "2.0",
			ID:      c.requestID.Add(1),
			Method:  "tools/list",
		}

		var resp *Response
		resp, err = c.send(req)
		if err != nil {
			return
		}

		var result struct {
			Tools []struct {
				Name        string            `json:"name"`
				Description string            `json:"description"`
				InputSchema domain.JSONSchema `json:"inputSchema"`
			} `json:"tools"`
		}

		if err = json.Unmarshal(resp.Result, &result); err != nil {
			return
		}

		for _, t := range result.Tools {
			c.tools = append(c.tools, domain.Tool{
				ID:          t.Name,
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.InputSchema,
			})
		}
	})

	return c.tools, err
}

// CallTool invokes a tool on the MCP server
func (c *Client) CallTool(ctx context.Context, name string, args map[string]any) (string, error) {
	req := &Request{
		JSONRPC: "2.0",
		ID:      c.requestID.Add(1),
		Method:  "tools/call",
		Params: map[string]any{
			"name":      name,
			"arguments": args,
		},
	}

	resp, err := c.send(req)
	if err != nil {
		return "", err
	}

	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}

	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return "", fmt.Errorf("unmarshal result: %w", err)
	}

	var output string
	for _, c := range result.Content {
		if c.Type == "text" {
			output += c.Text
		}
	}

	if result.IsError {
		return output, fmt.Errorf("tool error: %s", output)
	}

	return output, nil
}

// Close shuts down the MCP client
func (c *Client) Close() error {
	c.stdin.Close()
	return c.cmd.Process.Kill()
}

// Manager handles multiple MCP servers
type Manager struct {
	clients map[string]*Client
	mu      sync.RWMutex
}

func NewManager() *Manager {
	return &Manager{
		clients: make(map[string]*Client),
	}
}

// Connect connects to an MCP server
func (m *Manager) Connect(name string, server domain.MCPServer) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.clients[name]; exists {
		return nil // Already connected
	}

	client, err := NewClient(server.Command, server.Args, server.Env)
	if err != nil {
		return fmt.Errorf("connect to %s: %w", name, err)
	}

	m.clients[name] = client
	return nil
}

// GetTools returns all tools from all connected servers
func (m *Manager) GetTools(ctx context.Context) ([]domain.Tool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var allTools []domain.Tool
	for name, client := range m.clients {
		tools, err := client.ListTools(ctx)
		if err != nil {
			return nil, fmt.Errorf("list tools from %s: %w", name, err)
		}

		// Prefix tool names with server name
		for _, t := range tools {
			t.ID = fmt.Sprintf("mcp_%s_%s", name, t.ID)
			t.Name = fmt.Sprintf("mcp_%s_%s", name, t.Name)
			allTools = append(allTools, t)
		}
	}

	return allTools, nil
}

// CallTool calls a tool on the appropriate server
func (m *Manager) CallTool(ctx context.Context, toolName string, args map[string]any) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Parse server name from tool name (mcp_servername_toolname)
	for name, client := range m.clients {
		prefix := fmt.Sprintf("mcp_%s_", name)
		if len(toolName) > len(prefix) && toolName[:len(prefix)] == prefix {
			actualName := toolName[len(prefix):]
			return client.CallTool(ctx, actualName, args)
		}
	}

	return "", fmt.Errorf("no server found for tool: %s", toolName)
}

// Close closes all connections
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, client := range m.clients {
		client.Close()
	}
	m.clients = make(map[string]*Client)
}
