package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// maxModuTUIFileMentions caps how many paths the "@" popup offers. The popup
// itself only shows a handful at a time; the rest just cost scrolling.
const maxModuTUIFileMentions = 50

var moduTUIFileMentionSkipDirs = map[string]struct{}{
	".git": {}, ".svn": {}, ".hg": {}, ".bzr": {}, ".jj": {}, ".sl": {},
	"node_modules": {}, "vendor": {}, ".venv": {}, "__pycache__": {},
}

// listModuTUIFileMentions resolves an "@query" token into candidate repo
// paths. Inside a git repo it defers to git ls-files, which is both fast on
// large trees and already gitignore-aware (so build output and secrets don't
// show up); elsewhere it falls back to a bounded walk.
func listModuTUIFileMentions(cwd, query string) []string {
	paths, ok := gitTrackedFiles(cwd)
	if !ok {
		paths = walkFilesForMentions(cwd)
	}
	return rankFileMentions(paths, query, maxModuTUIFileMentions)
}

func gitTrackedFiles(cwd string) ([]string, bool) {
	cmd := exec.Command("git", "ls-files", "--cached", "--others", "--exclude-standard")
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err != nil {
		return nil, false
	}
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	paths := make([]string, 0, len(lines))
	for _, line := range lines {
		if line != "" {
			paths = append(paths, line)
		}
	}
	return paths, true
}

func walkFilesForMentions(cwd string) []string {
	var paths []string
	_ = filepath.WalkDir(cwd, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if path != cwd {
				if _, skip := moduTUIFileMentionSkipDirs[name]; skip || strings.HasPrefix(name, ".") {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if rel, relErr := filepath.Rel(cwd, path); relErr == nil {
			paths = append(paths, filepath.ToSlash(rel))
		}
		// A non-git tree has no ignore file to bound the walk, so stop once
		// there are plainly more candidates than the popup can use.
		if len(paths) >= maxModuTUIFileMentions*20 {
			return filepath.SkipAll
		}
		return nil
	})
	return paths
}

// rankFileMentions filters paths by a case-insensitive subsequence match on
// query and orders them so the most likely intent surfaces first: base-name
// matches before mid-path matches, earlier matches before later ones, then
// shorter paths, then alphabetical for a stable order.
func rankFileMentions(paths []string, query string, limit int) []string {
	query = strings.ToLower(strings.TrimSpace(query))
	type scored struct {
		path      string
		baseMatch bool
		index     int
	}
	matches := make([]scored, 0, len(paths))
	for _, path := range paths {
		lower := strings.ToLower(path)
		index := 0
		if query != "" {
			index = strings.Index(lower, query)
			if index < 0 {
				continue
			}
		}
		base := strings.ToLower(filepath.Base(path))
		matches = append(matches, scored{
			path:      path,
			baseMatch: query != "" && strings.Contains(base, query),
			index:     index,
		})
	}
	sort.Slice(matches, func(i, j int) bool {
		a, b := matches[i], matches[j]
		if a.baseMatch != b.baseMatch {
			return a.baseMatch
		}
		if a.index != b.index {
			return a.index < b.index
		}
		if len(a.path) != len(b.path) {
			return len(a.path) < len(b.path)
		}
		return a.path < b.path
	})
	if len(matches) > limit {
		matches = matches[:limit]
	}
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		out = append(out, match.path)
	}
	return out
}
