package main

import (
	"testing"

	modutui "github.com/openmodu/modu/pkg/modu-tui"
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

func TestModuTUIAssistantMessage(t *testing.T) {
	value := types.AssistantMessage{Role: "assistant", Model: "test-model"}

	if got, ok := moduTUIAssistantMessage(value); !ok || got.Model != "test-model" {
		t.Fatalf("value AssistantMessage: got %#v, %v", got, ok)
	}
	if got, ok := moduTUIAssistantMessage(&value); !ok || got.Model != "test-model" {
		t.Fatalf("pointer AssistantMessage: got %#v, %v", got, ok)
	}
	if _, ok := moduTUIAssistantMessage((*types.AssistantMessage)(nil)); ok {
		t.Fatal("nil *AssistantMessage should not be ok")
	}
	if _, ok := moduTUIAssistantMessage(types.UserMessage{}); ok {
		t.Fatal("non-assistant message should not be ok")
	}
}

func TestModuTUILiveAssistantTextEntry(t *testing.T) {
	t.Run("joins text blocks and skips other content", func(t *testing.T) {
		message := types.AssistantMessage{Content: []types.ContentBlock{
			&types.TextContent{Type: "text", Text: "hello"},
			&types.ThinkingContent{Type: "thinking", Thinking: "reasoning, not shown live"},
			&types.TextContent{Type: "text", Text: "world"},
		}}
		entry, ok := moduTUILiveAssistantTextEntry(message)
		if !ok {
			t.Fatal("expected a live entry")
		}
		if !entry.Streaming {
			t.Fatal("live entry should be marked Streaming")
		}
		if len(entry.Nodes) != 1 {
			t.Fatalf("expected a single node, got %d", len(entry.Nodes))
		}
		node, ok := entry.Nodes[0].(modutui.MarkdownNode)
		if !ok {
			t.Fatalf("expected a MarkdownNode (so it renders the same mid-stream as it will once finished), got %T", entry.Nodes[0])
		}
		if node.Text != "hello\n\nworld" {
			t.Fatalf("Text = %q, want joined text blocks only", node.Text)
		}
	})

	t.Run("no text content yields no entry", func(t *testing.T) {
		message := types.AssistantMessage{Content: []types.ContentBlock{
			&types.ThinkingContent{Type: "thinking", Thinking: "still thinking"},
		}}
		if _, ok := moduTUILiveAssistantTextEntry(message); ok {
			t.Fatal("a message with no text content should not produce a live entry")
		}
	})
}

func TestModuTUIFinalizeStreamedMessage(t *testing.T) {
	t.Run("removes the placeholder before appending, so thinking lands before text", func(t *testing.T) {
		var dispatched []any
		client := modutui.NewClient(func(msg any) { dispatched = append(dispatched, msg) })

		thinking := modutui.Entry{Role: modutui.RoleAssistant, Nodes: []modutui.Node{modutui.ThinkingNode{Text: "reasoning"}}}
		text := modutui.Entry{Role: modutui.RoleAssistant, Nodes: []modutui.Node{modutui.MarkdownNode{Text: "final reply"}}}
		moduTUIFinalizeStreamedMessage(client, "live-1", []modutui.Entry{thinking, text})

		if len(dispatched) != 3 {
			t.Fatalf("expected remove + 2 appends, got %d dispatched updates: %#v", len(dispatched), dispatched)
		}
		remove, ok := dispatched[0].(modutui.UpdateMsg).Update.(modutui.RemoveEntryUpdate)
		if !ok || remove.ID != "live-1" {
			t.Fatalf("first update should remove the live placeholder, got %#v", dispatched[0])
		}
		firstAppend, ok := dispatched[1].(modutui.UpdateMsg).Update.(modutui.AppendEntryUpdate)
		if !ok || len(firstAppend.Entry.Nodes) == 0 {
			t.Fatalf("second update should be an append, got %#v", dispatched[1])
		}
		if _, ok := firstAppend.Entry.Nodes[0].(modutui.ThinkingNode); !ok {
			t.Fatalf("thinking must be appended before text, got node %T first", firstAppend.Entry.Nodes[0])
		}
		secondAppend, ok := dispatched[2].(modutui.UpdateMsg).Update.(modutui.AppendEntryUpdate)
		if !ok {
			t.Fatalf("third update should be an append, got %#v", dispatched[2])
		}
		if _, ok := secondAppend.Entry.Nodes[0].(modutui.MarkdownNode); !ok {
			t.Fatalf("text must be appended after thinking, got node %T second", secondAppend.Entry.Nodes[0])
		}
	})

	t.Run("no placeholder to remove when the message never streamed text", func(t *testing.T) {
		var dispatched []any
		client := modutui.NewClient(func(msg any) { dispatched = append(dispatched, msg) })

		moduTUIFinalizeStreamedMessage(client, "", []modutui.Entry{
			{Role: modutui.RoleAssistant, Nodes: []modutui.Node{modutui.ToolNode{}}},
		})

		if len(dispatched) != 1 {
			t.Fatalf("expected exactly one append (no remove), got %d: %#v", len(dispatched), dispatched)
		}
		if _, ok := dispatched[0].(modutui.UpdateMsg).Update.(modutui.RemoveEntryUpdate); ok {
			t.Fatal("should not dispatch a remove when liveTextID is empty")
		}
	})
}
