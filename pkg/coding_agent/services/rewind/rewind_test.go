package rewind

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRestoreReplaysOldestBaselineAndDeletesCreatedFile(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "existing.txt")
	created := filepath.Join(dir, "created.txt")
	if err := os.WriteFile(existing, []byte("v0"), 0o640); err != nil {
		t.Fatal(err)
	}
	recorder := New()
	recorder.Begin("leaf-0", "first turn")
	recorder.Record(existing)
	recorder.Record(created)
	if err := os.WriteFile(existing, []byte("v1"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(created, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !recorder.Commit() {
		t.Fatal("first restore point was not committed")
	}

	recorder.Begin("leaf-1", "second turn")
	recorder.Record(existing)
	if err := os.WriteFile(existing, []byte("v2"), 0o640); err != nil {
		t.Fatal(err)
	}
	recorder.Commit()

	point, restored, err := recorder.Restore(0)
	if err != nil {
		t.Fatal(err)
	}
	if point.LeafID != "leaf-0" || len(restored) != 2 {
		t.Fatalf("restore result point=%#v restored=%#v", point, restored)
	}
	data, err := os.ReadFile(existing)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "v0" {
		t.Fatalf("existing content = %q", data)
	}
	if _, err := os.Stat(created); !os.IsNotExist(err) {
		t.Fatalf("created file still exists: %v", err)
	}
	if len(recorder.Points()) != 0 {
		t.Fatal("restored points were not removed")
	}
}

func TestRestoreRefusesExternalModification(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(path, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	recorder := New()
	recorder.Begin("", "turn")
	recorder.Record(path)
	if err := os.WriteFile(path, []byte("tool change"), 0o644); err != nil {
		t.Fatal(err)
	}
	recorder.Commit()
	if err := os.WriteFile(path, []byte("external change"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, _, err := recorder.Restore(0); err == nil || !strings.Contains(err.Error(), "changed outside") {
		t.Fatalf("restore error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "external change" {
		t.Fatalf("external content was overwritten: %q", data)
	}
}

func TestDiscardRemovesFailedMutationSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file.txt")
	recorder := New()
	recorder.Begin("", "turn")
	if !recorder.Record(path) {
		t.Fatal("record did not add snapshot")
	}
	recorder.Discard(path)
	if recorder.Commit() || len(recorder.Points()) != 0 {
		t.Fatal("discarded snapshot created a restore point")
	}
}
