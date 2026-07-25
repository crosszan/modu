package tui

import (
	"strings"
	"testing"
	"time"

	coding_agent "github.com/openmodu/modu/pkg/coding_agent"
	modutui "github.com/openmodu/modu/pkg/modu-tui"
)

func entryText(t *testing.T, entry modutui.Entry) string {
	t.Helper()
	if len(entry.Nodes) != 1 {
		t.Fatalf("expected one node, got %d", len(entry.Nodes))
	}
	node, ok := entry.Nodes[0].(modutui.TextNode)
	if !ok {
		t.Fatalf("expected a TextNode, got %T", entry.Nodes[0])
	}
	return node.Text
}

func fixedClock(start time.Time) func() time.Time {
	elapsed := 0
	return func() time.Time {
		elapsed++
		return start.Add(time.Duration(elapsed) * time.Second)
	}
}

func TestSubagentActivityRendersOneBlockPerRun(t *testing.T) {
	activity := NewSubagentActivity()
	activity.now = fixedClock(time.Unix(0, 0))

	start, ok := activity.HandleSessionEvent(coding_agent.SessionEvent{
		Type:           coding_agent.SessionEventSubagentStart,
		SubagentTaskID: "task-3",
		SubagentName:   "explorer",
		SubagentLabel:  "map auth flow",
		SubagentTask:   "Read every file under pkg/auth",
	})
	if !ok {
		t.Fatal("start not handled")
	}
	if start.ID != "subagent:task-3" {
		t.Fatalf("entry ID = %q, want subagent:task-3", start.ID)
	}
	if got := entryText(t, start); !strings.HasPrefix(got, "explorer · map auth flow") {
		t.Fatalf("header = %q", got)
	}

	for _, tool := range []struct{ name, detail string }{
		{"read", "pkg/auth/handler.go"},
		{"grep", "login"},
	} {
		if _, ok = activity.HandleSessionEvent(coding_agent.SessionEvent{
			Type:           coding_agent.SessionEventSubagentProgress,
			Reason:         "tool",
			SubagentTaskID: "task-3",
			ToolName:       tool.name,
			Message:        tool.detail,
		}); !ok {
			t.Fatal("tool progress not handled")
		}
	}
	turn, _ := activity.HandleSessionEvent(coding_agent.SessionEvent{
		Type:           coding_agent.SessionEventSubagentProgress,
		Reason:         "turn",
		SubagentTaskID: "task-3",
		SubagentTokens: 5100,
	})
	running := entryText(t, turn)
	for _, want := range []string{
		"explorer · map auth flow",
		"⎿ read(pkg/auth/handler.go)",
		"  grep(login)",
		"running (1 turn · 5.1K tokens ·",
	} {
		if !strings.Contains(running, want) {
			t.Errorf("running block missing %q:\n%s", want, running)
		}
	}

	stop, _ := activity.HandleSessionEvent(coding_agent.SessionEvent{
		Type:               coding_agent.SessionEventSubagentStop,
		SubagentTaskID:     "task-3",
		SubagentName:       "explorer",
		SubagentTurns:      3,
		SubagentTokens:     12400,
		SubagentDurationMs: 8200,
	})
	if stop.ID != start.ID {
		t.Fatalf("stop entry ID = %q, want the same block %q", stop.ID, start.ID)
	}
	final := entryText(t, stop)
	if !strings.Contains(final, "Done (3 turns · 12.4K tokens · 8s)") {
		t.Fatalf("final block missing closing tally:\n%s", final)
	}
	// The tool lines survive into the finished block.
	if !strings.Contains(final, "grep(login)") {
		t.Errorf("final block dropped the tool lines:\n%s", final)
	}
	// A finished run stops being tracked.
	if len(activity.runs) != 0 {
		t.Errorf("run map not cleaned up: %v", activity.runs)
	}
}

func TestSubagentActivityCollapsesOldToolLines(t *testing.T) {
	activity := NewSubagentActivity()
	activity.now = fixedClock(time.Unix(0, 0))
	activity.HandleSessionEvent(coding_agent.SessionEvent{
		Type:           coding_agent.SessionEventSubagentStart,
		SubagentTaskID: "task-4",
		SubagentName:   "explorer",
	})

	var entry modutui.Entry
	for _, name := range []string{"t1", "t2", "t3", "t4", "t5", "t6", "t7"} {
		entry, _ = activity.HandleSessionEvent(coding_agent.SessionEvent{
			Type:           coding_agent.SessionEventSubagentProgress,
			Reason:         "tool",
			SubagentTaskID: "task-4",
			ToolName:       name,
		})
	}

	text := entryText(t, entry)
	if !strings.Contains(text, "⎿ … +2 earlier") {
		t.Fatalf("expected the older calls to collapse:\n%s", text)
	}
	if strings.Contains(text, "t1") || strings.Contains(text, "t2") {
		t.Errorf("collapsed calls should not still be listed:\n%s", text)
	}
	if !strings.Contains(text, "t7") {
		t.Errorf("newest call missing:\n%s", text)
	}
	if lines := strings.Count(text, "\n"); lines > maxSubagentToolLines+2 {
		t.Errorf("block grew to %d lines:\n%s", lines+1, text)
	}
}

func TestSubagentActivityMarksFailuresAndErrors(t *testing.T) {
	activity := NewSubagentActivity()
	activity.now = fixedClock(time.Unix(0, 0))
	activity.HandleSessionEvent(coding_agent.SessionEvent{
		Type:           coding_agent.SessionEventSubagentStart,
		SubagentTaskID: "task-5",
		SubagentName:   "builder",
	})
	entry, _ := activity.HandleSessionEvent(coding_agent.SessionEvent{
		Type:           coding_agent.SessionEventSubagentProgress,
		Reason:         "tool",
		SubagentTaskID: "task-5",
		ToolName:       "bash",
		Message:        "go build ./...",
		ErrorMessage:   "failed",
	})
	if got := entryText(t, entry); !strings.Contains(got, "bash(go build ./...) ✗") {
		t.Fatalf("failed tool not marked:\n%s", got)
	}

	stop, _ := activity.HandleSessionEvent(coding_agent.SessionEvent{
		Type:           coding_agent.SessionEventSubagentStop,
		SubagentTaskID: "task-5",
		SubagentName:   "builder",
		ErrorMessage:   "subagent \"builder\": exceeded max_turns=20",
	})
	final := entryText(t, stop)
	if !strings.Contains(final, "error: subagent \"builder\": exceeded max_turns=20") {
		t.Fatalf("stop error missing:\n%s", final)
	}
	if strings.Contains(final, "Done") {
		t.Errorf("a failed run should not claim Done:\n%s", final)
	}
}

// A run already in flight when the UI attaches still gets a block rather than
// being dropped for want of a start event.
func TestSubagentActivityAdoptsRunWithoutStart(t *testing.T) {
	activity := NewSubagentActivity()
	activity.now = fixedClock(time.Unix(0, 0))
	entry, ok := activity.HandleSessionEvent(coding_agent.SessionEvent{
		Type:           coding_agent.SessionEventSubagentProgress,
		Reason:         "tool",
		SubagentTaskID: "task-6",
		SubagentName:   "explorer",
		ToolName:       "read",
		Message:        "main.go",
	})
	if !ok {
		t.Fatal("orphan progress not handled")
	}
	if got := entryText(t, entry); !strings.Contains(got, "explorer") || !strings.Contains(got, "read(main.go)") {
		t.Fatalf("adopted block missing content:\n%s", got)
	}
}

func TestSubagentActivityIgnoresUnrelatedEvents(t *testing.T) {
	activity := NewSubagentActivity()
	if _, ok := activity.HandleSessionEvent(coding_agent.SessionEvent{Type: coding_agent.SessionEventModelChange}); ok {
		t.Error("non-subagent event should fall through to the normal presenter")
	}
	// No run id means there is nothing to key a block on.
	if _, ok := activity.HandleSessionEvent(coding_agent.SessionEvent{
		Type:         coding_agent.SessionEventSubagentStart,
		SubagentName: "explorer",
	}); ok {
		t.Error("event without a run id should fall through")
	}
}
