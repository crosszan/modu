package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	coding_agent "github.com/openmodu/modu/pkg/coding_agent"
	modutui "github.com/openmodu/modu/pkg/modu-tui"
)

// screen drives a real Model the way the runner does — feeding it the same
// UpdateMsg the client dispatches — and returns the rendered terminal content
// with styling stripped. Asserting on this proves the block's shape on screen,
// which asserting on Entry contents alone does not.
func screen(t *testing.T, entries ...modutui.Entry) []string {
	t.Helper()
	var m tea.Model = modutui.NewModel(modutui.Options{Width: 72, Height: 24})
	for _, entry := range entries {
		m, _ = m.Update(modutui.UpdateMsg{Update: modutui.UpsertEntryUpdate{Entry: entry}})
	}
	view := m.(modutui.Model).View()
	var out []string
	for _, line := range strings.Split(ansi.Strip(view.Content), "\n") {
		if trimmed := strings.TrimRight(line, " "); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func lineWith(lines []string, want string) (int, bool) {
	for i, line := range lines {
		if strings.Contains(line, want) {
			return i, true
		}
	}
	return 0, false
}

// The block must land on screen as a nested shape: the agent header carries
// the assistant marker, and the child's tool calls indent underneath it.
func TestSubagentBlockRendersNested(t *testing.T) {
	activity := NewSubagentActivity()
	activity.now = fixedClock(time.Unix(0, 0))
	activity.HandleSessionEvent(coding_agent.SessionEvent{
		Type:           coding_agent.SessionEventSubagentStart,
		SubagentTaskID: "task-3",
		SubagentName:   "explorer",
		SubagentLabel:  "map auth flow",
	})
	var entry modutui.Entry
	for _, tc := range [][2]string{{"read", "pkg/auth/handler.go"}, {"grep", "login"}} {
		entry, _ = activity.HandleSessionEvent(coding_agent.SessionEvent{
			Type:           coding_agent.SessionEventSubagentProgress,
			Reason:         "tool",
			SubagentTaskID: "task-3",
			ToolName:       tc[0],
			Message:        tc[1],
		})
	}

	lines := screen(t, entry)
	header, ok := lineWith(lines, "explorer · map auth flow")
	if !ok {
		t.Fatalf("header not on screen:\n%s", strings.Join(lines, "\n"))
	}
	if !strings.HasPrefix(lines[header], "● ") {
		t.Errorf("header should carry the assistant marker, got %q", lines[header])
	}
	first, ok := lineWith(lines, "read(pkg/auth/handler.go)")
	if !ok {
		t.Fatalf("first tool line not on screen:\n%s", strings.Join(lines, "\n"))
	}
	if !strings.HasPrefix(lines[first], "  ⎿ ") {
		t.Errorf("first tool line should open the nested run, got %q", lines[first])
	}
	second, _ := lineWith(lines, "grep(login)")
	if !strings.HasPrefix(lines[second], "    ") {
		t.Errorf("later tool lines should align under the first, got %q", lines[second])
	}
	if second != first+1 {
		t.Errorf("tool lines should be consecutive, got rows %d and %d", first, second)
	}
}

// Every update rewrites the same row. Without this the run would append a new
// block per tool call and bury the conversation — the property the whole
// upsert-by-run-id design exists for.
func TestSubagentBlockReplacesInPlaceOnScreen(t *testing.T) {
	activity := NewSubagentActivity()
	activity.now = fixedClock(time.Unix(0, 0))
	activity.HandleSessionEvent(coding_agent.SessionEvent{
		Type:           coding_agent.SessionEventSubagentStart,
		SubagentTaskID: "task-3",
		SubagentName:   "explorer",
		SubagentLabel:  "map auth flow",
	})

	var updates []modutui.Entry
	for _, name := range []string{"read", "grep", "bash"} {
		entry, _ := activity.HandleSessionEvent(coding_agent.SessionEvent{
			Type:           coding_agent.SessionEventSubagentProgress,
			Reason:         "tool",
			SubagentTaskID: "task-3",
			ToolName:       name,
		})
		updates = append(updates, entry)
	}
	final, _ := activity.HandleSessionEvent(coding_agent.SessionEvent{
		Type:               coding_agent.SessionEventSubagentStop,
		SubagentTaskID:     "task-3",
		SubagentName:       "explorer",
		SubagentTurns:      3,
		SubagentTokens:     12400,
		SubagentDurationMs: 8200,
	})
	updates = append(updates, final)

	lines := screen(t, updates...)
	if got := strings.Count(strings.Join(lines, "\n"), "explorer · map auth flow"); got != 1 {
		t.Fatalf("header appears %d times, want 1 — updates are appending, not replacing:\n%s",
			got, strings.Join(lines, "\n"))
	}
	if _, ok := lineWith(lines, "Done (3 turns · 12.4K tokens · 8s)"); !ok {
		t.Fatalf("closing tally not on screen:\n%s", strings.Join(lines, "\n"))
	}
	// The superseded running footer must be gone, not left above the final one.
	if _, ok := lineWith(lines, "running ("); ok {
		t.Fatalf("stale running footer still on screen:\n%s", strings.Join(lines, "\n"))
	}
}

// A run's block must not crowd out the conversation around it.
func TestSubagentBlockStaysBoundedOnScreen(t *testing.T) {
	activity := NewSubagentActivity()
	activity.now = fixedClock(time.Unix(0, 0))
	activity.HandleSessionEvent(coding_agent.SessionEvent{
		Type:           coding_agent.SessionEventSubagentStart,
		SubagentTaskID: "task-9",
		SubagentName:   "explorer",
	})
	var entry modutui.Entry
	for i := 0; i < 40; i++ {
		entry, _ = activity.HandleSessionEvent(coding_agent.SessionEvent{
			Type:           coding_agent.SessionEventSubagentProgress,
			Reason:         "tool",
			SubagentTaskID: "task-9",
			ToolName:       "read",
			Message:        "file.go",
		})
	}

	lines := screen(t, entry)
	block := 0
	for _, line := range lines {
		if strings.Contains(line, "explorer") || strings.Contains(line, "read(file.go)") ||
			strings.Contains(line, "earlier") || strings.Contains(line, "running (") {
			block++
		}
	}
	// header + collapsed count + 5 tool lines + footer.
	if block > maxSubagentToolLines+3 {
		t.Fatalf("40 tool calls rendered %d block rows:\n%s", block, strings.Join(lines, "\n"))
	}
	if _, ok := lineWith(lines, "+35 earlier"); !ok {
		t.Fatalf("collapsed count missing:\n%s", strings.Join(lines, "\n"))
	}
}
