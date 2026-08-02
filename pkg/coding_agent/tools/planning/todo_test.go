package planning

import (
	"context"
	"strings"
	"testing"

	"github.com/openmodu/modu/pkg/types"
)

type fakeTodoStore struct {
	todos []TodoItem
}

func (s *fakeTodoStore) GetTodos() []TodoItem      { return s.todos }
func (s *fakeTodoStore) SetTodos(items []TodoItem) { s.todos = items }

func mustTodoText(t *testing.T, res types.ToolResult) string {
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

func TestTodoWriteToolBasics(t *testing.T) {
	tool := NewTodoWriteTool(&fakeTodoStore{})
	if tool.Name() != "todo_write" {
		t.Errorf("Name() = %q, want todo_write", tool.Name())
	}
}

func TestExecuteRequiresStore(t *testing.T) {
	tool := NewTodoWriteTool(nil)
	res, err := tool.Execute(context.Background(), "id", map[string]any{"todos": []any{}}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustTodoText(t, res), "not configured") {
		t.Errorf("unexpected result: %q", mustTodoText(t, res))
	}
}

func TestExecuteReplacesTodoList(t *testing.T) {
	store := &fakeTodoStore{}
	tool := NewTodoWriteTool(store)
	res, err := tool.Execute(context.Background(), "id", map[string]any{
		"todos": []any{
			map[string]any{"content": "task 1", "status": "completed"},
			map[string]any{"content": "task 2", "status": "in_progress"},
			map[string]any{"content": "task 3", "status": "pending"},
		},
	}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustTodoText(t, res), "3 item") {
		t.Errorf("unexpected result: %q", mustTodoText(t, res))
	}
	if len(store.todos) != 3 {
		t.Fatalf("expected 3 todos stored, got %d", len(store.todos))
	}
	if store.todos[1].Content != "task 2" || store.todos[1].Status != "in_progress" {
		t.Errorf("unexpected todo[1]: %+v", store.todos[1])
	}
}

func TestExecuteRejectsMissingTodosArray(t *testing.T) {
	tool := NewTodoWriteTool(&fakeTodoStore{})
	res, err := tool.Execute(context.Background(), "id", map[string]any{}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustTodoText(t, res), "must be an array") {
		t.Errorf("unexpected result: %q", mustTodoText(t, res))
	}
}

func TestExecuteRejectsEmptyContent(t *testing.T) {
	tool := NewTodoWriteTool(&fakeTodoStore{})
	res, err := tool.Execute(context.Background(), "id", map[string]any{
		"todos": []any{map[string]any{"content": "  ", "status": "pending"}},
	}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustTodoText(t, res), "cannot be empty") {
		t.Errorf("unexpected result: %q", mustTodoText(t, res))
	}
}

func TestExecuteRejectsInvalidStatus(t *testing.T) {
	tool := NewTodoWriteTool(&fakeTodoStore{})
	res, err := tool.Execute(context.Background(), "id", map[string]any{
		"todos": []any{map[string]any{"content": "task", "status": "done"}},
	}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustTodoText(t, res), "invalid todo status") {
		t.Errorf("unexpected result: %q", mustTodoText(t, res))
	}
}

func TestExecuteRejectsMultipleInProgress(t *testing.T) {
	store := &fakeTodoStore{}
	tool := NewTodoWriteTool(store)
	res, err := tool.Execute(context.Background(), "id", map[string]any{
		"todos": []any{
			map[string]any{"content": "task 1", "status": "in_progress"},
			map[string]any{"content": "task 2", "status": "in_progress"},
		},
	}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustTodoText(t, res), "at most one") {
		t.Errorf("unexpected result: %q", mustTodoText(t, res))
	}
	if store.todos != nil {
		t.Error("rejected update should not have mutated the store")
	}
}

func TestExecuteRejectsNonObjectTodoEntry(t *testing.T) {
	tool := NewTodoWriteTool(&fakeTodoStore{})
	res, err := tool.Execute(context.Background(), "id", map[string]any{
		"todos": []any{"not an object"},
	}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustTodoText(t, res), "must be an object") {
		t.Errorf("unexpected result: %q", mustTodoText(t, res))
	}
}

func TestExecuteEmptyTodosListClearsStore(t *testing.T) {
	store := &fakeTodoStore{todos: []TodoItem{{Content: "old", Status: "pending"}}}
	tool := NewTodoWriteTool(store)
	res, err := tool.Execute(context.Background(), "id", map[string]any{"todos": []any{}}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustTodoText(t, res), "0 item") {
		t.Errorf("unexpected result: %q", mustTodoText(t, res))
	}
	if len(store.todos) != 0 {
		t.Errorf("expected the todo list to be cleared, got %+v", store.todos)
	}
}
