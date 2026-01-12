package playwright

import (
	"fmt"
	"time"

	"github.com/playwright-community/playwright-go"
)

// Anti-detection script
const antiDetectScript = `
Object.defineProperty(navigator, 'webdriver', {
	get: () => undefined
});
Object.defineProperty(navigator, 'platform', {
	get: () => 'MacIntel'
});
Object.defineProperty(navigator, 'plugins', {
	get: () => [1, 2, 3, 4, 5]
});
Object.defineProperty(navigator, 'languages', {
	get: () => ['en-US', 'en']
});
`

// Page wraps playwright page with convenient methods
type Page struct {
	page playwright.Page
	ctx  *Context
}

// InjectAntiDetect injects anti-detection scripts
func (p *Page) InjectAntiDetect() error {
	return p.page.AddInitScript(playwright.Script{
		Content: playwright.String(antiDetectScript),
	})
}

// Goto navigates to URL
func (p *Page) Goto(url string, opts ...GotoOption) error {
	o := &GotoOptions{
		WaitUntil: "domcontentloaded",
		Timeout:   30000,
	}
	for _, opt := range opts {
		opt(o)
	}

	var waitUntil *playwright.WaitUntilState
	switch o.WaitUntil {
	case "load":
		waitUntil = playwright.WaitUntilStateLoad
	case "networkidle":
		waitUntil = playwright.WaitUntilStateNetworkidle
	case "commit":
		waitUntil = playwright.WaitUntilStateCommit
	default:
		waitUntil = playwright.WaitUntilStateDomcontentloaded
	}

	_, err := p.page.Goto(url, playwright.PageGotoOptions{
		WaitUntil: waitUntil,
		Timeout:   playwright.Float(o.Timeout),
	})
	return err
}

// GotoOptions for page navigation
type GotoOptions struct {
	WaitUntil string  // load, domcontentloaded, networkidle, commit
	Timeout   float64 // milliseconds
}

// GotoOption configures GotoOptions
type GotoOption func(*GotoOptions)

// WithWaitUntil sets wait until state
func WithWaitUntil(state string) GotoOption {
	return func(o *GotoOptions) {
		o.WaitUntil = state
	}
}

// WithTimeout sets navigation timeout
func WithTimeout(ms float64) GotoOption {
	return func(o *GotoOptions) {
		o.Timeout = ms
	}
}

// WaitForSelector waits for selector to appear
func (p *Page) WaitForSelector(selector string, timeout ...float64) (playwright.ElementHandle, error) {
	t := 10000.0
	if len(timeout) > 0 {
		t = timeout[0]
	}
	return p.page.WaitForSelector(selector, playwright.PageWaitForSelectorOptions{
		Timeout: playwright.Float(t),
	})
}

// WaitForTimeout waits for specified milliseconds
func (p *Page) WaitForTimeout(ms float64) {
	p.page.WaitForTimeout(ms)
}

// Wait waits for specified duration
func (p *Page) Wait(d time.Duration) {
	p.page.WaitForTimeout(float64(d.Milliseconds()))
}

// QuerySelector finds single element
func (p *Page) QuerySelector(selector string) (playwright.ElementHandle, error) {
	return p.page.QuerySelector(selector)
}

// QuerySelectorAll finds all matching elements
func (p *Page) QuerySelectorAll(selector string) ([]playwright.ElementHandle, error) {
	return p.page.QuerySelectorAll(selector)
}

// Content returns page HTML content
func (p *Page) Content() (string, error) {
	return p.page.Content()
}

// Title returns page title
func (p *Page) Title() (string, error) {
	return p.page.Title()
}

// URL returns current URL
func (p *Page) URL() string {
	return p.page.URL()
}

// Evaluate executes JavaScript and returns result
func (p *Page) Evaluate(expression string, args ...interface{}) (interface{}, error) {
	if len(args) > 0 {
		return p.page.Evaluate(expression, args[0])
	}
	return p.page.Evaluate(expression, nil)
}

// Click clicks on element
func (p *Page) Click(selector string) error {
	return p.page.Click(selector)
}

// Fill fills input field
func (p *Page) Fill(selector, value string) error {
	return p.page.Fill(selector, value)
}

// Type types text with delay
func (p *Page) Type(selector, text string, delay ...float64) error {
	d := 0.0
	if len(delay) > 0 {
		d = delay[0]
	}
	return p.page.Type(selector, text, playwright.PageTypeOptions{
		Delay: playwright.Float(d),
	})
}

// Press presses a key
func (p *Page) Press(selector, key string) error {
	return p.page.Press(selector, key)
}

// Screenshot takes a screenshot
func (p *Page) Screenshot(path string, fullPage ...bool) ([]byte, error) {
	fp := false
	if len(fullPage) > 0 {
		fp = fullPage[0]
	}
	return p.page.Screenshot(playwright.PageScreenshotOptions{
		Path:     playwright.String(path),
		FullPage: playwright.Bool(fp),
	})
}

// PDF generates PDF (only works in headless Chromium)
func (p *Page) PDF(path string) ([]byte, error) {
	return p.page.PDF(playwright.PagePdfOptions{
		Path: playwright.String(path),
	})
}

// Scroll scrolls the page
func (p *Page) Scroll(x, y int) error {
	_, err := p.page.Evaluate(fmt.Sprintf("window.scrollBy(%d, %d)", x, y))
	return err
}

// ScrollToBottom scrolls to page bottom
func (p *Page) ScrollToBottom() error {
	_, err := p.page.Evaluate("window.scrollTo(0, document.body.scrollHeight)")
	return err
}

// ScrollToTop scrolls to page top
func (p *Page) ScrollToTop() error {
	_, err := p.page.Evaluate("window.scrollTo(0, 0)")
	return err
}

// Close closes the page
func (p *Page) Close() error {
	return p.page.Close()
}

// Raw returns the underlying playwright.Page
func (p *Page) Raw() playwright.Page {
	return p.page
}

// Context returns the page's context
func (p *Page) Context() *Context {
	return p.ctx
}

// GetAttribute gets element attribute using Locator-based API
func (p *Page) GetAttribute(selector, name string) (string, error) {
	locator := p.page.Locator(selector).First()
	count, err := locator.Count()
	if err != nil {
		return "", err
	}
	if count == 0 {
		return "", fmt.Errorf("element not found: %s", selector)
	}
	return locator.GetAttribute(name)
}

// InnerText gets element inner text
func (p *Page) InnerText(selector string) (string, error) {
	el, err := p.page.QuerySelector(selector)
	if err != nil {
		return "", err
	}
	if el == nil {
		return "", fmt.Errorf("element not found: %s", selector)
	}
	return el.InnerText()
}

// InnerHTML gets element inner HTML
func (p *Page) InnerHTML(selector string) (string, error) {
	el, err := p.page.QuerySelector(selector)
	if err != nil {
		return "", err
	}
	if el == nil {
		return "", fmt.Errorf("element not found: %s", selector)
	}
	return el.InnerHTML()
}

// IsVisible checks if element is visible
func (p *Page) IsVisible(selector string) (bool, error) {
	return p.page.IsVisible(selector)
}

// Reload reloads the page
func (p *Page) Reload() error {
	_, err := p.page.Reload()
	return err
}

// GoBack navigates back
func (p *Page) GoBack() error {
	_, err := p.page.GoBack()
	return err
}

// GoForward navigates forward
func (p *Page) GoForward() error {
	_, err := p.page.GoForward()
	return err
}
