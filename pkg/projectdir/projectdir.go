// Package projectdir names modu's per-project directory.
//
// Project state lives under <cwd>/.modu/, matching the user-level ~/.modu.
// There is one name and no fallbacks: earlier versions scattered project
// state across .coding_agent/ and .modu_code/, and those roots are no longer
// read.
package projectdir

import "path/filepath"

// Root is the per-project directory name.
const Root = ".modu"

// Path returns the location of a project resource, file or directory.
func Path(cwd, name string) string {
	return filepath.Join(cwd, Root, name)
}
