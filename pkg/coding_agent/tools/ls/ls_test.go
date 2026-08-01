package ls

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openmodu/modu/pkg/coding_agent/tools/common"
	"github.com/openmodu/modu/pkg/types"
)

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

func setupDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"b.txt", "a.txt", "readme.md"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestLsToolBasics(t *testing.T) {
	tool := NewTool("/tmp")
	if tool.Name() != "ls" {
		t.Errorf("Name() = %q, want %q", tool.Name(), "ls")
	}
	if _, ok := tool.Parameters().(map[string]any); !ok {
		t.Fatal("Parameters() should return a map")
	}
}

func TestExecuteListsEntriesSortedCaseInsensitive(t *testing.T) {
	dir := setupDir(t)
	tool := &LsTool{cwd: dir}
	res, err := tool.Execute(context.Background(), "id", map[string]any{}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	got := mustText(t, res)
	lines := strings.Split(got, "\n")
	want := []string{"a.txt", "b.txt", "readme.md", "sub/"}
	if len(lines) != len(want) {
		t.Fatalf("got %d lines, want %d:\n%s", len(lines), len(want), got)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Errorf("line[%d] = %q, want %q", i, lines[i], want[i])
		}
	}
}

func TestExecuteDirectoriesSuffixedWithSlash(t *testing.T) {
	dir := setupDir(t)
	tool := &LsTool{cwd: dir}
	res, err := tool.Execute(context.Background(), "id", map[string]any{}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "sub/") {
		t.Errorf("directory should be suffixed with /, got:\n%s", mustText(t, res))
	}
}

func TestExecuteEmptyDirectoryMessage(t *testing.T) {
	dir := t.TempDir()
	tool := &LsTool{cwd: dir}
	res, err := tool.Execute(context.Background(), "id", map[string]any{}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if mustText(t, res) != "(empty directory)" {
		t.Errorf("got %q, want %q", mustText(t, res), "(empty directory)")
	}
}

func TestExecuteDirectoryNotFound(t *testing.T) {
	tool := &LsTool{cwd: t.TempDir()}
	res, err := tool.Execute(context.Background(), "id", map[string]any{"path": "missing"}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "directory not found") {
		t.Errorf("unexpected result: %q", mustText(t, res))
	}
}

func TestExecuteRejectsFileTarget(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	tool := &LsTool{cwd: dir}
	res, err := tool.Execute(context.Background(), "id", map[string]any{"path": "a.txt"}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "not a directory") {
		t.Errorf("unexpected result: %q", mustText(t, res))
	}
}

func TestExecuteLimitTruncatesAndReportsTotal(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 10; i++ {
		name := "f" + string(rune('a'+i)) + ".txt"
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	tool := &LsTool{cwd: dir}
	res, err := tool.Execute(context.Background(), "id", map[string]any{"limit": 3}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	got := mustText(t, res)
	if !strings.Contains(got, "10 entries total, showing first 3") {
		t.Errorf("expected a truncation notice, got:\n%s", got)
	}
}

func TestExecuteAcceptsNumericStringLimit(t *testing.T) {
	dir := setupDir(t)
	tool := &LsTool{cwd: dir}
	res, err := tool.Execute(context.Background(), "id", map[string]any{"limit": "2"}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "showing first 2") {
		t.Errorf("unexpected result: %q", mustText(t, res))
	}
}

func TestExecuteNonPositiveLimitFallsBackToDefault(t *testing.T) {
	dir := setupDir(t)
	tool := &LsTool{cwd: dir}
	res, err := tool.Execute(context.Background(), "id", map[string]any{"limit": 0}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// 4 entries, well under the default of 500 — no truncation notice.
	if strings.Contains(mustText(t, res), "showing first") {
		t.Errorf("limit=0 should fall back to the default, not truncate to 0, got:\n%s", mustText(t, res))
	}
}

func TestExecuteIgnoreStringPattern(t *testing.T) {
	dir := setupDir(t)
	tool := &LsTool{cwd: dir}
	res, err := tool.Execute(context.Background(), "id", map[string]any{"ignore": "*.txt"}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	got := mustText(t, res)
	if strings.Contains(got, "a.txt") || strings.Contains(got, "b.txt") {
		t.Errorf("*.txt entries should be ignored, got:\n%s", got)
	}
	if !strings.Contains(got, "readme.md") {
		t.Errorf("non-matching entries should remain, got:\n%s", got)
	}
}

func TestExecuteIgnoreArrayPatterns(t *testing.T) {
	dir := setupDir(t)
	tool := &LsTool{cwd: dir}
	res, err := tool.Execute(context.Background(), "id", map[string]any{
		"ignore": []any{"*.txt", "*.md"},
	}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	got := mustText(t, res)
	if !strings.Contains(got, "sub/") {
		t.Errorf("expected sub/ to remain, got:\n%s", got)
	}
	if strings.Contains(got, ".txt") || strings.Contains(got, ".md") {
		t.Errorf("ignored patterns should be filtered, got:\n%s", got)
	}
}

func TestExecuteIgnoreDirectoryPattern(t *testing.T) {
	dir := setupDir(t)
	tool := &LsTool{cwd: dir}
	res, err := tool.Execute(context.Background(), "id", map[string]any{"ignore": "sub/"}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if strings.Contains(mustText(t, res), "sub") {
		t.Errorf("sub/ should be ignored, got:\n%s", mustText(t, res))
	}
}

func TestExecuteWithArtifactsStore(t *testing.T) {
	dir := setupDir(t)
	artDir := t.TempDir()
	tool := &LsTool{cwd: dir, artifacts: common.NewArtifactStore(artDir)}
	res, err := tool.Execute(context.Background(), "id", map[string]any{}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "a.txt") {
		t.Errorf("unexpected result: %q", mustText(t, res))
	}
	details, ok := res.Details.(map[string]any)
	if !ok {
		t.Fatal("Details should be a map when an artifact store is configured")
	}
	output, ok := details["output"].(map[string]any)
	if !ok {
		t.Fatal("Details[output] missing")
	}
	if output["totalEntries"] != 4 {
		t.Errorf("totalEntries = %v, want 4", output["totalEntries"])
	}
}

func TestParseIgnorePatterns(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want []string
	}{
		{"nil", nil, nil},
		{"single string", "*.log", []string{"*.log"}},
		{"string slice", []string{"a", "b"}, []string{"a", "b"}},
		{"any slice of strings", []any{"a", "b"}, []string{"a", "b"}},
		{"any slice skips non-strings", []any{"a", 5, "b"}, []string{"a", "b"}},
		{"leading ./ stripped", "./build", []string{"build"}},
		{"surrounding whitespace trimmed", "  build  ", []string{"build"}},
		{"empty entries after trimming are dropped", []string{"a", "  ", ""}, []string{"a"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseIgnorePatterns(tt.in)
			if len(got) != len(tt.want) {
				t.Fatalf("parseIgnorePatterns(%#v) = %#v, want %#v", tt.in, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestIsIgnoredEntry(t *testing.T) {
	tests := []struct {
		name     string
		entry    string
		isDir    bool
		patterns []string
		want     bool
	}{
		{"exact file match", "a.txt", false, []string{"a.txt"}, true},
		{"glob match", "a.txt", false, []string{"*.txt"}, true},
		{"no match", "a.txt", false, []string{"*.md"}, false},
		{"directory trailing slash pattern", "build", true, []string{"build/"}, true},
		{"directory without trailing slash also matches", "build", true, []string{"build"}, true},
		{"double-star root match", "vendor", true, []string{"vendor/**"}, true},
		{"empty patterns never match", "a.txt", false, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isIgnoredEntry(tt.entry, tt.isDir, tt.patterns); got != tt.want {
				t.Errorf("isIgnoredEntry(%q, %v, %v) = %v, want %v", tt.entry, tt.isDir, tt.patterns, got, tt.want)
			}
		})
	}
}
