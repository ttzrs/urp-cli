// Package browser - Executor mode for headless automation.
package browser

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/proto"
)

// Executor runs headless browser automations.
type Executor struct {
	browser *Browser
	page    *rod.Page
	timeout time.Duration
}

// NewExecutor creates a headless browser for automation.
func NewExecutor(cfg Config) (*Executor, error) {
	cfg.Mode = ModeExecutor
	b, err := New(cfg)
	if err != nil {
		return nil, err
	}

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &Executor{
		browser: b,
		timeout: timeout,
	}, nil
}

// Navigate goes to a URL and waits for load.
func (e *Executor) Navigate(ctx context.Context, url string) error {
	page, err := e.browser.browser.Page(proto.TargetCreateTarget{URL: url})
	if err != nil {
		return fmt.Errorf("create page: %w", err)
	}
	e.page = page

	if err := page.WaitLoad(); err != nil {
		return fmt.Errorf("wait load: %w", err)
	}

	return nil
}

// Click clicks an element by selector.
func (e *Executor) Click(ctx context.Context, selector string) error {
	if e.page == nil {
		return fmt.Errorf("no page active")
	}

	el, err := e.findElement(selector)
	if err != nil {
		return err
	}

	return el.Click(proto.InputMouseButtonLeft, 1)
}

// Type enters text into an input field.
func (e *Executor) Type(ctx context.Context, selector, text string) error {
	if e.page == nil {
		return fmt.Errorf("no page active")
	}

	el, err := e.findElement(selector)
	if err != nil {
		return err
	}

	// Clear existing content
	if err := el.SelectAllText(); err == nil {
		_ = el.Input("")
	}

	return el.Input(text)
}

// Submit submits a form.
func (e *Executor) Submit(ctx context.Context, selector string) error {
	if e.page == nil {
		return fmt.Errorf("no page active")
	}

	el, err := e.findElement(selector)
	if err != nil {
		return err
	}

	// Find and submit the form
	form := el
	if el.MustProperty("tagName").Str() != "FORM" {
		// Find parent form
		formEl, err := e.page.Element("form:has(" + selector + ")")
		if err != nil {
			return fmt.Errorf("find form: %w", err)
		}
		form = formEl
	}

	_, err = form.Eval(`this.submit()`)
	return err
}

// Select chooses an option from a select element.
func (e *Executor) Select(ctx context.Context, selector string, values ...string) error {
	if e.page == nil {
		return fmt.Errorf("no page active")
	}

	el, err := e.findElement(selector)
	if err != nil {
		return err
	}

	return el.Select(values, true, rod.SelectorTypeText)
}

// WaitVisible waits for an element to become visible.
func (e *Executor) WaitVisible(ctx context.Context, selector string) error {
	if e.page == nil {
		return fmt.Errorf("no page active")
	}

	_, err := e.page.Timeout(e.timeout).Element(selector)
	return err
}

// WaitHidden waits for an element to disappear.
func (e *Executor) WaitHidden(ctx context.Context, selector string) error {
	if e.page == nil {
		return fmt.Errorf("no page active")
	}

	return e.page.Timeout(e.timeout).MustElement(selector).WaitInvisible()
}

// WaitNavigation waits for page navigation to complete.
func (e *Executor) WaitNavigation(ctx context.Context) error {
	if e.page == nil {
		return fmt.Errorf("no page active")
	}

	wait := e.page.WaitNavigation(proto.PageLifecycleEventNameLoad)
	wait()
	return nil
}

// Screenshot takes a screenshot of the page.
func (e *Executor) Screenshot(ctx context.Context, fullPage bool) ([]byte, error) {
	if e.page == nil {
		return nil, fmt.Errorf("no page active")
	}

	quality := 90
	return e.page.Screenshot(fullPage, &proto.PageCaptureScreenshot{
		Format:  proto.PageCaptureScreenshotFormatPng,
		Quality: &quality,
	})
}

// PDF generates a PDF of the page.
func (e *Executor) PDF(ctx context.Context) ([]byte, error) {
	if e.page == nil {
		return nil, fmt.Errorf("no page active")
	}

	reader, err := e.page.PDF(&proto.PagePrintToPDF{
		PrintBackground: true,
	})
	if err != nil {
		return nil, err
	}

	// Read all content from StreamReader
	var result []byte
	buf := make([]byte, 4096)
	for {
		n, readErr := reader.Read(buf)
		if n > 0 {
			result = append(result, buf[:n]...)
		}
		if readErr != nil {
			break
		}
	}
	return result, nil
}

// Eval executes JavaScript and returns the result.
func (e *Executor) Eval(ctx context.Context, script string) (interface{}, error) {
	if e.page == nil {
		return nil, fmt.Errorf("no page active")
	}

	result, err := e.page.Eval(script)
	if err != nil {
		return nil, err
	}

	return result.Value.Val(), nil
}

// GetText extracts text content from an element.
func (e *Executor) GetText(ctx context.Context, selector string) (string, error) {
	if e.page == nil {
		return "", fmt.Errorf("no page active")
	}

	el, err := e.findElement(selector)
	if err != nil {
		return "", err
	}

	return el.Text()
}

// GetAttribute gets an element's attribute value.
func (e *Executor) GetAttribute(ctx context.Context, selector, attr string) (string, error) {
	if e.page == nil {
		return "", fmt.Errorf("no page active")
	}

	el, err := e.findElement(selector)
	if err != nil {
		return "", err
	}

	val, err := el.Attribute(attr)
	if err != nil {
		return "", err
	}
	if val == nil {
		return "", nil
	}
	return *val, nil
}

// GetHTML returns the page's HTML content.
func (e *Executor) GetHTML(ctx context.Context) (string, error) {
	if e.page == nil {
		return "", fmt.Errorf("no page active")
	}

	return e.page.HTML()
}

// GetURL returns the current page URL.
func (e *Executor) GetURL(ctx context.Context) (string, error) {
	if e.page == nil {
		return "", fmt.Errorf("no page active")
	}

	info, err := e.page.Info()
	if err != nil {
		return "", err
	}
	return info.URL, nil
}

// Scroll scrolls the page.
func (e *Executor) Scroll(ctx context.Context, x, y float64) error {
	if e.page == nil {
		return fmt.Errorf("no page active")
	}

	return e.page.Mouse.Scroll(x, y, 1)
}

// Hover moves mouse over an element.
func (e *Executor) Hover(ctx context.Context, selector string) error {
	if e.page == nil {
		return fmt.Errorf("no page active")
	}

	el, err := e.findElement(selector)
	if err != nil {
		return err
	}

	return el.Hover()
}

// Press presses a keyboard key.
func (e *Executor) Press(ctx context.Context, key input.Key) error {
	if e.page == nil {
		return fmt.Errorf("no page active")
	}

	return e.page.Keyboard.Press(key)
}

// findElement locates an element with smart selector handling.
func (e *Executor) findElement(selector string) (*rod.Element, error) {
	// Handle different selector types
	switch {
	case strings.HasPrefix(selector, "//") || strings.HasPrefix(selector, "(//"):
		// XPath
		return e.page.Timeout(e.timeout).ElementX(selector)
	case strings.HasPrefix(selector, "text="):
		// Text content
		text := strings.TrimPrefix(selector, "text=")
		return e.page.Timeout(e.timeout).ElementR("*", text)
	default:
		// CSS selector
		return e.page.Timeout(e.timeout).Element(selector)
	}
}

// ExecuteFlow runs a sequence of actions.
func (e *Executor) ExecuteFlow(ctx context.Context, actions []Action) error {
	for i, action := range actions {
		if err := e.executeAction(ctx, action); err != nil {
			return fmt.Errorf("action %d (%s): %w", i, action.Type, err)
		}

		// Small delay between actions for stability
		if action.Delay > 0 {
			time.Sleep(action.Delay)
		} else {
			time.Sleep(100 * time.Millisecond)
		}
	}
	return nil
}

// Action represents an automation step.
type Action struct {
	Type     string        `json:"type"`     // navigate, click, type, submit, wait, scroll, screenshot
	Selector string        `json:"selector,omitempty"`
	Value    string        `json:"value,omitempty"`
	Delay    time.Duration `json:"delay,omitempty"`
}

// executeAction runs a single action.
func (e *Executor) executeAction(ctx context.Context, action Action) error {
	switch action.Type {
	case "navigate":
		return e.Navigate(ctx, action.Value)
	case "click":
		return e.Click(ctx, action.Selector)
	case "type":
		return e.Type(ctx, action.Selector, action.Value)
	case "submit":
		return e.Submit(ctx, action.Selector)
	case "wait":
		return e.WaitVisible(ctx, action.Selector)
	case "wait_navigation":
		return e.WaitNavigation(ctx)
	case "scroll":
		return e.Scroll(ctx, 0, 500)
	case "hover":
		return e.Hover(ctx, action.Selector)
	default:
		return fmt.Errorf("unknown action type: %s", action.Type)
	}
}

// Close releases browser resources.
func (e *Executor) Close() error {
	return e.browser.Close()
}

// Page returns the underlying rod.Page for advanced operations.
func (e *Executor) Page() *rod.Page {
	return e.page
}
