package playwright

import (
	"fmt"

	"github.com/playwright-community/playwright-go"
)

// Browser wraps playwright browser with convenient methods
type Browser struct {
	pw      *playwright.Playwright
	browser playwright.Browser
	opts    *Options
}

// New creates a new Browser instance
func New(opts ...Option) (*Browser, error) {
	o := DefaultOptions()
	for _, opt := range opts {
		opt(o)
	}

	pw, err := playwright.Run()
	if err != nil {
		return nil, fmt.Errorf("failed to start playwright: %w", err)
	}

	launchOpts := playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(o.Headless),
		Args:     o.Args,
	}

	if o.SlowMo > 0 {
		launchOpts.SlowMo = playwright.Float(o.SlowMo)
	}

	var browser playwright.Browser
	var launchErr error

	switch o.BrowserType {
	case "firefox":
		browser, launchErr = pw.Firefox.Launch(launchOpts)
	case "webkit":
		browser, launchErr = pw.WebKit.Launch(launchOpts)
	default:
		browser, launchErr = pw.Chromium.Launch(launchOpts)
	}

	if launchErr != nil {
		pw.Stop()
		return nil, fmt.Errorf("failed to launch browser: %w", launchErr)
	}

	return &Browser{
		pw:      pw,
		browser: browser,
		opts:    o,
	}, nil
}

// Close closes browser and playwright
func (b *Browser) Close() {
	if b.browser != nil {
		b.browser.Close()
	}
	if b.pw != nil {
		b.pw.Stop()
	}
}

// NewContext creates a new browser context
func (b *Browser) NewContext(opts ...ContextOption) (*Context, error) {
	co := DefaultContextOptions()
	for _, opt := range opts {
		opt(co)
	}

	contextOpts := playwright.BrowserNewContextOptions{
		UserAgent:         playwright.String(co.UserAgent),
		Locale:            playwright.String(co.Locale),
		TimezoneId:        playwright.String(co.Timezone),
		JavaScriptEnabled: playwright.Bool(true),
	}

	if co.ViewportWidth > 0 && co.ViewportHeight > 0 {
		contextOpts.Viewport = &playwright.Size{
			Width:  co.ViewportWidth,
			Height: co.ViewportHeight,
		}
	}

	ctx, err := b.browser.NewContext(contextOpts)
	if err != nil {
		return nil, err
	}

	return &Context{
		ctx:  ctx,
		opts: co,
	}, nil
}

// NewPage creates a new page with default context
func (b *Browser) NewPage(opts ...ContextOption) (*Page, error) {
	ctx, err := b.NewContext(opts...)
	if err != nil {
		return nil, err
	}

	return ctx.NewPage()
}

// Raw returns the underlying playwright.Browser
func (b *Browser) Raw() playwright.Browser {
	return b.browser
}

// Pw returns the underlying playwright instance (needed for persistent context)
func (b *Browser) Pw() *playwright.Playwright {
	return b.pw
}

// LaunchPersistentContext launches a browser with persistent user data directory
// This is similar to Python's launch_persistent_context and maintains login state
func LaunchPersistentContext(userDataDir string, opts ...Option) (*PersistentContext, error) {
	o := DefaultOptions()
	for _, opt := range opts {
		opt(o)
	}

	pw, err := playwright.Run()
	if err != nil {
		return nil, fmt.Errorf("failed to start playwright: %w", err)
	}

	launchOpts := playwright.BrowserTypeLaunchPersistentContextOptions{
		Headless: playwright.Bool(o.Headless),
		Args: append(o.Args,
			"--disable-blink-features=AutomationControlled",
			"--password-store=basic",
		),
		IgnoreDefaultArgs: []string{"--enable-automation"},
	}

	if o.SlowMo > 0 {
		launchOpts.SlowMo = playwright.Float(o.SlowMo)
	}

	var ctx playwright.BrowserContext
	var launchErr error

	switch o.BrowserType {
	case "firefox":
		ctx, launchErr = pw.Firefox.LaunchPersistentContext(userDataDir, launchOpts)
	case "webkit":
		ctx, launchErr = pw.WebKit.LaunchPersistentContext(userDataDir, launchOpts)
	default:
		ctx, launchErr = pw.Chromium.LaunchPersistentContext(userDataDir, launchOpts)
	}

	if launchErr != nil {
		pw.Stop()
		return nil, fmt.Errorf("failed to launch persistent context: %w", launchErr)
	}

	return &PersistentContext{
		pw:  pw,
		ctx: ctx,
	}, nil
}

// PersistentContext wraps a persistent browser context
type PersistentContext struct {
	pw  *playwright.Playwright
	ctx playwright.BrowserContext
}

// Close closes the context and playwright
func (p *PersistentContext) Close() {
	if p.ctx != nil {
		p.ctx.Close()
	}
	if p.pw != nil {
		p.pw.Stop()
	}
}

// Pages returns all pages in the context
func (p *PersistentContext) Pages() []playwright.Page {
	return p.ctx.Pages()
}

// NewPage creates a new page
func (p *PersistentContext) NewPage() (*Page, error) {
	page, err := p.ctx.NewPage()
	if err != nil {
		return nil, err
	}
	return &Page{
		page: page,
		ctx:  &Context{ctx: p.ctx},
	}, nil
}

// StorageState saves the storage state to a file
func (p *PersistentContext) StorageState(path string) error {
	if path != "" {
		_, err := p.ctx.StorageState(path)
		return err
	}
	_, err := p.ctx.StorageState()
	return err
}

// Raw returns the underlying BrowserContext
func (p *PersistentContext) Raw() playwright.BrowserContext {
	return p.ctx
}
