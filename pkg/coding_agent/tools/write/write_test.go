package write

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

func TestWriteToolBasics(t *testing.T) {
	tool := NewTool("/tmp")
	if tool.Name() != "write" {
		t.Errorf("Name() = %q, want %q", tool.Name(), "write")
	}
	if _, ok := tool.Parameters().(map[string]any); !ok {
		t.Fatal("Parameters() should return a map")
	}
}

func TestExecuteRequiresPath(t *testing.T) {
	tool := &WriteTool{cwd: t.TempDir()}
	res, err := tool.Execute(context.Background(), "id", map[string]any{"content": "hi"}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "path is required") {
		t.Errorf("unexpected result: %q", mustText(t, res))
	}
}

func TestExecuteRequiresContent(t *testing.T) {
	tool := &WriteTool{cwd: t.TempDir()}
	res, err := tool.Execute(context.Background(), "id", map[string]any{"path": "a.txt"}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "content is required") {
		t.Errorf("unexpected result: %q", mustText(t, res))
	}
}

func TestExecuteAllowsExplicitEmptyContent(t *testing.T) {
	dir := t.TempDir()
	tool := &WriteTool{cwd: dir}
	res, err := tool.Execute(context.Background(), "id", map[string]any{"path": "a.txt", "content": ""}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "created") {
		t.Errorf("unexpected result: %q", mustText(t, res))
	}
	data, err := os.ReadFile(filepath.Join(dir, "a.txt"))
	if err != nil || string(data) != "" {
		t.Errorf("expected an empty file, err=%v data=%q", err, data)
	}
}

func TestExecuteCreatesNewFile(t *testing.T) {
	dir := t.TempDir()
	tool := &WriteTool{cwd: dir}
	res, err := tool.Execute(context.Background(), "id", map[string]any{"path": "a.txt", "content": "hello"}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "File created successfully") {
		t.Errorf("unexpected result: %q", mustText(t, res))
	}
	details, ok := res.Details.(map[string]any)
	if !ok || details["type"] != "create" {
		t.Errorf("Details[type] should be create, got %#v", res.Details)
	}
	data, err := os.ReadFile(filepath.Join(dir, "a.txt"))
	if err != nil || string(data) != "hello" {
		t.Errorf("file content wrong, err=%v data=%q", err, data)
	}
}

func TestExecuteAcceptsFilePathAlias(t *testing.T) {
	dir := t.TempDir()
	tool := &WriteTool{cwd: dir}
	_, err := tool.Execute(context.Background(), "id", map[string]any{"file_path": "a.txt", "content": "hi"}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "a.txt")); err != nil {
		t.Errorf("file_path alias should create the file: %v", err)
	}
}

func TestExecuteCreatesParentDirectories(t *testing.T) {
	dir := t.TempDir()
	tool := &WriteTool{cwd: dir}
	_, err := tool.Execute(context.Background(), "id", map[string]any{"path": "a/b/c/file.txt", "content": "hi"}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "a", "b", "c", "file.txt")); err != nil {
		t.Errorf("expected nested directories to be created: %v", err)
	}
}

func TestExecuteUpdatesExistingFileWithoutTracking(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	tool := &WriteTool{cwd: dir}
	res, err := tool.Execute(context.Background(), "id", map[string]any{"path": "a.txt", "content": "new"}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "has been updated") {
		t.Errorf("unexpected result: %q", mustText(t, res))
	}
	details, ok := res.Details.(map[string]any)
	if !ok || details["type"] != "update" {
		t.Errorf("Details[type] should be update, got %#v", res.Details)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "new" {
		t.Errorf("file content = %q, want %q", string(data), "new")
	}
}

func TestExecuteRejectsDirectoryTarget(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "adir"), 0o755); err != nil {
		t.Fatal(err)
	}
	tool := &WriteTool{cwd: dir}
	res, err := tool.Execute(context.Background(), "id", map[string]any{"path": "adir", "content": "hi"}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "is a directory") {
		t.Errorf("unexpected result: %q", mustText(t, res))
	}
}

func TestExecuteRejectsUnreadExistingFileWhenTracked(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	readState := common.NewFileReadState()
	tool := &WriteTool{cwd: dir, readState: readState}
	res, err := tool.Execute(context.Background(), "id", map[string]any{"path": "a.txt", "content": "new"}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "has not been read yet") {
		t.Errorf("unexpected result: %q", mustText(t, res))
	}
	data, _ := os.ReadFile(path)
	if string(data) != "old" {
		t.Error("file should not have been overwritten without a prior read")
	}
}

func TestExecuteRejectsPartialReadBeforeOverwrite(t *testing.T) {
	// write always rewrites the whole file, so a partial (offset/limit) read
	// is not sufficient — unlike edit, which only needs the targeted region.
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	readState := common.NewFileReadState()
	readState.Record(path, "old", 0, true) // partial=true
	tool := &WriteTool{cwd: dir, readState: readState}
	res, err := tool.Execute(context.Background(), "id", map[string]any{"path": "a.txt", "content": "new"}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "has not been read yet") {
		t.Errorf("a partial read should be rejected same as no read: %q", mustText(t, res))
	}
}

func TestExecuteRejectsStaleReadBeforeOverwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(path, []byte("current on disk"), 0o644); err != nil {
		t.Fatal(err)
	}
	readState := common.NewFileReadState()
	readState.Record(path, "stale content", 0, false)
	tool := &WriteTool{cwd: dir, readState: readState}
	res, err := tool.Execute(context.Background(), "id", map[string]any{"path": "a.txt", "content": "new"}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "modified since read") {
		t.Errorf("unexpected result: %q", mustText(t, res))
	}
}

func TestExecuteAllowsOverwriteAfterMatchingFullRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(path, []byte("current"), 0o644); err != nil {
		t.Fatal(err)
	}
	readState := common.NewFileReadState()
	readState.Record(path, "current", 0, false)
	tool := &WriteTool{cwd: dir, readState: readState}
	res, err := tool.Execute(context.Background(), "id", map[string]any{"path": "a.txt", "content": "new"}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "has been updated") {
		t.Errorf("unexpected result: %q", mustText(t, res))
	}
}

func TestExecuteNewFileDoesNotRequireTrackedRead(t *testing.T) {
	dir := t.TempDir()
	readState := common.NewFileReadState()
	tool := &WriteTool{cwd: dir, readState: readState}
	res, err := tool.Execute(context.Background(), "id", map[string]any{"path": "new.txt", "content": "hi"}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "created") {
		t.Errorf("a brand-new file should not require a prior read: %q", mustText(t, res))
	}
}
