package backendtask

import (
	"context"
	"strings"
	"testing"

	"github.com/openmodu/modu/pkg/coding_agent/foundation/taskoutput"
	"github.com/openmodu/modu/pkg/types"
)

type fakeStore struct {
	tasks map[string]taskoutput.Task
}

func (s *fakeStore) Create(kind, summary string) string { return "" }
func (s *fakeStore) Complete(id, output string)         {}
func (s *fakeStore) Fail(id, errMsg string)             {}
func (s *fakeStore) Get(id string) (taskoutput.Task, bool) {
	task, ok := s.tasks[id]
	return task, ok
}
func (s *fakeStore) List() []taskoutput.Task {
	out := make([]taskoutput.Task, 0, len(s.tasks))
	for _, task := range s.tasks {
		out = append(out, task)
	}
	return out
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

func TestTaskOutputToolBasics(t *testing.T) {
	tool := NewTaskOutputTool(&fakeStore{})
	if tool.Name() != "task_output" {
		t.Errorf("Name() = %q, want task_output", tool.Name())
	}
}

func TestExecuteRequiresStore(t *testing.T) {
	tool := NewTaskOutputTool(nil)
	res, err := tool.Execute(context.Background(), "id", nil, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "not configured") {
		t.Errorf("unexpected result: %q", mustText(t, res))
	}
}

func TestExecuteListsAllTasksSortedByID(t *testing.T) {
	store := &fakeStore{tasks: map[string]taskoutput.Task{
		"task-2": {ID: "task-2", Status: "running", Summary: "second"},
		"task-1": {ID: "task-1", Status: "completed", Summary: "first"},
	}}
	tool := NewTaskOutputTool(store)
	res, err := tool.Execute(context.Background(), "id", nil, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	got := mustText(t, res)
	i1 := strings.Index(got, "task-1")
	i2 := strings.Index(got, "task-2")
	if i1 < 0 || i2 < 0 || i1 > i2 {
		t.Errorf("expected task-1 before task-2 in sorted output, got:\n%s", got)
	}
}

func TestExecuteNoTasks(t *testing.T) {
	tool := NewTaskOutputTool(&fakeStore{})
	res, err := tool.Execute(context.Background(), "id", nil, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "no background tasks") {
		t.Errorf("unexpected result: %q", mustText(t, res))
	}
}

func TestExecuteFetchesSpecificTask(t *testing.T) {
	store := &fakeStore{tasks: map[string]taskoutput.Task{
		"task-1": {
			ID: "task-1", Kind: "subagent", Status: "completed", Summary: "did a thing",
			Agent: "reviewer", Output: "the result",
		},
	}}
	tool := NewTaskOutputTool(store)
	res, err := tool.Execute(context.Background(), "id", map[string]any{"task_id": "task-1"}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	got := mustText(t, res)
	for _, want := range []string{"task-1", "subagent", "completed", "did a thing", "reviewer", "the result"} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q, got:\n%s", want, got)
		}
	}
}

func TestExecuteTaskNotFound(t *testing.T) {
	tool := NewTaskOutputTool(&fakeStore{})
	res, err := tool.Execute(context.Background(), "id", map[string]any{"task_id": "missing"}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "not found") {
		t.Errorf("unexpected result: %q", mustText(t, res))
	}
}

func TestExecuteIncludesErrorField(t *testing.T) {
	store := &fakeStore{tasks: map[string]taskoutput.Task{
		"task-1": {ID: "task-1", Status: "failed", Error: "something broke"},
	}}
	tool := NewTaskOutputTool(store)
	res, err := tool.Execute(context.Background(), "id", map[string]any{"task_id": "task-1"}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "something broke") {
		t.Errorf("unexpected result: %q", mustText(t, res))
	}
}
