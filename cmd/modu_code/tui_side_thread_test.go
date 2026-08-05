package main

import (
	"context"
	"strings"
	"sync"
	"testing"

	codetui "github.com/openmodu/modu/cmd/modu_code/internal/tui"
	coding_agent "github.com/openmodu/modu/pkg/coding_agent"
	modutui "github.com/openmodu/modu/pkg/modu-tui"
	"github.com/openmodu/modu/pkg/types"
)

type moduTUISideThreadSessionStub struct {
	mu       sync.Mutex
	begins   int
	prompts  []string
	snapshot coding_agent.SideThreadSnapshot
	hasSide  bool
}

func (s *moduTUISideThreadSessionStub) BeginSideThread() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.begins++
	s.hasSide = true
	s.snapshot = coding_agent.SideThreadSnapshot{}
	return nil
}

func (s *moduTUISideThreadSessionStub) PromptSideThread(
	_ context.Context,
	text string,
	_ []types.ImageContent,
	onEvent func(types.Event),
) error {
	s.mu.Lock()
	s.prompts = append(s.prompts, text)
	s.snapshot.Messages = append(s.snapshot.Messages, types.UserMessage{Role: types.RoleUser, Content: text})
	s.mu.Unlock()
	if onEvent != nil {
		onEvent(types.Event{
			Type: types.EventTypeMessageEnd,
			Message: types.AssistantMessage{
				Role:    types.RoleAssistant,
				Content: []types.ContentBlock{&types.TextContent{Type: "text", Text: "side reply"}},
			},
		})
	}
	return nil
}

func (s *moduTUISideThreadSessionStub) GetSideThreadSnapshot() (coding_agent.SideThreadSnapshot, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshot, s.hasSide
}

func (s *moduTUISideThreadSessionStub) Cwd() string { return "/tmp" }

type moduTUISideThreadRuntimeStub struct {
	active     bool
	interrupts int
}

func (r *moduTUISideThreadRuntimeStub) IsPromptActive() bool { return r.active }
func (r *moduTUISideThreadRuntimeStub) Interrupt()           { r.interrupts++ }
func (r *moduTUISideThreadRuntimeStub) RunOnceWithCompletion(run func(context.Context) error, complete func(error)) {
	r.active = true
	err := run(context.Background())
	r.active = false
	complete(err)
}

type moduTUISideThreadPendingRuntimeStub struct {
	active     bool
	interrupts int
	complete   func(error)
}

func (r *moduTUISideThreadPendingRuntimeStub) IsPromptActive() bool { return r.active }
func (r *moduTUISideThreadPendingRuntimeStub) Interrupt()           { r.interrupts++ }
func (r *moduTUISideThreadPendingRuntimeStub) RunOnceWithCompletion(_ func(context.Context) error, complete func(error)) {
	r.active = true
	r.complete = complete
}

func (r *moduTUISideThreadPendingRuntimeStub) finish(err error) {
	r.active = false
	complete := r.complete
	r.complete = nil
	complete(err)
}

func TestModuTUISideThreadRoutesFollowUpsAndSlashTextUntilExit(t *testing.T) {
	session := &moduTUISideThreadSessionStub{}
	runtime := &moduTUISideThreadRuntimeStub{}
	var updates []any
	client := modutui.NewClient(func(message any) {
		updates = append(updates, message)
	})
	controller := newModuTUISideThreadController(
		session,
		runtime,
		client,
		codetui.NewEventPresenter(nil, ""),
	)

	controller.Open("first question")
	if !controller.Submit("follow-up", nil) {
		t.Fatal("active side thread did not accept a follow-up")
	}
	if !controller.HandleCommand("/help") {
		t.Fatal("slash-looking follow-up escaped the active side thread")
	}
	if !controller.HandleCommand("/exit") {
		t.Fatal("/exit was not handled by the active side thread")
	}
	if controller.Submit("main again", nil) {
		t.Fatal("side controller remained active after /exit")
	}

	session.mu.Lock()
	defer session.mu.Unlock()
	if session.begins != 1 {
		t.Fatalf("BeginSideThread calls = %d", session.begins)
	}
	want := []string{"first question", "follow-up", "/help"}
	if len(session.prompts) != len(want) {
		t.Fatalf("side prompts = %#v", session.prompts)
	}
	for i := range want {
		if session.prompts[i] != want[i] {
			t.Fatalf("side prompts = %#v, want %#v", session.prompts, want)
		}
	}
	if len(updates) == 0 {
		t.Fatal("side thread did not publish any TUI updates")
	}
}

func TestModuTUISideThreadBareOpenResumesInProcessHistory(t *testing.T) {
	session := &moduTUISideThreadSessionStub{
		hasSide: true,
		snapshot: coding_agent.SideThreadSnapshot{
			Messages: []types.AgentMessage{
				types.UserMessage{Role: types.RoleUser, Content: "earlier"},
				types.AssistantMessage{Role: types.RoleAssistant},
			},
		},
	}
	runtime := &moduTUISideThreadRuntimeStub{}
	controller := newModuTUISideThreadController(
		session,
		runtime,
		modutui.NewClient(func(any) {}),
		codetui.NewEventPresenter(nil, ""),
	)

	controller.Open("")
	if !controller.Submit("continued", nil) {
		t.Fatal("bare /btw did not resume the in-process side thread")
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.begins != 0 || len(session.prompts) != 1 || session.prompts[0] != "continued" {
		t.Fatalf("resume began a new thread or lost follow-up: begins=%d prompts=%#v", session.begins, session.prompts)
	}
}

func TestModuTUISideThreadExitDrainsRunningTurnBeforeReturningToMain(t *testing.T) {
	session := &moduTUISideThreadSessionStub{}
	runtime := &moduTUISideThreadPendingRuntimeStub{}
	controller := newModuTUISideThreadController(
		session,
		runtime,
		modutui.NewClient(func(any) {}),
		codetui.NewEventPresenter(nil, ""),
	)

	controller.Open("running question")
	controller.Exit()
	if runtime.interrupts != 1 {
		t.Fatalf("interrupts = %d, want 1", runtime.interrupts)
	}
	if !controller.Submit("must not reach main yet", nil) {
		t.Fatal("input escaped to the main conversation while side turn was stopping")
	}

	runtime.finish(context.Canceled)
	if controller.Submit("main is available", nil) {
		t.Fatal("side controller stayed active after the running turn stopped")
	}
}

func TestModuTUISideThreadNoticesAreNotStyledAsAssistantMessages(t *testing.T) {
	session := &moduTUISideThreadSessionStub{}
	runtime := &moduTUISideThreadRuntimeStub{}
	var updates []any
	client := modutui.NewClient(func(message any) {
		updates = append(updates, message)
	})
	controller := newModuTUISideThreadController(
		session,
		runtime,
		client,
		codetui.NewEventPresenter(nil, ""),
	)

	controller.Open("first question")
	controller.HandleCommand("/exit")

	var notices []modutui.Entry
	for _, update := range updates {
		msg, ok := update.(modutui.UpdateMsg)
		if !ok {
			continue
		}
		appended, ok := msg.Update.(modutui.AppendEntryUpdate)
		if !ok || len(appended.Entry.Nodes) == 0 {
			continue
		}
		node, ok := appended.Entry.Nodes[0].(modutui.TextNode)
		if !ok || !strings.HasPrefix(node.Text, "btw · ") {
			continue
		}
		notices = append(notices, appended.Entry)
	}
	if len(notices) != 2 {
		t.Fatalf("expected the open and exit notices, got %d", len(notices))
	}

	for _, notice := range notices {
		// These are the TUI's own status lines, not model output: rendering
		// them as a normal assistant entry gives them a "●" marker that
		// wrongly implies the model said them.
		if notice.Role != modutui.RoleStatus {
			t.Fatalf("side-thread notice should use RoleStatus, not the assistant marker: %#v", notice)
		}
		if _, ok := notice.Nodes[0].(modutui.TextNode); !ok {
			t.Fatalf("side-thread notice should be a TextNode, got %T", notice.Nodes[0])
		}
	}

	// The open notice is deliberately two lines; markdown rendering would
	// have collapsed the newline into one run-on line.
	openText := notices[0].Nodes[0].(modutui.TextNode).Text
	if !strings.Contains(openText, "\n") {
		t.Fatalf("the open notice should keep its second line, got %q", openText)
	}
}
