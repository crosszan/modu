package memory

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Store manages persistent memory for the coding agent at two scopes:
//
//   - Global (~/.modu/memory/): shared across all projects.
//   - Project (<cwd>/memory/):          specific to the current project.
//
// Both scopes store a MEMORY.md for long-term facts and
// daily notes under YYYYMM/YYYYMMDD.md.
//
// The layout is deliberately fixed and flat. Nothing here lets the model
// invent directories, and that is the point rather than an omission: a
// model-maintained folder tree degrades on its own over a long-running
// session — a new folder per almost-synonymous topic, two folders meaning
// the same thing, and later notes filed somewhere their own path no longer
// describes, at which point retrieval by path stops finding them. Smaller
// local models reach that state much sooner than frontier ones, and modu is
// explicitly used with local models.
//
// What a tree actually buys is a narrower search space, and the global vs
// project split already provides that at the level that matters. So if you
// are here to add nested categories, add the guardrails that keep them
// coherent (merging, renaming, re-filing) in the same change — the tree
// alone is the easy half.
type Store struct {
	globalDir  string // e.g. ~/.modu/memory
	projectDir string // e.g. <cwd>/memory

	organizeMu sync.Mutex
	organizing bool
}

var errMemorySearchTruncated = errors.New("memory search truncated")

type Entry struct {
	Path  string
	IsDir bool
}

type SearchMatch struct {
	Path      string
	Line      int
	StartLine int
	Content   string
}

// New creates a Store backed by two directories.
// agentDir is the global config dir (e.g. ~/.modu/);
// cwd is the current project directory.
func New(agentDir, cwd string) *Store {
	globalDir := filepath.Join(agentDir, "memory")
	projectDir := filepath.Join(cwd, ".modu_code", "memory")
	os.MkdirAll(globalDir, 0o755)
	os.MkdirAll(projectDir, 0o755)
	return &Store{
		globalDir:  globalDir,
		projectDir: projectDir,
	}
}

// ── read ──────────────────────────────────────────────────────────────────────

// ReadLongTerm reads the project-scoped MEMORY.md (default scope for tools).
func (ms *Store) ReadLongTerm() string {
	return ms.ReadProjectLongTerm()
}

// ReadProjectLongTerm reads <cwd>/memory/MEMORY.md.
func (ms *Store) ReadProjectLongTerm() string {
	data, err := os.ReadFile(filepath.Join(ms.projectDir, "MEMORY.md"))
	if err != nil {
		return ""
	}
	return string(data)
}

// ReadGlobalLongTerm reads ~/.modu/memory/MEMORY.md.
func (ms *Store) ReadGlobalLongTerm() string {
	data, err := os.ReadFile(filepath.Join(ms.globalDir, "MEMORY.md"))
	if err != nil {
		return ""
	}
	return string(data)
}

func (ms *Store) ReadProjectSummary() string {
	return readString(filepath.Join(ms.projectDir, "memory_summary.md"))
}

func (ms *Store) ReadGlobalSummary() string {
	return readString(filepath.Join(ms.globalDir, "memory_summary.md"))
}

// ── write ─────────────────────────────────────────────────────────────────────

// WriteLongTerm writes to the project-scoped MEMORY.md (default scope for tools).
func (ms *Store) WriteLongTerm(content string) error {
	return ms.WriteProjectLongTerm(content)
}

// WriteProjectLongTerm overwrites <cwd>/memory/MEMORY.md.
func (ms *Store) WriteProjectLongTerm(content string) error {
	return os.WriteFile(filepath.Join(ms.projectDir, "MEMORY.md"), []byte(content), 0o600)
}

// WriteGlobalLongTerm overwrites ~/.modu/memory/MEMORY.md.
func (ms *Store) WriteGlobalLongTerm(content string) error {
	return os.WriteFile(filepath.Join(ms.globalDir, "MEMORY.md"), []byte(content), 0o600)
}

func (ms *Store) WriteProjectSummary(content string) error {
	return os.WriteFile(filepath.Join(ms.projectDir, "memory_summary.md"), []byte(content), 0o600)
}

func (ms *Store) WriteGlobalSummary(content string) error {
	return os.WriteFile(filepath.Join(ms.globalDir, "memory_summary.md"), []byte(content), 0o600)
}

// ── long-term entry editing ───────────────────────────────────────────────────

// ErrMemoryEntryNotFound and ErrMemoryEntryAmbiguous report why an entry could
// not be addressed. Ambiguity is an error rather than a "first match wins"
// because the caller is trying to correct or remove a specific memory, and
// silently picking one of several candidates would corrupt the others.
var (
	ErrMemoryEntryNotFound  = errors.New("no long-term memory entry matches")
	ErrMemoryEntryAmbiguous = errors.New("match is ambiguous")
)

// LongTermEntries returns the long-term memory of a scope split into entries.
// Entries are the blank-line-separated blocks that record_long_term appends,
// so this is the same unit the model wrote in.
func (ms *Store) LongTermEntries(scope string) []string {
	return splitMemoryEntries(ms.readLongTermForScope(scope))
}

// UpdateLongTerm replaces the single entry containing match with content.
//
// Until this existed a memory could only be appended to: a wrong preference or
// a stale path stayed until the next Organize pass happened to merge it away,
// and Organize only promises to drop duplicates and superseded repetition —
// it has no way to know a fact is simply wrong.
func (ms *Store) UpdateLongTerm(scope, match, content string) error {
	return ms.rewriteLongTermEntry(scope, match, content, false)
}

// DeleteLongTerm removes the single entry containing match.
func (ms *Store) DeleteLongTerm(scope, match string) error {
	return ms.rewriteLongTermEntry(scope, match, "", true)
}

func (ms *Store) rewriteLongTermEntry(scope, match, content string, remove bool) error {
	match = strings.TrimSpace(match)
	if match == "" {
		return ErrMemoryEntryNotFound
	}
	entries := splitMemoryEntries(ms.readLongTermForScope(scope))

	var found int
	var index int
	for i, entry := range entries {
		if strings.Contains(entry, match) {
			found++
			index = i
		}
	}
	switch {
	case found == 0:
		return ErrMemoryEntryNotFound
	case found > 1:
		return fmt.Errorf("%w: %d entries contain that text", ErrMemoryEntryAmbiguous, found)
	}

	if remove {
		entries = append(entries[:index], entries[index+1:]...)
	} else {
		entries[index] = strings.TrimSpace(content)
	}
	return ms.writeLongTermForScope(scope, joinMemoryEntries(entries))
}

func (ms *Store) readLongTermForScope(scope string) string {
	if strings.EqualFold(scope, "global") || strings.EqualFold(scope, "user") {
		return ms.ReadGlobalLongTerm()
	}
	return ms.ReadProjectLongTerm()
}

func (ms *Store) writeLongTermForScope(scope, content string) error {
	if strings.EqualFold(scope, "global") || strings.EqualFold(scope, "user") {
		return ms.WriteGlobalLongTerm(content)
	}
	return ms.WriteProjectLongTerm(content)
}

// splitMemoryEntries splits on blank lines, matching how record_long_term
// joins entries. Blank runs of any length are treated as one separator so a
// file a human has edited still splits the way it looks on screen.
func splitMemoryEntries(content string) []string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	var entries []string
	for _, block := range strings.Split(content, "\n\n") {
		if trimmed := strings.TrimSpace(block); trimmed != "" {
			entries = append(entries, trimmed)
		}
	}
	return entries
}

func joinMemoryEntries(entries []string) string {
	return strings.Join(entries, "\n\n")
}

// ── daily notes ───────────────────────────────────────────────────────────────

// AppendToday appends content to today's project-scoped daily note.
func (ms *Store) AppendToday(content string) error {
	return ms.appendTodayToDir(ms.projectDir, content)
}

func (ms *Store) appendTodayToDir(dir, content string) error {
	today := time.Now().Format("20060102")
	monthDir := today[:6]
	filePath := filepath.Join(dir, monthDir, today+".md")

	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return err
	}

	var existing string
	if data, err := os.ReadFile(filePath); err == nil {
		existing = string(data)
	}

	var newContent string
	if existing == "" {
		newContent = fmt.Sprintf("# %s\n\n", time.Now().Format("2006-01-02")) + content
	} else {
		newContent = existing + "\n" + content
	}
	return os.WriteFile(filePath, []byte(newContent), 0o600)
}

// GetRecentDailyNotes returns daily notes from the last N days (project scope).
func (ms *Store) GetRecentDailyNotes(days int) string {
	return ms.recentDailyNotesFromDir(ms.projectDir, days)
}

func (ms *Store) recentDailyNotesFromDir(dir string, days int) string {
	var sb strings.Builder
	first := true
	for i := 0; i < days; i++ {
		date := time.Now().AddDate(0, 0, -i)
		dateStr := date.Format("20060102")
		filePath := filepath.Join(dir, dateStr[:6], dateStr+".md")
		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}
		if !first {
			sb.WriteString("\n\n---\n\n")
		}
		sb.Write(data)
		first = false
	}
	return sb.String()
}

// ── scoped read path ─────────────────────────────────────────────────────────

func (ms *Store) List(scope, relPath string, maxResults int) ([]Entry, bool, error) {
	root := ms.rootForScope(scope)
	target, rel, err := resolveMemoryPath(root, relPath)
	if err != nil {
		return nil, false, err
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		return nil, false, err
	}
	if maxResults <= 0 {
		maxResults = 50
	}
	out := make([]Entry, 0, min(len(entries), maxResults))
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if len(out) >= maxResults {
			return out, true, nil
		}
		path := name
		if rel != "" {
			path = filepath.ToSlash(filepath.Join(rel, name))
		}
		out = append(out, Entry{Path: path, IsDir: entry.IsDir()})
	}
	return out, false, nil
}

func (ms *Store) Read(scope, relPath string, lineOffset, maxLines int) (string, bool, error) {
	root := ms.rootForScope(scope)
	target, _, err := resolveMemoryPath(root, relPath)
	if err != nil {
		return "", false, err
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return "", false, err
	}
	lines := strings.Split(string(data), "\n")
	if lineOffset <= 0 {
		lineOffset = 1
	}
	if lineOffset > len(lines) {
		return "", false, nil
	}
	if maxLines <= 0 {
		maxLines = 120
	}
	start := lineOffset - 1
	end := min(start+maxLines, len(lines))
	truncated := end < len(lines)
	return strings.Join(lines[start:end], "\n"), truncated, nil
}

func (ms *Store) Search(scope, query, relPath string, contextLines, maxResults int) ([]SearchMatch, bool, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, false, fmt.Errorf("query is required")
	}
	root := ms.rootForScope(scope)
	target, _, err := resolveMemoryPath(root, relPath)
	if err != nil {
		return nil, false, err
	}
	if maxResults <= 0 {
		maxResults = 20
	}
	if contextLines < 0 {
		contextLines = 0
	}
	var matches []SearchMatch
	err = filepath.WalkDir(target, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Name() != "." && strings.HasPrefix(entry.Name(), ".") {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			if !strings.Contains(line, query) {
				continue
			}
			if len(matches) >= maxResults {
				return errMemorySearchTruncated
			}
			start := max(0, i-contextLines)
			end := min(len(lines), i+contextLines+1)
			rel, _ := filepath.Rel(root, path)
			matches = append(matches, SearchMatch{
				Path:      filepath.ToSlash(rel),
				Line:      i + 1,
				StartLine: start + 1,
				Content:   strings.Join(lines[start:end], "\n"),
			})
		}
		return nil
	})
	truncated := false
	if err == errMemorySearchTruncated {
		err = nil
		truncated = true
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Path == matches[j].Path {
			return matches[i].Line < matches[j].Line
		}
		return matches[i].Path < matches[j].Path
	})
	return matches, truncated, err
}

// ── context for system prompt ─────────────────────────────────────────────────

const memoryDetailsInstruction = "Detailed memory is available through the `memo` tool using list, read, and search operations."

// GetMemoryContext returns merged memory from both scopes for the system prompt.
// Global memory appears first; project memory is labelled separately.
func (ms *Store) GetMemoryContext() string {
	globalSummary := ms.ReadGlobalSummary()
	projectSummary := ms.ReadProjectSummary()
	if globalSummary != "" || projectSummary != "" {
		var sb strings.Builder
		sb.WriteString("## Memory Summary\n\n")
		sb.WriteString(memoryDetailsInstruction)
		sb.WriteString("\n\n")
		if globalSummary != "" {
			sb.WriteString("### Global\n\n")
			sb.WriteString(globalSummary)
		}
		if projectSummary != "" {
			if globalSummary != "" {
				sb.WriteString("\n\n---\n\n")
			}
			sb.WriteString("### Project\n\n")
			sb.WriteString(projectSummary)
		}
		return sb.String()
	}

	global := ms.ReadGlobalLongTerm()
	project := ms.ReadProjectLongTerm()
	recent := ms.GetRecentDailyNotes(3)

	if global == "" && project == "" && recent == "" {
		return ""
	}

	var sb strings.Builder

	if global != "" {
		sb.WriteString("## Global Memory\n\n")
		sb.WriteString(global)
	}

	if project != "" {
		if sb.Len() > 0 {
			sb.WriteString("\n\n---\n\n")
		}
		sb.WriteString("## Project Memory\n\n")
		sb.WriteString(project)
	}

	if recent != "" {
		if sb.Len() > 0 {
			sb.WriteString("\n\n---\n\n")
		}
		sb.WriteString("## Recent Daily Notes\n\n")
		sb.WriteString(recent)
	}

	return sb.String()
}

func (ms *Store) GetGlobalMemoryContext() string {
	return scopedMemoryContext("Global", ms.ReadGlobalSummary(), ms.ReadGlobalLongTerm())
}

func (ms *Store) GetProjectMemoryContext() string {
	return scopedMemoryContext("Project", ms.ReadProjectSummary(), ms.ReadProjectLongTerm())
}

func scopedMemoryContext(scopeName, summary, fallback string) string {
	if v := strings.TrimSpace(summary); v != "" {
		return fmt.Sprintf("## %s Memory Summary\n\n%s\n\n%s", scopeName, memoryDetailsInstruction, v)
	}
	if v := strings.TrimSpace(fallback); v != "" {
		return fmt.Sprintf("## %s Memory\n\n%s", scopeName, v)
	}
	return ""
}

func (ms *Store) rootForScope(scope string) string {
	if strings.EqualFold(scope, "global") || strings.EqualFold(scope, "user") {
		return ms.globalDir
	}
	return ms.projectDir
}

func readString(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

func resolveMemoryPath(root, relPath string) (string, string, error) {
	relPath = strings.TrimSpace(relPath)
	if relPath == "" {
		return root, "", nil
	}
	relPath = filepath.Clean(relPath)
	if filepath.IsAbs(relPath) || relPath == ".." || strings.HasPrefix(relPath, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("path must stay within memory root")
	}
	for _, part := range strings.Split(filepath.ToSlash(relPath), "/") {
		if part == "" || part == "." || part == ".." || strings.HasPrefix(part, ".") {
			return "", "", fmt.Errorf("path must not contain hidden or parent components")
		}
	}
	return filepath.Join(root, relPath), filepath.ToSlash(relPath), nil
}
