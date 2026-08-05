package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestRankFileMentions(t *testing.T) {
	paths := []string{
		"cmd/modu_code/main.go",
		"pkg/modu-tui/model.go",
		"pkg/modu-tui/model_test.go",
		"docs/model-notes.md",
		"internal/a/b/c/deeply/nested/model.go",
	}

	t.Run("empty query returns everything shortest-first", func(t *testing.T) {
		got := rankFileMentions(paths, "", 10)
		if len(got) != len(paths) {
			t.Fatalf("got %d paths, want all %d", len(got), len(paths))
		}
		if got[0] != "docs/model-notes.md" {
			t.Fatalf("shortest path should sort first, got %q", got[0])
		}
	})

	t.Run("base-name matches outrank mid-path matches", func(t *testing.T) {
		got := rankFileMentions(paths, "model.go", 10)
		if len(got) == 0 {
			t.Fatal("expected matches")
		}
		if got[0] != "pkg/modu-tui/model.go" {
			t.Fatalf("got[0] = %q, want the shortest base-name match first", got[0])
		}
		for _, path := range got {
			if path == "docs/model-notes.md" {
				t.Fatal("model-notes.md does not contain the query as a substring")
			}
		}
	})

	t.Run("filters out non-matches", func(t *testing.T) {
		got := rankFileMentions(paths, "zzz-nothing", 10)
		if len(got) != 0 {
			t.Fatalf("expected no matches, got %#v", got)
		}
	})

	t.Run("is case insensitive", func(t *testing.T) {
		got := rankFileMentions(paths, "MAIN.GO", 10)
		if len(got) != 1 || got[0] != "cmd/modu_code/main.go" {
			t.Fatalf("case-insensitive match failed, got %#v", got)
		}
	})

	t.Run("respects the limit", func(t *testing.T) {
		got := rankFileMentions(paths, "", 2)
		if len(got) != 2 {
			t.Fatalf("got %d paths, want the limit of 2", len(got))
		}
	})
}

func TestListModuTUIFileMentionsUsesGitAndRespectsIgnore(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init")

	write := func(rel, content string) {
		t.Helper()
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(".gitignore", "ignored.txt\nbuild/\n")
	write("keep.go", "package main")
	write("nested/also_keep.go", "package nested")
	write("ignored.txt", "secret")
	write("build/artifact.bin", "binary")

	got := listModuTUIFileMentions(dir, "")
	found := map[string]bool{}
	for _, path := range got {
		found[path] = true
	}
	for _, want := range []string{"keep.go", "nested/also_keep.go"} {
		if !found[want] {
			t.Fatalf("expected %q in results, got %#v", want, got)
		}
	}
	for _, unwanted := range []string{"ignored.txt", "build/artifact.bin"} {
		if found[unwanted] {
			t.Fatalf("gitignored path %q should not be offered, got %#v", unwanted, got)
		}
	}

	filtered := listModuTUIFileMentions(dir, "also")
	if len(filtered) != 1 || filtered[0] != "nested/also_keep.go" {
		t.Fatalf("query filtering failed, got %#v", filtered)
	}
}

func TestListModuTUIFileMentionsFallsBackOutsideGit(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "node_modules", "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plain.go"), []byte("package main"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "node_modules", "pkg", "dep.js"), []byte("//"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := listModuTUIFileMentions(dir, "")
	found := map[string]bool{}
	for _, path := range got {
		found[path] = true
	}
	if !found["plain.go"] {
		t.Fatalf("expected plain.go from the non-git fallback walk, got %#v", got)
	}
	if found["node_modules/pkg/dep.js"] {
		t.Fatalf("node_modules should be skipped by the fallback walk, got %#v", got)
	}
}
