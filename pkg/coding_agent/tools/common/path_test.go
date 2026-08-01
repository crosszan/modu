package common

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpandPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory available")
	}
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain relative path", "foo/bar", "foo/bar"},
		{"tilde alone", "~", home},
		{"tilde slash path", "~/foo", filepath.Join(home, "foo")},
		{"double-quoted path", `"foo bar"`, "foo bar"},
		{"single-quoted path", `'foo bar'`, "foo bar"},
		{"surrounding whitespace trimmed", "  foo  ", "foo"},
		{"unicode non-breaking space becomes regular space", "foo bar", "foo bar"},
		{"tab is preserved, not collapsed", "foo\tbar", "foo\tbar"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExpandPath(tt.in); got != tt.want {
				t.Errorf("ExpandPath(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestResolveToCwd(t *testing.T) {
	tests := []struct {
		name string
		path string
		cwd  string
		want string
	}{
		{"relative path joins cwd", "foo.txt", "/base/dir", "/base/dir/foo.txt"},
		{"absolute path ignores cwd", "/abs/foo.txt", "/base/dir", "/abs/foo.txt"},
		{"dot segments are cleaned", "../foo.txt", "/base/dir", "/base/foo.txt"},
		{"redundant slashes are cleaned", "a//b", "/base", "/base/a/b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResolveToCwd(tt.path, tt.cwd); got != tt.want {
				t.Errorf("ResolveToCwd(%q, %q) = %q, want %q", tt.path, tt.cwd, got, tt.want)
			}
		})
	}
}

func TestResolveReadPathReturnsDirectHitWithoutNormalization(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "exists.txt")
	if err := os.WriteFile(target, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ResolveReadPath("exists.txt", dir)
	if err != nil {
		t.Fatalf("ResolveReadPath: %v", err)
	}
	if got != target {
		t.Errorf("ResolveReadPath() = %q, want %q", got, target)
	}
}

func TestResolveReadPathReturnsResolvedPathEvenWhenMissing(t *testing.T) {
	dir := t.TempDir()
	got, err := ResolveReadPath("missing.txt", dir)
	if err != nil {
		t.Fatalf("ResolveReadPath should not error for a missing file, it defers to the caller's stat: %v", err)
	}
	want := filepath.Join(dir, "missing.txt")
	if got != want {
		t.Errorf("ResolveReadPath() = %q, want %q", got, want)
	}
}
