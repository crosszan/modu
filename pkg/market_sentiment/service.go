package market_sentiment

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type RawCollector interface {
	Collect(context.Context) (RawSnapshot, error)
}

type SnapshotStore interface {
	Load() ([]Snapshot, error)
	Latest() (Snapshot, bool, error)
	Save(Snapshot) error
}

type Explainer interface {
	Explain(context.Context, Snapshot) (string, error)
}

type Service struct {
	collector RawCollector
	store     SnapshotStore
	explainer Explainer
	now       func() time.Time
	mu        sync.Mutex
}

func NewService(collector RawCollector, store SnapshotStore, explainer Explainer) *Service {
	if explainer == nil {
		explainer = RuleExplainer{}
	}
	return &Service{collector: collector, store: store, explainer: explainer, now: time.Now}
}

func (s *Service) Refresh(ctx context.Context) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.collector == nil || s.store == nil {
		return Snapshot{}, fmt.Errorf("market sentiment: collector and store are required")
	}
	raw, err := s.collector.Collect(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	history, err := s.store.Load()
	if err != nil {
		return Snapshot{}, err
	}
	applyIndustryCache(&raw, history)
	snapshot := CalculateSnapshot(raw)
	if s.now == nil {
		s.now = time.Now
	}
	snapshot.UpdatedAt = s.now()
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].TradeDate < snapshot.TradeDate {
			snapshot.Change = round2(snapshot.Score - history[i].Score)
			break
		}
	}
	analysis, explainErr := s.explainer.Explain(ctx, snapshot)
	if explainErr != nil {
		if snapshot.Errors == nil {
			snapshot.Errors = map[string]string{}
		}
		snapshot.Errors["agent_explanation"] = explainErr.Error()
		analysis, _ = (RuleExplainer{}).Explain(ctx, snapshot)
	}
	snapshot.Analysis = analysis
	if err := s.store.Save(snapshot); err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}

func applyIndustryCache(raw *RawSnapshot, history []Snapshot) {
	if len(raw.Industries) > 0 {
		if raw.IndustryDataDate == "" && !raw.TradeDate.IsZero() {
			raw.IndustryDataDate = raw.TradeDate.Format("2006-01-02")
		}
		return
	}
	for i := len(history) - 1; i >= 0; i-- {
		if len(history[i].Industries) == 0 {
			continue
		}
		raw.Industries = append([]Industry(nil), history[i].Industries...)
		raw.IndustryCached = true
		raw.IndustryDataDate = history[i].IndustryDataDate
		if raw.IndustryDataDate == "" {
			raw.IndustryDataDate = history[i].TradeDate
		}
		if raw.Errors == nil {
			raw.Errors = map[string]string{}
		}
		raw.Errors["eastmoney_industries_cache"] = "实时行业数据不可用，使用 " + raw.IndustryDataDate + " 的本地缓存"
		return
	}
}

func (s *Service) Current() (Snapshot, bool, error) {
	if s.store == nil {
		return Snapshot{}, false, fmt.Errorf("market sentiment: store is required")
	}
	return s.store.Latest()
}

func (s *Service) History() ([]Snapshot, error) {
	if s.store == nil {
		return nil, fmt.Errorf("market sentiment: store is required")
	}
	return s.store.Load()
}
