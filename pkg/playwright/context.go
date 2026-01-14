package playwright

import (
	"github.com/playwright-community/playwright-go"
)

// Context wraps playwright browser context
type Context struct {
	ctx  playwright.BrowserContext
	opts *ContextOptions
}

// NewPage creates a new page in this context
func (c *Context) NewPage() (*Page, error) {
	page, err := c.ctx.NewPage()
	if err != nil {
		return nil, err
	}

	p := &Page{
		page: page,
		ctx:  c,
	}

	// Apply anti-detection if enabled
	if c.opts.AntiDetect {
		if err := p.InjectAntiDetect(); err != nil {
			page.Close()
			return nil, err
		}
	}

	return p, nil
}

// Close closes the context
func (c *Context) Close() error {
	return c.ctx.Close()
}

// Raw returns the underlying playwright.BrowserContext
func (c *Context) Raw() playwright.BrowserContext {
	return c.ctx
}

// Cookies returns all cookies
func (c *Context) Cookies(urls ...string) ([]playwright.Cookie, error) {
	return c.ctx.Cookies(urls...)
}

// AddCookies adds cookies to context
func (c *Context) AddCookies(cookies []playwright.OptionalCookie) error {
	return c.ctx.AddCookies(cookies)
}

// ClearCookies clears all cookies
func (c *Context) ClearCookies() error {
	return c.ctx.ClearCookies()
}

// StorageState returns the storage state for this context
// This includes cookies, localStorage, and sessionStorage
func (c *Context) StorageState(path ...string) (*playwright.StorageState, error) {
	if len(path) > 0 && path[0] != "" {
		return c.ctx.StorageState(path[0])
	}
	return c.ctx.StorageState()
}
