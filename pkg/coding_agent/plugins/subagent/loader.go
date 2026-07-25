package subagent

import (
	"os"
	"path/filepath"
	"strings"
)

// Loader discovers and holds subagent definitions from global and project directories.
type Loader struct {
	definitions map[string]*SubagentDefinition
}

// NewLoader creates an empty Loader.
func NewLoader() *Loader {
	return &Loader{definitions: make(map[string]*SubagentDefinition)}
}

// Discover loads subagent definitions from, in increasing priority:
//   - ~/.claude/agents/           (global, "user" source — Claude Code layout)
//   - {agentDir}/agents/          (global, "user" source)
//   - {cwd}/.claude/agents/       (project, "project" source — Claude Code layout)
//   - {cwd}/.coding_agent/agents/ (project, "project" source)
//
// The .claude paths are scanned so a repo already carrying Claude Code agent
// profiles works without moving files; a same-named profile in modu's own
// directory wins because later loads overwrite earlier ones.
//
// Missing directories are silently skipped.
func (l *Loader) Discover(agentDir, cwd string) {
	if home, err := os.UserHomeDir(); err == nil {
		l.loadFromDir(filepath.Join(home, ".claude", "agents"), "user")
	}
	l.loadFromDir(filepath.Join(agentDir, "agents"), "user")
	l.loadFromDir(filepath.Join(cwd, ".claude", "agents"), "project")
	l.loadFromDir(filepath.Join(cwd, ".coding_agent", "agents"), "project")
}

// DiscoverExtra loads subagent definitions from explicit agent directories.
// Later directories override earlier ones.
func (l *Loader) DiscoverExtra(dirs ...string) {
	for _, dir := range dirs {
		if strings.TrimSpace(dir) == "" {
			continue
		}
		l.loadFromDir(dir, "extra")
	}
}

func (l *Loader) loadFromDir(dir, source string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return // directory absent or unreadable — skip silently
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		def, err := ParseDefinition(filepath.Join(dir, entry.Name()), source)
		if err != nil {
			continue
		}
		// Later source (project) overwrites earlier (global).
		l.definitions[def.Name] = def
	}
}

// Get returns the definition for the given name, or (nil, false) if not found.
func (l *Loader) Get(name string) (*SubagentDefinition, bool) {
	def, ok := l.definitions[name]
	return def, ok
}

// List returns all discovered definitions.
func (l *Loader) List() []*SubagentDefinition {
	result := make([]*SubagentDefinition, 0, len(l.definitions))
	for _, def := range l.definitions {
		result = append(result, def)
	}
	return result
}

// Count returns the number of discovered definitions.
func (l *Loader) Count() int {
	return len(l.definitions)
}

// Reset clears all loaded definitions so callers can re-run Discover after a
// mutation (create/update/delete) without leaking the prior state.
func (l *Loader) Reset() {
	l.definitions = make(map[string]*SubagentDefinition)
}
