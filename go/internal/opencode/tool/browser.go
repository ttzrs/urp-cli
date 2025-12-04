package tool

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/joss/urp/internal/opencode/domain"
)

// Browser provides web automation via Chrome DevTools Protocol.
// Uses go-rod for reliable browser control with auto-wait, shadow DOM support,
// and proper cleanup (no zombie processes).
type Browser struct {
	mu      sync.Mutex
	browser *rod.Browser
	pages   map[string]*rod.Page // page ID -> Page
}

func NewBrowser() *Browser {
	return &Browser{
		pages: make(map[string]*rod.Page),
	}
}

func (b *Browser) Info() domain.Tool {
	return domain.Tool{
		Name: "browser",
		Description: `Automate web browser for navigation, interaction, and scraping.
Actions:
  launch    - Start browser (headless by default)
  navigate  - Go to URL, wait for page load
  screenshot - Capture current page as image
  click     - Click element by CSS selector
  type      - Type text into element
  select    - Select dropdown option
  scroll    - Scroll page or element
  evaluate  - Run JavaScript and return result
  content   - Get page HTML or text content
  elements  - Query elements and return info
  close     - Close page or browser

Selectors: Use CSS selectors. For text matching, prefix with "text="
Example: "text=Submit" finds element containing "Submit"`,
		Parameters: domain.JSONSchema{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]any{
					"type": "string",
					"enum": []string{
						"launch", "navigate", "screenshot", "click", "type",
						"select", "scroll", "evaluate", "content", "elements", "close",
					},
					"description": "Browser action to perform",
				},
				"url": map[string]any{
					"type":        "string",
					"description": "URL for navigate action",
				},
				"selector": map[string]any{
					"type":        "string",
					"description": "CSS selector for element actions. Use 'text=...' for text matching",
				},
				"text": map[string]any{
					"type":        "string",
					"description": "Text to type (for type action)",
				},
				"value": map[string]any{
					"type":        "string",
					"description": "Value for select action",
				},
				"script": map[string]any{
					"type":        "string",
					"description": "JavaScript to evaluate",
				},
				"headless": map[string]any{
					"type":        "boolean",
					"description": "Run headless (default: true)",
				},
				"page_id": map[string]any{
					"type":        "string",
					"description": "Page ID for multi-page sessions",
				},
				"direction": map[string]any{
					"type":        "string",
					"enum":        []string{"up", "down", "left", "right"},
					"description": "Scroll direction",
				},
				"amount": map[string]any{
					"type":        "integer",
					"description": "Scroll amount in pixels (default: 300)",
				},
				"full_page": map[string]any{
					"type":        "boolean",
					"description": "Capture full scrollable page (for screenshot)",
				},
				"timeout": map[string]any{
					"type":        "integer",
					"description": "Timeout in seconds (default: 30)",
				},
			},
			"required": []string{"action"},
		},
	}
}

func (b *Browser) Execute(ctx context.Context, args map[string]any) (*Result, error) {
	action, _ := args["action"].(string)
	if action == "" {
		return nil, ErrInvalidArgs
	}

	timeout := 30 * time.Second
	if t, ok := args["timeout"].(float64); ok && t > 0 {
		timeout = time.Duration(t) * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	switch action {
	case "launch":
		return b.launch(ctx, args)
	case "navigate":
		return b.navigate(ctx, args)
	case "screenshot":
		return b.screenshot(ctx, args)
	case "click":
		return b.click(ctx, args)
	case "type":
		return b.typeText(ctx, args)
	case "select":
		return b.selectOption(ctx, args)
	case "scroll":
		return b.scroll(ctx, args)
	case "evaluate":
		return b.evaluate(ctx, args)
	case "content":
		return b.content(ctx, args)
	case "elements":
		return b.elements(ctx, args)
	case "close":
		return b.closeBrowser(ctx, args)
	default:
		return &Result{Output: fmt.Sprintf("Unknown action: %s", action)}, nil
	}
}

// launch starts the browser
func (b *Browser) launch(ctx context.Context, args map[string]any) (*Result, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.browser != nil {
		return &Result{Output: "Browser already running"}, nil
	}

	headless := true
	if h, ok := args["headless"].(bool); ok {
		headless = h
	}

	// Find or download browser
	path, _ := launcher.LookPath()
	l := launcher.New().Bin(path).Headless(headless)

	controlURL, err := l.Launch()
	if err != nil {
		return &Result{
			Output: fmt.Sprintf("Failed to launch browser: %v", err),
			Error:  err,
		}, nil
	}

	browser := rod.New().ControlURL(controlURL)
	if err := browser.Connect(); err != nil {
		return &Result{
			Output: fmt.Sprintf("Failed to connect to browser: %v", err),
			Error:  err,
		}, nil
	}

	b.browser = browser

	mode := "headless"
	if !headless {
		mode = "visible"
	}
	return &Result{
		Title:  "Browser Launched",
		Output: fmt.Sprintf("Browser started (%s mode)", mode),
	}, nil
}

// getOrCreatePage returns existing page or creates new one
func (b *Browser) getOrCreatePage(ctx context.Context, args map[string]any) (*rod.Page, string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Auto-launch if needed
	if b.browser == nil {
		b.mu.Unlock()
		if _, err := b.launch(ctx, map[string]any{"headless": true}); err != nil {
			b.mu.Lock()
			return nil, "", err
		}
		b.mu.Lock()
	}

	pageID, _ := args["page_id"].(string)

	if pageID != "" {
		if page, ok := b.pages[pageID]; ok {
			return page, pageID, nil
		}
	}

	// Create new page
	page, err := b.browser.Page(proto.TargetCreateTarget{})
	if err != nil {
		return nil, "", err
	}

	// Generate simple page ID
	pageID = fmt.Sprintf("page_%d", len(b.pages)+1)
	b.pages[pageID] = page

	return page, pageID, nil
}

// navigate goes to URL
func (b *Browser) navigate(ctx context.Context, args map[string]any) (*Result, error) {
	url, _ := args["url"].(string)
	if url == "" {
		return &Result{Output: "url required for navigate"}, nil
	}

	page, pageID, err := b.getOrCreatePage(ctx, args)
	if err != nil {
		return &Result{Output: fmt.Sprintf("Failed to get page: %v", err), Error: err}, nil
	}

	if err := page.Navigate(url); err != nil {
		return &Result{Output: fmt.Sprintf("Navigation failed: %v", err), Error: err}, nil
	}

	// Wait for page to be stable
	if err := page.WaitStable(time.Second); err != nil {
		// Non-fatal, page may still be usable
	}

	title := page.MustInfo().Title

	return &Result{
		Title:  fmt.Sprintf("Navigated: %s", title),
		Output: fmt.Sprintf("Page loaded: %s\nPage ID: %s", url, pageID),
		Metadata: map[string]any{
			"page_id": pageID,
			"url":     url,
			"title":   title,
		},
	}, nil
}

// screenshot captures page
func (b *Browser) screenshot(ctx context.Context, args map[string]any) (*Result, error) {
	page, pageID, err := b.getOrCreatePage(ctx, args)
	if err != nil {
		return &Result{Output: fmt.Sprintf("Failed to get page: %v", err), Error: err}, nil
	}

	fullPage, _ := args["full_page"].(bool)

	var imgData []byte
	if fullPage {
		imgData, err = page.Screenshot(true, nil)
	} else {
		imgData, err = page.Screenshot(false, nil)
	}

	if err != nil {
		return &Result{Output: fmt.Sprintf("Screenshot failed: %v", err), Error: err}, nil
	}

	encoded := base64.StdEncoding.EncodeToString(imgData)

	return &Result{
		Title:  "Page Screenshot",
		Output: fmt.Sprintf("Screenshot captured (%d bytes, page: %s)", len(imgData), pageID),
		Images: []domain.ImagePart{
			{
				Base64:    encoded,
				MediaType: "image/png",
			},
		},
		Metadata: map[string]any{
			"page_id": pageID,
		},
	}, nil
}

// findElement locates element by selector
func (b *Browser) findElement(page *rod.Page, selector string) (*rod.Element, error) {
	// Support text= prefix for text matching
	if strings.HasPrefix(selector, "text=") {
		text := strings.TrimPrefix(selector, "text=")
		return page.ElementR("*", text)
	}
	return page.Element(selector)
}

// click element
func (b *Browser) click(ctx context.Context, args map[string]any) (*Result, error) {
	selector, _ := args["selector"].(string)
	if selector == "" {
		return &Result{Output: "selector required for click"}, nil
	}

	page, pageID, err := b.getOrCreatePage(ctx, args)
	if err != nil {
		return &Result{Output: fmt.Sprintf("Failed to get page: %v", err), Error: err}, nil
	}

	el, err := b.findElement(page, selector)
	if err != nil {
		return &Result{Output: fmt.Sprintf("Element not found: %s", selector), Error: err}, nil
	}

	if err := el.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return &Result{Output: fmt.Sprintf("Click failed: %v", err), Error: err}, nil
	}

	// Wait for any navigation or DOM changes
	page.WaitStable(500 * time.Millisecond)

	return &Result{
		Title:  "Clicked",
		Output: fmt.Sprintf("Clicked element: %s (page: %s)", selector, pageID),
		Metadata: map[string]any{
			"page_id":  pageID,
			"selector": selector,
		},
	}, nil
}

// typeText into element
func (b *Browser) typeText(ctx context.Context, args map[string]any) (*Result, error) {
	selector, _ := args["selector"].(string)
	text, _ := args["text"].(string)

	if selector == "" {
		return &Result{Output: "selector required"}, nil
	}
	if text == "" {
		return &Result{Output: "text required"}, nil
	}

	page, pageID, err := b.getOrCreatePage(ctx, args)
	if err != nil {
		return &Result{Output: fmt.Sprintf("Failed to get page: %v", err), Error: err}, nil
	}

	el, err := b.findElement(page, selector)
	if err != nil {
		return &Result{Output: fmt.Sprintf("Element not found: %s", selector), Error: err}, nil
	}

	// Clear and type
	if err := el.SelectAllText(); err == nil {
		el.Input("")
	}

	if err := el.Input(text); err != nil {
		return &Result{Output: fmt.Sprintf("Type failed: %v", err), Error: err}, nil
	}

	preview := text
	if len(preview) > 30 {
		preview = preview[:30] + "..."
	}

	return &Result{
		Title:  "Typed",
		Output: fmt.Sprintf("Typed into %s: %q (page: %s)", selector, preview, pageID),
		Metadata: map[string]any{
			"page_id":  pageID,
			"selector": selector,
		},
	}, nil
}

// selectOption from dropdown
func (b *Browser) selectOption(ctx context.Context, args map[string]any) (*Result, error) {
	selector, _ := args["selector"].(string)
	value, _ := args["value"].(string)

	if selector == "" || value == "" {
		return &Result{Output: "selector and value required"}, nil
	}

	page, pageID, err := b.getOrCreatePage(ctx, args)
	if err != nil {
		return &Result{Output: fmt.Sprintf("Failed to get page: %v", err), Error: err}, nil
	}

	el, err := b.findElement(page, selector)
	if err != nil {
		return &Result{Output: fmt.Sprintf("Element not found: %s", selector), Error: err}, nil
	}

	if err := el.Select([]string{value}, true, rod.SelectorTypeText); err != nil {
		return &Result{Output: fmt.Sprintf("Select failed: %v", err), Error: err}, nil
	}

	return &Result{
		Title:  "Selected",
		Output: fmt.Sprintf("Selected %q from %s (page: %s)", value, selector, pageID),
	}, nil
}

// scroll page
func (b *Browser) scroll(ctx context.Context, args map[string]any) (*Result, error) {
	page, pageID, err := b.getOrCreatePage(ctx, args)
	if err != nil {
		return &Result{Output: fmt.Sprintf("Failed to get page: %v", err), Error: err}, nil
	}

	direction, _ := args["direction"].(string)
	if direction == "" {
		direction = "down"
	}

	amount := 300
	if a, ok := args["amount"].(float64); ok && a > 0 {
		amount = int(a)
	}

	var deltaX, deltaY float64
	switch direction {
	case "down":
		deltaY = float64(amount)
	case "up":
		deltaY = -float64(amount)
	case "right":
		deltaX = float64(amount)
	case "left":
		deltaX = -float64(amount)
	}

	// Use mouse wheel scroll
	page.Mouse.Scroll(deltaX, deltaY, 1)

	return &Result{
		Title:  "Scrolled",
		Output: fmt.Sprintf("Scrolled %s by %d pixels (page: %s)", direction, amount, pageID),
	}, nil
}

// evaluate JavaScript
func (b *Browser) evaluate(ctx context.Context, args map[string]any) (*Result, error) {
	script, _ := args["script"].(string)
	if script == "" {
		return &Result{Output: "script required"}, nil
	}

	page, pageID, err := b.getOrCreatePage(ctx, args)
	if err != nil {
		return &Result{Output: fmt.Sprintf("Failed to get page: %v", err), Error: err}, nil
	}

	result, err := page.Eval(script)
	if err != nil {
		return &Result{Output: fmt.Sprintf("Eval failed: %v", err), Error: err}, nil
	}

	output := fmt.Sprintf("%v", result.Value)
	if len(output) > 10000 {
		output = output[:10000] + "\n... (truncated)"
	}

	return &Result{
		Title:  "JavaScript Result",
		Output: output,
		Metadata: map[string]any{
			"page_id": pageID,
		},
	}, nil
}

// content returns page HTML or text
func (b *Browser) content(ctx context.Context, args map[string]any) (*Result, error) {
	page, pageID, err := b.getOrCreatePage(ctx, args)
	if err != nil {
		return &Result{Output: fmt.Sprintf("Failed to get page: %v", err), Error: err}, nil
	}

	html, err := page.HTML()
	if err != nil {
		return &Result{Output: fmt.Sprintf("Failed to get content: %v", err), Error: err}, nil
	}

	// Convert to text
	text := htmlToText(html)
	if len(text) > 50000 {
		text = text[:50000] + "\n... (truncated)"
	}

	info := page.MustInfo()

	return &Result{
		Title:  fmt.Sprintf("Content: %s", info.Title),
		Output: text,
		Metadata: map[string]any{
			"page_id": pageID,
			"url":     info.URL,
			"title":   info.Title,
		},
	}, nil
}

// elements queries and returns element info
func (b *Browser) elements(ctx context.Context, args map[string]any) (*Result, error) {
	selector, _ := args["selector"].(string)
	if selector == "" {
		return &Result{Output: "selector required"}, nil
	}

	page, pageID, err := b.getOrCreatePage(ctx, args)
	if err != nil {
		return &Result{Output: fmt.Sprintf("Failed to get page: %v", err), Error: err}, nil
	}

	els, err := page.Elements(selector)
	if err != nil {
		return &Result{Output: fmt.Sprintf("Query failed: %v", err), Error: err}, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d elements matching '%s':\n\n", len(els), selector))

	for i, el := range els {
		if i >= 20 {
			sb.WriteString(fmt.Sprintf("... and %d more\n", len(els)-20))
			break
		}

		tag, _ := el.Property("tagName")
		text, _ := el.Text()
		if len(text) > 100 {
			text = text[:100] + "..."
		}
		text = strings.TrimSpace(strings.ReplaceAll(text, "\n", " "))

		box := el.MustShape().Box()

		sb.WriteString(fmt.Sprintf("%d. <%v> at (%d,%d) %dx%d",
			i+1, strings.ToLower(tag.Str()),
			int(box.X), int(box.Y), int(box.Width), int(box.Height)))

		if text != "" {
			sb.WriteString(fmt.Sprintf(" - %q", text))
		}
		sb.WriteString("\n")
	}

	return &Result{
		Title:  "Elements",
		Output: sb.String(),
		Metadata: map[string]any{
			"page_id": pageID,
			"count":   len(els),
		},
	}, nil
}

// closeBrowser closes page or entire browser
func (b *Browser) closeBrowser(ctx context.Context, args map[string]any) (*Result, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	pageID, _ := args["page_id"].(string)

	// Close specific page
	if pageID != "" {
		if page, ok := b.pages[pageID]; ok {
			page.Close()
			delete(b.pages, pageID)
			return &Result{Output: fmt.Sprintf("Closed page: %s", pageID)}, nil
		}
		return &Result{Output: fmt.Sprintf("Page not found: %s", pageID)}, nil
	}

	// Close all
	if b.browser != nil {
		for id, page := range b.pages {
			page.Close()
			delete(b.pages, id)
		}
		b.browser.Close()
		b.browser = nil
		return &Result{Output: "Browser closed"}, nil
	}

	return &Result{Output: "No browser running"}, nil
}

// Cleanup ensures browser is closed
func (b *Browser) Cleanup() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.browser != nil {
		for _, page := range b.pages {
			page.Close()
		}
		b.browser.Close()
		b.browser = nil
		b.pages = make(map[string]*rod.Page)
	}
}

var _ Executor = (*Browser)(nil)
