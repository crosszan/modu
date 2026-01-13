package notebooklm

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/crosszan/modu/pkg/playwright"
	"github.com/crosszan/modu/repos/notebooklm/rpc"
)

// Login performs browser-based Google authentication
func Login() error {
	fmt.Fprintln(os.Stderr, "Opening browser for Google login...")
	fmt.Fprintln(os.Stderr, "Please sign in to your Google account.")

	// Create browser in non-headless mode for manual login
	browser, err := playwright.New(
		playwright.WithHeadless(false),
		playwright.WithBrowserType("chromium"),
	)
	if err != nil {
		return fmt.Errorf("failed to create browser: %w", err)
	}
	defer browser.Close()

	// Create page with anti-detection
	page, err := browser.NewPage(
		playwright.WithAntiDetect(true),
		playwright.WithViewport(1280, 800),
	)
	if err != nil {
		return fmt.Errorf("failed to create page: %w", err)
	}
	defer page.Close()

	// Navigate to NotebookLM
	if err := page.Goto(rpc.BaseURL, playwright.WithWaitUntil("networkidle")); err != nil {
		return fmt.Errorf("failed to navigate: %w", err)
	}

	fmt.Fprintln(os.Stderr, "Waiting for login to complete...")
	fmt.Fprintln(os.Stderr, "The browser will close automatically once you're logged in.")

	// Wait for successful login by checking for NotebookLM-specific elements
	// or URL patterns that indicate successful authentication
	maxWait := 5 * time.Minute
	pollInterval := 2 * time.Second
	start := time.Now()

	for time.Since(start) < maxWait {
		// Check current URL
		currentURL := page.URL()

		// If we're on the main NotebookLM page (not accounts.google.com), we're logged in
		if isLoggedInURL(currentURL) {
			// Verify by checking for CSRF token in page source
			content, err := page.Content()
			if err == nil {
				if _, err := ExtractCSRFToken(content); err == nil {
					fmt.Fprintln(os.Stderr, "Login successful!")

					// Save cookies
					if err := saveBrowserCookies(page); err != nil {
						return fmt.Errorf("failed to save cookies: %w", err)
					}

					fmt.Fprintf(os.Stderr, "Credentials saved to %s\n", GetStoragePath())
					return nil
				}
			}
		}

		time.Sleep(pollInterval)
	}

	return fmt.Errorf("login timed out after %v", maxWait)
}

// isLoggedInURL checks if the URL indicates successful login
func isLoggedInURL(url string) bool {
	// Check if we're on the main NotebookLM page
	if len(url) < len(rpc.BaseURL) {
		return false
	}

	// Should be on notebooklm.google.com, not accounts.google.com
	return findSubstring(url, "notebooklm.google.com") >= 0 &&
		findSubstring(url, "accounts.google.com") < 0
}

// saveBrowserCookies extracts and saves cookies from the browser
func saveBrowserCookies(page *playwright.Page) error {
	// Get cookies from the page's browser context
	ctx := page.Context()
	cookies, err := ctx.Cookies()
	if err != nil {
		return fmt.Errorf("failed to get cookies: %w", err)
	}

	// Convert to our format
	var pwCookies []PlaywrightCookie
	for _, c := range cookies {
		pwCookies = append(pwCookies, PlaywrightCookie{
			Name:   c.Name,
			Value:  c.Value,
			Domain: c.Domain,
		})
	}

	return SaveStorageState(pwCookies)
}

// LoginWithExistingCookies tries to use existing cookies, falls back to interactive login
func LoginWithExistingCookies() (*Client, error) {
	// Try to load existing auth
	if StorageExists() {
		client, err := NewClientFromStorage("")
		if err == nil {
			// Verify by refreshing tokens
			if err := client.RefreshTokens(context.Background()); err == nil {
				return client, nil
			}
		}
		fmt.Fprintln(os.Stderr, "Existing session expired, need to re-login")
	}

	// No valid session, need interactive login
	if err := Login(); err != nil {
		return nil, err
	}

	return NewClientFromStorage("")
}
