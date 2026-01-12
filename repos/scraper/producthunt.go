package scraper

import (
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/crosszan/modu/pkg/playwright"
)

// ScrapePH scrapes Product Hunt front page
func ScrapePH(limit int) ([]NewsItem, error) {
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

	if err := page.Goto("https://www.producthunt.com/", playwright.WithTimeout(60000)); err != nil {
		return nil, fmt.Errorf("failed to load PH: %w", err)
	}

	// Wait for content
	page.Wait(2000)

	// Get HTML content and extract from embedded JSON
	html, err := page.Content()
	if err != nil {
		return nil, err
	}

	return extractPHFromHTML(html, limit)
}

// extractPHFromHTML extracts Product Hunt items from HTML content
func extractPHFromHTML(html string, limit int) ([]NewsItem, error) {
	var items []NewsItem

	// Extract the embedded Apollo GraphQL data
	pattern := regexp.MustCompile(`window\[Symbol\.for\("ApolloSSRDataTransport"\)\].*?\.push\((.*?)\);?</script>`)
	matches := pattern.FindStringSubmatch(html)

	if len(matches) < 2 {
		return items, nil
	}

	jsonStr := matches[1]
	// Replace JavaScript undefined with null
	jsonStr = regexp.MustCompile(`\bundefined\b`).ReplaceAllString(jsonStr, "null")

	var data map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		return items, nil
	}

	rehydrate, ok := data["rehydrate"].(map[string]interface{})
	if !ok {
		return items, nil
	}

	for _, value := range rehydrate {
		valueMap, ok := value.(map[string]interface{})
		if !ok {
			continue
		}

		feedData, ok := valueMap["data"].(map[string]interface{})
		if !ok {
			continue
		}

		homefeed, ok := feedData["homefeed"].(map[string]interface{})
		if !ok {
			continue
		}

		edges, ok := homefeed["edges"].([]interface{})
		if !ok {
			continue
		}

		for _, edge := range edges {
			edgeMap, ok := edge.(map[string]interface{})
			if !ok {
				continue
			}

			node, ok := edgeMap["node"].(map[string]interface{})
			if !ok {
				continue
			}

			posts, ok := node["items"].([]interface{})
			if !ok {
				continue
			}

			for _, post := range posts {
				if len(items) >= limit {
					break
				}

				postMap, ok := post.(map[string]interface{})
				if !ok {
					continue
				}

				name, _ := postMap["name"].(string)
				tagline, _ := postMap["tagline"].(string)
				slug, _ := postMap["slug"].(string)

				if name == "" || slug == "" {
					continue
				}

				itemURL := fmt.Sprintf("https://www.producthunt.com/posts/%s", slug)

				var score, comments *int
				if s, ok := postMap["latestScore"].(float64); ok {
					scoreInt := int(s)
					score = &scoreInt
				}
				if c, ok := postMap["commentsCount"].(float64); ok {
					commentsInt := int(c)
					comments = &commentsInt
				}

				items = append(items, NewsItem{
					Title:    name,
					URL:      itemURL,
					Source:   "producthunt",
					Score:    score,
					Comments: comments,
					Tagline:  tagline,
				})
			}

			if len(items) > 0 {
				break
			}
		}

		if len(items) > 0 {
			break
		}
	}

	return items, nil
}
