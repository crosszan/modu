package memory

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLongTermRoundTrip(t *testing.T) {
	m := New(t.TempDir(), t.TempDir())

	if got := m.ReadLongTerm(); got != "" {
		t.Fatalf("expected empty project memory, got %q", got)
	}
	if err := m.WriteLongTerm("project fact"); err != nil {
		t.Fatal(err)
	}
	if got := m.ReadLongTerm(); got != "project fact" {
		t.Fatalf("project round-trip = %q", got)
	}

	if err := m.WriteGlobalLongTerm("global fact"); err != nil {
		t.Fatal(err)
	}
	if got := m.ReadGlobalLongTerm(); got != "global fact" {
		t.Fatalf("global round-trip = %q", got)
	}
}

func TestGetMemoryContextMergesScopes(t *testing.T) {
	m := New(t.TempDir(), t.TempDir())
	if m.GetMemoryContext() != "" {
		t.Fatal("empty store should yield empty context")
	}

	_ = m.WriteGlobalLongTerm("g")
	_ = m.WriteProjectLongTerm("p")
	_ = m.AppendToday("note today")

	ctx := m.GetMemoryContext()
	for _, want := range []string{"## Global Memory", "g", "## Project Memory", "p", "## Recent Daily Notes", "note today"} {
		if !strings.Contains(ctx, want) {
			t.Fatalf("context missing %q:\n%s", want, ctx)
		}
	}
	// Global must appear before project.
	if strings.Index(ctx, "## Global Memory") > strings.Index(ctx, "## Project Memory") {
		t.Fatal("global memory should appear before project memory")
	}
}

func TestAppendTodayAccumulates(t *testing.T) {
	m := New(t.TempDir(), t.TempDir())
	_ = m.AppendToday("first")
	_ = m.AppendToday("second")
	notes := m.GetRecentDailyNotes(1)
	if !strings.Contains(notes, "first") || !strings.Contains(notes, "second") {
		t.Fatalf("expected both notes, got %q", notes)
	}
}

func TestGetMemoryContextUsesSummaryWhenPresent(t *testing.T) {
	m := New(t.TempDir(), t.TempDir())
	_ = m.WriteGlobalLongTerm("full global")
	_ = m.WriteProjectLongTerm("full project")
	_ = m.WriteProjectSummary("project summary")

	ctx := m.GetMemoryContext()
	for _, want := range []string{"## Memory Summary", "project summary", "memo"} {
		if !strings.Contains(ctx, want) {
			t.Fatalf("context missing %q:\n%s", want, ctx)
		}
	}
	for _, unwanted := range []string{"full global", "full project", "Recent Daily Notes"} {
		if strings.Contains(ctx, unwanted) {
			t.Fatalf("summary context should not include %q:\n%s", unwanted, ctx)
		}
	}
}

func TestScopedMemoryContextsUseSummaryWhenPresent(t *testing.T) {
	m := New(t.TempDir(), t.TempDir())
	_ = m.WriteGlobalLongTerm("full global")
	_ = m.WriteProjectLongTerm("full project")
	_ = m.WriteGlobalSummary("global summary")
	_ = m.WriteProjectSummary("project summary")

	global := m.GetGlobalMemoryContext()
	if !strings.Contains(global, "## Global Memory Summary") || !strings.Contains(global, "global summary") || !strings.Contains(global, "memo") {
		t.Fatalf("global context missing summary content:\n%s", global)
	}
	if strings.Contains(global, "full global") {
		t.Fatalf("global summary context should not include raw memory:\n%s", global)
	}

	project := m.GetProjectMemoryContext()
	if !strings.Contains(project, "## Project Memory Summary") || !strings.Contains(project, "project summary") || !strings.Contains(project, "memo") {
		t.Fatalf("project context missing summary content:\n%s", project)
	}
	if strings.Contains(project, "full project") {
		t.Fatalf("project summary context should not include raw memory:\n%s", project)
	}
}

func TestScopedReadPathListReadSearch(t *testing.T) {
	m := New(t.TempDir(), t.TempDir())
	if err := os.MkdirAll(filepath.Join(m.projectDir, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(m.projectDir, "notes", "one.md"), []byte("alpha\nneedle\nomega\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	entries, truncated, err := m.List("project", "notes", 10)
	if err != nil {
		t.Fatal(err)
	}
	if truncated || len(entries) != 1 || entries[0].Path != "notes/one.md" {
		t.Fatalf("unexpected entries truncated=%v entries=%+v", truncated, entries)
	}

	content, truncated, err := m.Read("project", "notes/one.md", 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	if content != "needle" || !truncated {
		t.Fatalf("read content=%q truncated=%v", content, truncated)
	}

	matches, truncated, err := m.Search("project", "needle", "", 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if truncated || len(matches) != 1 {
		t.Fatalf("unexpected search truncated=%v matches=%+v", truncated, matches)
	}
	if matches[0].Path != "notes/one.md" || matches[0].Line != 2 || !strings.Contains(matches[0].Content, "alpha\nneedle\nomega") {
		t.Fatalf("unexpected match: %+v", matches[0])
	}
}

func TestScopedReadPathTruncationRequiresMoreResults(t *testing.T) {
	m := New(t.TempDir(), t.TempDir())
	if err := os.WriteFile(filepath.Join(m.projectDir, "a.md"), []byte("needle\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(m.projectDir, "b.md"), []byte("needle\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	entries, truncated, err := m.List("project", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if truncated || len(entries) != 2 {
		t.Fatalf("exact list limit should not be truncated: truncated=%v entries=%+v", truncated, entries)
	}

	entries, truncated, err = m.List("project", "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if !truncated || len(entries) != 1 {
		t.Fatalf("list over limit should be truncated: truncated=%v entries=%+v", truncated, entries)
	}

	matches, truncated, err := m.Search("project", "needle", "", 0, 2)
	if err != nil {
		t.Fatal(err)
	}
	if truncated || len(matches) != 2 {
		t.Fatalf("exact search limit should not be truncated: truncated=%v matches=%+v", truncated, matches)
	}

	matches, truncated, err = m.Search("project", "needle", "", 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !truncated || len(matches) != 1 {
		t.Fatalf("search over limit should be truncated: truncated=%v matches=%+v", truncated, matches)
	}
}

func TestScopedReadPathRejectsTraversal(t *testing.T) {
	m := New(t.TempDir(), t.TempDir())
	if _, _, err := m.List("project", "../outside", 10); err == nil {
		t.Fatal("expected parent traversal to fail")
	}
	if _, _, err := m.Read("project", ".hidden", 1, 1); err == nil {
		t.Fatal("expected hidden path to fail")
	}
}

func TestLongTermEntriesSplitsOnBlankLines(t *testing.T) {
	m := New(t.TempDir(), t.TempDir())
	// Matches how record_long_term joins: entries separated by a blank line.
	if err := m.WriteLongTerm("first fact\n\nsecond fact\nwith a wrapped line\n\nthird fact"); err != nil {
		t.Fatal(err)
	}
	got := m.LongTermEntries("project")
	if len(got) != 3 {
		t.Fatalf("entries = %#v, want 3", got)
	}
	if got[1] != "second fact\nwith a wrapped line" {
		t.Fatalf("a multi-line entry should stay whole, got %q", got[1])
	}
}

func TestLongTermEntriesToleratesHandEditedSpacing(t *testing.T) {
	m := New(t.TempDir(), t.TempDir())
	// A human editing MEMORY.md leaves runs of blank lines and trailing
	// newlines; those should not become empty entries.
	if err := m.WriteLongTerm("\n\nfirst\n\n\n\nsecond\n\n"); err != nil {
		t.Fatal(err)
	}
	if got := m.LongTermEntries("project"); len(got) != 2 || got[0] != "first" || got[1] != "second" {
		t.Fatalf("entries = %#v, want [first second]", got)
	}
}

func TestUpdateLongTermReplacesOnlyTheMatchedEntry(t *testing.T) {
	m := New(t.TempDir(), t.TempDir())
	if err := m.WriteLongTerm("uses yarn\n\nruns tests with go test\n\ndeploys on fridays"); err != nil {
		t.Fatal(err)
	}
	if err := m.UpdateLongTerm("project", "uses yarn", "uses pnpm"); err != nil {
		t.Fatalf("UpdateLongTerm: %v", err)
	}
	got := m.LongTermEntries("project")
	want := []string{"uses pnpm", "runs tests with go test", "deploys on fridays"}
	if len(got) != len(want) {
		t.Fatalf("entries = %#v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("entries = %#v, want %#v", got, want)
		}
	}
}

func TestDeleteLongTermRemovesOnlyTheMatchedEntry(t *testing.T) {
	m := New(t.TempDir(), t.TempDir())
	if err := m.WriteLongTerm("keep me\n\nstale fact\n\nkeep me too"); err != nil {
		t.Fatal(err)
	}
	if err := m.DeleteLongTerm("project", "stale fact"); err != nil {
		t.Fatalf("DeleteLongTerm: %v", err)
	}
	got := m.LongTermEntries("project")
	if len(got) != 2 || got[0] != "keep me" || got[1] != "keep me too" {
		t.Fatalf("entries = %#v", got)
	}
	// The neighbours must still be separated, not run together.
	if !strings.Contains(m.ReadLongTerm(), "keep me\n\nkeep me too") {
		t.Fatalf("remaining file = %q", m.ReadLongTerm())
	}
}

func TestLongTermEditRefusesAmbiguousMatchWithoutChangingAnything(t *testing.T) {
	m := New(t.TempDir(), t.TempDir())
	original := "deploy uses docker\n\ntest uses docker"
	if err := m.WriteLongTerm(original); err != nil {
		t.Fatal(err)
	}
	// "docker" is in both entries. Picking one would silently corrupt the
	// other, so this has to fail and leave the file untouched.
	err := m.UpdateLongTerm("project", "docker", "uses podman")
	if !errors.Is(err, ErrMemoryEntryAmbiguous) {
		t.Fatalf("err = %v, want ErrMemoryEntryAmbiguous", err)
	}
	if m.ReadLongTerm() != original {
		t.Fatalf("memory changed despite the ambiguous match: %q", m.ReadLongTerm())
	}

	// A longer match disambiguates it.
	if err := m.UpdateLongTerm("project", "deploy uses docker", "deploy uses podman"); err != nil {
		t.Fatalf("UpdateLongTerm with a unique match: %v", err)
	}
	if got := m.LongTermEntries("project"); got[0] != "deploy uses podman" || got[1] != "test uses docker" {
		t.Fatalf("entries = %#v", got)
	}
}

func TestLongTermEditReportsMissingMatch(t *testing.T) {
	m := New(t.TempDir(), t.TempDir())
	if err := m.WriteLongTerm("only fact"); err != nil {
		t.Fatal(err)
	}
	if err := m.UpdateLongTerm("project", "nothing like this", "x"); !errors.Is(err, ErrMemoryEntryNotFound) {
		t.Fatalf("err = %v, want ErrMemoryEntryNotFound", err)
	}
	if err := m.DeleteLongTerm("project", ""); !errors.Is(err, ErrMemoryEntryNotFound) {
		t.Fatalf("empty match err = %v, want ErrMemoryEntryNotFound", err)
	}
	if m.ReadLongTerm() != "only fact" {
		t.Fatalf("memory changed on a failed match: %q", m.ReadLongTerm())
	}
}

func TestLongTermEditKeepsScopesSeparate(t *testing.T) {
	m := New(t.TempDir(), t.TempDir())
	if err := m.WriteLongTerm("shared wording"); err != nil {
		t.Fatal(err)
	}
	if err := m.WriteGlobalLongTerm("shared wording"); err != nil {
		t.Fatal(err)
	}
	if err := m.DeleteLongTerm("global", "shared wording"); err != nil {
		t.Fatalf("DeleteLongTerm: %v", err)
	}
	if m.ReadGlobalLongTerm() != "" {
		t.Fatalf("global should be empty, got %q", m.ReadGlobalLongTerm())
	}
	if m.ReadLongTerm() != "shared wording" {
		t.Fatalf("editing global must not touch project memory, got %q", m.ReadLongTerm())
	}
}

func TestContextStatsDistinguishSummaryFromRawMemory(t *testing.T) {
	m := New(t.TempDir(), t.TempDir())

	// Nothing stored: memory costs a prompt nothing.
	m.GetMemoryContext()
	if got := m.ContextStats(); got.Source != "empty" || got.Bytes != 0 {
		t.Fatalf("stats with no memory = %#v", got)
	}

	// No summary yet, so the whole long-term memory goes into every prompt.
	bulk := strings.Repeat("a long remembered fact\n", 200)
	if err := m.WriteProjectLongTerm(bulk); err != nil {
		t.Fatal(err)
	}
	m.GetMemoryContext()
	raw := m.ContextStats()
	if raw.Source != "raw" {
		t.Fatalf("source = %q, want raw", raw.Source)
	}
	if raw.Bytes < len(bulk) {
		t.Fatalf("raw context (%d) should carry the full memory (%d)", raw.Bytes, len(bulk))
	}

	// Once organized, the summary is what gets injected instead.
	if err := m.WriteProjectSummary("one short summary line"); err != nil {
		t.Fatal(err)
	}
	m.GetMemoryContext()
	summarized := m.ContextStats()
	if summarized.Source != "summary" {
		t.Fatalf("source = %q, want summary", summarized.Source)
	}
	// This comparison is the whole point of the instrumentation: it is what
	// tells you organizing bought anything at all.
	if summarized.Bytes >= raw.Bytes {
		t.Fatalf("summary context (%d) should be smaller than raw (%d)", summarized.Bytes, raw.Bytes)
	}
}
