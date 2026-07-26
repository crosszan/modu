package projectdir

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPathUsesTheModuRoot(t *testing.T) {
	cwd := t.TempDir()
	for _, name := range []string{"agents", "settings.json", "memory"} {
		want := filepath.Join(cwd, ".modu", name)
		if got := Path(cwd, name); got != want {
			t.Errorf("Path(%q) = %q, want %q", name, got, want)
		}
	}
}

// There is one project root. An older layout on disk must not change where
// modu reads or writes — a silent second location is how state ends up split
// across directories in the first place.
func TestPathIgnoresRetiredRoots(t *testing.T) {
	cwd := t.TempDir()
	for _, retired := range []string{".coding_agent", ".modu_code", ".claude"} {
		if err := os.MkdirAll(filepath.Join(cwd, retired, "agents"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	want := filepath.Join(cwd, ".modu", "agents")
	if got := Path(cwd, "agents"); got != want {
		t.Fatalf("Path = %q, want %q", got, want)
	}
}
