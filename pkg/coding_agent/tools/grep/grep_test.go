package grep

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openmodu/modu/pkg/types"
)

func newTestTool(t *testing.T) (*GrepTool, string) {
	t.Helper()
	dir := t.TempDir()
	return &GrepTool{cwd: dir}, dir
}

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
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

// runBoth exercises the same scenario through both the (environment-
// dependent) Execute() entry point and the built-in fallback directly, so
// the built-in implementation stays covered even in environments without
// ripgrep installed.
func runBoth(t *testing.T, tool *GrepTool, args map[string]any) (viaExecute, viaBuiltin types.ToolResult) {
	t.Helper()
	viaExecute, err := tool.Execute(context.Background(), "id", args, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	pattern, _ := args["pattern"].(string)
	searchPath := tool.cwd
	if p, ok := args["path"].(string); ok && p != "" {
		searchPath = filepath.Join(tool.cwd, p)
	}
	glob, _ := args["glob"].(string)
	globPatterns := parseGrepGlobPatterns(glob)
	opts, errResult := parseGrepOptions(args)
	if errResult != nil {
		t.Fatalf("parseGrepOptions: %v", mustText(t, *errResult))
	}
	viaBuiltin, err = tool.executeBuiltin(context.Background(), pattern, searchPath, globPatterns, opts, "id")
	if err != nil {
		t.Fatalf("executeBuiltin: %v", err)
	}
	return viaExecute, viaBuiltin
}

func TestGrepToolBasics(t *testing.T) {
	tool := NewTool("/tmp")
	if tool.Name() != "grep" {
		t.Errorf("Name() = %q, want %q", tool.Name(), "grep")
	}
	if _, ok := tool.Parameters().(map[string]any); !ok {
		t.Fatal("Parameters() should return a map")
	}
}

func TestExecuteRequiresPattern(t *testing.T) {
	tool, _ := newTestTool(t)
	res, err := tool.Execute(context.Background(), "id", map[string]any{}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "pattern is required") {
		t.Errorf("unexpected result: %q", mustText(t, res))
	}
}

func TestExecutePathDoesNotExist(t *testing.T) {
	tool, _ := newTestTool(t)
	res, err := tool.Execute(context.Background(), "id", map[string]any{
		"pattern": "foo",
		"path":    "does-not-exist",
	}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "does not exist") {
		t.Errorf("unexpected result: %q", mustText(t, res))
	}
}

func TestExecuteDefaultModeFilesWithMatches(t *testing.T) {
	tool, dir := newTestTool(t)
	writeFile(t, dir, "a.txt", "hello world\n")
	writeFile(t, dir, "b.txt", "no match here\n")

	viaExecute, viaBuiltin := runBoth(t, tool, map[string]any{"pattern": "hello"})
	for _, res := range []types.ToolResult{viaExecute, viaBuiltin} {
		got := mustText(t, res)
		if !strings.Contains(got, "a.txt") {
			t.Errorf("expected a.txt in files_with_matches output, got:\n%s", got)
		}
		if strings.Contains(got, "b.txt") {
			t.Errorf("b.txt should not match, got:\n%s", got)
		}
	}
}

func TestExecuteContentModeShowsMatchingLines(t *testing.T) {
	tool, dir := newTestTool(t)
	writeFile(t, dir, "a.txt", "line one\nhello world\nline three\n")

	viaExecute, viaBuiltin := runBoth(t, tool, map[string]any{
		"pattern":     "hello",
		"output_mode": "content",
	})
	for _, res := range []types.ToolResult{viaExecute, viaBuiltin} {
		got := mustText(t, res)
		if !strings.Contains(got, "hello world") {
			t.Errorf("expected matching line, got:\n%s", got)
		}
		if !strings.Contains(got, "2:") {
			t.Errorf("expected a line number, got:\n%s", got)
		}
	}
}

func TestExecuteContentModeWithoutLineNumbers(t *testing.T) {
	tool, dir := newTestTool(t)
	writeFile(t, dir, "a.txt", "hello world\n")

	viaExecute, viaBuiltin := runBoth(t, tool, map[string]any{
		"pattern":     "hello",
		"output_mode": "content",
		"-n":          false,
	})
	for _, res := range []types.ToolResult{viaExecute, viaBuiltin} {
		got := mustText(t, res)
		if strings.Contains(got, "1:hello") {
			t.Errorf("line numbers should be suppressed, got:\n%s", got)
		}
		if !strings.Contains(got, "hello world") {
			t.Errorf("expected matching content, got:\n%s", got)
		}
	}
}

func TestExecuteCountMode(t *testing.T) {
	tool, dir := newTestTool(t)
	writeFile(t, dir, "a.txt", "foo\nfoo\nbar\n")

	viaExecute, viaBuiltin := runBoth(t, tool, map[string]any{
		"pattern":     "foo",
		"output_mode": "count",
	})
	for _, res := range []types.ToolResult{viaExecute, viaBuiltin} {
		got := mustText(t, res)
		if !strings.Contains(got, "a.txt:2") {
			t.Errorf("expected a.txt:2, got:\n%s", got)
		}
		if !strings.Contains(got, "Found 2 total occurrence(s) across 1 file(s)") {
			t.Errorf("expected a summary line, got:\n%s", got)
		}
	}
}

func TestExecuteInvalidOutputMode(t *testing.T) {
	tool, dir := newTestTool(t)
	writeFile(t, dir, "a.txt", "hello\n")
	res, err := tool.Execute(context.Background(), "id", map[string]any{
		"pattern":     "hello",
		"output_mode": "bogus",
	}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "invalid output_mode") {
		t.Errorf("unexpected result: %q", mustText(t, res))
	}
}

func TestExecuteNoMatches(t *testing.T) {
	tool, dir := newTestTool(t)
	writeFile(t, dir, "a.txt", "hello world\n")

	viaExecute, viaBuiltin := runBoth(t, tool, map[string]any{"pattern": "nomatchhere"})
	for _, res := range []types.ToolResult{viaExecute, viaBuiltin} {
		if !strings.Contains(mustText(t, res), "No files found") {
			t.Errorf("unexpected result: %q", mustText(t, res))
		}
	}
}

func TestExecuteCaseInsensitive(t *testing.T) {
	tool, dir := newTestTool(t)
	writeFile(t, dir, "a.txt", "Hello World\n")

	viaExecute, viaBuiltin := runBoth(t, tool, map[string]any{
		"pattern":     "hello",
		"ignore_case": true,
		"output_mode": "content",
	})
	for _, res := range []types.ToolResult{viaExecute, viaBuiltin} {
		if !strings.Contains(mustText(t, res), "Hello World") {
			t.Errorf("case-insensitive search should match, got: %q", mustText(t, res))
		}
	}
}

func TestExecuteCaseSensitiveByDefault(t *testing.T) {
	tool, dir := newTestTool(t)
	writeFile(t, dir, "a.txt", "Hello World\n")

	viaExecute, viaBuiltin := runBoth(t, tool, map[string]any{
		"pattern":     "hello",
		"output_mode": "content",
	})
	for _, res := range []types.ToolResult{viaExecute, viaBuiltin} {
		if strings.Contains(mustText(t, res), "No matches found.") == false && strings.Contains(mustText(t, res), "Hello World") {
			t.Errorf("case-sensitive search should not match different case, got: %q", mustText(t, res))
		}
	}
}

func TestExecuteLiteralModeTreatsPatternAsExactString(t *testing.T) {
	tool, dir := newTestTool(t)
	writeFile(t, dir, "a.txt", "a.b.c\naxbxc\n")

	viaExecute, viaBuiltin := runBoth(t, tool, map[string]any{
		"pattern":     "a.b.c",
		"literal":     true,
		"output_mode": "content",
	})
	for _, res := range []types.ToolResult{viaExecute, viaBuiltin} {
		got := mustText(t, res)
		if !strings.Contains(got, "a.b.c") {
			t.Errorf("literal pattern should match the exact string, got: %q", got)
		}
		if strings.Contains(got, "axbxc") {
			t.Errorf("literal pattern should not match via regex wildcard, got: %q", got)
		}
	}
}

func TestExecuteGlobFiltersFiles(t *testing.T) {
	tool, dir := newTestTool(t)
	writeFile(t, dir, "a.go", "hello\n")
	writeFile(t, dir, "b.md", "hello\n")

	viaExecute, viaBuiltin := runBoth(t, tool, map[string]any{
		"pattern": "hello",
		"glob":    "*.go",
	})
	for _, res := range []types.ToolResult{viaExecute, viaBuiltin} {
		got := mustText(t, res)
		if !strings.Contains(got, "a.go") {
			t.Errorf("expected a.go, got:\n%s", got)
		}
		if strings.Contains(got, "b.md") {
			t.Errorf("b.md should be filtered out by glob, got:\n%s", got)
		}
	}
}

func TestExecuteTypeFiltersFiles(t *testing.T) {
	tool, dir := newTestTool(t)
	writeFile(t, dir, "a.go", "hello\n")
	writeFile(t, dir, "b.py", "hello\n")

	viaExecute, viaBuiltin := runBoth(t, tool, map[string]any{
		"pattern": "hello",
		"type":    "go",
	})
	for _, res := range []types.ToolResult{viaExecute, viaBuiltin} {
		got := mustText(t, res)
		if !strings.Contains(got, "a.go") {
			t.Errorf("expected a.go, got:\n%s", got)
		}
		if strings.Contains(got, "b.py") {
			t.Errorf("b.py should be filtered out by type=go, got:\n%s", got)
		}
	}
}

func TestExecuteContextLines(t *testing.T) {
	tool, dir := newTestTool(t)
	writeFile(t, dir, "a.txt", "1\n2\n3\nMATCH\n5\n6\n7\n")

	viaExecute, viaBuiltin := runBoth(t, tool, map[string]any{
		"pattern":     "MATCH",
		"output_mode": "content",
		"context":     1,
	})
	for _, res := range []types.ToolResult{viaExecute, viaBuiltin} {
		got := mustText(t, res)
		if !strings.Contains(got, "3") || !strings.Contains(got, "MATCH") || !strings.Contains(got, "5") {
			t.Errorf("expected 1 line of context on each side, got:\n%s", got)
		}
		if strings.Contains(got, "\n1\n") || strings.Contains(got, ":1\n") {
			t.Errorf("context=1 should not include line 2 lines away, got:\n%s", got)
		}
	}
}

func TestExecuteVCSDirectoriesExcluded(t *testing.T) {
	tool, dir := newTestTool(t)
	writeFile(t, dir, ".git/config", "hello\n")
	writeFile(t, dir, "a.txt", "hello\n")

	viaExecute, viaBuiltin := runBoth(t, tool, map[string]any{"pattern": "hello"})
	for _, res := range []types.ToolResult{viaExecute, viaBuiltin} {
		got := mustText(t, res)
		if strings.Contains(got, ".git") {
			t.Errorf(".git directory should be excluded from search, got:\n%s", got)
		}
		if !strings.Contains(got, "a.txt") {
			t.Errorf("expected a.txt in results, got:\n%s", got)
		}
	}
}

func TestExecuteHeadLimitAliasesLimit(t *testing.T) {
	tool, dir := newTestTool(t)
	for i := 0; i < 5; i++ {
		name := "f" + string(rune('a'+i)) + ".txt"
		writeFile(t, dir, name, "hello\n")
	}
	res, err := tool.Execute(context.Background(), "id", map[string]any{
		"pattern":    "hello",
		"head_limit": 2,
	}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	got := mustText(t, res)
	if !strings.Contains(got, "Found 2 file(s)") {
		t.Errorf("head_limit=2 should cap results to 2 files, got:\n%s", got)
	}
}

func TestExecuteBuiltinDirectlyBasic(t *testing.T) {
	tool, dir := newTestTool(t)
	writeFile(t, dir, "a.txt", "needle in a haystack\n")
	res, err := tool.executeBuiltin(context.Background(), "needle", dir, nil, grepOptions{
		outputMode:  outputModeContent,
		lineNumbers: true,
		limit:       defaultGrepLimit,
	}, "id")
	if err != nil {
		t.Fatalf("executeBuiltin: %v", err)
	}
	if !strings.Contains(mustText(t, res), "needle in a haystack") {
		t.Errorf("unexpected result: %q", mustText(t, res))
	}
}

func TestExecuteBuiltinInvalidRegexReturnsError(t *testing.T) {
	tool, dir := newTestTool(t)
	res, err := tool.executeBuiltin(context.Background(), "(unclosed", dir, nil, grepOptions{
		outputMode: outputModeFilesWithMatches,
		limit:      defaultGrepLimit,
	}, "id")
	if err != nil {
		t.Fatalf("executeBuiltin: %v", err)
	}
	if !strings.Contains(mustText(t, res), "invalid regex") {
		t.Errorf("unexpected result: %q", mustText(t, res))
	}
}

func TestParseGrepGlobPatterns(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", nil},
		{"single pattern", "*.go", []string{"*.go"}},
		{"comma separated", "*.go,*.md", []string{"*.go", "*.md"}},
		{"space separated", "*.go *.md", []string{"*.go", "*.md"}},
		{"brace pattern kept intact", "*.{ts,tsx}", []string{"*.{ts,tsx}"}},
		{"whitespace around commas trimmed", "*.go, *.md", []string{"*.go", "*.md"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseGrepGlobPatterns(tt.in)
			if len(got) != len(tt.want) {
				t.Fatalf("parseGrepGlobPatterns(%q) = %#v, want %#v", tt.in, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestExpandBraceGlob(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"no braces", "*.go", []string{"*.go"}},
		{"simple brace expansion", "*.{ts,tsx}", []string{"*.ts", "*.tsx"}},
		{"prefix and suffix preserved", "src/*.{js,jsx}", []string{"src/*.js", "src/*.jsx"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expandBraceGlob(tt.in)
			if len(got) != len(tt.want) {
				t.Fatalf("expandBraceGlob(%q) = %#v, want %#v", tt.in, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestMatchesFileType(t *testing.T) {
	tests := []struct {
		path     string
		fileType string
		want     bool
	}{
		{"a.go", "go", true},
		{"a.py", "go", false},
		{"a.jsx", "js", true},
		{"a.tsx", "ts", true},
		{"a.md", "markdown", true},
		{"a.yml", "yaml", true},
		{"a.txt", "go", false},
		{"a.GO", "go", true}, // case-insensitive
	}
	for _, tt := range tests {
		t.Run(tt.path+"_"+tt.fileType, func(t *testing.T) {
			if got := matchesFileType(tt.path, tt.fileType); got != tt.want {
				t.Errorf("matchesFileType(%q, %q) = %v, want %v", tt.path, tt.fileType, got, tt.want)
			}
		})
	}
}

func TestApplyGrepWindow(t *testing.T) {
	lines := []string{"a", "b", "c", "d", "e"}
	tests := []struct {
		name          string
		opts          grepOptions
		wantLines     []string
		wantTruncated bool
	}{
		{"no window returns everything", grepOptions{unlimited: true}, lines, false},
		{"limit caps results", grepOptions{limit: 2}, []string{"a", "b"}, true},
		{"offset skips leading results", grepOptions{offset: 2, unlimited: true}, []string{"c", "d", "e"}, true},
		{"offset beyond length returns empty", grepOptions{offset: 10, unlimited: true}, []string{}, true},
		{"offset+limit within bounds", grepOptions{offset: 1, limit: 2}, []string{"b", "c"}, true},
		// "truncated" also fires when offset>0 alone, even if this window
		// reaches the end of the results — it means "something was hidden"
		// (here, items before the offset), not strictly "more items follow".
		// See the comment on grepWindowMessage for why the message wording
		// stays generic rather than claiming there's more ahead.
		{"offset+limit covering the rest is still flagged (offset hid earlier items)", grepOptions{offset: 3, limit: 10}, []string{"d", "e"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, truncated := applyGrepWindow(lines, tt.opts)
			if len(got) != len(tt.wantLines) {
				t.Fatalf("applyGrepWindow() lines = %#v, want %#v", got, tt.wantLines)
			}
			for i := range got {
				if got[i] != tt.wantLines[i] {
					t.Errorf("[%d] = %q, want %q", i, got[i], tt.wantLines[i])
				}
			}
			if truncated != tt.wantTruncated {
				t.Errorf("truncated = %v, want %v", truncated, tt.wantTruncated)
			}
		})
	}
}

func TestSummarizeGrepCountLines(t *testing.T) {
	total, files := summarizeGrepCountLines([]string{"a.txt:3", "b.txt:5", "malformed"})
	if total != 8 || files != 2 {
		t.Errorf("summarizeGrepCountLines() = (%d, %d), want (8, 2)", total, files)
	}
}

func TestParseGrepLimit(t *testing.T) {
	var opts grepOptions
	parseGrepLimit(&opts, 0)
	if !opts.unlimited || opts.limit != 0 {
		t.Errorf("limit=0 should mean unlimited, got %+v", opts)
	}

	opts = grepOptions{}
	parseGrepLimit(&opts, 50)
	if opts.unlimited || opts.limit != 50 {
		t.Errorf("limit=50 should set a bounded limit, got %+v", opts)
	}
}

func TestShouldSkipGrepDir(t *testing.T) {
	for _, dir := range []string{".git", ".svn", ".hg"} {
		if !shouldSkipGrepDir(dir) {
			t.Errorf("shouldSkipGrepDir(%q) = false, want true", dir)
		}
	}
	if shouldSkipGrepDir("src") {
		t.Error("shouldSkipGrepDir(\"src\") = true, want false")
	}
}

func TestSortedKeys(t *testing.T) {
	set := map[string]struct{}{"c": {}, "a": {}, "b": {}}
	got := sortedKeys(set, 20)
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("sortedKeys() = %#v, want %#v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestSortedKeysRespectsLimit(t *testing.T) {
	set := map[string]struct{}{"a": {}, "b": {}, "c": {}}
	got := sortedKeys(set, 2)
	if len(got) != 2 {
		t.Errorf("sortedKeys with limit=2 returned %d keys, want 2", len(got))
	}
}

func TestRelativeGrepPath(t *testing.T) {
	cwd := "/home/user/project"
	tests := []struct {
		name       string
		path       string
		searchPath string
		want       string
	}{
		{"under cwd", "/home/user/project/src/a.go", cwd, "src/a.go"},
		{"path equals search path", "/home/user/project", cwd, "project"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := relativeGrepPath(tt.path, tt.searchPath, cwd); got != tt.want {
				t.Errorf("relativeGrepPath(%q, %q, %q) = %q, want %q", tt.path, tt.searchPath, cwd, got, tt.want)
			}
		})
	}
}

func TestNoGrepMatchesMessage(t *testing.T) {
	if got := noGrepMatchesMessage(outputModeFilesWithMatches); got != "No files found" {
		t.Errorf("noGrepMatchesMessage(files_with_matches) = %q", got)
	}
	if got := noGrepMatchesMessage(outputModeContent); got != "No matches found." {
		t.Errorf("noGrepMatchesMessage(content) = %q", got)
	}
	if got := noGrepMatchesMessage(outputModeCount); got != "No matches found." {
		t.Errorf("noGrepMatchesMessage(count) = %q", got)
	}
}
