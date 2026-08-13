package sessionipc

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type recordingBackend struct {
	mu       sync.Mutex
	sessions map[string]SessionInfo
	messages []Message
	resumed  []string
	status   DeliveryStatus
}

type recordingHandler struct {
	mu        sync.Mutex
	streaming bool
	messages  []Message
}

func (h *recordingHandler) State() SessionState {
	h.mu.Lock()
	defer h.mu.Unlock()
	return SessionState{Streaming: h.streaming}
}

func (h *recordingHandler) Deliver(_ context.Context, message Message) (DeliveryResult, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.messages = append(h.messages, message)
	status := DeliveryStarted
	if h.streaming {
		status = DeliveryQueued
	}
	return DeliveryResult{Status: status}, nil
}

func (b *recordingBackend) List(_ context.Context) ([]SessionInfo, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]SessionInfo, 0, len(b.sessions))
	for _, session := range b.sessions {
		out = append(out, session)
	}
	return out, nil
}

func (b *recordingBackend) Deliver(_ context.Context, message Message) (DeliveryResult, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	info, ok := b.sessions[message.ToSessionID]
	if !ok {
		return DeliveryResult{}, ErrSessionNotFound
	}
	if info.Status == SessionStatusNotLoaded {
		b.resumed = append(b.resumed, message.ToSessionID)
		info.Status = SessionStatusIdle
		b.sessions[message.ToSessionID] = info
	}
	b.messages = append(b.messages, message)
	status := b.status
	if status == "" {
		status = DeliveryStarted
	}
	return DeliveryResult{Status: status}, nil
}

func TestSingleSocketListsAndRoutesSessions(t *testing.T) {
	runtimeDir := shortTestRuntimeDir(t)
	backend := &recordingBackend{sessions: map[string]SessionInfo{
		"session-a": {SessionID: "session-a", Cwd: "/repo/a", Status: SessionStatusIdle},
		"session-b": {SessionID: "session-b", Cwd: "/repo/b", Status: SessionStatusBusy},
	}}
	server := startTestServer(t, runtimeDir, backend)
	client := newTestClient(t, runtimeDir, "session-a")

	sessions, err := client.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("List() returned %d sessions, want 2", len(sessions))
	}
	result, err := client.Send(context.Background(), "session-b", "review parser.go", "")
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if result.Status != DeliveryStarted {
		t.Fatalf("delivery status = %q, want %q", result.Status, DeliveryStarted)
	}

	backend.mu.Lock()
	defer backend.mu.Unlock()
	if len(backend.messages) != 1 || backend.messages[0].FromSessionID != "session-a" || backend.messages[0].Text != "review parser.go" {
		t.Fatalf("messages = %#v", backend.messages)
	}

	entries, err := os.ReadDir(runtimeDir)
	if err != nil {
		t.Fatal(err)
	}
	sockets := 0
	for _, entry := range entries {
		if entry.Type()&os.ModeSocket != 0 || strings.HasSuffix(entry.Name(), ".sock") {
			sockets++
		}
	}
	if sockets != 1 || server.SocketPath() != filepath.Join(runtimeDir, "ipc.sock") {
		t.Fatalf("socket count = %d, path = %q; want one ipc.sock", sockets, server.SocketPath())
	}
}

func TestSendResumesHistoricalSessionBeforeStartingTurn(t *testing.T) {
	runtimeDir := shortTestRuntimeDir(t)
	backend := &recordingBackend{sessions: map[string]SessionInfo{
		"historical": {SessionID: "historical", Cwd: "/repo/old", Status: SessionStatusNotLoaded},
	}}
	startTestServer(t, runtimeDir, backend)
	client := newTestClient(t, runtimeDir, "sender")

	sessions, err := client.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if !containsSession(sessions, "historical", SessionStatusNotLoaded) {
		t.Fatalf("sessions = %#v, want historical notLoaded session", sessions)
	}
	if _, err := client.Send(context.Background(), "historical", "continue the work", ""); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	backend.mu.Lock()
	defer backend.mu.Unlock()
	if len(backend.resumed) != 1 || backend.resumed[0] != "historical" {
		t.Fatalf("resumed = %#v, want historical", backend.resumed)
	}
	if len(backend.messages) != 1 || backend.messages[0].ToSessionID != "historical" {
		t.Fatalf("messages = %#v", backend.messages)
	}
}

func TestLiveBusySessionIsListedAndReceivesFollowUp(t *testing.T) {
	runtimeDir := shortTestRuntimeDir(t)
	startTestServer(t, runtimeDir, &recordingBackend{sessions: map[string]SessionInfo{}})
	busyHandler := &recordingHandler{streaming: true}
	target, err := NewClient(ClientOptions{RuntimeDir: runtimeDir, SessionID: "busy", Cwd: "/repo/busy", Handler: busyHandler})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = target.Close() })
	sender := newTestClient(t, runtimeDir, "sender")

	sessions, err := sender.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !containsSession(sessions, "busy", SessionStatusBusy) {
		t.Fatalf("sessions = %#v, want busy live session", sessions)
	}
	result, err := sender.Send(context.Background(), "busy", "follow up", "")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != DeliveryQueued {
		t.Fatalf("status = %q, want queued", result.Status)
	}
	busyHandler.mu.Lock()
	defer busyHandler.mu.Unlock()
	if len(busyHandler.messages) != 1 || busyHandler.messages[0].Text != "follow up" {
		t.Fatalf("messages = %#v", busyHandler.messages)
	}
}

func TestServerDeduplicatesMessageID(t *testing.T) {
	runtimeDir := shortTestRuntimeDir(t)
	backend := &recordingBackend{sessions: map[string]SessionInfo{
		"target": {SessionID: "target", Status: SessionStatusIdle},
	}}
	startTestServer(t, runtimeDir, backend)
	client := newTestClient(t, runtimeDir, "sender")
	params := sendParams{MessageID: "same-id", TargetSessionID: "target", Message: "once", SentAt: 1}
	var first DeliveryResult
	if err := client.rpc.call(context.Background(), "turn/start", params, &first); err != nil {
		t.Fatal(err)
	}
	var second DeliveryResult
	if err := client.rpc.call(context.Background(), "turn/start", params, &second); err != nil {
		t.Fatal(err)
	}
	if first.Status != DeliveryStarted || second.Status != DeliveryDuplicate {
		t.Fatalf("statuses = %q, %q", first.Status, second.Status)
	}
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if len(backend.messages) != 1 {
		t.Fatalf("messages = %#v, want one delivery", backend.messages)
	}
}

func TestServerRejectsSecondOwnerAndCleansSocket(t *testing.T) {
	runtimeDir := shortTestRuntimeDir(t)
	server := startTestServer(t, runtimeDir, &recordingBackend{sessions: map[string]SessionInfo{}})
	second, err := StartServer(ServerOptions{RuntimeDir: runtimeDir, Backend: &recordingBackend{sessions: map[string]SessionInfo{}}})
	if second != nil {
		_ = second.Close()
	}
	if !errors.Is(err, ErrServerAlreadyRunning) {
		t.Fatalf("second StartServer() error = %v, want ErrServerAlreadyRunning", err)
	}

	path := server.SocketPath()
	if err := server.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("socket still exists after close: %v", err)
	}
}

func TestClientRejectsOversizedAndSelfMessages(t *testing.T) {
	runtimeDir := shortTestRuntimeDir(t)
	startTestServer(t, runtimeDir, &recordingBackend{sessions: map[string]SessionInfo{
		"sender": {SessionID: "sender", Status: SessionStatusIdle},
		"target": {SessionID: "target", Status: SessionStatusIdle},
	}})
	client := newTestClient(t, runtimeDir, "sender")

	if _, err := client.Send(context.Background(), "target", strings.Repeat("x", MaxMessageBytes+1), ""); !errors.Is(err, ErrMessageTooLarge) {
		t.Fatalf("oversized Send() error = %v, want ErrMessageTooLarge", err)
	}
	if _, err := client.Send(context.Background(), "sender", "loop", ""); !errors.Is(err, ErrInvalidMessage) {
		t.Fatalf("self Send() error = %v, want ErrInvalidMessage", err)
	}
}

func TestDefaultRuntimeDirIsShortAndScopedByAgentDir(t *testing.T) {
	first := DefaultRuntimeDir(filepath.Join(t.TempDir(), strings.Repeat("long-segment-", 20)))
	second := DefaultRuntimeDir(t.TempDir())
	if first == second {
		t.Fatalf("different agent dirs resolved to the same runtime dir %q", first)
	}
	if len(filepath.Join(first, "ipc.sock")) >= 100 {
		t.Fatalf("default socket path is too long for conservative Unix limits: %q", first)
	}
}

func startTestServer(t *testing.T, runtimeDir string, backend Backend) *Server {
	t.Helper()
	server, err := StartServer(ServerOptions{RuntimeDir: runtimeDir, Backend: backend})
	if err != nil {
		t.Fatalf("StartServer() error = %v", err)
	}
	t.Cleanup(func() { _ = server.Close() })
	return server
}

func newTestClient(t *testing.T, runtimeDir, sessionID string) *Client {
	t.Helper()
	client, err := NewClient(ClientOptions{RuntimeDir: runtimeDir, SessionID: sessionID, Handler: noopHandler{}})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	return client
}

type noopHandler struct{}

func (noopHandler) State() SessionState { return SessionState{} }
func (noopHandler) Deliver(context.Context, Message) (DeliveryResult, error) {
	return DeliveryResult{Status: DeliveryStarted}, nil
}

func shortTestRuntimeDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "modu-ipc-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func containsSession(sessions []SessionInfo, id string, status SessionStatus) bool {
	for _, session := range sessions {
		if session.SessionID == id && session.Status == status {
			return true
		}
	}
	return false
}
