package market_sentiment

import (
	"path/filepath"
	"testing"
	"time"
)

func TestFileStoreUpsertsByTradeDate(t *testing.T) {
	store := NewFileStore(filepath.Join(t.TempDir(), "history.json"))
	first := Snapshot{TradeDate: "2026-08-14", Score: 40, UpdatedAt: time.Now()}
	second := Snapshot{TradeDate: "2026-08-14", Score: 60, UpdatedAt: time.Now().Add(time.Second)}
	third := Snapshot{TradeDate: "2026-08-13", Score: 50, UpdatedAt: time.Now().Add(-time.Hour)}

	for _, snapshot := range []Snapshot{first, second, third} {
		if err := store.Save(snapshot); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}
	history, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("history len = %d, want 2", len(history))
	}
	if history[0].TradeDate != "2026-08-13" || history[1].Score != 60 {
		t.Fatalf("history = %#v", history)
	}
}

func TestFileStoreEnrichesLegacyErrorsWithDataNotices(t *testing.T) {
	store := NewFileStore(filepath.Join(t.TempDir(), "history.json"))
	if err := store.Save(Snapshot{
		TradeDate: "2026-08-14",
		Errors:    map[string]string{"northbound": "timeout"},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	history, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(history) != 1 || len(history[0].DataNotices) != 1 {
		t.Fatalf("history = %#v", history)
	}
}
