package toolresult

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

func mustDetails(t *testing.T, res types.ToolResult) map[string]any {
	t.Helper()
	details, ok := res.Details.(map[string]any)
	if !ok {
		t.Fatalf("Details is %T, want map[string]any", res.Details)
	}
	return details
}

func TestToolBasics(t *testing.T) {
	tool := NewTool(nil)
	if tool.Name() != "read_tool_result" {
		t.Errorf("Name() = %q, want read_tool_result", tool.Name())
	}
}

func TestExecuteRequiresStore(t *testing.T) {
	tool := NewTool(nil)
	res, err := tool.Execute(context.Background(), "id", map[string]any{
		"call_id": "x", "offset": 1, "limit": 10,
	}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.IsError || !strings.Contains(mustText(t, res), "not configured") {
		t.Errorf("unexpected result: %q (isError=%v)", mustText(t, res), res.IsError)
	}
}

func TestExecuteRequiresCallID(t *testing.T) {
	store := common.NewArtifactStore(t.TempDir())
	tool := NewTool(store)
	res, err := tool.Execute(context.Background(), "id", map[string]any{"offset": 1, "limit": 10}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "call_id is required") {
		t.Errorf("unexpected result: %q", mustText(t, res))
	}
}

func TestExecuteAcceptsToolCallIDAlias(t *testing.T) {
	store := common.NewArtifactStore(t.TempDir())
	if _, err := store.Put("call-1", "output", []byte("line1\nline2\n")); err != nil {
		t.Fatal(err)
	}
	tool := NewTool(store)
	res, err := tool.Execute(context.Background(), "id", map[string]any{
		"tool_call_id": "call-1", "offset": 1, "limit": 10,
	}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "line1") {
		t.Errorf("unexpected result: %q", mustText(t, res))
	}
}

func TestExecuteRequiresOffsetAndLimit(t *testing.T) {
	store := common.NewArtifactStore(t.TempDir())
	tool := NewTool(store)

	res, err := tool.Execute(context.Background(), "id", map[string]any{"call_id": "x", "limit": 10}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "offset is required") {
		t.Errorf("unexpected result: %q", mustText(t, res))
	}

	res, err = tool.Execute(context.Background(), "id", map[string]any{"call_id": "x", "offset": 1}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "limit is required") {
		t.Errorf("unexpected result: %q", mustText(t, res))
	}
}

func TestExecuteRejectsNonPositiveOffsetOrLimit(t *testing.T) {
	store := common.NewArtifactStore(t.TempDir())
	tool := NewTool(store)
	res, err := tool.Execute(context.Background(), "id", map[string]any{"call_id": "x", "offset": 0, "limit": 10}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "offset is required") {
		t.Errorf("offset=0 should be rejected same as missing, got: %q", mustText(t, res))
	}
}

func TestExecuteMissingArtifact(t *testing.T) {
	store := common.NewArtifactStore(t.TempDir())
	tool := NewTool(store)
	res, err := tool.Execute(context.Background(), "id", map[string]any{
		"call_id": "does-not-exist", "offset": 1, "limit": 10,
	}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.IsError || !strings.Contains(mustText(t, res), "not found") {
		t.Errorf("unexpected result: %q (isError=%v)", mustText(t, res), res.IsError)
	}
}

func TestExecuteReturnsPagedWindow(t *testing.T) {
	dir := t.TempDir()
	store := common.NewArtifactStore(dir)
	content := "line1\nline2\nline3\nline4\nline5\n"
	if _, err := store.Put("call-1", "output", []byte(content)); err != nil {
		t.Fatal(err)
	}
	tool := NewTool(store)

	res, err := tool.Execute(context.Background(), "id", map[string]any{
		"call_id": "call-1", "offset": 2, "limit": 2,
	}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	got := mustText(t, res)
	if !strings.Contains(got, "2\tline2") || !strings.Contains(got, "3\tline3") {
		t.Errorf("expected lines 2-3, got:\n%s", got)
	}
	if strings.Contains(got, "line1") || strings.Contains(got, "line4") {
		t.Errorf("should not include lines outside the window, got:\n%s", got)
	}
	details := mustDetails(t, res)
	if details["hasMore"] != true {
		t.Errorf("hasMore = %v, want true (2 lines remain after the window)", details["hasMore"])
	}
	if details["returnedLines"] != 2 {
		t.Errorf("returnedLines = %v, want 2", details["returnedLines"])
	}
}

func TestExecuteHasMoreFalseWhenWindowCoversRestOfFile(t *testing.T) {
	dir := t.TempDir()
	store := common.NewArtifactStore(dir)
	if _, err := store.Put("call-1", "output", []byte("line1\nline2\nline3\n")); err != nil {
		t.Fatal(err)
	}
	tool := NewTool(store)
	res, err := tool.Execute(context.Background(), "id", map[string]any{
		"call_id": "call-1", "offset": 1, "limit": 100,
	}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	details := mustDetails(t, res)
	if details["hasMore"] != false {
		t.Errorf("hasMore = %v, want false when the whole file fit in one window", details["hasMore"])
	}
}

func TestExecuteOffsetBeyondEndReturnsPlaceholder(t *testing.T) {
	dir := t.TempDir()
	store := common.NewArtifactStore(dir)
	if _, err := store.Put("call-1", "output", []byte("line1\nline2\n")); err != nil {
		t.Fatal(err)
	}
	tool := NewTool(store)
	res, err := tool.Execute(context.Background(), "id", map[string]any{
		"call_id": "call-1", "offset": 100, "limit": 10,
	}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "beyond the artifact output") {
		t.Errorf("unexpected result: %q", mustText(t, res))
	}
}

// Regression test for the hasMore bug: readLineWindow used to infer hasMore
// from whether the last ReadString call happened to end in io.EOF, but a
// properly newline-terminated final line reads as a normal success (err ==
// nil) even when it's genuinely the last line in the file — which made
// hasMore default to true whenever the window's last line ended with '\n',
// regardless of whether anything followed it.
func TestReadLineWindowHasMoreExactlyReflectsRemainingContent(t *testing.T) {
	dir := t.TempDir()

	exact := filepath.Join(dir, "exact.txt")
	if err := os.WriteFile(exact, []byte("1\n2\n3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, hasMore, err := readLineWindow(context.Background(), exact, 1, 3)
	if err != nil {
		t.Fatalf("readLineWindow: %v", err)
	}
	if hasMore {
		t.Error("hasMore should be false: the window covered every line in the file")
	}

	withExtra := filepath.Join(dir, "extra.txt")
	if err := os.WriteFile(withExtra, []byte("1\n2\n3\n4\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, hasMore, err = readLineWindow(context.Background(), withExtra, 1, 3)
	if err != nil {
		t.Fatalf("readLineWindow: %v", err)
	}
	if !hasMore {
		t.Error("hasMore should be true: one more line remains after the window")
	}

	noTrailingNewline := filepath.Join(dir, "no-trailing-nl.txt")
	if err := os.WriteFile(noTrailingNewline, []byte("1\n2\n3"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, hasMore, err = readLineWindow(context.Background(), noTrailingNewline, 1, 3)
	if err != nil {
		t.Fatalf("readLineWindow: %v", err)
	}
	if hasMore {
		t.Error("hasMore should be false: the window covered every line, including a final line with no trailing newline")
	}
}
