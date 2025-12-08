package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/joss/urp/internal/config"
	"github.com/joss/urp/internal/memory"
	"github.com/joss/urp/internal/opencode/graphstore"
	"github.com/joss/urp/internal/opencode/session"
)

// StatusResponse is the API response for /api/status
type StatusResponse struct {
	GraphConnected bool   `json:"graphConnected"`
	Project        string `json:"project"`
	EventCount     int    `json:"eventCount"`
	Workers        int    `json:"workers"`
	Focus          *Focus `json:"focus"`
	Ctx            Ctx    `json:"ctx"`
	MemgraphURL    string `json:"memgraphUrl"`
}

// Focus represents the current focus target
type Focus struct {
	Target string `json:"target"`
	Depth  int    `json:"depth"`
}

// Ctx represents context window usage
type Ctx struct {
	Tokens int `json:"tokens"`
	Files  int `json:"files"`
}

// SessionResponse is the API response for /api/sessions
type SessionResponse struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	UpdatedAt int64  `json:"updatedAt"`
	Messages  int    `json:"messages"`
}

// PromptRequest is the API request for /api/prompt
type PromptRequest struct {
	Prompt    string `json:"prompt"`
	SessionID string `json:"sessionId,omitempty"`
}

// serveCmd creates the serve command for desktop integration
func serveCmd() *cobra.Command {
	var port int

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start HTTP server for desktop GUI",
		Long: `Start an HTTP server that exposes URP functionality via REST API.

This is used by the Tauri desktop application to communicate with URP.

Endpoints:
  GET  /api/status    - Get current URP status
  GET  /api/sessions  - List sessions
  POST /api/prompt    - Send prompt to agent
  GET  /api/focus     - Get current focus
  POST /api/focus     - Set focus target
  GET  /health        - Health check`,
		Run: func(cmd *cobra.Command, args []string) {
			runServer(port)
		},
	}

	cmd.Flags().IntVarP(&port, "port", "p", 7878, "Port to listen on")
	return cmd
}

func runServer(port int) {
	mux := http.NewServeMux()

	// CORS middleware
	corsHandler := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}
			h(w, r)
		}
	}

	// Health check
	mux.HandleFunc("/health", corsHandler(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))

	// Status endpoint
	mux.HandleFunc("/api/status", corsHandler(handleStatus))

	// Sessions endpoint
	mux.HandleFunc("/api/sessions", corsHandler(handleSessions))

	// Prompt endpoint
	mux.HandleFunc("/api/prompt", corsHandler(handlePrompt))

	// Focus endpoints
	mux.HandleFunc("/api/focus", corsHandler(handleFocus))

	// SSE streaming endpoint for agent responses
	mux.HandleFunc("/api/stream", corsHandler(handleStream))

	fmt.Printf("🚀 URP Server listening on http://localhost:%d\n", port)
	fmt.Println("   Endpoints: /api/status, /api/sessions, /api/prompt, /api/stream, /api/focus")

	if err := http.ListenAndServe(fmt.Sprintf(":%d", port), mux); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := context.Background()
	project := config.Env().Project
	if project == "" {
		project = "unknown"
	}

	connected := false
	eventCount := 0

	if db != nil {
		if err := db.Ping(ctx); err == nil {
			connected = true
			records, err := db.Execute(ctx,
				"MATCH (e:TerminalEvent) RETURN count(e) as count", nil)
			if err == nil && len(records) > 0 {
				if c, ok := records[0]["count"].(int64); ok {
					eventCount = int(c)
				}
			}
		}
	}

	// Get worker count
	workers := 0
	if db != nil {
		records, err := db.Execute(ctx,
			"MATCH (c:Container {type: 'worker'}) WHERE c.status = 'running' RETURN count(c) as count", nil)
		if err == nil && len(records) > 0 {
			if c, ok := records[0]["count"].(int64); ok {
				workers = int(c)
			}
		}
	}

	// Focus state will be tracked via the focus endpoint
	var focus *Focus
	// TODO: Add focus state tracking to FocusService

	resp := StatusResponse{
		GraphConnected: connected,
		Project:        project,
		EventCount:     eventCount,
		Workers:        workers,
		Focus:          focus,
		Ctx: Ctx{
			Tokens: 0, // TODO: track actual token usage
			Files:  0, // TODO: track loaded files
		},
		MemgraphURL: config.Env().Neo4jURI,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func handleSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := context.Background()
	var sessions []SessionResponse

	if db != nil {
		store := graphstore.New(db)
		mgr := session.NewManager(store)

		dir, _ := os.Getwd()
		list, err := mgr.List(ctx, dir, 20)
		if err == nil {
			for _, s := range list {
				msgs, _ := mgr.GetMessages(ctx, s.ID)
				sessions = append(sessions, SessionResponse{
					ID:        s.ID,
					Title:     s.Title,
					UpdatedAt: s.UpdatedAt.UnixMilli(),
					Messages:  len(msgs),
				})
			}
		}
	}

	if sessions == nil {
		sessions = []SessionResponse{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sessions)
}

func handlePrompt(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req PromptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Prompt == "" {
		http.Error(w, "Prompt is required", http.StatusBadRequest)
		return
	}

	// TODO: Actually process the prompt through the agent
	// For now, just acknowledge receipt
	resp := map[string]interface{}{
		"status":    "received",
		"prompt":    req.Prompt,
		"timestamp": time.Now().UnixMilli(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// currentFocus tracks the last focus operation (in-memory for now)
var currentFocus *Focus

func handleFocus(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()

	if db == nil {
		http.Error(w, "Graph not connected", http.StatusServiceUnavailable)
		return
	}

	focusSvc := memory.NewFocusService(db)

	switch r.Method {
	case "GET":
		w.Header().Set("Content-Type", "application/json")
		if currentFocus == nil {
			json.NewEncoder(w).Encode(map[string]interface{}{"focus": nil})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"focus": currentFocus,
		})

	case "POST":
		var req struct {
			Target string `json:"target"`
			Depth  int    `json:"depth"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		result, err := focusSvc.Focus(ctx, req.Target, req.Depth)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Track current focus
		currentFocus = &Focus{
			Target: req.Target,
			Depth:  req.Depth,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":  true,
			"entities": len(result.Entities),
			"rendered": result.Rendered,
		})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// SSE Event types
type SSEEvent struct {
	Type    string      `json:"type"`
	Data    interface{} `json:"data"`
	ID      string      `json:"id,omitempty"`
}

// handleStream handles SSE streaming for agent responses
func handleStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req PromptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Prompt == "" {
		http.Error(w, "Prompt is required", http.StatusBadRequest)
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// Send initial event
	sendSSE(w, flusher, SSEEvent{
		Type: "start",
		Data: map[string]interface{}{
			"prompt":    req.Prompt,
			"timestamp": time.Now().UnixMilli(),
		},
	})

	// TODO: Replace with actual agent execution
	// For now, simulate streaming response
	responses := []string{
		"Analyzing prompt...",
		"Searching codebase...",
		"Found relevant context.",
		"Generating response...",
		fmt.Sprintf("Processing: %s", req.Prompt),
	}

	for i, chunk := range responses {
		time.Sleep(300 * time.Millisecond)
		sendSSE(w, flusher, SSEEvent{
			Type: "chunk",
			Data: map[string]interface{}{
				"content": chunk,
				"index":   i,
			},
			ID: fmt.Sprintf("%d", i),
		})
	}

	// Send completion event
	sendSSE(w, flusher, SSEEvent{
		Type: "done",
		Data: map[string]interface{}{
			"totalChunks": len(responses),
			"timestamp":   time.Now().UnixMilli(),
		},
	})
}

func sendSSE(w http.ResponseWriter, flusher http.Flusher, event SSEEvent) {
	data, _ := json.Marshal(event)
	fmt.Fprintf(w, "event: %s\n", event.Type)
	if event.ID != "" {
		fmt.Fprintf(w, "id: %s\n", event.ID)
	}
	fmt.Fprintf(w, "data: %s\n\n", data)
	flusher.Flush()
}
