package market_sentiment

import (
	"strings"
	"testing"
)

func TestBuildDataNoticesCombinesIndustryFailureAndCacheFallback(t *testing.T) {
	notices := BuildDataNotices(map[string]string{
		"eastmoney_industries":       "primary EOF; fallback timeout",
		"eastmoney_industries_cache": "实时行业数据不可用，使用 2026-08-15 的本地缓存",
	})
	if len(notices) != 1 {
		t.Fatalf("notices = %#v, want one combined notice", notices)
	}
	notice := notices[0]
	if notice.Title != "行业实时数据失败，已使用缓存" {
		t.Fatalf("title = %q", notice.Title)
	}
	if !strings.Contains(notice.Impact, "市场广度") || !strings.Contains(notice.Fallback, "2026-08-15") {
		t.Fatalf("notice = %#v", notice)
	}
	if !strings.Contains(notice.Suggestion, "再次刷新") || !strings.Contains(notice.Detail, "primary EOF") {
		t.Fatalf("notice = %#v", notice)
	}
}

func TestBuildDataNoticesExplainsTHSFailure(t *testing.T) {
	notices := BuildDataNotices(map[string]string{"ths_hot_stocks": "HTTP 503"})
	if len(notices) != 1 {
		t.Fatalf("notices = %#v", notices)
	}
	if !strings.Contains(notices[0].Impact, "涨跌停情绪") || !strings.Contains(notices[0].Fallback, "50 分") {
		t.Fatalf("notice = %#v", notices[0])
	}
}
