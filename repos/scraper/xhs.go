package scraper

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/crosszan/modu/pkg/playwright"
)

// ScrapeXHS scrapes Xiaohongshu (Little Red Book) explore page for trending posts
func ScrapeXHS(limit int) ([]NewsItem, error) {
	browser, err := playwright.New()
	if err != nil {
		return nil, err
	}
	defer browser.Close()

	page, err := browser.NewPage()
	if err != nil {
		return nil, err
	}
	defer page.Close()

	// Navigate to XHS explore page
	if err := page.Goto("https://www.xiaohongshu.com/explore", playwright.WithTimeout(30000)); err != nil {
		return nil, fmt.Errorf("failed to load XHS: %w", err)
	}

	// Wait for page to load and extract data
	var items []NewsItem
	var lastErr string

	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			page.Wait(2 * time.Second)
			page.Reload()
		}

		// Wait for content to load
		page.Wait(3 * time.Second)

		// Try to extract from __INITIAL_STATE__
		html, err := page.Content()
		if err != nil {
			lastErr = fmt.Sprintf("get content failed: %v", err)
			continue
		}

		items, err = extractXHSFromHTML(html, limit)
		if err != nil {
			lastErr = fmt.Sprintf("extract failed: %v", err)
			continue
		}

		if len(items) > 0 {
			return items, nil
		}

		// If no items from __INITIAL_STATE__, try DOM extraction
		items, err = extractXHSFromDOM(page, limit)
		if err != nil {
			lastErr = fmt.Sprintf("DOM extract failed: %v", err)
			continue
		}

		if len(items) > 0 {
			return items, nil
		}

		lastErr = fmt.Sprintf("attempt %d: no posts found", attempt+1)
	}

	return nil, fmt.Errorf("failed after 3 attempts: %s", lastErr)
}

// extractXHSFromHTML extracts posts from window.__INITIAL_STATE__ JSON
func extractXHSFromHTML(html string, limit int) ([]NewsItem, error) {
	var items []NewsItem

	// Find __INITIAL_STATE__ JSON
	pattern := regexp.MustCompile(`window\.__INITIAL_STATE__\s*=\s*(\{.+?\})\s*</script>`)
	matches := pattern.FindStringSubmatch(html)

	if len(matches) < 2 {
		return items, nil
	}

	jsonStr := matches[1]
	// Replace JavaScript undefined with null
	jsonStr = regexp.MustCompile(`:\s*undefined`).ReplaceAllString(jsonStr, `: null`)

	var data map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		// Try to fix common JSON issues
		jsonStr = strings.ReplaceAll(jsonStr, "undefined", "null")
		if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
			return items, nil // Not a fatal error, will try DOM extraction
		}
	}

	// Try to find notes in explore page data
	// Structure: homeFeed.feeds or explore.feeds
	items = extractNotesFromData(data, limit)

	return items, nil
}

// extractNotesFromData recursively searches for note data in the JSON structure
func extractNotesFromData(data map[string]interface{}, limit int) []NewsItem {
	var items []NewsItem

	// Try different possible paths for explore page
	paths := [][]string{
		{"homeFeed", "feeds"},
		{"explore", "feeds"},
		{"feed", "feeds"},
		{"homeFeedData", "data"},
	}

	for _, path := range paths {
		current := interface{}(data)
		found := true

		for _, key := range path {
			if m, ok := current.(map[string]interface{}); ok {
				if val, exists := m[key]; exists {
					current = val
				} else {
					found = false
					break
				}
			} else {
				found = false
				break
			}
		}

		if found {
			if feeds, ok := current.([]interface{}); ok {
				for _, feed := range feeds {
					if len(items) >= limit {
						break
					}

					item := extractNoteItem(feed)
					if item != nil {
						items = append(items, *item)
					}
				}
			}
		}

		if len(items) > 0 {
			break
		}
	}

	// If still no items, try to find noteCard in any structure
	if len(items) == 0 {
		items = searchForNotes(data, limit)
	}

	return items
}

// extractNoteItem extracts a single note item from feed data
func extractNoteItem(feed interface{}) *NewsItem {
	feedMap, ok := feed.(map[string]interface{})
	if !ok {
		return nil
	}

	// Try to get noteCard
	noteCard, ok := feedMap["noteCard"].(map[string]interface{})
	if !ok {
		// Maybe the feed itself is the note
		noteCard = feedMap
	}

	// Extract fields
	title := getStringField(noteCard, "title", "displayTitle", "desc")
	noteID := getStringField(feedMap, "id", "noteId", "note_id")

	if noteID == "" {
		noteID = getStringField(noteCard, "id", "noteId", "note_id")
	}

	if title == "" && noteID == "" {
		return nil
	}

	// Build URL
	url := ""
	if noteID != "" {
		url = fmt.Sprintf("https://www.xiaohongshu.com/explore/%s", noteID)
	}

	// Get user info
	author := ""
	if user, ok := noteCard["user"].(map[string]interface{}); ok {
		author = getStringField(user, "nickname", "nickName", "name")
	}

	// Get interaction counts
	var likes, comments *int
	if interactInfo, ok := noteCard["interactInfo"].(map[string]interface{}); ok {
		if l, ok := interactInfo["likedCount"].(float64); ok {
			likeInt := int(l)
			likes = &likeInt
		}
		if c, ok := interactInfo["commentCount"].(float64); ok {
			commentInt := int(c)
			comments = &commentInt
		}
	}

	// Also try direct fields
	if likes == nil {
		if l, ok := noteCard["likedCount"].(float64); ok {
			likeInt := int(l)
			likes = &likeInt
		}
	}

	return &NewsItem{
		Title:    title,
		URL:      url,
		Source:   "xiaohongshu",
		Author:   author,
		Score:    likes,
		Comments: comments,
	}
}

// searchForNotes recursively searches for notes in nested structures
func searchForNotes(data interface{}, limit int) []NewsItem {
	var items []NewsItem

	switch v := data.(type) {
	case map[string]interface{}:
		// Check if this looks like a note
		if hasNoteFields(v) {
			if item := extractNoteItem(v); item != nil {
				items = append(items, *item)
			}
		}

		// Recursively search
		for _, val := range v {
			if len(items) >= limit {
				break
			}
			found := searchForNotes(val, limit-len(items))
			items = append(items, found...)
		}

	case []interface{}:
		for _, val := range v {
			if len(items) >= limit {
				break
			}
			found := searchForNotes(val, limit-len(items))
			items = append(items, found...)
		}
	}

	return items
}

// hasNoteFields checks if a map looks like a note object
func hasNoteFields(m map[string]interface{}) bool {
	noteFields := []string{"noteCard", "title", "displayTitle", "noteId", "interactInfo"}
	for _, field := range noteFields {
		if _, ok := m[field]; ok {
			return true
		}
	}
	return false
}

// extractXHSFromDOM extracts posts by evaluating JavaScript in the page
func extractXHSFromDOM(page *playwright.Page, limit int) ([]NewsItem, error) {
	var items []NewsItem

	// Use JavaScript to extract note cards from DOM
	result, err := page.Evaluate(`
		(() => {
			const notes = [];
			// Try to find note cards
			const cards = document.querySelectorAll('[class*="note-item"], [class*="feeds-page"] section, a[href*="/explore/"]');

			cards.forEach((card, index) => {
				if (notes.length >= ` + fmt.Sprintf("%d", limit) + `) return;

				// Try to extract info
				const link = card.href || card.querySelector('a')?.href || '';
				const titleEl = card.querySelector('[class*="title"], [class*="desc"], h3, p');
				const title = titleEl?.textContent?.trim() || '';
				const authorEl = card.querySelector('[class*="author"], [class*="nickname"], [class*="name"]');
				const author = authorEl?.textContent?.trim() || '';

				// Extract note ID from URL
				const noteIdMatch = link.match(/\/explore\/([a-zA-Z0-9]+)/);
				const noteId = noteIdMatch ? noteIdMatch[1] : '';

				if (noteId || title) {
					notes.push({
						noteId: noteId,
						title: title || '无标题',
						author: author,
						url: noteId ? 'https://www.xiaohongshu.com/explore/' + noteId : link
					});
				}
			});

			return notes;
		})()
	`)

	if err != nil {
		return nil, err
	}

	// Parse result
	if notes, ok := result.([]interface{}); ok {
		for _, note := range notes {
			if noteMap, ok := note.(map[string]interface{}); ok {
				title, _ := noteMap["title"].(string)
				url, _ := noteMap["url"].(string)
				author, _ := noteMap["author"].(string)

				if title != "" || url != "" {
					items = append(items, NewsItem{
						Title:  title,
						URL:    url,
						Source: "xiaohongshu",
						Author: author,
					})
				}
			}
		}
	}

	return items, nil
}

// getStringField tries to get a string value from multiple possible field names
func getStringField(m map[string]interface{}, fields ...string) string {
	for _, field := range fields {
		if val, ok := m[field].(string); ok && val != "" {
			return val
		}
	}
	return ""
}

// md5Hash calculates MD5 hash of a string
func md5Hash(s string) string {
	h := md5.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}
