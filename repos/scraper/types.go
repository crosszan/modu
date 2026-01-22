package scraper

import (
	"encoding/json"
	"fmt"
	"strings"
)

// NewsItem represents a scraped news item
type NewsItem struct {
	Title     string `json:"title"`
	URL       string `json:"url"`
	Source    string `json:"source"`
	Score     *int   `json:"score,omitempty"`
	Comments  *int   `json:"comments,omitempty"`
	Author    string `json:"author,omitempty"`
	Tagline   string `json:"tagline,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
}

// Selectors defines CSS selectors for newsletter scraping
type Selectors struct {
	Container string
	Title     string
	Link      string
	Date      string
}

// DefaultSelectors returns default CSS selectors
func DefaultSelectors() Selectors {
	return Selectors{
		Container: "article, .post-preview, .post, [class*='post-item'], .newsletter-item",
		Title:     "h1, h2, h3, [class*='title'], .headline",
		Link:      "a",
		Date:      "time, .date, [datetime], [class*='date']",
	}
}

// OutputFormat represents the output format type
type OutputFormat string

const (
	FormatText     OutputFormat = "text"
	FormatMarkdown OutputFormat = "markdown"
	FormatJSON     OutputFormat = "json"
)

// FormatOutput formats scraped items for output
func FormatOutput(items []NewsItem, format OutputFormat) (string, error) {
	switch format {
	case FormatJSON:
		return formatJSON(items)
	case FormatMarkdown:
		return formatMarkdown(items), nil
	default:
		return formatText(items), nil
	}
}

func formatJSON(items []NewsItem) (string, error) {
	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func formatMarkdown(items []NewsItem) string {
	var lines []string
	var currentSource string

	for _, item := range items {
		if item.Source != currentSource {
			if currentSource != "" {
				lines = append(lines, "")
			}
			lines = append(lines, fmt.Sprintf("## %s", strings.ToUpper(item.Source)))
			lines = append(lines, "")
			currentSource = item.Source
		}

		var scoreInfo, commentsInfo string
		if item.Score != nil {
			scoreInfo = fmt.Sprintf(" (%d pts)", *item.Score)
		}
		if item.Comments != nil {
			commentsInfo = fmt.Sprintf(" | %d comments", *item.Comments)
		}

		lines = append(lines, fmt.Sprintf("- [%s](%s)%s%s", item.Title, item.URL, scoreInfo, commentsInfo))

		if item.Tagline != "" {
			lines = append(lines, fmt.Sprintf("  > %s", item.Tagline))
		}
	}

	return strings.Join(lines, "\n")
}

func formatText(items []NewsItem) string {
	var lines []string
	var currentSource string
	idx := 1

	for _, item := range items {
		if item.Source != currentSource {
			if currentSource != "" {
				lines = append(lines, "")
			}
			lines = append(lines, fmt.Sprintf("=== %s ===", strings.ToUpper(item.Source)))
			currentSource = item.Source
		}

		var scoreInfo, commentsInfo string
		if item.Score != nil {
			scoreInfo = fmt.Sprintf(" [%d pts]", *item.Score)
		}
		if item.Comments != nil {
			commentsInfo = fmt.Sprintf(" [%d comments]", *item.Comments)
		}

		lines = append(lines, fmt.Sprintf("%d. %s%s%s", idx, item.Title, scoreInfo, commentsInfo))
		lines = append(lines, fmt.Sprintf("   %s", item.URL))

		if item.Tagline != "" {
			lines = append(lines, fmt.Sprintf("   -> %s", item.Tagline))
		}

		idx++
	}

	return strings.Join(lines, "\n")
}

// intPtr helper to create int pointer
func intPtr(i int) *int {
	return &i
}

// DouyinLiveInfo represents douyin live stream information
type DouyinLiveInfo struct {
	RoomID      string `json:"room_id"`
	URL         string `json:"url"`
	IsLive      bool   `json:"is_live"`
	Title       string `json:"title,omitempty"`
	StreamURL   string `json:"stream_url,omitempty"`
	Streamer    string `json:"streamer,omitempty"`
	ViewerCount *int   `json:"viewer_count,omitempty"`
	Cover       string `json:"cover,omitempty"`
}

// FormatDouyinLiveInfo formats douyin live info for output
func FormatDouyinLiveInfo(info *DouyinLiveInfo, format OutputFormat) (string, error) {
	switch format {
	case FormatJSON:
		return formatDouyinJSON(info)
	case FormatMarkdown:
		return formatDouyinMarkdown(info), nil
	default:
		return formatDouyinText(info), nil
	}
}

func formatDouyinJSON(info *DouyinLiveInfo) (string, error) {
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func formatDouyinMarkdown(info *DouyinLiveInfo) string {
	var lines []string
	lines = append(lines, "## 抖音直播间信息")
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("- **房间ID**: %s", info.RoomID))
	lines = append(lines, fmt.Sprintf("- **主播**: %s", info.Streamer))
	lines = append(lines, fmt.Sprintf("- **标题**: %s", info.Title))
	lines = append(lines, fmt.Sprintf("- **直播状态**: %s", formatLiveStatus(info.IsLive)))
	if info.ViewerCount != nil {
		lines = append(lines, fmt.Sprintf("- **观看人数**: %d", *info.ViewerCount))
	}
	if info.IsLive && info.StreamURL != "" {
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("**流地址**: %s", info.StreamURL))
	}
	return strings.Join(lines, "\n")
}

func formatDouyinText(info *DouyinLiveInfo) string {
	var lines []string
	lines = append(lines, "=== 抖音直播间信息 ===")
	lines = append(lines, fmt.Sprintf("房间ID: %s", info.RoomID))
	lines = append(lines, fmt.Sprintf("主播: %s", info.Streamer))
	lines = append(lines, fmt.Sprintf("标题: %s", info.Title))
	lines = append(lines, fmt.Sprintf("直播状态: %s", formatLiveStatus(info.IsLive)))
	if info.ViewerCount != nil {
		lines = append(lines, fmt.Sprintf("观看人数: %d", *info.ViewerCount))
	}
	if info.IsLive && info.StreamURL != "" {
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("流地址: %s", info.StreamURL))
	}
	return strings.Join(lines, "\n")
}

func formatLiveStatus(isLive bool) string {
	if isLive {
		return "直播中"
	}
	return "未开播"
}
