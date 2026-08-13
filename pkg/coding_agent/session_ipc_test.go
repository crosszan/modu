package coding_agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openmodu/modu/pkg/coding_agent/services/session"
	"github.com/openmodu/modu/pkg/coding_agent/sessionipc"
	"github.com/openmodu/modu/pkg/types"
)

func TestSessionIPCBackendListsLoadedAndHistoricalSessions(t *testing.T) {
	agentDir := filepath.Join(t.TempDir(), ".modu")
	loaded := newAppServerTestSession(t, agentDir, t.TempDir(), "", immediateTextStream("loaded done"))
	historical := newAppServerTestSession(t, agentDir, t.TempDir(), "", immediateTextStream("historical done"))
	historicalID := historical.GetSessionID()
	historicalFile := historical.GetSessionFile()
	historical.Close("test")

	backend := newSessionIPCBackend(agentDir, func(info session.SessionInfo) (*CodingSession, error) {
		return newAppServerTestSessionFromInfo(t, agentDir, info, immediateTextStream("resumed done")), nil
	})
	backend.Attach(loaded)
	infos, err := backend.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	assertIPCSessionStatus(t, infos, loaded.GetSessionID(), sessionipc.SessionStatusIdle)
	assertIPCSessionStatus(t, infos, historicalID, sessionipc.SessionStatusNotLoaded)
	if historicalFile == "" {
		t.Fatal("historical session was not persisted")
	}
}

func TestSessionIPCBackendResumesHistoricalSessionAndStartsTurn(t *testing.T) {
	agentDir := filepath.Join(t.TempDir(), ".modu")
	historical := newAppServerTestSession(t, agentDir, t.TempDir(), "", immediateTextStream("initial done"))
	historicalID := historical.GetSessionID()
	historical.Close("test")

	resumed := make(chan *CodingSession, 1)
	backend := newSessionIPCBackend(agentDir, func(info session.SessionInfo) (*CodingSession, error) {
		session := newAppServerTestSessionFromInfo(t, agentDir, info, immediateTextStream("resumed done"))
		resumed <- session
		return session, nil
	})
	result, err := backend.Deliver(context.Background(), sessionipc.Message{
		MessageID: "message-1", FromSessionID: "sender", ToSessionID: historicalID,
		Mode: sessionipc.ModeFollowUp, Text: "continue historical work",
	})
	if err != nil {
		t.Fatalf("Deliver() error = %v", err)
	}
	if result.Status != sessionipc.DeliveryStarted {
		t.Fatalf("status = %q, want started", result.Status)
	}

	var target *CodingSession
	select {
	case target = <-resumed:
	case <-time.After(time.Second):
		t.Fatal("historical session was not resumed")
	}
	waitForCondition(t, time.Second, func() bool { return sessionContainsUserText(target, "continue historical work") })
	waitForCondition(t, time.Second, func() bool {
		infos, listErr := backend.List(context.Background())
		return listErr == nil && ipcSessionHasStatus(infos, historicalID, sessionipc.SessionStatusIdle)
	})
	if err := backend.Release(context.Background(), historicalID); err != nil {
		t.Fatalf("Release() error = %v", err)
	}
	infos, err := backend.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	assertIPCSessionStatus(t, infos, historicalID, sessionipc.SessionStatusNotLoaded)
}

func TestSessionIPCToolsUseSingleBroker(t *testing.T) {
	agentDir := filepath.Join(t.TempDir(), ".modu")
	runtimeDir := shortCodingAgentRuntimeDir(t)
	backend := newSessionIPCBackend(agentDir, nil)
	server, err := sessionipc.StartServer(sessionipc.ServerOptions{RuntimeDir: runtimeDir, Backend: backend})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close() })

	sender := newIPCClientTestSession(t, agentDir, runtimeDir, t.TempDir(), immediateTextStream("sender done"))
	target := newIPCClientTestSession(t, agentDir, runtimeDir, t.TempDir(), immediateTextStream("target done"))
	backend.Attach(sender)
	backend.Attach(target)

	result := executeSessionTool(t, sender, "session_send", map[string]any{
		"target_session_id": target.GetSessionID(),
		"message":           "inspect parser.go",
	})
	if result.IsError {
		t.Fatalf("session_send returned error: %s", toolResultText(result))
	}
	waitForCondition(t, time.Second, func() bool { return sessionContainsUserText(target, "inspect parser.go") })
	if got := sender.sessionIPC.SocketPath(); got != filepath.Join(runtimeDir, "ipc.sock") {
		t.Fatalf("socket path = %q, want one ipc.sock", got)
	}
}

func shortCodingAgentRuntimeDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "modu-code-ipc-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func newIPCClientTestSession(t *testing.T, agentDir, runtimeDir, cwd string, stream types.StreamFn) *CodingSession {
	t.Helper()
	session, err := NewCodingSession(CodingSessionOptions{
		Cwd: cwd, AgentDir: agentDir,
		Model:     &types.Model{ID: "test", ProviderID: "test", ContextWindow: 32768},
		GetAPIKey: func(string) (string, error) { return "", nil }, StreamFn: stream,
		EnableSessionIPC: true, SessionIPCRuntimeDir: runtimeDir,
	})
	if err != nil {
		t.Fatalf("NewCodingSession() error = %v", err)
	}
	t.Cleanup(func() { session.Close("test") })
	return session
}

func newAppServerTestSession(t *testing.T, agentDir, cwd, resumeID string, stream types.StreamFn) *CodingSession {
	t.Helper()
	session, err := NewCodingSession(CodingSessionOptions{
		Cwd: cwd, AgentDir: agentDir, ResumeSessionID: resumeID,
		Model:     &types.Model{ID: "test", ProviderID: "test", ContextWindow: 32768},
		GetAPIKey: func(string) (string, error) { return "", nil }, StreamFn: stream,
	})
	if err != nil {
		t.Fatalf("NewCodingSession() error = %v", err)
	}
	if resumeID == "" {
		if err := session.sessionManager.Flush(); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { session.Close("test") })
	return session
}

func newAppServerTestSessionFromInfo(t *testing.T, agentDir string, info session.SessionInfo, stream types.StreamFn) *CodingSession {
	return newAppServerTestSession(t, agentDir, info.Cwd, info.ID, stream)
}

func assertIPCSessionStatus(t *testing.T, infos []sessionipc.SessionInfo, id string, status sessionipc.SessionStatus) {
	t.Helper()
	for _, info := range infos {
		if info.SessionID == id {
			if info.Status != status {
				t.Fatalf("session %s status = %q, want %q", id, info.Status, status)
			}
			return
		}
	}
	t.Fatalf("session %s not found in %#v", id, infos)
}

func ipcSessionHasStatus(infos []sessionipc.SessionInfo, id string, status sessionipc.SessionStatus) bool {
	for _, info := range infos {
		if info.SessionID == id && info.Status == status {
			return true
		}
	}
	return false
}

func immediateTextStream(text string) types.StreamFn {
	return func(_ context.Context, _ *types.Model, _ *types.LLMContext, _ *types.SimpleStreamOptions) (types.EventStream, error) {
		stream := types.NewEventStream()
		message := &types.AssistantMessage{Role: types.RoleAssistant, StopReason: "stop", Content: []types.ContentBlock{&types.TextContent{Type: "text", Text: text}}}
		go func() {
			stream.Push(types.StreamEvent{Type: types.EventDone, Message: message})
			stream.Resolve(message, nil)
			stream.Close()
		}()
		return stream, nil
	}
}

func executeSessionTool(t *testing.T, session *CodingSession, name string, args map[string]any) types.ToolResult {
	t.Helper()
	for _, tool := range session.GetAgent().GetState().Tools {
		if tool.Name() == name {
			result, err := tool.Execute(context.Background(), "test-call", args, nil)
			if err != nil {
				t.Fatal(err)
			}
			return result
		}
	}
	t.Fatalf("tool %q not registered", name)
	return types.ToolResult{}
}

func toolResultText(result types.ToolResult) string {
	var out strings.Builder
	for _, block := range result.Content {
		if text, ok := block.(*types.TextContent); ok {
			out.WriteString(text.Text)
		}
	}
	return out.String()
}

func sessionContainsUserText(session *CodingSession, want string) bool {
	for _, message := range session.GetMessages() {
		if user, ok := message.(types.UserMessage); ok && strings.Contains(user.Content.(string), want) {
			return true
		}
	}
	return false
}

func waitForCondition(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("condition was not met before timeout")
}
