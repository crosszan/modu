package market_sentiment

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestTencentClientParsesGBKQuoteAndHistory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/quote":
			_, _ = w.Write([]byte(`v_sh000300="1~HS300~000300~3500~3490~3495~~~~~~~~~~~~~~~~~~~~~~~~~10~0.29~3520~3480~x~100~200~1.10~~~~3520~3480~1.15~0~0~0~-1~-1~1.20~0~";`))
		case "/kline":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":0,"data":{"sh000300":{"day":[["2026-08-13","3490","3495","3510","3480","1000"],["2026-08-14","3495","3500","3520","3480","1200"]]}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewTencentClient(server.Client())
	client.QuoteURL = server.URL + "/quote?q=%s"
	client.KlineURL = server.URL + "/kline?param=%s,day,,,%d,qfq"

	quotes, err := client.Quotes(context.Background(), []Security{{Key: IndexHS300, Symbol: "sh000300"}})
	if err != nil {
		t.Fatalf("Quotes: %v", err)
	}
	if len(quotes) != 1 || quotes[0].Key != IndexHS300 || quotes[0].Price != 3500 {
		t.Fatalf("quotes = %#v", quotes)
	}

	bars, err := client.History(context.Background(), Security{Key: IndexHS300, Symbol: "sh000300"}, 30)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(bars) != 2 || bars[1].Close != 3500 || bars[1].Volume != 1200 {
		t.Fatalf("bars = %#v", bars)
	}
}

func TestEastMoneyClientSerializesAndSpacesRequests(t *testing.T) {
	called := make(chan time.Time, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called <- time.Now()
		_, _ = w.Write([]byte(`{"data":{"diff":[{"f14":"行业A","f3":1.2,"f104":10,"f105":5,"f140":"龙头"}]}}`))
	}))
	defer server.Close()

	client := NewEastMoneyClient(server.Client())
	client.IndustryURL = server.URL
	client.MinInterval = 30 * time.Millisecond
	client.Jitter = nil

	if _, err := client.Industries(context.Background()); err != nil {
		t.Fatalf("first Industries: %v", err)
	}
	if _, err := client.Industries(context.Background()); err != nil {
		t.Fatalf("second Industries: %v", err)
	}
	first, second := <-called, <-called
	if elapsed := second.Sub(first); elapsed < 25*time.Millisecond {
		t.Fatalf("request interval = %s, want >=25ms", elapsed)
	}
}

func TestEastMoneyClientFallsBackToDelayHost(t *testing.T) {
	primaryCalls := 0
	fallbackCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/primary":
			primaryCalls++
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
		case "/fallback":
			fallbackCalls++
			_, _ = w.Write([]byte(`{"data":{"diff":[{"f14":"行业A","f3":1.2,"f104":10,"f105":5}]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewEastMoneyClient(server.Client())
	client.IndustryURL = server.URL + "/primary"
	client.IndustryFallbackURL = server.URL + "/fallback"
	client.MinInterval = 0
	client.Jitter = nil

	industries, err := client.Industries(context.Background())
	if err != nil {
		t.Fatalf("Industries: %v", err)
	}
	if len(industries) != 1 || industries[0].Name != "行业A" {
		t.Fatalf("industries = %#v", industries)
	}
	if primaryCalls != 2 || fallbackCalls != 1 {
		t.Fatalf("calls = primary %d, fallback %d; want 2 and 1", primaryCalls, fallbackCalls)
	}
}

func TestTHSClientParsesHotStocks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "2026-08-14") {
			t.Fatalf("path = %q, want trade date", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"errocode":0,"data":[{"code":"600000","name":"A","reason":"算力+芯片","zhangfu":"9.98","huanshou":"5.2","chengjiaoe":"1234"}]}`))
	}))
	defer server.Close()

	client := NewTHSClient(server.Client())
	client.HotURL = server.URL + "/%s"
	got, err := client.HotStocks(context.Background(), "2026-08-14")
	if err != nil {
		t.Fatalf("HotStocks: %v", err)
	}
	if len(got) != 1 || got[0].Code != "600000" || got[0].ChangePct != 9.98 {
		t.Fatalf("hot stocks = %#v", got)
	}
}
