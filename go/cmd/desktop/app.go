package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/joss/urp/internal/config"
	"github.com/joss/urp/internal/graph"
	"github.com/joss/urp/internal/memory"
	"github.com/joss/urp/internal/opencode/agent"
	"github.com/joss/urp/internal/opencode/domain"
	"github.com/joss/urp/internal/opencode/graphstore"
	"github.com/joss/urp/internal/opencode/provider"
	"github.com/joss/urp/internal/opencode/session"
	"github.com/joss/urp/internal/opencode/tool"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct holds the application state
type App struct {
	ctx     context.Context
	db      graph.Driver
	mu      sync.RWMutex
	focus   *FocusState
	agent   *agent.Agent
	session *domain.Session
	store   *graphstore.Store
	sessMgr *session.Manager
}

// FocusState represents the current focus target
type FocusState struct {
	Target string `json:"target"`
	Depth  int    `json:"depth"`
}

// Status represents the URP status for the frontend
type Status struct {
	GraphConnected bool        `json:"graphConnected"`
	Project        string      `json:"project"`
	EventCount     int         `json:"eventCount"`
	Workers        int         `json:"workers"`
	Focus          *FocusState `json:"focus"`
	Ctx            CtxInfo     `json:"ctx"`
	MemgraphURL    string      `json:"memgraphUrl"`
}

// CtxInfo represents context window usage
type CtxInfo struct {
	Tokens int `json:"tokens"`
	Files  int `json:"files"`
}

// Session represents a chat session
type Session struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	UpdatedAt int64  `json:"updatedAt"`
	Messages  int    `json:"messages"`
}

// StreamChunk is emitted during streaming
type StreamChunk struct {
	Type    string      `json:"type"`
	Content string      `json:"content,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

// NewApp creates a new App instance
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.connectGraph()
	a.initAgent()
}

// shutdown is called when the app closes
func (a *App) shutdown(ctx context.Context) {
	if a.db != nil {
		a.db.Close()
	}
}

func (a *App) connectGraph() {
	cfg := graph.Config{
		URI: config.Env().Neo4jURI,
	}
	if cfg.URI == "" {
		cfg.URI = "bolt://localhost:7687"
	}
	db, err := graph.NewMemgraph(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Graph connect error: %v\n", err)
		return
	}
	a.db = db
	a.store = graphstore.New(db)
	a.sessMgr = session.NewManager(a.store)
}

func (a *App) initAgent() {
	// Create provider with custom base URL (LiteLLM proxy)
	p, err := provider.Default.Create(provider.ProviderAnthropic,
		provider.WithBaseURL("http://100.105.212.98:8317/v1/"),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Provider error: %v\n", err)
		return
	}

	// Get agent config
	agentCfg := agent.BuiltinAgents()["build"]
	agentCfg.Model = &domain.ModelConfig{
		ProviderID: "anthropic",
		ModelID:    "claude-sonnet-4-20250514",
	}

	// Create tool registry
	wd, _ := os.Getwd()
	tools := tool.DefaultRegistry(wd)

	// Create agent
	a.agent = agent.New(agentCfg, p, tools)

	// Set working directory
	if wd, err := os.Getwd(); err == nil {
		a.agent.SetWorkDir(wd)
	}

	// Create or load session
	if a.store != nil {
		wd, _ := os.Getwd()
		sess, err := a.sessMgr.GetOrCreate(a.ctx, wd)
		if err == nil {
			a.session = sess
		}
	}
}

// GetStatus returns the current URP status
func (a *App) GetStatus() Status {
	a.mu.RLock()
	focus := a.focus
	a.mu.RUnlock()

	project := config.Env().Project
	if project == "" {
		project = "unknown"
	}

	connected := false
	eventCount := 0
	workers := 0

	if a.db != nil {
		if err := a.db.Ping(a.ctx); err == nil {
			connected = true

			records, err := a.db.Execute(a.ctx,
				"MATCH (e:TerminalEvent) RETURN count(e) as count", nil)
			if err == nil && len(records) > 0 {
				if c, ok := records[0]["count"].(int64); ok {
					eventCount = int(c)
				}
			}

			records, err = a.db.Execute(a.ctx,
				"MATCH (c:Container {type: 'worker'}) WHERE c.status = 'running' RETURN count(c) as count", nil)
			if err == nil && len(records) > 0 {
				if c, ok := records[0]["count"].(int64); ok {
					workers = int(c)
				}
			}
		}
	}

	return Status{
		GraphConnected: connected,
		Project:        project,
		EventCount:     eventCount,
		Workers:        workers,
		Focus:          focus,
		Ctx: CtxInfo{
			Tokens: 0,
			Files:  0,
		},
		MemgraphURL: config.Env().Neo4jURI,
	}
}

// GetSessions returns the list of sessions
func (a *App) GetSessions() []Session {
	var sessions []Session

	if a.db != nil {
		st := graphstore.New(a.db)
		mgr := session.NewManager(st)

		dir, _ := os.Getwd()
		list, err := mgr.List(a.ctx, dir, 20)
		if err == nil {
			for _, s := range list {
				msgs, _ := mgr.GetMessages(a.ctx, s.ID)
				sessions = append(sessions, Session{
					ID:        s.ID,
					Title:     s.Title,
					UpdatedAt: s.UpdatedAt.UnixMilli(),
					Messages:  len(msgs),
				})
			}
		}
	}

	if sessions == nil {
		sessions = []Session{}
	}
	return sessions
}

// GetFocus returns the current focus state
func (a *App) GetFocus() *FocusState {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.focus
}

// SetFocus sets the focus target
func (a *App) SetFocus(target string, depth int) (map[string]interface{}, error) {
	if a.db == nil {
		return nil, fmt.Errorf("graph not connected")
	}

	focusSvc := memory.NewFocusService(a.db)
	result, err := focusSvc.Focus(a.ctx, target, depth)
	if err != nil {
		return nil, err
	}

	a.mu.Lock()
	a.focus = &FocusState{Target: target, Depth: depth}
	a.mu.Unlock()

	return map[string]interface{}{
		"success":  true,
		"entities": len(result.Entities),
		"rendered": result.Rendered,
	}, nil
}

// SendPrompt sends a prompt and streams the response via events
func (a *App) SendPrompt(prompt string) {
	// Emit start event
	runtime.EventsEmit(a.ctx, "stream", StreamChunk{
		Type: "start",
		Data: map[string]interface{}{
			"prompt":    prompt,
			"timestamp": time.Now().UnixMilli(),
		},
	})

	// Check if agent is initialized
	if a.agent == nil {
		runtime.EventsEmit(a.ctx, "stream", StreamChunk{
			Type:    "error",
			Content: "Agent not initialized. Check ANTHROPIC_API_KEY.",
		})
		return
	}

	// Ensure we have a session
	if a.session == nil {
		wd, _ := os.Getwd()
		a.session = &domain.Session{
			ID:        fmt.Sprintf("desktop-%d", time.Now().UnixNano()),
			Directory: wd,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
	}

	// Get existing messages
	var messages []*domain.Message
	if a.store != nil {
		msgs, _ := a.sessMgr.GetMessages(a.ctx, a.session.ID)
		messages = msgs
	}

	// Run agent
	events, err := a.agent.Run(a.ctx, a.session, messages, prompt)
	if err != nil {
		runtime.EventsEmit(a.ctx, "stream", StreamChunk{
			Type:    "error",
			Content: err.Error(),
		})
		return
	}

	// Stream events to frontend
	go func() {
		for event := range events {
			switch event.Type {
			case domain.StreamEventText:
				runtime.EventsEmit(a.ctx, "stream", StreamChunk{
					Type:    "chunk",
					Content: event.Content,
				})
			case domain.StreamEventToolCall:
				if tc, ok := event.Part.(domain.ToolCallPart); ok {
					runtime.EventsEmit(a.ctx, "stream", StreamChunk{
						Type:    "chunk",
						Content: fmt.Sprintf("\n[Tool: %s]\n", tc.Name),
					})
				}
			case domain.StreamEventToolDone:
				runtime.EventsEmit(a.ctx, "stream", StreamChunk{
					Type:    "chunk",
					Content: "[Tool done]\n",
				})
			case domain.StreamEventError:
				runtime.EventsEmit(a.ctx, "stream", StreamChunk{
					Type:    "error",
					Content: event.Error.Error(),
				})
			case domain.StreamEventDone:
				runtime.EventsEmit(a.ctx, "stream", StreamChunk{
					Type: "done",
					Data: map[string]interface{}{
						"timestamp": time.Now().UnixMilli(),
					},
				})
			}
		}
	}()
}

// GetWorkingDirectory returns the current working directory
func (a *App) GetWorkingDirectory() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	return filepath.Base(dir)
}
