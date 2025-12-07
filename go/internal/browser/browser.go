// Package browser provides Chrome automation via go-rod.
// Two modes: Observer (full Chrome for learning) and Executor (headless for automation).
package browser

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

// Mode defines browser operation mode.
type Mode int

const (
	// ModeObserver runs Chrome with GUI for user interaction logging.
	ModeObserver Mode = iota
	// ModeExecutor runs headless Chrome for automation.
	ModeExecutor
)

// Event represents a recorded browser event.
type Event struct {
	Type      string            `json:"type"`      // click, input, navigate, scroll, etc.
	Timestamp time.Time         `json:"timestamp"`
	URL       string            `json:"url,omitempty"`
	Selector  string            `json:"selector,omitempty"`
	Value     string            `json:"value,omitempty"`
	Position  *Position         `json:"position,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// Position represents x,y coordinates.
type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// Session represents a browser recording session.
type Session struct {
	ID        string    `json:"id"`
	StartURL  string    `json:"start_url"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at,omitempty"`
	Events    []Event   `json:"events"`
}

// Browser wraps go-rod with observer/executor capabilities.
type Browser struct {
	mu       sync.RWMutex
	mode     Mode
	browser  *rod.Browser
	launcher *launcher.Launcher
	session  *Session
	events   chan Event
	recorder EventRecorder
}

// EventRecorder persists browser events (e.g., to Memgraph).
type EventRecorder interface {
	RecordEvent(ctx context.Context, sessionID string, event Event) error
	SaveSession(ctx context.Context, session *Session) error
}

// Config configures browser behavior.
type Config struct {
	Mode        Mode
	StartURL    string
	UserDataDir string        // Persist profile between sessions
	Timeout     time.Duration // Default page timeout
	Recorder    EventRecorder // Optional event recorder
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		Mode:    ModeExecutor,
		Timeout: 30 * time.Second,
	}
}

// New creates a new browser instance.
func New(cfg Config) (*Browser, error) {
	var l *launcher.Launcher

	switch cfg.Mode {
	case ModeObserver:
		// Full Chrome with GUI for observation
		l = launcher.New().
			Headless(false).
			Devtools(true) // Open DevTools for debugging
		if cfg.UserDataDir != "" {
			l = l.UserDataDir(cfg.UserDataDir)
		}
	case ModeExecutor:
		// Headless for automation
		l = launcher.New().
			Headless(true).
			NoSandbox(true) // Required for containers
	}

	u, err := l.Launch()
	if err != nil {
		return nil, fmt.Errorf("launch browser: %w", err)
	}

	browser := rod.New().ControlURL(u)
	if err := browser.Connect(); err != nil {
		return nil, fmt.Errorf("connect to browser: %w", err)
	}

	if cfg.Timeout > 0 {
		browser = browser.Timeout(cfg.Timeout)
	}

	b := &Browser{
		mode:     cfg.Mode,
		browser:  browser,
		launcher: l,
		events:   make(chan Event, 100),
		recorder: cfg.Recorder,
	}

	return b, nil
}

// StartSession begins a new recording session.
func (b *Browser) StartSession(ctx context.Context, startURL string) (*Session, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.session = &Session{
		ID:        fmt.Sprintf("browser-%d", time.Now().UnixNano()),
		StartURL:  startURL,
		StartedAt: time.Now(),
		Events:    make([]Event, 0),
	}

	// Navigate to start URL
	page, err := b.browser.Page(proto.TargetCreateTarget{URL: startURL})
	if err != nil {
		return nil, fmt.Errorf("create page: %w", err)
	}

	if err := page.WaitLoad(); err != nil {
		return nil, fmt.Errorf("wait load: %w", err)
	}

	// Record navigation event
	b.recordEvent(Event{
		Type:      "navigate",
		Timestamp: time.Now(),
		URL:       startURL,
	})

	// Start event listener in observer mode
	if b.mode == ModeObserver {
		go b.observeEvents(ctx, page)
	}

	return b.session, nil
}

// EndSession stops recording and saves the session.
func (b *Browser) EndSession(ctx context.Context) (*Session, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.session == nil {
		return nil, fmt.Errorf("no active session")
	}

	b.session.EndedAt = time.Now()

	// Save session if recorder available
	if b.recorder != nil {
		if err := b.recorder.SaveSession(ctx, b.session); err != nil {
			return nil, fmt.Errorf("save session: %w", err)
		}
	}

	session := b.session
	b.session = nil
	return session, nil
}

// recordEvent adds an event to the session.
func (b *Browser) recordEvent(event Event) {
	if b.session == nil {
		return
	}

	b.session.Events = append(b.session.Events, event)

	// Async persist if recorder available
	if b.recorder != nil {
		select {
		case b.events <- event:
		default:
			// Buffer full, skip
		}
	}
}

// observeEvents listens for DOM events in observer mode.
func (b *Browser) observeEvents(ctx context.Context, page *rod.Page) {
	// Enable CDP domains for event capture
	_ = proto.DOMEnable{}.Call(page)
	_ = proto.NetworkEnable{}.Call(page)
	_ = proto.PageEnable{}.Call(page)

	// Listen for navigation
	go page.EachEvent(func(e *proto.PageFrameNavigated) {
		b.mu.Lock()
		b.recordEvent(Event{
			Type:      "navigate",
			Timestamp: time.Now(),
			URL:       e.Frame.URL,
		})
		b.mu.Unlock()
	})()

	// Listen for clicks via CDP
	go page.EachEvent(func(e *proto.DOMChildNodeInserted) {
		// DOM mutations - could track dynamically added elements
	})()

	// Process events for persistence
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case event := <-b.events:
				if b.recorder != nil && b.session != nil {
					_ = b.recorder.RecordEvent(ctx, b.session.ID, event)
				}
			}
		}
	}()
}

// Page returns the current page for direct manipulation.
func (b *Browser) Page() (*rod.Page, error) {
	pages, err := b.browser.Pages()
	if err != nil {
		return nil, err
	}
	if len(pages) == 0 {
		return nil, fmt.Errorf("no pages open")
	}
	return pages[0], nil
}

// Close shuts down the browser.
func (b *Browser) Close() error {
	if b.browser != nil {
		return b.browser.Close()
	}
	return nil
}

// Mode returns the browser mode.
func (b *Browser) Mode() Mode {
	return b.mode
}

// Session returns the current session.
func (b *Browser) Session() *Session {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.session
}
