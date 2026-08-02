package main

import (
	"fmt"
	"strings"

	codetui "github.com/openmodu/modu/cmd/modu_code/internal/tui"
	coding_agent "github.com/openmodu/modu/pkg/coding_agent"
	modutui "github.com/openmodu/modu/pkg/modu-tui"
	"github.com/openmodu/modu/pkg/types"
)

type moduTUIEventBindings struct {
	session       *coding_agent.CodingSession
	client        modutui.Client
	workflow      *moduTUIWorkflowController
	presenter     codetui.EventPresenter
	subagents     *codetui.SubagentActivity
	duration      *moduTUIAgentDurationTracker
	refreshFooter func()
}

func (b moduTUIEventBindings) Subscribe() func() {
	// liveTextID names the transcript entry the assistant's still-streaming
	// reply is upserted into. Assigned on the first message_update text
	// delta of a turn, reused for every delta after it, and cleared at
	// message_end once the final entry has taken over the same id.
	var liveTextID string
	var liveTextSeq int

	unsubAgent := b.session.Subscribe(func(ev types.Event) {
		b.duration.Handle(ev)
		b.workflow.HandleToolEvent(ev)

		switch ev.Type {
		case types.EventTypeMessageUpdate:
			if message, ok := moduTUIAssistantMessage(ev.Message); ok {
				if entry, ok := moduTUILiveAssistantTextEntry(message); ok {
					if liveTextID == "" {
						liveTextSeq++
						liveTextID = fmt.Sprintf("live-assistant-text-%d", liveTextSeq)
					}
					entry.ID = liveTextID
					b.client.UpsertEntry(entry)
				}
			}
		case types.EventTypeMessageEnd:
			entries := b.presenter.AgentEvent(ev, b.session.Cwd())
			liveIdx := moduTUIClaimLiveTextEntry(entries, liveTextID)
			liveTextID = ""
			for i, entry := range entries {
				if i == liveIdx {
					b.client.UpsertEntry(entry)
				} else {
					b.client.AppendEntry(entry)
				}
			}
		default:
			for _, entry := range b.presenter.AgentEvent(ev, b.session.Cwd()) {
				b.client.AppendEntry(entry)
			}
		}

		if moduTUITodoRefreshEvent(ev) {
			b.client.SetTodos(moduTUITodos(b.session))
		}
		if ev.Type == types.EventTypeMessageEnd {
			b.refreshFooter()
		}
	})
	unsubSession := b.session.SubscribeSession(func(ev coding_agent.SessionEvent) {
		// A subagent run owns one transcript block that it rewrites as it
		// goes, so it upserts instead of appending and takes precedence over
		// the presenter's flat lifecycle lines.
		if entry, ok := b.subagents.HandleSessionEvent(ev); ok {
			b.client.UpsertEntry(entry)
		} else if !b.workflow.HandleSessionEvent(ev) {
			if entry, ok := b.presenter.SessionEvent(ev); ok {
				b.client.AppendEntry(entry)
			}
		}
		b.refreshFooter()
	})
	return func() {
		unsubSession()
		unsubAgent()
	}
}

func moduTUITodoRefreshEvent(ev types.Event) bool {
	if ev.Type != types.EventTypeToolExecutionEnd || ev.IsError {
		return false
	}
	return ev.ToolName == "exit_plan_mode" || ev.ToolName == "todo_write"
}

// moduTUIAssistantMessage extracts the AssistantMessage carried by a
// message_update/message_end event, which arrives as either a value or a
// pointer depending on the source.
func moduTUIAssistantMessage(msg types.AgentMessage) (types.AssistantMessage, bool) {
	switch m := msg.(type) {
	case types.AssistantMessage:
		return m, true
	case *types.AssistantMessage:
		if m == nil {
			return types.AssistantMessage{}, false
		}
		return *m, true
	default:
		return types.AssistantMessage{}, false
	}
}

// moduTUILiveAssistantTextEntry builds a transcript entry for the assistant
// reply's text so far. It renders as markdown (MarkdownNode) so the reply
// looks the same while streaming as it will once finished, instead of
// showing raw markdown syntax that suddenly formats at message_end. The
// entry is marked Streaming so the render cache never holds onto it and
// Model throttles how often it actually re-parses (see
// streamRenderThrottle in pkg/modu-tui) rather than reparsing on every
// delta. message_end swaps it for the final entry via
// moduTUIClaimLiveTextEntry.
func moduTUILiveAssistantTextEntry(message types.AssistantMessage) (modutui.Entry, bool) {
	var parts []string
	for _, block := range message.Content {
		if text, ok := block.(*types.TextContent); ok && text != nil && text.Text != "" {
			parts = append(parts, text.Text)
		}
	}
	joined := strings.TrimSpace(strings.Join(parts, "\n\n"))
	if joined == "" {
		return modutui.Entry{}, false
	}
	return modutui.Entry{
		Role:      modutui.RoleAssistant,
		Nodes:     []modutui.Node{modutui.MarkdownNode{Text: joined}},
		Streaming: true,
	}, true
}

// moduTUIClaimLiveTextEntry stamps liveID onto the first finalized text
// entry in entries so upserting it (instead of appending) replaces the live
// placeholder in place. Returns -1 (nothing to claim) when liveID is empty
// or entries has no plain markdown-text entry, in which case the caller
// falls back to appending every entry exactly as before streaming existed.
func moduTUIClaimLiveTextEntry(entries []modutui.Entry, liveID string) int {
	if liveID == "" {
		return -1
	}
	for i := range entries {
		if len(entries[i].Nodes) != 1 {
			continue
		}
		if _, ok := entries[i].Nodes[0].(modutui.MarkdownNode); ok {
			entries[i].ID = liveID
			return i
		}
	}
	return -1
}
