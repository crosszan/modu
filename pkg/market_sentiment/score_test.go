package market_sentiment

import (
	"math"
	"testing"
	"time"
)

func TestCalculateSnapshotProducesNineWeightedComponents(t *testing.T) {
	raw := RawSnapshot{
		TradeDate: time.Date(2026, 8, 14, 0, 0, 0, 0, time.Local),
		Quotes: []Quote{
			{Key: IndexShanghai, ChangePct: 0.5, AmplitudePct: 1.2},
			{Key: IndexHS300, ChangePct: 0.8, AmplitudePct: 1.1},
			{Key: IndexCSI1000, ChangePct: 1.8, AmplitudePct: 2.0},
			{Key: IndexChiNext, ChangePct: 1.2, AmplitudePct: 1.8},
			{Key: ETFBond, ChangePct: 0.1, AmplitudePct: 0.1},
		},
		Histories: map[string][]Bar{
			IndexHS300:    risingBars(25, 100, 1),
			IndexCSI1000:  risingBars(25, 100, 2),
			IndexShanghai: volumeBars(25, 100, 1_000_000, 1_800_000),
			ETFBond:       risingBars(25, 100, 0.05),
		},
		Industries: []Industry{
			{Name: "行业A", ChangePct: 2, UpCount: 80, DownCount: 20},
			{Name: "行业B", ChangePct: -1, UpCount: 30, DownCount: 70},
			{Name: "行业C", ChangePct: 0.5, UpCount: 60, DownCount: 40},
		},
		HotStocks: make([]HotStock, 100),
	}

	got := CalculateSnapshot(raw)
	if len(got.Components) != 9 {
		t.Fatalf("components = %d, want 9", len(got.Components))
	}
	var weight float64
	for _, component := range got.Components {
		weight += component.Weight
		if component.Score < 0 || component.Score > 100 {
			t.Fatalf("component %s score = %v, want [0,100]", component.Key, component.Score)
		}
	}
	if math.Abs(weight-1) > 1e-9 {
		t.Fatalf("weight sum = %v, want 1", weight)
	}
	if got.Score < 0 || got.Score > 100 {
		t.Fatalf("score = %v, want [0,100]", got.Score)
	}
	if got.State == "" {
		t.Fatal("state must not be empty")
	}
}

func TestCalculateSnapshotUsesNeutralScoreForMissingSource(t *testing.T) {
	got := CalculateSnapshot(RawSnapshot{TradeDate: time.Date(2026, 8, 14, 0, 0, 0, 0, time.Local)})
	if len(got.Components) != 9 {
		t.Fatalf("components = %d, want 9", len(got.Components))
	}
	for _, component := range got.Components {
		if component.Score != 50 {
			t.Fatalf("component %s score = %v, want neutral 50", component.Key, component.Score)
		}
		if component.Status != StatusMissing {
			t.Fatalf("component %s status = %q, want %q", component.Key, component.Status, StatusMissing)
		}
	}
	if got.Score != 50 || got.State != "中性" {
		t.Fatalf("snapshot = score %v state %q, want 50 中性", got.Score, got.State)
	}
}

func TestClassifyStateBoundaries(t *testing.T) {
	tests := []struct {
		score float64
		want  string
	}{
		{0, "极度恐惧"}, {19.99, "极度恐惧"}, {20, "恐惧"},
		{40, "中性"}, {60, "贪婪"}, {80, "极度贪婪"}, {100, "极度贪婪"},
	}
	for _, tt := range tests {
		if got := ClassifyState(tt.score); got != tt.want {
			t.Errorf("ClassifyState(%v) = %q, want %q", tt.score, got, tt.want)
		}
	}
}

func risingBars(count int, start, step float64) []Bar {
	bars := make([]Bar, count)
	for i := range bars {
		bars[i] = Bar{Close: start + float64(i)*step, Volume: 1_000_000}
	}
	return bars
}

func volumeBars(count int, start, normalVolume, lastVolume float64) []Bar {
	bars := risingBars(count, start, 1)
	for i := range bars {
		bars[i].Volume = normalVolume
	}
	bars[len(bars)-1].Volume = lastVolume
	return bars
}
