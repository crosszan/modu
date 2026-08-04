// Package trust persists per-directory trust decisions used by the coding
// agent's approval service.
package trust

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type Decision uint8

const (
	Undecided Decision = iota
	Trusted
	Untrusted
)

func (d Decision) String() string {
	switch d {
	case Trusted:
		return "trusted"
	case Untrusted:
		return "untrusted"
	default:
		return "undecided"
	}
}

type Result struct {
	Decision Decision
	Path     string
	Found    bool
	Session  bool
}

type Manager struct {
	path string

	mu      sync.RWMutex
	data    map[string]*bool
	session map[string]bool
}

func New(path string) (*Manager, error) {
	manager := &Manager{
		path:    path,
		data:    make(map[string]*bool),
		session: make(map[string]bool),
	}
	if path == "" {
		return manager, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return manager, nil
		}
		return nil, fmt.Errorf("read trust store: %w", err)
	}
	if len(data) == 0 {
		return manager, nil
	}
	if err := json.Unmarshal(data, &manager.data); err != nil {
		return nil, fmt.Errorf("parse trust store: %w", err)
	}
	if manager.data == nil {
		manager.data = make(map[string]*bool)
		return manager, nil
	}
	normalized := make(map[string]*bool, len(manager.data))
	for dir, decision := range manager.data {
		normalized[canonicalPath(dir)] = decision
	}
	manager.data = normalized
	return manager, nil
}

func (m *Manager) Status(dir string) Result {
	dir = canonicalPath(dir)
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, candidate := range ancestors(dir) {
		if m.session[candidate] {
			return Result{Decision: Trusted, Path: candidate, Found: true, Session: true}
		}
		if value, ok := m.data[candidate]; ok {
			decision := Undecided
			if value != nil && *value {
				decision = Trusted
			} else if value != nil {
				decision = Untrusted
			}
			return Result{Decision: decision, Path: candidate, Found: true}
		}
	}
	return Result{Decision: Undecided}
}

func (m *Manager) IsTrusted(dir string) bool {
	return m.Status(dir).Decision == Trusted
}

func (m *Manager) SetPersistent(dir string, decision Decision) error {
	dir = canonicalPath(dir)
	if dir == "" {
		return fmt.Errorf("trust directory is required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.session, dir)
	switch decision {
	case Trusted:
		value := true
		m.data[dir] = &value
	case Untrusted:
		value := false
		m.data[dir] = &value
	default:
		m.data[dir] = nil
	}
	return m.saveLocked()
}

func (m *Manager) SetSession(dir string) error {
	dir = canonicalPath(dir)
	if dir == "" {
		return fmt.Errorf("trust directory is required")
	}
	m.mu.Lock()
	m.session[dir] = true
	m.mu.Unlock()
	return nil
}

func (m *Manager) saveLocked() error {
	if m.path == "" {
		return fmt.Errorf("trust store path is not configured")
	}
	if err := os.MkdirAll(filepath.Dir(m.path), 0o700); err != nil {
		return fmt.Errorf("create trust store directory: %w", err)
	}
	data, err := json.MarshalIndent(m.data, "", "  ")
	if err != nil {
		return fmt.Errorf("encode trust store: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(m.path), ".trust-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary trust store: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("secure temporary trust store: %w", err)
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return fmt.Errorf("write temporary trust store: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temporary trust store: %w", err)
	}
	if err := os.Rename(tmpPath, m.path); err != nil {
		return fmt.Errorf("replace trust store: %w", err)
	}
	return nil
}

func canonicalPath(path string) string {
	if path == "" {
		return ""
	}
	path = filepath.Clean(path)
	if absolute, err := filepath.Abs(path); err == nil {
		path = absolute
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	return filepath.Clean(path)
}

func ancestors(path string) []string {
	if path == "" {
		return nil
	}
	var result []string
	for {
		result = append(result, path)
		parent := filepath.Dir(path)
		if parent == path {
			return result
		}
		path = parent
	}
}
