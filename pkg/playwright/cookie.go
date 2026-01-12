package playwright

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/playwright-community/playwright-go"
)

// CookieStore manages cookie persistence
type CookieStore struct {
	path string
}

// NewCookieStore creates a new cookie store
func NewCookieStore(path string) *CookieStore {
	return &CookieStore{path: path}
}

// Save saves cookies from context to file
func (s *CookieStore) Save(ctx *Context) error {
	cookies, err := ctx.Cookies()
	if err != nil {
		return err
	}

	// Ensure directory exists
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cookies, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.path, data, 0644)
}

// Load loads cookies from file into context
func (s *CookieStore) Load(ctx *Context) error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}

	var cookies []playwright.Cookie
	if err := json.Unmarshal(data, &cookies); err != nil {
		return err
	}

	optionalCookies := make([]playwright.OptionalCookie, len(cookies))
	for i, c := range cookies {
		optionalCookies[i] = playwright.OptionalCookie{
			Name:     c.Name,
			Value:    c.Value,
			Domain:   &c.Domain,
			Path:     &c.Path,
			Expires:  &c.Expires,
			HttpOnly: &c.HttpOnly,
			Secure:   &c.Secure,
			SameSite: c.SameSite,
		}
	}

	return ctx.AddCookies(optionalCookies)
}

// Exists checks if cookie file exists
func (s *CookieStore) Exists() bool {
	_, err := os.Stat(s.path)
	return err == nil
}

// Delete removes the cookie file
func (s *CookieStore) Delete() error {
	return os.Remove(s.path)
}

// Path returns the cookie file path
func (s *CookieStore) Path() string {
	return s.path
}

// SaveCookies is a helper to save cookies directly
func SaveCookies(ctx *Context, path string) error {
	return NewCookieStore(path).Save(ctx)
}

// LoadCookies is a helper to load cookies directly
func LoadCookies(ctx *Context, path string) error {
	return NewCookieStore(path).Load(ctx)
}
