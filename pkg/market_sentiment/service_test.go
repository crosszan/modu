package market_sentiment

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeCollector struct{ raw RawSnapshot }

func (f fakeCollector) Collect(context.Context) (RawSnapshot, error) { return f.raw, nil }

type fakeExplainer struct{ text string }

func (f fakeExplainer) Explain(context.Context, Snapshot) (string, error) { return f.text, nil }

func TestServiceRefreshCalculatesChangeExplainsAndPersists(t *testing.T) {
	store := NewFileStore(filepath.Join(t.TempDir(), "history.json"))
	if err := store.Save(Snapshot{TradeDate: "2026-08-13", Score: 40, UpdatedAt: time.Now()}); err != nil {
		t.Fatalf("seed store: %v", err)
	}
	service := NewService(fakeCollector{raw: RawSnapshot{
		TradeDate: time.Date(2026, 8, 14, 0, 0, 0, 0, time.Local),
	}}, store, fakeExplainer{text: "agent analysis"})

	got, err := service.Refresh(context.Background())
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if got.Change != 10 {
		t.Fatalf("change = %v, want 10", got.Change)
	}
	if got.Analysis != "agent analysis" {
		t.Fatalf("analysis = %q", got.Analysis)
	}
	history, err := service.History()
	if err != nil || len(history) != 2 {
		t.Fatalf("history = %#v, err = %v", history, err)
	}
}

func TestServiceRefreshUsesLastIndustryCacheAsProxy(t *testing.T) {
	store := NewFileStore(filepath.Join(t.TempDir(), "history.json"))
	if err := store.Save(Snapshot{
		TradeDate:        "2026-08-13",
		Score:            40,
		UpdatedAt:        time.Now(),
		IndustryDataDate: "2026-08-13",
		Industries:       []Industry{{Name: "行业A", ChangePct: 1.2, UpCount: 10, DownCount: 5}},
	}); err != nil {
		t.Fatalf("seed store: %v", err)
	}
	service := NewService(fakeCollector{raw: RawSnapshot{
		TradeDate: time.Date(2026, 8, 14, 0, 0, 0, 0, time.Local),
		Errors:    map[string]string{"eastmoney_industries": "EOF"},
	}}, store, fakeExplainer{text: "agent analysis"})

	got, err := service.Refresh(context.Background())
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if got.IndustryDataDate != "2026-08-13" || len(got.Industries) != 1 {
		t.Fatalf("industry cache = date %q, rows %#v", got.IndustryDataDate, got.Industries)
	}
	for _, key := range []string{componentBreadth, componentSector} {
		component, ok := findComponent(got.Components, key)
		if !ok {
			t.Fatalf("component %q not found", key)
		}
		if component.Status != StatusProxy || !strings.Contains(component.Message, "2026-08-13") {
			t.Fatalf("component %q = %#v", key, component)
		}
	}
	if got.Errors["eastmoney_industries"] != "EOF" {
		t.Fatalf("errors = %#v", got.Errors)
	}
}

func findComponent(components []Component, key string) (Component, bool) {
	for _, component := range components {
		if component.Key == key {
			return component, true
		}
	}
	return Component{}, false
}
