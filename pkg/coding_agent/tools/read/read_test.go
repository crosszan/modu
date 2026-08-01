package read

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openmodu/modu/pkg/coding_agent/tools/common"
	"github.com/openmodu/modu/pkg/types"
)

func newTestTool(t *testing.T) (*ReadTool, string) {
	t.Helper()
	dir := t.TempDir()
	return &ReadTool{cwd: dir}, dir
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

func TestReadToolBasics(t *testing.T) {
	tool := NewTool("/tmp")
	if tool.Name() != "read" {
		t.Errorf("Name() = %q, want %q", tool.Name(), "read")
	}
	if _, ok := tool.Parameters().(map[string]any); !ok {
		t.Fatal("Parameters() should return a map")
	}
}

func TestExecuteRequiresPath(t *testing.T) {
	tool, _ := newTestTool(t)
	res, err := tool.Execute(context.Background(), "id", map[string]any{}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "path is required") {
		t.Errorf("unexpected result: %q", mustText(t, res))
	}
}

func TestExecuteReturnsNumberedLines(t *testing.T) {
	tool, dir := newTestTool(t)
	writeFile(t, dir, "a.txt", "one\ntwo\nthree\n")
	res, err := tool.Execute(context.Background(), "id", map[string]any{"path": "a.txt"}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	got := mustText(t, res)
	for _, want := range []string{"1\tone", "2\ttwo", "3\tthree"} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q, got:\n%s", want, got)
		}
	}
}

func TestExecuteAcceptsFilePathAlias(t *testing.T) {
	tool, dir := newTestTool(t)
	writeFile(t, dir, "a.txt", "hello\n")
	res, err := tool.Execute(context.Background(), "id", map[string]any{"file_path": "a.txt"}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "1\thello") {
		t.Errorf("unexpected result: %q", mustText(t, res))
	}
}

func TestExecuteFileNotFound(t *testing.T) {
	tool, _ := newTestTool(t)
	res, err := tool.Execute(context.Background(), "id", map[string]any{"path": "missing.txt"}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "file not found") {
		t.Errorf("unexpected result: %q", mustText(t, res))
	}
}

func TestExecuteRejectsDirectory(t *testing.T) {
	tool, dir := newTestTool(t)
	if err := os.Mkdir(filepath.Join(dir, "adir"), 0o755); err != nil {
		t.Fatal(err)
	}
	res, err := tool.Execute(context.Background(), "id", map[string]any{"path": "adir"}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "is a directory") {
		t.Errorf("unexpected result: %q", mustText(t, res))
	}
}

func TestExecuteEmptyFileReturnsWarning(t *testing.T) {
	tool, dir := newTestTool(t)
	writeFile(t, dir, "empty.txt", "")
	res, err := tool.Execute(context.Background(), "id", map[string]any{"path": "empty.txt"}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "empty") {
		t.Errorf("unexpected result: %q", mustText(t, res))
	}
}

func TestExecuteOffsetBeyondEndOfFileReturnsWarning(t *testing.T) {
	tool, dir := newTestTool(t)
	writeFile(t, dir, "a.txt", "one\ntwo\nthree\n")
	res, err := tool.Execute(context.Background(), "id", map[string]any{"path": "a.txt", "offset": 100}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	got := mustText(t, res)
	if !strings.Contains(got, "shorter than") || !strings.Contains(got, "3 lines") {
		t.Errorf("unexpected result: %q", got)
	}
}

func TestExecuteOffsetAndLimitReturnRequestedRange(t *testing.T) {
	tool, dir := newTestTool(t)
	writeFile(t, dir, "a.txt", "1\n2\n3\n4\n5\n")
	res, err := tool.Execute(context.Background(), "id", map[string]any{
		"path":   "a.txt",
		"offset": 2,
		"limit":  2,
	}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	got := mustText(t, res)
	if !strings.Contains(got, "2\t2") || !strings.Contains(got, "3\t3") {
		t.Errorf("expected lines 2-3, got:\n%s", got)
	}
	if strings.Contains(got, "\t1\n") || strings.Contains(got, "4\t4") {
		t.Errorf("should not include lines outside offset/limit, got:\n%s", got)
	}
}

func TestExecuteAcceptsNumericStringOffsetAndLimit(t *testing.T) {
	tool, dir := newTestTool(t)
	writeFile(t, dir, "a.txt", "1\n2\n3\n")
	res, err := tool.Execute(context.Background(), "id", map[string]any{
		"path":   "a.txt",
		"offset": "2",
		"limit":  "1",
	}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	got := mustText(t, res)
	if !strings.Contains(got, "2\t2") {
		t.Errorf("expected line 2, got:\n%s", got)
	}
}

func TestExecuteRejectsNegativeOffset(t *testing.T) {
	tool, dir := newTestTool(t)
	writeFile(t, dir, "a.txt", "1\n2\n")
	res, err := tool.Execute(context.Background(), "id", map[string]any{"path": "a.txt", "offset": -1}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "offset must be") {
		t.Errorf("unexpected result: %q", mustText(t, res))
	}
}

func TestExecuteRejectsZeroLimit(t *testing.T) {
	tool, dir := newTestTool(t)
	writeFile(t, dir, "a.txt", "1\n2\n")
	res, err := tool.Execute(context.Background(), "id", map[string]any{"path": "a.txt", "limit": 0}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "limit must be") {
		t.Errorf("unexpected result: %q", mustText(t, res))
	}
}

func TestExecuteTruncatesLongFileWithoutExplicitLimit(t *testing.T) {
	tool, dir := newTestTool(t)
	var b strings.Builder
	for i := 0; i < common.ReadMaxLines+50; i++ {
		b.WriteString("line\n")
	}
	writeFile(t, dir, "big.txt", b.String())

	res, err := tool.Execute(context.Background(), "id", map[string]any{"path": "big.txt"}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	got := mustText(t, res)
	if !strings.Contains(got, "truncated") {
		t.Errorf("expected a truncation notice for a file over ReadMaxLines, got:\n%.200s...", got)
	}
	details, ok := res.Details.(map[string]any)
	if !ok || details["truncated"] != true {
		t.Errorf("Details[truncated] should be true, got %#v", res.Details)
	}
}

func TestExecuteExplicitLimitAllowsPastReadMaxLinesRegionWithoutBytesError(t *testing.T) {
	// A file larger than ReadMaxBytes without an explicit limit is rejected;
	// with an explicit limit, offset/limit should still work.
	tool, dir := newTestTool(t)
	var b strings.Builder
	for i := 0; i < 5; i++ {
		b.WriteString(strings.Repeat("a", 100) + "\n")
	}
	writeFile(t, dir, "a.txt", b.String())

	res, err := tool.Execute(context.Background(), "id", map[string]any{"path": "a.txt", "limit": 2}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "1\t") {
		t.Errorf("unexpected result: %q", mustText(t, res))
	}
}

func TestExecuteRejectsHugeFileWithoutExplicitLimit(t *testing.T) {
	tool, dir := newTestTool(t)
	// One line so line-count truncation can't kick in first; only the byte
	// limit should trigger the reject-without-limit path.
	writeFile(t, dir, "big.txt", strings.Repeat("a", common.ReadMaxBytes+1000))

	res, err := tool.Execute(context.Background(), "id", map[string]any{"path": "big.txt"}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "too large") {
		t.Errorf("expected a too-large error, got: %q", mustText(t, res))
	}
}

func TestExecuteRejectsBinaryFile(t *testing.T) {
	tool, dir := newTestTool(t)
	path := filepath.Join(dir, "a.data")
	if err := os.WriteFile(path, []byte{0, 1, 2, 3, 0, 0, 0}, 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := tool.Execute(context.Background(), "id", map[string]any{"path": "a.data"}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "binary") {
		t.Errorf("unexpected result: %q", mustText(t, res))
	}
}

func TestExecuteRejectsKnownBinaryExtension(t *testing.T) {
	tool, dir := newTestTool(t)
	writeFile(t, dir, "a.zip", "not actually a zip but has the extension")
	res, err := tool.Execute(context.Background(), "id", map[string]any{"path": "a.zip"}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "binary") {
		t.Errorf("unexpected result: %q", mustText(t, res))
	}
}

func TestExecuteReadsImageAsBase64(t *testing.T) {
	tool, dir := newTestTool(t)
	pngBytes := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}
	if err := os.WriteFile(filepath.Join(dir, "a.png"), pngBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := tool.Execute(context.Background(), "id", map[string]any{"path": "a.png"}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(res.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(res.Content))
	}
	img, ok := res.Content[0].(*types.ImageContent)
	if !ok {
		t.Fatalf("content block is %T, want *types.ImageContent", res.Content[0])
	}
	if img.MimeType != "image/png" {
		t.Errorf("MimeType = %q, want image/png", img.MimeType)
	}
	if img.Data == "" {
		t.Error("Data should not be empty")
	}
}

func TestExecuteBlocksDevicePaths(t *testing.T) {
	tool := &ReadTool{cwd: "/"}
	res, err := tool.Execute(context.Background(), "id", map[string]any{"path": "/dev/zero"}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "block or produce infinite output") {
		t.Errorf("unexpected result: %q", mustText(t, res))
	}
}

func TestExecuteRecordsReadStateWhenTracked(t *testing.T) {
	dir := t.TempDir()
	readState := common.NewFileReadState()
	tool := &ReadTool{cwd: dir, readState: readState}
	path := writeFile(t, dir, "a.txt", "hello\n")

	if _, err := tool.Execute(context.Background(), "id", map[string]any{"path": "a.txt"}, nil); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	record, ok := readState.Get(path)
	if !ok {
		t.Fatal("expected the read to be recorded")
	}
	if record.Content != "hello\n" {
		t.Errorf("recorded content = %q, want %q", record.Content, "hello\n")
	}
	if record.Partial {
		t.Error("a full, untruncated read should be recorded as non-partial")
	}
}

func TestExecuteRecordsPartialReadStateWhenOffsetLimitUsed(t *testing.T) {
	dir := t.TempDir()
	readState := common.NewFileReadState()
	tool := &ReadTool{cwd: dir, readState: readState}
	path := writeFile(t, dir, "a.txt", "1\n2\n3\n")

	if _, err := tool.Execute(context.Background(), "id", map[string]any{"path": "a.txt", "limit": 1}, nil); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	record, ok := readState.Get(path)
	if !ok {
		t.Fatal("expected the read to be recorded")
	}
	if !record.Partial {
		t.Error("a limited read should be recorded as partial")
	}
}

func TestExecuteReadsSimpleNotebook(t *testing.T) {
	tool, dir := newTestTool(t)
	notebook := `{
		"metadata": {"language_info": {"name": "python"}},
		"cells": [
			{"cell_type": "code", "id": "c1", "source": ["print('hi')"], "outputs": [
				{"output_type": "stream", "text": ["hi\n"]}
			]}
		]
	}`
	writeFile(t, dir, "a.ipynb", notebook)
	res, err := tool.Execute(context.Background(), "id", map[string]any{"path": "a.ipynb"}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var all strings.Builder
	for _, block := range res.Content {
		if text, ok := block.(*types.TextContent); ok {
			all.WriteString(text.Text)
		}
	}
	got := all.String()
	if !strings.Contains(got, "print('hi')") {
		t.Errorf("notebook output missing source, got:\n%s", got)
	}
	if !strings.Contains(got, "hi") {
		t.Errorf("notebook output missing stream text, got:\n%s", got)
	}
}

func TestExecuteRejectsInvalidNotebookJSON(t *testing.T) {
	tool, dir := newTestTool(t)
	writeFile(t, dir, "bad.ipynb", "{not valid json")
	res, err := tool.Execute(context.Background(), "id", map[string]any{"path": "bad.ipynb"}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "not valid JSON") {
		t.Errorf("unexpected result: %q", mustText(t, res))
	}
}

func TestExecuteEmptyNotebookReturnsWarning(t *testing.T) {
	tool, dir := newTestTool(t)
	writeFile(t, dir, "empty.ipynb", `{"metadata": {}, "cells": []}`)
	res, err := tool.Execute(context.Background(), "id", map[string]any{"path": "empty.ipynb"}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "no cells") {
		t.Errorf("unexpected result: %q", mustText(t, res))
	}
}

func TestSplitFileLines(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    []string
	}{
		{"no trailing newline", "a\nb\nc", []string{"a", "b", "c"}},
		{"trailing newline does not add empty line", "a\nb\n", []string{"a", "b"}},
		{"CRLF normalized to LF", "a\r\nb\r\n", []string{"a", "b"}},
		{"empty content", "", []string{""}},
		{"single newline", "\n", []string{""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitFileLines(tt.content)
			if len(got) != len(tt.want) {
				t.Fatalf("splitFileLines(%q) = %#v, want %#v", tt.content, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("line[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestIsLikelyBinary(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want bool
	}{
		{"empty is not binary", []byte{}, false},
		{"plain text", []byte("hello world\nline two\n"), false},
		{"text with tabs and CR", []byte("hello\tworld\r\n"), false},
		{"NUL byte is always binary", []byte("hello\x00world"), true},
		{"mostly control bytes is binary", []byte{0x01, 0x02, 0x03, 0x04, 0x05, 'a', 'b'}, true},
		{"a few control bytes under threshold is not binary", append([]byte{0x01}, []byte(strings.Repeat("a", 100))...), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isLikelyBinary(tt.data); got != tt.want {
				t.Errorf("isLikelyBinary(%v) = %v, want %v", tt.data, got, tt.want)
			}
		})
	}
}

func TestIsBlockedDevicePath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/dev/zero", true},
		{"/dev/null", false}, // reading /dev/null is harmless (empty), not blocked
		{"/proc/self/fd/0", true},
		{"/proc/1234/fd/1", true},
		{"/proc/self/fd/5", false}, // only fds 0-2 are blocked
		{"/home/user/file.txt", false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := isBlockedDevicePath(tt.path); got != tt.want {
				t.Errorf("isBlockedDevicePath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestAlternateScreenshotPath(t *testing.T) {
	thin := " "
	tests := []struct {
		name    string
		path    string
		wantOK  bool
		wantAlt string
	}{
		{"regular space AM to thin space", "shot AM.png", true, "shot" + thin + "AM.png"},
		{"regular space PM to thin space", "shot PM.png", true, "shot" + thin + "PM.png"},
		{"thin space AM to regular space", "shot" + thin + "AM.png", true, "shot AM.png"},
		{"not a screenshot name", "regular.png", false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := alternateScreenshotPath(tt.path)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && got != tt.wantAlt {
				t.Errorf("alternateScreenshotPath(%q) = %q, want %q", tt.path, got, tt.wantAlt)
			}
		})
	}
}

func TestNotebookString(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want string
	}{
		{"nil", nil, ""},
		{"string", "hello", "hello"},
		{"string slice joins without separator", []string{"a", "b", "c"}, "abc"},
		{"any slice of strings joins", []any{"a", "b"}, "ab"},
		{"any slice with non-string elements skips them", []any{"a", 5, "b"}, "ab"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := notebookString(tt.in); got != tt.want {
				t.Errorf("notebookString(%#v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
