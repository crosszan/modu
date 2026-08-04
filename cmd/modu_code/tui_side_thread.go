package main

import (
	"context"
	"fmt"
	"strings"
	"sync"

	codetui "github.com/openmodu/modu/cmd/modu_code/internal/tui"
	coding_agent "github.com/openmodu/modu/pkg/coding_agent"
	modutui "github.com/openmodu/modu/pkg/modu-tui"
	"github.com/openmodu/modu/pkg/types"
)

type moduTUISideThreadSession interface {
	BeginSideThread() error
	PromptSideThread(context.Context, string, []types.ImageContent, func(types.Event)) error
	GetSideThreadSnapshot() (coding_agent.SideThreadSnapshot, bool)
	Cwd() string
}

type moduTUISideThreadRuntime interface {
	IsPromptActive() bool
	RunOnceWithCompletion(func(context.Context) error, func(error))
	Interrupt()
}

type moduTUISideThreadRequest struct {
	text   string
	images []types.ImageContent
}

type moduTUISideThreadController struct {
	session   moduTUISideThreadSession
	runtime   moduTUISideThreadRuntime
	client    modutui.Client
	presenter codetui.EventPresenter

	mu          sync.Mutex
	active      bool
	running     bool
	exiting     bool
	queue       []moduTUISideThreadRequest
	generation  int
	liveTextID  string
	liveTextSeq int
}

func newModuTUISideThreadController(
	session moduTUISideThreadSession,
	runtime moduTUISideThreadRuntime,
	client modutui.Client,
	presenter codetui.EventPresenter,
) *moduTUISideThreadController {
	return &moduTUISideThreadController{
		session:   session,
		runtime:   runtime,
		client:    client,
		presenter: presenter,
	}
}

func (c *moduTUISideThreadController) Open(question string) {
	if c == nil || c.session == nil || c.runtime == nil {
		return
	}
	question = strings.TrimSpace(question)
	if question == "" {
		if c.runtime.IsPromptActive() {
			c.appendInfo("wait for the current task to finish before resuming /btw")
			return
		}
		snapshot, ok := c.session.GetSideThreadSnapshot()
		if !ok {
			c.appendInfo("usage: /btw <question>")
			return
		}
		c.mu.Lock()
		c.active = true
		c.generation++
		c.mu.Unlock()
		c.appendInfo(fmt.Sprintf("btw · resumed temporary side thread (%d messages)\nContinue typing, or use /exit to return.", len(snapshot.Messages)))
		c.setIdleStatus()
		return
	}
	if c.runtime.IsPromptActive() {
		c.appendInfo("wait for the current task to finish before starting /btw")
		return
	}
	if err := c.session.BeginSideThread(); err != nil {
		c.appendInfo("error: " + err.Error())
		return
	}

	c.mu.Lock()
	c.active = true
	c.running = false
	c.queue = nil
	c.generation++
	c.liveTextID = ""
	c.mu.Unlock()
	c.appendInfo("btw · temporary side thread\nContinue typing, or use /exit to return.")
	c.Submit(question, nil)
}

func (c *moduTUISideThreadController) Submit(text string, images []types.ImageContent) bool {
	if c == nil {
		return false
	}
	request := moduTUISideThreadRequest{
		text:   text,
		images: append([]types.ImageContent(nil), images...),
	}
	c.mu.Lock()
	if c.exiting {
		c.mu.Unlock()
		c.appendInfo("btw · waiting for the interrupted side turn to stop")
		return true
	}
	if !c.active {
		c.mu.Unlock()
		return false
	}
	if c.running {
		c.queue = append(c.queue, request)
		c.mu.Unlock()
		c.client.SetStatus("btw · queued", 0)
		return true
	}
	c.running = true
	generation := c.generation
	c.mu.Unlock()
	c.run(request, generation)
	return true
}

func (c *moduTUISideThreadController) HandleCommand(line string) bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	active := c.active || c.exiting
	c.mu.Unlock()
	if !active {
		return false
	}

	invocation, ok := codetui.ParseCommand(line)
	if ok && (invocation.Name == "/exit" || invocation.Name == "/quit") {
		c.Exit()
		return true
	}
	// While side mode is active, commands other than its exit commands are
	// questions for the temporary thread. This prevents an accidental main
	// session mutation from a slash-looking follow-up.
	return c.Submit(line, nil)
}

func (c *moduTUISideThreadController) Exit() {
	if c == nil || c.session == nil {
		return
	}
	c.mu.Lock()
	if !c.active && !c.exiting {
		c.mu.Unlock()
		return
	}
	if c.exiting {
		c.mu.Unlock()
		return
	}
	wasRunning := c.running
	c.active = false
	c.queue = nil
	c.generation++
	c.exiting = wasRunning
	c.liveTextID = ""
	c.mu.Unlock()
	if wasRunning && c.runtime != nil {
		c.runtime.Interrupt()
		c.appendInfo("btw · leaving after the active side turn stops")
		return
	}
	c.appendInfo("btw · returned to main conversation")
	c.client.SetStatus("idle", 0)
}

func (c *moduTUISideThreadController) run(request moduTUISideThreadRequest, generation int) {
	c.client.SetStatus("btw · running", 0)
	c.runtime.RunOnceWithCompletion(
		func(ctx context.Context) error {
			return c.session.PromptSideThread(ctx, request.text, request.images, c.handleEvent)
		},
		func(_ error) {
			c.complete(generation)
		},
	)
}

func (c *moduTUISideThreadController) complete(generation int) {
	c.mu.Lock()
	if c.exiting {
		c.running = false
		c.exiting = false
		c.mu.Unlock()
		c.appendInfo("btw · returned to main conversation")
		c.client.SetStatus("idle", 0)
		return
	}
	if !c.active || c.generation != generation {
		c.mu.Unlock()
		return
	}
	c.running = false
	if len(c.queue) == 0 {
		c.mu.Unlock()
		c.setIdleStatus()
		return
	}
	next := c.queue[0]
	c.queue = c.queue[1:]
	c.running = true
	c.mu.Unlock()
	c.run(next, generation)
}

func (c *moduTUISideThreadController) handleEvent(event types.Event) {
	switch event.Type {
	case types.EventTypeMessageUpdate:
		if message, ok := moduTUIAssistantMessage(event.Message); ok {
			if entry, ok := moduTUILiveAssistantTextEntry(message); ok {
				c.mu.Lock()
				if c.liveTextID == "" {
					c.liveTextSeq++
					c.liveTextID = fmt.Sprintf("live-btw-assistant-text-%d", c.liveTextSeq)
				}
				entry.ID = c.liveTextID
				c.mu.Unlock()
				c.client.UpsertEntry(entry)
			}
		}
	case types.EventTypeMessageEnd:
		c.mu.Lock()
		liveTextID := c.liveTextID
		c.liveTextID = ""
		c.mu.Unlock()
		moduTUIFinalizeStreamedMessage(c.client, liveTextID, c.presenter.AgentEvent(event, c.session.Cwd()))
	default:
		for _, entry := range c.presenter.AgentEvent(event, c.session.Cwd()) {
			c.client.AppendEntry(entry)
		}
	}
}

func (c *moduTUISideThreadController) setIdleStatus() {
	c.client.SetStatus("btw · side thread · /exit to return", 0)
}

func (c *moduTUISideThreadController) appendInfo(text string) {
	c.client.AppendEntry(modutui.Entry{
		Role:  modutui.RoleAssistant,
		Nodes: []modutui.Node{modutui.MarkdownNode{Text: text}},
	})
}
