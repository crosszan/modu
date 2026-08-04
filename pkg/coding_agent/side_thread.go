package coding_agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/openmodu/modu/pkg/agent"
	"github.com/openmodu/modu/pkg/types"
)

// SideThreadSnapshot contains only the messages created after the temporary
// side thread forked from the main conversation.
type SideThreadSnapshot struct {
	Messages []types.AgentMessage
}

type sideThread struct {
	agent          *agent.Agent
	baseMessageLen int
	sessionID      string
	cwd            string
}

// BeginSideThread forks the current in-memory conversation into a temporary
// agent. Starting another side thread replaces the previous temporary thread.
func (s *engine) BeginSideThread() error {
	if s == nil || s.agent == nil {
		return fmt.Errorf("session is not initialized")
	}
	if s.agent.GetState().IsStreaming {
		return fmt.Errorf("wait for the current task to finish before starting /btw")
	}

	s.refreshDynamicSystemPrompt()
	state := s.agent.GetState()
	messages := append([]types.AgentMessage(nil), state.Messages...)
	tools := append([]types.Tool(nil), state.Tools...)
	var approveTool func(string, string, map[string]any) (types.ToolApprovalDecision, error)
	if s.approvalManager != nil {
		approveTool = s.approvalManager.Approve
	}
	temporaryAgent := agent.NewAgent(types.Config{
		GetAPIKey:   s.getAPIKey,
		ApproveTool: approveTool,
		InitialState: &types.State{
			SystemPrompt:  state.SystemPrompt,
			Model:         state.Model,
			ThinkingLevel: state.ThinkingLevel,
			Tools:         tools,
			Messages:      messages,
		},
		StreamFn: s.streamFn,
	})
	next := &sideThread{
		agent:          temporaryAgent,
		baseMessageLen: len(messages),
		sessionID:      s.GetSessionID(),
		cwd:            s.cwd,
	}

	s.sideThreadMu.Lock()
	previous := s.sideThread
	s.sideThread = next
	s.sideThreadMu.Unlock()
	if previous != nil && previous.agent != nil {
		previous.agent.Abort()
	}
	return nil
}

// PromptSideThread sends one message to the active temporary side thread.
// Events are delivered only to onEvent; main-session subscribers do not see
// them and the resulting messages are never persisted.
func (s *engine) PromptSideThread(
	ctx context.Context,
	text string,
	images []types.ImageContent,
	onEvent func(types.Event),
) error {
	thread, ok := s.currentSideThread()
	if !ok {
		return fmt.Errorf("no temporary side thread; start one with /btw <question>")
	}
	if strings.TrimSpace(text) == "" && len(images) == 0 {
		return fmt.Errorf("side-thread message is empty")
	}
	if len(images) > 0 {
		if err := s.validateImageInput(images); err != nil {
			return err
		}
	}
	unsubscribe := func() {}
	if onEvent != nil {
		unsubscribe = thread.agent.Subscribe(onEvent)
	}
	defer unsubscribe()

	if len(images) > 0 {
		return thread.agent.Prompt(ctx, userMessageWithImages(text, images))
	}
	return thread.agent.Prompt(ctx, text)
}

// GetSideThreadSnapshot returns the side-only conversation history. The
// inherited main conversation is intentionally omitted.
func (s *engine) GetSideThreadSnapshot() (SideThreadSnapshot, bool) {
	thread, ok := s.currentSideThread()
	if !ok {
		return SideThreadSnapshot{}, false
	}
	state := thread.agent.GetState()
	if thread.baseMessageLen >= len(state.Messages) {
		return SideThreadSnapshot{}, true
	}
	return SideThreadSnapshot{
		Messages: append([]types.AgentMessage(nil), state.Messages[thread.baseMessageLen:]...),
	}, true
}

// AbortSideThread cancels the current side-thread turn without removing its
// completed temporary history.
func (s *engine) AbortSideThread() {
	thread, ok := s.currentSideThread()
	if ok {
		thread.agent.Abort()
	}
}

// ClearSideThread discards the in-process side thread and its history.
func (s *engine) ClearSideThread() {
	if s == nil {
		return
	}
	s.sideThreadMu.Lock()
	thread := s.sideThread
	s.sideThread = nil
	s.sideThreadMu.Unlock()
	if thread != nil && thread.agent != nil {
		thread.agent.Abort()
	}
}

func (s *engine) currentSideThread() (*sideThread, bool) {
	if s == nil {
		return nil, false
	}
	s.sideThreadMu.Lock()
	defer s.sideThreadMu.Unlock()
	thread := s.sideThread
	if thread == nil || thread.agent == nil {
		return nil, false
	}
	if thread.sessionID != s.GetSessionID() || thread.cwd != s.cwd {
		thread.agent.Abort()
		s.sideThread = nil
		return nil, false
	}
	return thread, true
}
