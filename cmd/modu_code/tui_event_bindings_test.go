package main

import (
	"testing"

	"github.com/openmodu/modu/pkg/types"
)

func TestModuTUITodoRefreshEvent(t *testing.T) {
	tests := []struct {
		name string
		ev   types.Event
		want bool
	}{
		{
			name: "plan approval seeds todos",
			ev:   types.Event{Type: types.EventTypeToolExecutionEnd, ToolName: "exit_plan_mode"},
			want: true,
		},
		{
			name: "todo write updates todos",
			ev:   types.Event{Type: types.EventTypeToolExecutionEnd, ToolName: "todo_write"},
			want: true,
		},
		{
			name: "unrelated tool does not revive stale todos",
			ev:   types.Event{Type: types.EventTypeToolExecutionEnd, ToolName: "read"},
		},
		{
			name: "todo tool start has not updated state",
			ev:   types.Event{Type: types.EventTypeToolExecutionStart, ToolName: "todo_write"},
		},
		{
			name: "failed todo write has not updated state",
			ev:   types.Event{Type: types.EventTypeToolExecutionEnd, ToolName: "todo_write", IsError: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := moduTUITodoRefreshEvent(tt.ev); got != tt.want {
				t.Fatalf("moduTUITodoRefreshEvent(%#v) = %v, want %v", tt.ev, got, tt.want)
			}
		})
	}
}
