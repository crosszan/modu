// Package projectdir resolves modu's per-project directories.
//
// Project state lives under <cwd>/.modu/, matching the user-level ~/.modu.
// Two older roots are still read so existing checkouts keep working without
// the user moving anything:
//
//   - .coding_agent/ — agents, skills, prompts, packages, workflows, settings
//   - .modu_code/    — memory
//
// New files are always written to .modu/. The legacy roots are read-only
// compatibility, not a second supported location.
package projectdir

import (
	"os"
	"path/filepath"
)

// Root is the current per-project directory name.
const Root = ".modu"

// legacyRoots are read for compatibility, oldest layout first. Both are
// checked for every resource: a checkout may have picked up either name
// depending on which version of modu created it.
var legacyRoots = []string{".coding_agent", ".modu_code"}

// Path returns the current location of a project resource. Use it when
// writing — new files belong in the new layout even when a legacy directory
// still exists.
func Path(cwd, name string) string {
	return filepath.Join(cwd, Root, name)
}

// Search returns every directory a resource may live in, lowest priority
// first. Callers that scan the list in order and let later entries win end
// up preferring .modu while still seeing legacy content.
func Search(cwd, name string) []string {
	dirs := make([]string, 0, len(legacyRoots)+1)
	for _, root := range legacyRoots {
		dirs = append(dirs, filepath.Join(cwd, root, name))
	}
	return append(dirs, Path(cwd, name))
}

// SearchPreferredFirst is Search reversed: .modu first, then the legacy
// roots. Use it with loaders that keep the first registration of a name.
func SearchPreferredFirst(cwd, name string) []string {
	dirs := Search(cwd, name)
	out := make([]string, 0, len(dirs))
	for i := len(dirs) - 1; i >= 0; i-- {
		out = append(out, dirs[i])
	}
	return out
}

// Resolve returns the one path a resource should be read from: the .modu
// location when it exists, otherwise the first legacy location that does,
// otherwise the .modu location so a fresh write lands in the new layout.
// Works for both directories and files.
func Resolve(cwd, name string) string {
	current := Path(cwd, name)
	if exists(current) {
		return current
	}
	for _, root := range legacyRoots {
		if legacy := filepath.Join(cwd, root, name); exists(legacy) {
			return legacy
		}
	}
	return current
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
