package projectdir

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPathAlwaysUsesCurrentLayout(t *testing.T) {
	cwd := t.TempDir()
	// Even with a legacy directory present, a write target is the new layout.
	if err := os.MkdirAll(filepath.Join(cwd, ".coding_agent", "agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(cwd, ".modu", "agents")
	if got := Path(cwd, "agents"); got != want {
		t.Fatalf("Path = %q, want %q", got, want)
	}
}

func TestSearchOrdersLegacyBeforeCurrent(t *testing.T) {
	cwd := t.TempDir()
	dirs := Search(cwd, "workflows")
	if len(dirs) != 3 {
		t.Fatalf("Search returned %d dirs, want 3: %v", len(dirs), dirs)
	}
	// Callers let later entries win, so .modu must come last.
	if got, want := dirs[len(dirs)-1], filepath.Join(cwd, ".modu", "workflows"); got != want {
		t.Fatalf("last search dir = %q, want %q", got, want)
	}
	for _, legacy := range []string{".coding_agent", ".modu_code"} {
		found := false
		for _, dir := range dirs[:len(dirs)-1] {
			if dir == filepath.Join(cwd, legacy, "workflows") {
				found = true
			}
		}
		if !found {
			t.Errorf("Search did not include the %s root: %v", legacy, dirs)
		}
	}
}

func TestSearchPreferredFirstIsSearchReversed(t *testing.T) {
	cwd := t.TempDir()
	preferred := SearchPreferredFirst(cwd, "skills")
	if got, want := preferred[0], filepath.Join(cwd, ".modu", "skills"); got != want {
		t.Fatalf("first preferred dir = %q, want %q", got, want)
	}
	search := Search(cwd, "skills")
	if len(preferred) != len(search) {
		t.Fatalf("preferred has %d dirs, search has %d", len(preferred), len(search))
	}
	for i := range search {
		if preferred[i] != search[len(search)-1-i] {
			t.Fatalf("SearchPreferredFirst is not Search reversed:\n%v\n%v", preferred, search)
		}
	}
}

func TestResolvePrefersCurrentThenLegacy(t *testing.T) {
	t.Run("nothing exists yet", func(t *testing.T) {
		cwd := t.TempDir()
		want := filepath.Join(cwd, ".modu", "memory")
		if got := Resolve(cwd, "memory"); got != want {
			t.Fatalf("Resolve = %q, want the current layout %q", got, want)
		}
	})

	t.Run("only legacy exists", func(t *testing.T) {
		cwd := t.TempDir()
		legacy := filepath.Join(cwd, ".modu_code", "memory")
		if err := os.MkdirAll(legacy, 0o755); err != nil {
			t.Fatal(err)
		}
		if got := Resolve(cwd, "memory"); got != legacy {
			t.Fatalf("Resolve = %q, want the legacy dir %q — an existing checkout must keep its content", got, legacy)
		}
	})

	t.Run("both exist", func(t *testing.T) {
		cwd := t.TempDir()
		for _, root := range []string{".modu_code", ".modu"} {
			if err := os.MkdirAll(filepath.Join(cwd, root, "memory"), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		want := filepath.Join(cwd, ".modu", "memory")
		if got := Resolve(cwd, "memory"); got != want {
			t.Fatalf("Resolve = %q, want %q — the current layout wins once it exists", got, want)
		}
	})

	t.Run("resolves files too", func(t *testing.T) {
		cwd := t.TempDir()
		if err := os.MkdirAll(filepath.Join(cwd, ".coding_agent"), 0o755); err != nil {
			t.Fatal(err)
		}
		legacy := filepath.Join(cwd, ".coding_agent", "settings.json")
		if err := os.WriteFile(legacy, []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
		if got := Resolve(cwd, "settings.json"); got != legacy {
			t.Fatalf("Resolve = %q, want %q", got, legacy)
		}
	})
}
