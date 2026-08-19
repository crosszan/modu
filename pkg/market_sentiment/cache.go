package market_sentiment

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

type FileStore struct {
	path string
	mu   sync.Mutex
}

func NewFileStore(path string) *FileStore { return &FileStore{path: path} }

func (s *FileStore) Load() ([]Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.load()
}

func (s *FileStore) Latest() (Snapshot, bool, error) {
	history, err := s.Load()
	if err != nil || len(history) == 0 {
		return Snapshot{}, false, err
	}
	return history[len(history)-1], true, nil
}

func (s *FileStore) Save(snapshot Snapshot) error {
	if snapshot.TradeDate == "" {
		return errors.New("market sentiment: trade date is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	history, err := s.load()
	if err != nil {
		return err
	}
	updated := false
	for i := range history {
		if history[i].TradeDate == snapshot.TradeDate {
			history[i] = snapshot
			updated = true
			break
		}
	}
	if !updated {
		history = append(history, snapshot)
	}
	sort.Slice(history, func(i, j int) bool { return history[i].TradeDate < history[j].TradeDate })

	data, err := json.MarshalIndent(history, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal market sentiment history: %w", err)
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create market sentiment cache directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".history-*.json")
	if err != nil {
		return fmt.Errorf("create market sentiment cache temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write market sentiment cache: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close market sentiment cache: %w", err)
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		return fmt.Errorf("replace market sentiment cache: %w", err)
	}
	return nil
}

func (s *FileStore) load() ([]Snapshot, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return []Snapshot{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read market sentiment cache: %w", err)
	}
	if len(data) == 0 {
		return []Snapshot{}, nil
	}
	var history []Snapshot
	if err := json.Unmarshal(data, &history); err != nil {
		return nil, fmt.Errorf("decode market sentiment cache: %w", err)
	}
	for i := range history {
		if len(history[i].DataNotices) == 0 && len(history[i].Errors) > 0 {
			history[i].DataNotices = BuildDataNotices(history[i].Errors)
		}
	}
	sort.Slice(history, func(i, j int) bool { return history[i].TradeDate < history[j].TradeDate })
	return history, nil
}
