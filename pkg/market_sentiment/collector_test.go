package market_sentiment

import (
	"context"
	"errors"
	"testing"
)

type fakeTencentSource struct {
	quotes  []Quote
	history map[string][]Bar
	err     error
}

func (f fakeTencentSource) Quotes(context.Context, []Security) ([]Quote, error) {
	return f.quotes, f.err
}

func (f fakeTencentSource) History(_ context.Context, security Security, _ int) ([]Bar, error) {
	return f.history[security.Key], nil
}

type fakeEastMoneySource struct {
	industries []Industry
	dragon     []DragonTigerStock
	news       []NewsItem
	err        error
}

func (f fakeEastMoneySource) Industries(context.Context) ([]Industry, error) {
	return f.industries, f.err
}
func (f fakeEastMoneySource) DragonTiger(context.Context, string) ([]DragonTigerStock, error) {
	return f.dragon, f.err
}
func (f fakeEastMoneySource) GlobalNews(context.Context, int) ([]NewsItem, error) {
	return f.news, f.err
}

type fakeTHSSource struct{ stocks []HotStock }

func (f fakeTHSSource) HotStocks(context.Context, string) ([]HotStock, error) { return f.stocks, nil }

type fakeNorthboundSource struct{ err error }

func (f fakeNorthboundSource) Realtime(context.Context) ([]NorthboundPoint, error) {
	return nil, f.err
}

func TestCollectorUsesLatestQuoteDateAndKeepsPartialErrors(t *testing.T) {
	collector := NewCollector(
		fakeTencentSource{
			quotes:  []Quote{{Key: IndexHS300, TradeTime: "20260814151400", Price: 3500}},
			history: map[string][]Bar{IndexHS300: risingBars(25, 100, 1)},
		},
		fakeEastMoneySource{industries: []Industry{{Name: "行业A", UpCount: 10, DownCount: 5}}},
		fakeTHSSource{stocks: []HotStock{{Code: "600000", ChangePct: 10}}},
		fakeNorthboundSource{err: errors.New("northbound unavailable")},
	)

	raw, err := collector.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if got := raw.TradeDate.Format("2006-01-02"); got != "2026-08-14" {
		t.Fatalf("trade date = %q, want 2026-08-14", got)
	}
	if raw.Errors["northbound"] == "" {
		t.Fatalf("errors = %#v, want northbound error", raw.Errors)
	}
	if len(raw.Industries) != 1 || len(raw.HotStocks) != 1 {
		t.Fatalf("raw = %#v", raw)
	}
}
