package modutui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestQueuedBlockIsEmptyWithNothingPending(t *testing.T) {
	if lines := (QueuedBlock{}).RenderWidth(80); lines != nil {
		t.Fatalf("an empty queue should render nothing, got %#v", lines)
	}
}

func TestQueuedBlockListsMessagesWithACount(t *testing.T) {
	lines := QueuedBlock{Messages: []string{"跑测试", "然后提交"}}.RenderWidth(80)
	rendered := ansi.Strip(strings.Join(lines, "\n"))
	if !strings.Contains(rendered, "已排队 2 条") {
		t.Fatalf("header should count pending messages:\n%s", rendered)
	}
	for _, message := range []string{"跑测试", "然后提交"} {
		if !strings.Contains(rendered, message) {
			t.Fatalf("message %q missing:\n%s", message, rendered)
		}
	}
	if !strings.Contains(rendered, "Backspace") {
		t.Fatalf("header should say how to take a message back:\n%s", rendered)
	}
}

func TestQueuedBlockCollapsesBeyondMaxRows(t *testing.T) {
	lines := QueuedBlock{Messages: []string{"a", "b", "c", "d"}, MaxRows: 2}.RenderWidth(40)
	rendered := ansi.Strip(strings.Join(lines, "\n"))
	if !strings.Contains(rendered, "还有 2 条") {
		t.Fatalf("the overflow line should say how many are hidden:\n%s", rendered)
	}
	if strings.Contains(rendered, "c") || strings.Contains(rendered, "d") {
		t.Fatalf("messages past MaxRows should be collapsed:\n%s", rendered)
	}
	// Header + MaxRows entries + the overflow line.
	if len(lines) != 4 {
		t.Fatalf("line count = %d, want 4", len(lines))
	}
}

func TestQueuedBlockFlattensMultilineMessages(t *testing.T) {
	// One row per message keeps the region a reminder of what is pending
	// rather than a second transcript competing with the real one.
	lines := QueuedBlock{Messages: []string{"first line\n\nsecond line"}}.RenderWidth(80)
	if len(lines) != 2 {
		t.Fatalf("a multi-line message should still occupy one row, got %#v", lines)
	}
	if got := ansi.Strip(lines[1]); !strings.Contains(got, "first line second line") {
		t.Fatalf("newlines should collapse to spaces, got %q", got)
	}
}
