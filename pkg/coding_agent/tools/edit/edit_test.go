package edit

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openmodu/modu/pkg/coding_agent/tools/common"
	"github.com/openmodu/modu/pkg/types"
)

func newTestTool(t *testing.T) (*EditTool, string) {
	t.Helper()
	dir := t.TempDir()
	return &EditTool{cwd: dir}, dir
}

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func mustText(t *testing.T, res types.ToolResult) string {
	t.Helper()
	if len(res.Content) == 0 {
		t.Fatal("result has no content blocks")
	}
	text, ok := res.Content[0].(*types.TextContent)
	if !ok {
		t.Fatalf("content block is %T, want *types.TextContent", res.Content[0])
	}
	return text.Text
}

func TestEditToolBasics(t *testing.T) {
	tool := NewTool("/tmp")
	if tool.Name() != "edit" {
		t.Errorf("Name() = %q, want %q", tool.Name(), "edit")
	}
	if _, ok := tool.Parameters().(map[string]any); !ok {
		t.Fatal("Parameters() should return a map")
	}
}

func TestExecuteReplacesUniqueMatch(t *testing.T) {
	tool, dir := newTestTool(t)
	writeFile(t, dir, "a.txt", "hello world")

	res, err := tool.Execute(context.Background(), "id", map[string]any{
		"path":     "a.txt",
		"old_text": "world",
		"new_text": "there",
	}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "Successfully edited") {
		t.Fatalf("unexpected result: %q", mustText(t, res))
	}
	data, _ := os.ReadFile(filepath.Join(dir, "a.txt"))
	if string(data) != "hello there" {
		t.Errorf("file content = %q, want %q", string(data), "hello there")
	}
}

func TestExecuteAcceptsFilePathAndAliasArgs(t *testing.T) {
	tool, dir := newTestTool(t)
	writeFile(t, dir, "a.txt", "hello world")

	res, err := tool.Execute(context.Background(), "id", map[string]any{
		"file_path":  "a.txt",
		"old_string": "world",
		"new_string": "there",
	}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "Successfully edited") {
		t.Fatalf("aliases should work the same as the primary arg names: %q", mustText(t, res))
	}
}

func TestExecuteRejectsMissingPath(t *testing.T) {
	tool, _ := newTestTool(t)
	res, err := tool.Execute(context.Background(), "id", map[string]any{
		"old_text": "a",
		"new_text": "b",
	}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "path is required") {
		t.Errorf("unexpected error: %q", mustText(t, res))
	}
}

func TestExecuteRejectsIdenticalOldAndNewText(t *testing.T) {
	tool, dir := newTestTool(t)
	writeFile(t, dir, "a.txt", "hello")
	res, err := tool.Execute(context.Background(), "id", map[string]any{
		"path":     "a.txt",
		"old_text": "same",
		"new_text": "same",
	}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "No changes to make") {
		t.Errorf("unexpected error: %q", mustText(t, res))
	}
}

func TestExecuteRejectsMissingOldTextNotFound(t *testing.T) {
	tool, dir := newTestTool(t)
	writeFile(t, dir, "a.txt", "hello world")
	res, err := tool.Execute(context.Background(), "id", map[string]any{
		"path":     "a.txt",
		"old_text": "goodbye",
		"new_text": "hi",
	}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "not found") {
		t.Errorf("unexpected error: %q", mustText(t, res))
	}
}

func TestExecuteRejectsFileNotFound(t *testing.T) {
	tool, _ := newTestTool(t)
	res, err := tool.Execute(context.Background(), "id", map[string]any{
		"path":     "missing.txt",
		"old_text": "a",
		"new_text": "b",
	}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "file not found") {
		t.Errorf("unexpected error: %q", mustText(t, res))
	}
}

func TestExecuteRejectsAmbiguousMatchWithoutReplaceAll(t *testing.T) {
	tool, dir := newTestTool(t)
	writeFile(t, dir, "a.txt", "foo foo foo")
	res, err := tool.Execute(context.Background(), "id", map[string]any{
		"path":     "a.txt",
		"old_text": "foo",
		"new_text": "bar",
	}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "appears 3 times") {
		t.Errorf("unexpected error: %q", mustText(t, res))
	}
	data, _ := os.ReadFile(filepath.Join(dir, "a.txt"))
	if string(data) != "foo foo foo" {
		t.Error("file should not have been modified when the match is ambiguous")
	}
}

func TestExecuteReplaceAllReplacesEveryOccurrence(t *testing.T) {
	tool, dir := newTestTool(t)
	writeFile(t, dir, "a.txt", "foo foo foo")
	res, err := tool.Execute(context.Background(), "id", map[string]any{
		"path":        "a.txt",
		"old_text":    "foo",
		"new_text":    "bar",
		"replace_all": true,
	}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "3 replacement") {
		t.Errorf("unexpected result: %q", mustText(t, res))
	}
	data, _ := os.ReadFile(filepath.Join(dir, "a.txt"))
	if string(data) != "bar bar bar" {
		t.Errorf("file content = %q, want %q", string(data), "bar bar bar")
	}
}

func TestExecuteReplaceAllAcceptsBooleanString(t *testing.T) {
	tool, dir := newTestTool(t)
	writeFile(t, dir, "a.txt", "foo foo")
	res, err := tool.Execute(context.Background(), "id", map[string]any{
		"path":        "a.txt",
		"old_text":    "foo",
		"new_text":    "bar",
		"replace_all": "true",
	}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "2 replacement") {
		t.Errorf("replace_all=\"true\" string should behave like boolean true: %q", mustText(t, res))
	}
}

func TestExecuteEmptyOldTextCreatesNewFile(t *testing.T) {
	tool, dir := newTestTool(t)
	res, err := tool.Execute(context.Background(), "id", map[string]any{
		"path":     "new.txt",
		"old_text": "",
		"new_text": "hello",
	}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "Successfully edited") {
		t.Fatalf("unexpected result: %q", mustText(t, res))
	}
	data, err := os.ReadFile(filepath.Join(dir, "new.txt"))
	if err != nil {
		t.Fatalf("expected the new file to be created: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("file content = %q, want %q", string(data), "hello")
	}
}

func TestExecuteEmptyOldTextCreatesNestedDirectories(t *testing.T) {
	tool, dir := newTestTool(t)
	_, err := tool.Execute(context.Background(), "id", map[string]any{
		"path":     "a/b/c/new.txt",
		"old_text": "",
		"new_text": "hi",
	}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "a", "b", "c", "new.txt")); err != nil {
		t.Errorf("expected nested directories to be created: %v", err)
	}
}

func TestExecuteEmptyOldTextRejectsExistingNonEmptyFile(t *testing.T) {
	tool, dir := newTestTool(t)
	writeFile(t, dir, "a.txt", "already has content")
	res, err := tool.Execute(context.Background(), "id", map[string]any{
		"path":     "a.txt",
		"old_text": "",
		"new_text": "overwrite",
	}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "already exists") {
		t.Errorf("unexpected result: %q", mustText(t, res))
	}
	data, _ := os.ReadFile(filepath.Join(dir, "a.txt"))
	if string(data) != "already has content" {
		t.Error("existing non-empty file must not be overwritten")
	}
}

func TestExecuteEmptyOldTextAllowsWritingToWhitespaceOnlyFile(t *testing.T) {
	tool, dir := newTestTool(t)
	writeFile(t, dir, "a.txt", "   \n\n  ")
	res, err := tool.Execute(context.Background(), "id", map[string]any{
		"path":     "a.txt",
		"old_text": "",
		"new_text": "real content",
	}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "Successfully edited") {
		t.Fatalf("a whitespace-only file should be treated like an empty file: %q", mustText(t, res))
	}
	data, _ := os.ReadFile(filepath.Join(dir, "a.txt"))
	if string(data) != "real content" {
		t.Errorf("file content = %q, want %q", string(data), "real content")
	}
}

func TestExecuteRejectsDirectoryTarget(t *testing.T) {
	tool, dir := newTestTool(t)
	if err := os.Mkdir(filepath.Join(dir, "adir"), 0o755); err != nil {
		t.Fatal(err)
	}
	res, err := tool.Execute(context.Background(), "id", map[string]any{
		"path":     "adir",
		"old_text": "a",
		"new_text": "b",
	}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "is a directory") {
		t.Errorf("unexpected result: %q", mustText(t, res))
	}
}

func TestExecuteRejectsNotebookTarget(t *testing.T) {
	tool, dir := newTestTool(t)
	writeFile(t, dir, "a.ipynb", `{"cells": []}`)
	res, err := tool.Execute(context.Background(), "id", map[string]any{
		"path":     "a.ipynb",
		"old_text": "cells",
		"new_text": "notebook",
	}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "Jupyter Notebook") {
		t.Errorf("unexpected result: %q", mustText(t, res))
	}
}

func TestExecutePreservesCRLFLineEndings(t *testing.T) {
	tool, dir := newTestTool(t)
	writeFile(t, dir, "a.txt", "line1\r\nline2\r\nline3")
	res, err := tool.Execute(context.Background(), "id", map[string]any{
		"path":     "a.txt",
		"old_text": "line2",
		"new_text": "replaced",
	}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "Successfully edited") {
		t.Fatalf("unexpected result: %q", mustText(t, res))
	}
	data, _ := os.ReadFile(filepath.Join(dir, "a.txt"))
	want := "line1\r\nreplaced\r\nline3"
	if string(data) != want {
		t.Errorf("file content = %q, want %q (CRLF should be preserved)", string(data), want)
	}
}

func TestExecuteStripsBOMBeforeMatching(t *testing.T) {
	tool, dir := newTestTool(t)
	writeFile(t, dir, "a.txt", "\xef\xbb\xbfhello world")
	res, err := tool.Execute(context.Background(), "id", map[string]any{
		"path":     "a.txt",
		"old_text": "hello",
		"new_text": "hi",
	}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "Successfully edited") {
		t.Fatalf("BOM should be stripped before matching: %q", mustText(t, res))
	}
}

func TestExecuteFuzzyMatchToleratesTrailingWhitespaceDifference(t *testing.T) {
	tool, dir := newTestTool(t)
	// The file has trailing whitespace on a line; old_text doesn't.
	writeFile(t, dir, "a.txt", "func foo() {   \n\treturn\n}")
	res, err := tool.Execute(context.Background(), "id", map[string]any{
		"path":     "a.txt",
		"old_text": "func foo() {\n\treturn\n}",
		"new_text": "func foo() {\n\treturn 1\n}",
	}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "fuzzy match") {
		t.Errorf("expected a fuzzy-match result, got %q", mustText(t, res))
	}
	data, _ := os.ReadFile(filepath.Join(dir, "a.txt"))
	if !strings.Contains(string(data), "return 1") {
		t.Errorf("fuzzy match should still have applied the edit, got %q", string(data))
	}
}

func TestExecuteRejectsStaleReadWhenTracked(t *testing.T) {
	dir := t.TempDir()
	readState := common.NewFileReadState()
	tool := &EditTool{cwd: dir, readState: readState}
	path := writeFile(t, dir, "a.txt", "hello world")

	// No Record() call — simulates never having read the file first.
	res, err := tool.Execute(context.Background(), "id", map[string]any{
		"path":     "a.txt",
		"old_text": "world",
		"new_text": "there",
	}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "has not been read yet") {
		t.Errorf("unexpected result: %q", mustText(t, res))
	}

	// Now record a stale (outdated) version and confirm the edit is still
	// rejected because content no longer matches.
	readState.Record(path, "hello world (old)", 0, false)
	res, err = tool.Execute(context.Background(), "id", map[string]any{
		"path":     "a.txt",
		"old_text": "world",
		"new_text": "there",
	}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "modified since read") {
		t.Errorf("unexpected result: %q", mustText(t, res))
	}
}

func TestExecuteAllowsEditAfterMatchingRead(t *testing.T) {
	dir := t.TempDir()
	readState := common.NewFileReadState()
	tool := &EditTool{cwd: dir, readState: readState}
	path := writeFile(t, dir, "a.txt", "hello world")
	readState.Record(path, "hello world", 0, false)

	res, err := tool.Execute(context.Background(), "id", map[string]any{
		"path":     "a.txt",
		"old_text": "world",
		"new_text": "there",
	}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "Successfully edited") {
		t.Errorf("edit should succeed once the recorded read matches current content: %q", mustText(t, res))
	}
}

func TestReplaceAllExact(t *testing.T) {
	got := replaceAllExact("foo bar foo baz foo", "foo", "X")
	want := "X bar X baz X"
	if got != want {
		t.Errorf("replaceAllExact() = %q, want %q", got, want)
	}
}

func TestReplaceAllExactNoMatches(t *testing.T) {
	got := replaceAllExact("bar baz", "foo", "X")
	if got != "bar baz" {
		t.Errorf("replaceAllExact() = %q, want unchanged", got)
	}
}

func TestShouldStripTrailingNewlineForDelete(t *testing.T) {
	tests := []struct {
		name    string
		content string
		end     int
		old     string
		newText string
		want    bool
	}{
		{"delete removes its own trailing newline", "line1\nline2\n", 6, "line1\n", "", false},
		{"delete without trailing newline in old strips the next one", "line1\nline2\n", 5, "line1", "", true},
		{"non-empty replacement never strips", "line1\nline2\n", 5, "line1", "X", false},
		{"nothing to strip at end of content", "line1", 5, "line1", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldStripTrailingNewlineForDelete(tt.content, tt.end, tt.old, tt.newText); got != tt.want {
				t.Errorf("shouldStripTrailingNewlineForDelete() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNormalizeWhitespace(t *testing.T) {
	got := normalizeWhitespace("  hello   world  \n\tfoo  ")
	want := "hello world foo"
	if got != want {
		t.Errorf("normalizeWhitespace() = %q, want %q", got, want)
	}
}

func TestNormalizeForFuzzyMatch(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"trailing whitespace stripped per line", "foo   \nbar\t\n", "foo\nbar\n"},
		{"smart double quotes normalized", "“hello”", `"hello"`},
		{"smart single quotes normalized", "‘hi’", "'hi'"},
		{"en dash normalized", "a–b", "a-b"},
		{"nbsp normalized to space", "a b", "a b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeForFuzzyMatch(tt.in); got != tt.want {
				t.Errorf("normalizeForFuzzyMatch(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestSplitLineSpans(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    int // number of spans
	}{
		{"empty content", "", 1},
		{"single line no newline", "abc", 1},
		{"two lines", "abc\ndef", 2},
		{"trailing newline adds an empty final span", "abc\n", 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitLineSpans(tt.content)
			if len(got) != tt.want {
				t.Errorf("splitLineSpans(%q) has %d spans, want %d: %#v", tt.content, len(got), tt.want, got)
			}
		})
	}
}

func TestFindFuzzyLineMatches(t *testing.T) {
	content := "func foo() {   \n\treturn 1\n}\n"
	old := "func foo() {\n\treturn 1\n}"
	matches := findFuzzyLineMatches(content, old)
	if len(matches) != 1 {
		t.Fatalf("expected 1 fuzzy match, got %d: %#v", len(matches), matches)
	}
	if matches[0].text != "func foo() {   \n\treturn 1\n}" {
		t.Errorf("matched text = %q", matches[0].text)
	}
}

func TestFindFuzzyLineMatchesNoMatch(t *testing.T) {
	content := "completely different content"
	old := "func foo() {\n\treturn 1\n}"
	matches := findFuzzyLineMatches(content, old)
	if len(matches) != 0 {
		t.Errorf("expected no matches, got %#v", matches)
	}
}

func TestPreserveQuoteStyle(t *testing.T) {
	tests := []struct {
		name          string
		oldText       string
		actualOldText string
		newText       string
		want          string
	}{
		{"identical text returns new text unchanged", "same", "same", `say "hi"`, `say "hi"`},
		{"curly double quotes in actual match applied to replacement", `say "hi"`, "say “hi”", `say "bye"`, "say “bye”"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := preserveQuoteStyle(tt.oldText, tt.actualOldText, tt.newText); got != tt.want {
				t.Errorf("preserveQuoteStyle() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGenerateDiffIncludesContextAndChangedLines(t *testing.T) {
	fileContent := "a\nb\nold\nc\nd"
	diff := generateDiff("old", "new", fileContent, "path.txt")
	if !strings.Contains(diff, "- 3  old") {
		t.Errorf("diff missing removed line marker:\n%s", diff)
	}
	if !strings.Contains(diff, "+ 3  new") {
		t.Errorf("diff missing added line marker:\n%s", diff)
	}
	if !strings.Contains(diff, "path.txt") {
		t.Errorf("diff missing file path header:\n%s", diff)
	}
}
