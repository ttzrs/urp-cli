// Package browser - Observer mode for learning user interactions.
package browser

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// Observer captures user interactions in Chrome Full mode.
type Observer struct {
	browser  *Browser
	page     *rod.Page
	handlers []func(Event)
}

// NewObserver creates an observer with Chrome Full (GUI mode).
func NewObserver(cfg Config) (*Observer, error) {
	cfg.Mode = ModeObserver
	b, err := New(cfg)
	if err != nil {
		return nil, err
	}

	return &Observer{
		browser:  b,
		handlers: make([]func(Event), 0),
	}, nil
}

// OnEvent registers a handler for captured events.
func (o *Observer) OnEvent(handler func(Event)) {
	o.handlers = append(o.handlers, handler)
}

// Start begins observation at the given URL.
func (o *Observer) Start(ctx context.Context, url string) error {
	session, err := o.browser.StartSession(ctx, url)
	if err != nil {
		return err
	}

	o.page, err = o.browser.Page()
	if err != nil {
		return err
	}

	// Inject event listeners into the page
	if err := o.injectListeners(ctx); err != nil {
		return fmt.Errorf("inject listeners: %w", err)
	}

	fmt.Printf("Observer started: session=%s url=%s\n", session.ID, url)
	return nil
}

// injectListeners adds JavaScript event listeners for comprehensive tracking.
func (o *Observer) injectListeners(ctx context.Context) error {
	// JavaScript to capture all user interactions
	script := `
	(function() {
		if (window.__urpObserver) return;
		window.__urpObserver = true;

		const events = [];
		const maxEvents = 1000;

		function recordEvent(type, data) {
			if (events.length >= maxEvents) events.shift();
			events.push({
				type: type,
				timestamp: Date.now(),
				url: window.location.href,
				...data
			});
		}

		// Click events
		document.addEventListener('click', function(e) {
			const target = e.target;
			recordEvent('click', {
				selector: getSelector(target),
				text: target.innerText?.substring(0, 100),
				position: { x: e.clientX, y: e.clientY },
				tagName: target.tagName.toLowerCase()
			});
		}, true);

		// Input events
		document.addEventListener('input', function(e) {
			const target = e.target;
			if (target.type === 'password') return; // Skip passwords
			recordEvent('input', {
				selector: getSelector(target),
				value: target.value?.substring(0, 200),
				inputType: target.type
			});
		}, true);

		// Form submissions
		document.addEventListener('submit', function(e) {
			recordEvent('submit', {
				selector: getSelector(e.target),
				action: e.target.action
			});
		}, true);

		// Scroll events (throttled)
		let scrollTimeout;
		document.addEventListener('scroll', function(e) {
			clearTimeout(scrollTimeout);
			scrollTimeout = setTimeout(function() {
				recordEvent('scroll', {
					scrollX: window.scrollX,
					scrollY: window.scrollY
				});
			}, 100);
		}, true);

		// Helper to generate CSS selector
		function getSelector(el) {
			if (!el || el === document.body) return 'body';

			// Prefer ID
			if (el.id) return '#' + el.id;

			// Try data attributes
			if (el.dataset.testid) return '[data-testid="' + el.dataset.testid + '"]';

			// Build path
			let path = [];
			while (el && el !== document.body) {
				let selector = el.tagName.toLowerCase();
				if (el.className && typeof el.className === 'string') {
					const classes = el.className.split(' ').filter(c => c && !c.includes(':'));
					if (classes.length > 0) {
						selector += '.' + classes.slice(0, 2).join('.');
					}
				}
				path.unshift(selector);
				el = el.parentElement;
				if (path.length > 3) break;
			}
			return path.join(' > ');
		}

		// Expose for extraction
		window.__urpGetEvents = function() {
			const result = events.slice();
			events.length = 0;
			return result;
		};
	})();
	`

	_, err := o.page.Eval(script)
	return err
}

// CollectEvents retrieves recorded events from the page.
func (o *Observer) CollectEvents(ctx context.Context) ([]Event, error) {
	if o.page == nil {
		return nil, fmt.Errorf("no page active")
	}

	result, err := o.page.Eval(`window.__urpGetEvents ? window.__urpGetEvents() : []`)
	if err != nil {
		return nil, err
	}

	var events []Event
	arr := result.Value.Arr()
	for _, item := range arr {
		obj := item.Map()
		event := Event{
			Type:      obj["type"].Str(),
			Timestamp: time.UnixMilli(int64(obj["timestamp"].Num())),
			URL:       obj["url"].Str(),
		}

		if sel, ok := obj["selector"]; ok {
			event.Selector = sel.Str()
		}
		if val, ok := obj["value"]; ok {
			event.Value = val.Str()
		}
		if pos, ok := obj["position"]; ok {
			posMap := pos.Map()
			event.Position = &Position{
				X: posMap["x"].Num(),
				Y: posMap["y"].Num(),
			}
		}

		// Notify handlers
		for _, h := range o.handlers {
			h(event)
		}

		events = append(events, event)
	}

	return events, nil
}

// TakeSnapshot captures current page state.
func (o *Observer) TakeSnapshot(ctx context.Context) (*PageSnapshot, error) {
	if o.page == nil {
		return nil, fmt.Errorf("no page active")
	}

	// Get page info
	info, err := o.page.Info()
	if err != nil {
		return nil, err
	}

	// Get HTML
	html, err := o.page.HTML()
	if err != nil {
		return nil, err
	}

	// Screenshot
	quality := 80
	screenshot, err := o.page.Screenshot(true, &proto.PageCaptureScreenshot{
		Format:  proto.PageCaptureScreenshotFormatPng,
		Quality: &quality,
	})
	if err != nil {
		return nil, err
	}

	return &PageSnapshot{
		URL:        info.URL,
		Title:      info.Title,
		HTML:       html,
		Screenshot: screenshot,
		Timestamp:  time.Now(),
	}, nil
}

// PageSnapshot represents a captured page state.
type PageSnapshot struct {
	URL        string    `json:"url"`
	Title      string    `json:"title"`
	HTML       string    `json:"html"`
	Screenshot []byte    `json:"-"`
	Timestamp  time.Time `json:"timestamp"`
}

// ExtractSelectors finds all interactive elements on the page.
func (o *Observer) ExtractSelectors(ctx context.Context) ([]ElementInfo, error) {
	if o.page == nil {
		return nil, fmt.Errorf("no page active")
	}

	script := `
	(function() {
		const elements = [];
		const interactiveSelectors = 'a, button, input, select, textarea, [onclick], [role="button"]';

		document.querySelectorAll(interactiveSelectors).forEach(function(el) {
			const rect = el.getBoundingClientRect();
			if (rect.width === 0 || rect.height === 0) return;

			elements.push({
				tagName: el.tagName.toLowerCase(),
				type: el.type || '',
				id: el.id,
				name: el.name || '',
				className: el.className || '',
				text: el.innerText?.substring(0, 100) || '',
				placeholder: el.placeholder || '',
				href: el.href || '',
				bounds: { x: rect.x, y: rect.y, width: rect.width, height: rect.height }
			});
		});

		return elements;
	})();
	`

	result, err := o.page.Eval(script)
	if err != nil {
		return nil, err
	}

	var elements []ElementInfo
	for _, item := range result.Value.Arr() {
		obj := item.Map()
		elem := ElementInfo{
			TagName:     obj["tagName"].Str(),
			Type:        obj["type"].Str(),
			ID:          obj["id"].Str(),
			Name:        obj["name"].Str(),
			ClassName:   obj["className"].Str(),
			Text:        strings.TrimSpace(obj["text"].Str()),
			Placeholder: obj["placeholder"].Str(),
			Href:        obj["href"].Str(),
		}
		if bounds, ok := obj["bounds"]; ok {
			b := bounds.Map()
			elem.Bounds = &Bounds{
				X:      b["x"].Num(),
				Y:      b["y"].Num(),
				Width:  b["width"].Num(),
				Height: b["height"].Num(),
			}
		}
		elements = append(elements, elem)
	}

	return elements, nil
}

// ElementInfo describes an interactive element.
type ElementInfo struct {
	TagName     string  `json:"tag_name"`
	Type        string  `json:"type,omitempty"`
	ID          string  `json:"id,omitempty"`
	Name        string  `json:"name,omitempty"`
	ClassName   string  `json:"class_name,omitempty"`
	Text        string  `json:"text,omitempty"`
	Placeholder string  `json:"placeholder,omitempty"`
	Href        string  `json:"href,omitempty"`
	Bounds      *Bounds `json:"bounds,omitempty"`
}

// Bounds represents element position and size.
type Bounds struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// Stop ends observation.
func (o *Observer) Stop(ctx context.Context) (*Session, error) {
	return o.browser.EndSession(ctx)
}

// Close releases resources.
func (o *Observer) Close() error {
	return o.browser.Close()
}
