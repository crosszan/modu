package playwright

// Options for browser launch
type Options struct {
	Headless    bool
	BrowserType string // chromium, firefox, webkit
	SlowMo      float64
	Args        []string
}

// Option is a function that configures Options
type Option func(*Options)

// DefaultOptions returns default browser options
func DefaultOptions() *Options {
	return &Options{
		Headless:    true,
		BrowserType: "chromium",
		Args: []string{
			"--disable-blink-features=AutomationControlled",
			"--disable-dev-shm-usage",
			"--no-sandbox",
			"--disable-setuid-sandbox",
		},
	}
}

// WithHeadless sets headless mode
func WithHeadless(headless bool) Option {
	return func(o *Options) {
		o.Headless = headless
	}
}

// WithBrowserType sets browser type (chromium, firefox, webkit)
func WithBrowserType(browserType string) Option {
	return func(o *Options) {
		o.BrowserType = browserType
	}
}

// WithSlowMo sets slow motion delay in milliseconds
func WithSlowMo(ms float64) Option {
	return func(o *Options) {
		o.SlowMo = ms
	}
}

// WithArgs sets browser launch arguments
func WithArgs(args []string) Option {
	return func(o *Options) {
		o.Args = args
	}
}

// ContextOptions for browser context
type ContextOptions struct {
	UserAgent      string
	ViewportWidth  int
	ViewportHeight int
	Locale         string
	Timezone       string
	AntiDetect     bool
}

// ContextOption is a function that configures ContextOptions
type ContextOption func(*ContextOptions)

// DefaultContextOptions returns default context options
func DefaultContextOptions() *ContextOptions {
	return &ContextOptions{
		UserAgent:      "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		ViewportWidth:  1920,
		ViewportHeight: 1080,
		Locale:         "en-US",
		Timezone:       "America/New_York",
		AntiDetect:     true,
	}
}

// WithUserAgent sets user agent
func WithUserAgent(ua string) ContextOption {
	return func(o *ContextOptions) {
		o.UserAgent = ua
	}
}

// WithViewport sets viewport size
func WithViewport(width, height int) ContextOption {
	return func(o *ContextOptions) {
		o.ViewportWidth = width
		o.ViewportHeight = height
	}
}

// WithLocale sets locale
func WithLocale(locale string) ContextOption {
	return func(o *ContextOptions) {
		o.Locale = locale
	}
}

// WithTimezone sets timezone
func WithTimezone(tz string) ContextOption {
	return func(o *ContextOptions) {
		o.Timezone = tz
	}
}

// WithAntiDetect enables/disables anti-detection scripts
func WithAntiDetect(enabled bool) ContextOption {
	return func(o *ContextOptions) {
		o.AntiDetect = enabled
	}
}
