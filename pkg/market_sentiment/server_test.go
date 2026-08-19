package market_sentiment

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHTTPHandlerRefreshAndReturnsHistory(t *testing.T) {
	store := NewFileStore(filepath.Join(t.TempDir(), "history.json"))
	service := NewService(fakeCollector{raw: RawSnapshot{
		TradeDate: time.Date(2026, 8, 14, 0, 0, 0, 0, time.Local),
	}}, store, fakeExplainer{text: "analysis"})
	handler := NewHTTPHandler(service)

	request := httptest.NewRequest(http.MethodPost, "/api/refresh", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("refresh status = %d, body = %s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/api/history", nil)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("history status = %d, body = %s", response.Code, response.Body.String())
	}
	var history []Snapshot
	if err := json.Unmarshal(response.Body.Bytes(), &history); err != nil {
		t.Fatalf("decode history: %v", err)
	}
	if len(history) != 1 || history[0].TradeDate != "2026-08-14" {
		t.Fatalf("history = %#v", history)
	}

	request = httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Body.Len() < 100 {
		t.Fatalf("index status = %d, len = %d", response.Code, response.Body.Len())
	}
	if body := response.Body.String(); !strings.Contains(body, "系统处理") || !strings.Contains(body, "技术详情") {
		t.Fatalf("index does not contain actionable data failure hints")
	}
}
