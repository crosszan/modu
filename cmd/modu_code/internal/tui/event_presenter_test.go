package tui

import (
	"strings"
	"testing"

	coding_agent "github.com/openmodu/modu/pkg/coding_agent"
	modutui "github.com/openmodu/modu/pkg/modu-tui"
	"github.com/openmodu/modu/pkg/types"
)

type toolNodePresenterStub struct {
	eventCalls  int
	callCalls   int
	resultCalls int
}

func (s *toolNodePresenterStub) EventNode(event types.Event, _ string) (modutui.ToolNode, bool) {
	s.eventCalls++
	return modutui.ToolNode{Call: modutui.ToolCall{ID: event.ToolCallID, Name: event.ToolName}}, true
}

func (s *toolNodePresenterStub) CallNode(call *types.ToolCallContent, _ string) modutui.ToolNode {
	s.callCalls++
	return modutui.ToolNode{Call: modutui.ToolCall{ID: call.ID, Name: call.Name}}
}

func (s *toolNodePresenterStub) ResultNode(result types.ToolResultMessage, _ string) modutui.ToolNode {
	s.resultCalls++
	return modutui.ToolNode{Call: modutui.ToolCall{ID: result.ToolCallID, Name: result.ToolName, Done: true}}
}

func TestEventPresenterSkipsSubmittedUserMessageEnd(t *testing.T) {
	presenter := NewEventPresenter(&toolNodePresenterStub{}, "compact")
	got := presenter.AgentEvent(types.Event{
		Type:    types.EventTypeMessageEnd,
		Message: types.UserMessage{Role: types.RoleUser, Content: "hello"},
	}, "")
	if len(got) != 0 {
		t.Fatalf("entries = %#v", got)
	}
}

func TestEventPresenterGroupsThinkingBeforeAssistantContent(t *testing.T) {
	tools := &toolNodePresenterStub{}
	presenter := NewEventPresenter(tools, "compact")
	got := presenter.AgentMessage(types.AssistantMessage{
		Role: types.RoleAssistant,
		Content: []types.ContentBlock{
			&types.TextContent{Type: "text", Text: "answer"},
			&types.ThinkingContent{Type: "thinking", Thinking: "first"},
			&types.ToolCallContent{Type: "toolCall", ID: "call-1", Name: "read"},
			&types.ThinkingContent{Type: "thinking", Thinking: "second"},
		},
	}, "")

	if len(got) != 3 {
		t.Fatalf("entries = %#v", got)
	}
	thinking, ok := got[0].Nodes[0].(modutui.ThinkingNode)
	if !ok || !strings.Contains(thinking.Text, "first") || !strings.Contains(thinking.Text, "second") {
		t.Fatalf("thinking entry = %#v", got[0])
	}
	if text, ok := got[1].Nodes[0].(modutui.MarkdownNode); !ok || text.Text != "answer" {
		t.Fatalf("answer entry = %#v", got[1])
	}
	if tool, ok := got[2].Nodes[0].(modutui.ToolNode); !ok || tool.Call.ID != "call-1" {
		t.Fatalf("tool entry = %#v", got[2])
	}
	if tools.callCalls != 1 {
		t.Fatalf("tool calls = %d", tools.callCalls)
	}
}

func TestEventPresenterDelegatesToolLifecycle(t *testing.T) {
	tools := &toolNodePresenterStub{}
	presenter := NewEventPresenter(tools, "compact")

	started := presenter.AgentEvent(types.Event{
		Type:       types.EventTypeToolExecutionStart,
		ToolCallID: "call-1",
		ToolName:   "bash",
	}, "")
	result := presenter.AgentMessage(types.ToolResultMessage{
		Role:       types.RoleToolResult,
		ToolCallID: "call-1",
		ToolName:   "bash",
	}, "")

	if len(started) != 1 || started[0].ID != "call-1" || len(result) != 1 || result[0].ID != "call-1" {
		t.Fatalf("started = %#v, result = %#v", started, result)
	}
	if tools.eventCalls != 1 || tools.resultCalls != 1 {
		t.Fatalf("event calls = %d, result calls = %d", tools.eventCalls, tools.resultCalls)
	}
}

func TestEventPresenterMapsSessionEvents(t *testing.T) {
	presenter := NewEventPresenter(nil, "------------- compact -------------")

	denied, ok := presenter.SessionEvent(coding_agent.SessionEvent{
		Type:     coding_agent.SessionEventPermissionDeny,
		ToolName: "bash",
		Reason:   "dangerous command",
	})
	if !ok {
		t.Fatal("permission event was not presented")
	}
	// Lifecycle lines are the TUI's own status output, not model output: they
	// must use RoleStatus (dim "·" marker, not the assistant "●") and a
	// TextNode, so an embedded newline like this one survives instead of
	// being collapsed by markdown into a single run-on line.
	if denied.Role != modutui.RoleStatus {
		t.Fatalf("session lifecycle entry should use RoleStatus, got %#v", denied)
	}
	node, ok := denied.Nodes[0].(modutui.TextNode)
	if !ok {
		t.Fatalf("session lifecycle entry should be a TextNode, got %T", denied.Nodes[0])
	}
	if !strings.Contains(node.Text, "bash") || !strings.Contains(node.Text, "dangerous command") {
		t.Fatalf("permission entry = %#v", denied)
	}
	if !strings.Contains(node.Text, "\n") {
		t.Fatalf("the reason should stay on its own line, got %q", node.Text)
	}

	compact, ok := presenter.SessionEvent(coding_agent.SessionEvent{
		Type: coding_agent.SessionEventCompactionDone,
	})
	if !ok || !compact.Plain {
		t.Fatalf("compact entry = %#v, %v", compact, ok)
	}
	compactNode, ok := compact.Nodes[0].(modutui.TextNode)
	if !ok || compactNode.Text != "------------- compact -------------" {
		t.Fatalf("compact node = %#v", compact.Nodes)
	}
}

func TestEventPresenterCompactsSubagentLifecycle(t *testing.T) {
	presenter := NewEventPresenter(nil, "")

	start, ok := presenter.SessionEvent(coding_agent.SessionEvent{
		Type:         coding_agent.SessionEventSubagentStart,
		SubagentName: "goal-verifier",
		SubagentTask: "The agent claims the following objective is complete:\nline one\nline two\n" + strings.Repeat("x", 400),
	})
	if !ok {
		t.Fatal("subagent start not presented")
	}
	startText := start.Nodes[0].(modutui.TextNode).Text
	if !strings.Contains(startText, "subagent start: goal-verifier") {
		t.Fatalf("start missing name: %q", startText)
	}
	if strings.Contains(startText, "\n") {
		t.Fatalf("start should be a single line, got:\n%s", startText)
	}
	if len([]rune(startText)) > 140 {
		t.Fatalf("start not truncated (%d runes): %q", len([]rune(startText)), startText)
	}

	stop, _ := presenter.SessionEvent(coding_agent.SessionEvent{
		Type:         coding_agent.SessionEventSubagentStop,
		SubagentName: "goal-verifier",
		ErrorMessage: "subagent \"goal-verifier\": subagent exceeded max_turns=20",
	})
	stopText := stop.Nodes[0].(modutui.TextNode).Text
	if strings.Contains(stopText, "\n") {
		t.Fatalf("stop should be a single line, got:\n%s", stopText)
	}
	if !strings.Contains(stopText, "error:") || !strings.Contains(stopText, "max_turns=20") {
		t.Fatalf("stop should keep the error: %q", stopText)
	}
}

func TestEventPresenterSubagentClosingStats(t *testing.T) {
	presenter := NewEventPresenter(nil, "")

	start, ok := presenter.SessionEvent(coding_agent.SessionEvent{
		Type:          coding_agent.SessionEventSubagentStart,
		SubagentName:  "explorer",
		SubagentLabel: "map auth flow",
		SubagentTask:  "Read every file under pkg/auth and summarise the login path",
	})
	if !ok {
		t.Fatal("subagent start not presented")
	}
	startText := start.Nodes[0].(modutui.TextNode).Text
	// The short label wins over the raw task text.
	if !strings.Contains(startText, "map auth flow") || strings.Contains(startText, "Read every file") {
		t.Fatalf("start should use the label: %q", startText)
	}

	stop, _ := presenter.SessionEvent(coding_agent.SessionEvent{
		Type:               coding_agent.SessionEventSubagentStop,
		SubagentName:       "explorer",
		SubagentLabel:      "map auth flow",
		SubagentResult:     "the login path starts in handler.go",
		SubagentTurns:      3,
		SubagentTokens:     12400,
		SubagentDurationMs: 8200,
	})
	stopText := stop.Nodes[0].(modutui.TextNode).Text
	if !strings.Contains(stopText, "Done (3 turns · 12.4K tokens · 8s)") {
		t.Fatalf("stop missing closing stats: %q", stopText)
	}
	if strings.Contains(stopText, "\n") {
		t.Fatalf("stop should be a single line, got:\n%s", stopText)
	}
}

func TestSubagentRunStatsTextOmitsMissingFigures(t *testing.T) {
	if got := subagentRunStatsText(coding_agent.SessionEvent{}); got != "" {
		t.Fatalf("empty run should render no stats, got %q", got)
	}
	got := subagentRunStatsText(coding_agent.SessionEvent{SubagentTurns: 1, SubagentDurationMs: 125_000})
	if got != "(1 turn · 2m5s)" {
		t.Fatalf("stats = %q, want (1 turn · 2m5s)", got)
	}
}
