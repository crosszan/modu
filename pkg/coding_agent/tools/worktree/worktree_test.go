package worktree

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/openmodu/modu/pkg/types"
)

type fakeManager struct {
	enterPath string
	enterErr  error
	exitErr   error
	active    string
}

func (m *fakeManager) EnterWorktree() (string, error) { return m.enterPath, m.enterErr }
func (m *fakeManager) ExitWorktree() error            { return m.exitErr }
func (m *fakeManager) ActiveWorktree() string         { return m.active }

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

func TestEnterWorktreeToolBasics(t *testing.T) {
	tool := NewEnterWorktreeTool(&fakeManager{})
	if tool.Name() != "enter_worktree" {
		t.Errorf("Name() = %q, want enter_worktree", tool.Name())
	}
}

func TestEnterWorktreeToolSuccess(t *testing.T) {
	tool := NewEnterWorktreeTool(&fakeManager{enterPath: "/tmp/wt/abc"})
	res, err := tool.Execute(context.Background(), "id", nil, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "/tmp/wt/abc") {
		t.Errorf("unexpected result: %q", mustText(t, res))
	}
}

func TestEnterWorktreeToolPropagatesError(t *testing.T) {
	tool := NewEnterWorktreeTool(&fakeManager{enterErr: errors.New("not a git repository")})
	res, err := tool.Execute(context.Background(), "id", nil, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "not a git repository") {
		t.Errorf("unexpected result: %q", mustText(t, res))
	}
}

func TestEnterWorktreeToolNilManager(t *testing.T) {
	tool := NewEnterWorktreeTool(nil)
	res, err := tool.Execute(context.Background(), "id", nil, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "not configured") {
		t.Errorf("unexpected result: %q", mustText(t, res))
	}
}

func TestExitWorktreeToolBasics(t *testing.T) {
	tool := NewExitWorktreeTool(&fakeManager{})
	if tool.Name() != "exit_worktree" {
		t.Errorf("Name() = %q, want exit_worktree", tool.Name())
	}
}

func TestExitWorktreeToolSuccess(t *testing.T) {
	tool := NewExitWorktreeTool(&fakeManager{})
	res, err := tool.Execute(context.Background(), "id", nil, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "exited worktree") {
		t.Errorf("unexpected result: %q", mustText(t, res))
	}
}

func TestExitWorktreeToolPropagatesError(t *testing.T) {
	tool := NewExitWorktreeTool(&fakeManager{exitErr: errors.New("boom")})
	res, err := tool.Execute(context.Background(), "id", nil, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "boom") {
		t.Errorf("unexpected result: %q", mustText(t, res))
	}
}

func TestExitWorktreeToolNilManager(t *testing.T) {
	tool := NewExitWorktreeTool(nil)
	res, err := tool.Execute(context.Background(), "id", nil, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "not configured") {
		t.Errorf("unexpected result: %q", mustText(t, res))
	}
}
