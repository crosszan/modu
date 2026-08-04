// Package rewind records pre-mutation file snapshots for turn-level rollback.
package rewind

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const MaxSnapshotBytes = 16 * 1024 * 1024

type fileState struct {
	Path     string
	Existed  bool
	TooLarge bool
	Mode     os.FileMode
	Content  []byte
	Hash     [sha256.Size]byte
}

type RestorePoint struct {
	Number    int
	Time      time.Time
	LeafID    string
	Label     string
	FileCount int
	snapshots []fileSnapshot
}

type fileSnapshot struct {
	before fileState
	after  fileState
}

type Recorder struct {
	mu sync.Mutex

	active  bool
	leafID  string
	label   string
	pending map[string]fileState
	order   []string
	points  []RestorePoint
}

func New() *Recorder {
	return &Recorder{pending: make(map[string]fileState)}
}

func (r *Recorder) Begin(leafID, label string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.active = true
	r.leafID = leafID
	r.label = label
	r.pending = make(map[string]fileState)
	r.order = nil
}

// Record captures a path's state before its first mutation in the active turn.
// It returns true when a new snapshot was added.
func (r *Recorder) Record(path string) bool {
	if r == nil {
		return false
	}
	path = canonicalPath(path)
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.active || path == "" {
		return false
	}
	if _, exists := r.pending[path]; exists {
		return false
	}
	r.pending[path] = capture(path)
	r.order = append(r.order, path)
	return true
}

// Discard removes a just-added snapshot after a mutation failed.
func (r *Recorder) Discard(path string) {
	if r == nil {
		return
	}
	path = canonicalPath(path)
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.pending[path]; !exists {
		return
	}
	delete(r.pending, path)
	for i, item := range r.order {
		if item == path {
			r.order = append(r.order[:i], r.order[i+1:]...)
			break
		}
	}
}

func (r *Recorder) Commit() bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	defer r.clearPendingLocked()
	if !r.active || len(r.order) == 0 {
		return false
	}
	snapshots := make([]fileSnapshot, 0, len(r.order))
	for _, path := range r.order {
		snapshots = append(snapshots, fileSnapshot{
			before: r.pending[path],
			after:  capture(path),
		})
	}
	r.points = append(r.points, RestorePoint{
		Number:    len(r.points) + 1,
		Time:      time.Now().UTC(),
		LeafID:    r.leafID,
		Label:     r.label,
		FileCount: len(snapshots),
		snapshots: snapshots,
	})
	return true
}

func (r *Recorder) Reset() {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.points = nil
	r.clearPendingLocked()
	r.mu.Unlock()
}

func (r *Recorder) Points() []RestorePoint {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]RestorePoint, len(r.points))
	copy(result, r.points)
	return result
}

func (r *Recorder) Restore(index int) (RestorePoint, []string, error) {
	if r == nil {
		return RestorePoint{}, nil, fmt.Errorf("rewind is unavailable")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if index < 0 || index >= len(r.points) {
		return RestorePoint{}, nil, fmt.Errorf("restore point %d out of range (have %d)", index+1, len(r.points))
	}
	target := r.points[index]

	before := make(map[string]fileState)
	expected := make(map[string]fileState)
	var order []string
	for pointIndex := index; pointIndex < len(r.points); pointIndex++ {
		for _, snapshot := range r.points[pointIndex].snapshots {
			path := snapshot.before.Path
			if _, exists := before[path]; !exists {
				before[path] = snapshot.before
				order = append(order, path)
			}
			expected[path] = snapshot.after
		}
	}
	for _, path := range order {
		if before[path].TooLarge {
			return RestorePoint{}, nil, fmt.Errorf("%s is too large to restore safely", path)
		}
		if expected[path].TooLarge {
			return RestorePoint{}, nil, fmt.Errorf("%s current tracked state is too large to verify safely", path)
		}
		current := capture(path)
		if !sameState(current, expected[path]) {
			return RestorePoint{}, nil, fmt.Errorf("%s changed outside tracked write/edit calls; rewind refused", path)
		}
	}

	applied := make([]string, 0, len(order))
	for _, path := range order {
		if err := apply(before[path]); err != nil {
			for i := len(applied) - 1; i >= 0; i-- {
				_ = apply(expected[applied[i]])
			}
			return RestorePoint{}, nil, fmt.Errorf("restore %s: %w", path, err)
		}
		applied = append(applied, path)
	}
	r.points = r.points[:index]
	for i := range r.points {
		r.points[i].Number = i + 1
	}
	return target, applied, nil
}

func (r *Recorder) clearPendingLocked() {
	r.active = false
	r.leafID = ""
	r.label = ""
	r.pending = make(map[string]fileState)
	r.order = nil
}

func capture(path string) fileState {
	state := fileState{Path: canonicalPath(path)}
	info, err := os.Lstat(state.Path)
	if err != nil {
		return state
	}
	state.Existed = true
	state.Mode = info.Mode()
	if !info.Mode().IsRegular() || info.Size() > MaxSnapshotBytes {
		state.TooLarge = true
		return state
	}
	content, err := os.ReadFile(state.Path)
	if err != nil {
		state.TooLarge = true
		return state
	}
	state.Content = content
	state.Hash = sha256.Sum256(content)
	return state
}

func sameState(current, expected fileState) bool {
	if current.Existed != expected.Existed || current.TooLarge != expected.TooLarge {
		return false
	}
	if !current.Existed {
		return true
	}
	if current.TooLarge {
		return current.Mode == expected.Mode
	}
	return current.Mode == expected.Mode && current.Hash == expected.Hash
}

func apply(state fileState) error {
	if state.TooLarge {
		return fmt.Errorf("snapshot content was not retained")
	}
	if !state.Existed {
		if err := os.Remove(state.Path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(state.Path), 0o755); err != nil {
		return err
	}
	mode := state.Mode.Perm()
	if mode == 0 {
		mode = 0o644
	}
	tmp, err := os.CreateTemp(filepath.Dir(state.Path), ".modu-rewind-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if _, err := bytes.NewReader(state.Content).WriteTo(tmp); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, state.Path)
}

func canonicalPath(path string) string {
	if path == "" {
		return ""
	}
	if absolute, err := filepath.Abs(path); err == nil {
		path = absolute
	}
	return filepath.Clean(path)
}
