package modutui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestCustomBlockFactoryOverridesEntryRendering(t *testing.T) {
	m := NewModel(Options{
		Width:  40,
		Height: 8,
		InitialEntries: []Entry{
			{Role: RoleAssistant, Nodes: []Node{TextNode{Text: "original"}}},
		},
		BlockFactories: []EntryBlockFactory{
			func(entry Entry) (Block, bool) {
				text := entry.Nodes[0].(TextNode).Text
				return TextBlock{Marker: "X ", Text: "factory " + text}, true
			},
		},
	})
	got := strings.Join(m.Lines(), "\n")
	if !strings.Contains(got, "factory original") {
		t.Fatalf("custom block factory was not used:\n%s", got)
	}
}

func TestDefaultAssistantMarkerIsWhite(t *testing.T) {
	if got, want := assistantMarkerStyle.GetForeground(), lipgloss.Color("231"); got != want {
		t.Fatalf("assistant marker foreground = %#v, want %#v", got, want)
	}
}

func TestPlainEntryRendersWithoutMarker(t *testing.T) {
	m := NewModel(Options{
		Width:  40,
		Height: 8,
		InitialEntries: []Entry{
			{Role: RoleAssistant, Nodes: []Node{TextNode{Text: "✓ Completed (2s)"}}, Plain: true},
		},
	})
	got := strings.Join(m.Lines(), "\n")
	if !strings.Contains(got, "✓ Completed (2s)") {
		t.Fatalf("plain message missing text:\n%s", got)
	}
	if strings.Contains(got, "● ✓ Completed") {
		t.Fatalf("plain message should not render assistant marker:\n%s", got)
	}
}

func TestEntryMarkerDistinguishesStatusFromAssistant(t *testing.T) {
	tests := []struct {
		name  string
		entry Entry
		want  string
	}{
		{"user", Entry{Role: RoleUser}, "❯ "},
		{"assistant", Entry{Role: RoleAssistant}, "● "},
		{"status", Entry{Role: RoleStatus}, "· "},
		{"plain wins over role", Entry{Role: RoleStatus, Plain: true}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ansi.Strip(entryMarker(tt.entry)); got != tt.want {
				t.Fatalf("entryMarker = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStatusEntriesRenderDimmed(t *testing.T) {
	status := defaultBlockFromEntry(Entry{Role: RoleStatus, Nodes: []Node{TextNode{Text: "subagent start"}}})
	group, ok := status.(nodeGroupBlock)
	if !ok {
		t.Fatalf("expected a nodeGroupBlock, got %T", status)
	}
	if !group.Dim {
		t.Fatal("a RoleStatus entry should render dimmed so it reads as chrome, not conversation")
	}

	assistant := defaultBlockFromEntry(Entry{Role: RoleAssistant, Nodes: []Node{TextNode{Text: "reply"}}})
	if assistant.(nodeGroupBlock).Dim {
		t.Fatal("assistant text must not be dimmed")
	}
}

func TestStatusEntryAlignsContinuationLinesUnderTheMarker(t *testing.T) {
	// The reported problem was status lines hugging the left edge while every
	// other message is indented two columns; the fix must also keep an
	// entry's own continuation lines aligned with its first line.
	m := NewModel(Options{Width: 60, Height: 16})
	m.entries = append(m.entries, Entry{
		Role:  RoleStatus,
		Nodes: []Node{TextNode{Text: "btw · temporary side thread\nContinue typing, or use /exit to return."}},
	})
	m.rebuild()

	var got []string
	for _, line := range strings.Split(ansi.Strip(m.render()), "\n") {
		if strings.Contains(line, "btw") || strings.Contains(line, "Continue typing") {
			got = append(got, strings.TrimRight(line, " "))
		}
	}
	if len(got) != 2 {
		t.Fatalf("expected the notice to stay on two lines, got %#v", got)
	}
	if got[0] != "· btw · temporary side thread" {
		t.Fatalf("first line = %q, want the dim marker prefix", got[0])
	}
	if got[1] != "  Continue typing, or use /exit to return." {
		t.Fatalf("continuation = %q, want it aligned under the first line's text", got[1])
	}
}
