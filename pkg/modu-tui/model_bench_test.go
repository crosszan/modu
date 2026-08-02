package modutui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// benchHistoryText is a realistic-ish assistant reply: a few paragraphs, a
// list, and a fenced code block, so glamour has real work to do parsing and
// styling it — not just a one-line string.
func benchHistoryText(i int) string {
	return fmt.Sprintf(`Sure, here's how to do step %d:

1. First, open the config file
2. Then update the following field:

`+"```go\nfunc example%d() int {\n\treturn %d\n}\n```"+`

That should cover it. Let me know if anything else comes up.`, i, i, i)
}

// BenchmarkRebuildLongTranscriptWithStreamingTail simulates the real-world
// shape of a long-running session: N finished exchanges already in the
// transcript, then a reply streaming in on the tail via repeated deltas
// (each delta is an UpsertEntryUpdate on the same entry id, mirroring how
// message_update deltas land). It only touches pre-existing public API
// (NewModel/Options/Entry/UpdateMsg) so the exact same benchmark can run
// unmodified against the pre-cache code for a before/after comparison.
func BenchmarkRebuildLongTranscriptWithStreamingTail(b *testing.B) {
	for _, history := range []int{20, 200, 1000} {
		b.Run(fmt.Sprintf("history=%d", history), func(b *testing.B) {
			initial := make([]Entry, 0, history*2)
			for i := 0; i < history; i++ {
				initial = append(initial,
					Entry{Role: RoleUser, Nodes: []Node{TextNode{Text: fmt.Sprintf("question %d", i)}}},
					Entry{Role: RoleAssistant, Nodes: []Node{MarkdownNode{Text: benchHistoryText(i)}}},
				)
			}
			var model tea.Model = NewModel(Options{Width: 100, Height: 30, InitialEntries: initial})

			b.ResetTimer()
			for n := 0; n < b.N; n++ {
				var text strings.Builder
				for delta := 0; delta < 40; delta++ {
					text.WriteString("token ")
					next, _ := model.Update(UpdateMsg{Update: UpsertEntryUpdate{Entry: Entry{
						ID:    "streaming-reply",
						Role:  RoleAssistant,
						Nodes: []Node{TextNode{Text: text.String()}},
					}}})
					model = next
				}
			}
		})
	}
}

// BenchmarkStreamingDeltaRenderCost isolates the other half of the
// optimization: rendering a growing streaming reply as plain text (current
// behavior) versus running it through the full markdown/glamour pipeline on
// every delta (what naively wiring message_update straight to a
// MarkdownNode would have cost). Both paths exist in the current code, so
// this comparison doesn't need an old/new checkout.
func BenchmarkStreamingDeltaRenderCost(b *testing.B) {
	width := 100
	ctx := RenderContext{ContentWidth: width, Markdown: markdownRenderer(width)}

	b.Run("plain_text_TextNode", func(b *testing.B) {
		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			var text strings.Builder
			for delta := 0; delta < 40; delta++ {
				text.WriteString("token ")
				_ = TextBlock{Text: text.String()}.Render(ctx)
			}
		}
	})

	b.Run("full_markdown_MarkdownBlock", func(b *testing.B) {
		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			var text strings.Builder
			for delta := 0; delta < 40; delta++ {
				text.WriteString("token ")
				_ = MarkdownBlock{Text: text.String()}.Render(ctx)
			}
		}
	})
}

// BenchmarkStreamRenderThrottle quantifies streamRenderThrottle's effect:
// the live assistant reply is a MarkdownNode (so it looks formatted while
// streaming, not raw-then-formatted at message_end — see
// moduTUILiveAssistantTextEntry), which means without throttling, every
// delta would re-run glamour's full markdown parse via Update(). "unthrottled"
// simulates that (bypasses the throttle to call rebuild after every delta,
// i.e. what this codebase did for the few minutes between "make streaming
// look formatted" and "add the throttle" — never shipped, but the
// regression this benchmark guards against). "throttled" simulates 40
// deltas landing within one streamRenderThrottle window followed by a
// single flush, which is what actually ships: Update() marks the entry
// dirty on every delta (cheap) and one streamRenderTickMsg does the one
// real rebuild.
func BenchmarkStreamRenderThrottle(b *testing.B) {
	b.Run("unthrottled_rebuild_per_delta", func(b *testing.B) {
		var model tea.Model = NewModel(Options{Width: 100, Height: 30})
		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			var text strings.Builder
			for delta := 0; delta < 40; delta++ {
				text.WriteString("token ")
				next, _ := model.Update(UpdateMsg{Update: UpsertEntryUpdate{Entry: Entry{
					ID:    "streaming-reply",
					Role:  RoleAssistant,
					Nodes: []Node{MarkdownNode{Text: text.String()}},
					// Streaming deliberately omitted: this arm exists to show
					// what unthrottled MarkdownNode streaming would have cost.
				}}})
				model = next
			}
		}
	})

	b.Run("throttled_one_flush_per_40_deltas", func(b *testing.B) {
		var model tea.Model = NewModel(Options{Width: 100, Height: 30})
		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			var text strings.Builder
			for delta := 0; delta < 40; delta++ {
				text.WriteString("token ")
				next, _ := model.Update(UpdateMsg{Update: UpsertEntryUpdate{Entry: Entry{
					ID:        "streaming-reply",
					Role:      RoleAssistant,
					Nodes:     []Node{MarkdownNode{Text: text.String()}},
					Streaming: true,
				}}})
				model = next
			}
			next, _ := model.Update(streamRenderTickMsg{})
			model = next
		}
	})
}
