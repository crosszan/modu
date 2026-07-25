package main

import (
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
	unsubAgent := b.session.Subscribe(func(ev types.Event) {
		b.duration.Handle(ev)
		b.workflow.HandleToolEvent(ev)
		for _, entry := range b.presenter.AgentEvent(ev, b.session.Cwd()) {
			b.client.AppendEntry(entry)
		}
		if ev.Type == types.EventTypeToolExecutionEnd {
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
