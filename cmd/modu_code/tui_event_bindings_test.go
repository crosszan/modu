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

func TestModuTUIClaimLiveTextEntry(t *testing.T) {
	t.Run("stamps the first markdown text entry", func(t *testing.T) {
		entries := []modutui.Entry{
			{Role: modutui.RoleAssistant, Nodes: []modutui.Node{modutui.ThinkingNode{Text: "thinking"}}},
			{Role: modutui.RoleAssistant, Nodes: []modutui.Node{modutui.MarkdownNode{Text: "final reply"}}},
			{Role: modutui.RoleAssistant, Nodes: []modutui.Node{modutui.ToolNode{}}},
		}
		idx := moduTUIClaimLiveTextEntry(entries, "live-1")
		if idx != 1 {
			t.Fatalf("idx = %d, want 1", idx)
		}
		if entries[1].ID != "live-1" {
			t.Fatalf("claimed entry ID = %q, want live-1", entries[1].ID)
		}
		if entries[0].ID != "" || entries[2].ID != "" {
			t.Fatalf("only the text entry should be stamped, got %#v", entries)
		}
	})

	t.Run("empty liveID claims nothing", func(t *testing.T) {
		entries := []modutui.Entry{
			{Role: modutui.RoleAssistant, Nodes: []modutui.Node{modutui.MarkdownNode{Text: "final reply"}}},
		}
		if idx := moduTUIClaimLiveTextEntry(entries, ""); idx != -1 {
			t.Fatalf("idx = %d, want -1 when liveID is empty", idx)
		}
	})

	t.Run("no markdown text entry claims nothing", func(t *testing.T) {
		entries := []modutui.Entry{
			{Role: modutui.RoleAssistant, Nodes: []modutui.Node{modutui.ToolNode{}}},
		}
		if idx := moduTUIClaimLiveTextEntry(entries, "live-1"); idx != -1 {
			t.Fatalf("idx = %d, want -1 when no entry matches", idx)
		}
	})
}
