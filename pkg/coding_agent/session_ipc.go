package coding_agent

import (
	"context"
	"fmt"
	"sync"

	"github.com/openmodu/modu/pkg/coding_agent/services/session"
	"github.com/openmodu/modu/pkg/coding_agent/sessionipc"
)

// SessionIPCSessionFactory resumes a persisted session for the app-server.
// The returned session must not register another IPC client.
type SessionIPCSessionFactory func(session.SessionInfo) (*CodingSession, error)

// SessionIPCBackend exposes persisted and app-server-owned sessions to the
// single-socket sessionipc server.
type SessionIPCBackend struct {
	agentDir string
	factory  SessionIPCSessionFactory

	mu     sync.Mutex
	loaded map[string]*managedIPCSession
}

type managedIPCSession struct {
	session *CodingSession
	handler *sessionIPCHandler
	owned   bool
}

// NewSessionIPCBackend creates the history/loading backend used by the Modu
// app-server process.
func NewSessionIPCBackend(agentDir string, factory SessionIPCSessionFactory) *SessionIPCBackend {
	return &SessionIPCBackend{
		agentDir: agentDir,
		factory:  factory,
		loaded:   make(map[string]*managedIPCSession),
	}
}

func newSessionIPCBackend(agentDir string, factory SessionIPCSessionFactory) *SessionIPCBackend {
	return NewSessionIPCBackend(agentDir, factory)
}

// Attach registers an already-created in-process session. It is mainly useful
// for embedded hosts; the standalone app-server loads historical sessions via
// the factory instead.
func (b *SessionIPCBackend) Attach(target *CodingSession) {
	if b == nil || target == nil || target.engine == nil {
		return
	}
	handler := newSessionIPCHandler(target.engine)
	handler.activate()
	b.mu.Lock()
	b.loaded[target.GetSessionID()] = &managedIPCSession{session: target, handler: handler}
	b.mu.Unlock()
}

// List returns persisted sessions and marks sessions already owned by this
// backend as idle or busy. Persisted sessions are not loaded merely by listing.
func (b *SessionIPCBackend) List(_ context.Context) ([]sessionipc.SessionInfo, error) {
	stored, err := session.ListAll(b.agentDir)
	if err != nil {
		return nil, err
	}

	b.mu.Lock()
	loaded := make(map[string]*managedIPCSession, len(b.loaded))
	for id, managed := range b.loaded {
		loaded[id] = managed
	}
	b.mu.Unlock()

	result := make([]sessionipc.SessionInfo, 0, len(stored)+len(loaded))
	seen := make(map[string]struct{}, len(stored)+len(loaded))
	for _, info := range stored {
		status := sessionipc.SessionStatusNotLoaded
		if managed := loaded[info.ID]; managed != nil {
			status = statusForSessionIPCHandler(managed.handler)
		}
		result = append(result, sessionipc.SessionInfo{
			SessionID: info.ID,
			Cwd:       info.Cwd,
			Name:      info.Name,
			Status:    status,
		})
		seen[info.ID] = struct{}{}
	}
	for id, managed := range loaded {
		if _, ok := seen[id]; ok {
			continue
		}
		result = append(result, sessionipc.SessionInfo{
			SessionID: id,
			Cwd:       managed.session.cwd,
			Name:      managed.session.sessionName,
			Status:    statusForSessionIPCHandler(managed.handler),
		})
	}
	return result, nil
}

// Deliver restores a historical session on first use and starts or queues the
// incoming turn using the same semantics as a live attached session.
func (b *SessionIPCBackend) Deliver(ctx context.Context, message sessionipc.Message) (sessionipc.DeliveryResult, error) {
	b.mu.Lock()
	managed := b.loaded[message.ToSessionID]
	if managed == nil {
		if b.factory == nil {
			b.mu.Unlock()
			return sessionipc.DeliveryResult{}, fmt.Errorf("%w: %s", sessionipc.ErrSessionNotFound, message.ToSessionID)
		}
		info, err := b.findStoredSession(message.ToSessionID)
		if err != nil {
			b.mu.Unlock()
			return sessionipc.DeliveryResult{}, err
		}
		target, err := b.factory(info)
		if err != nil {
			b.mu.Unlock()
			return sessionipc.DeliveryResult{}, fmt.Errorf("resume session %s: %w", message.ToSessionID, err)
		}
		handler := newSessionIPCHandler(target.engine)
		handler.activate()
		managed = &managedIPCSession{session: target, handler: handler, owned: true}
		b.loaded[message.ToSessionID] = managed
	}
	result, err := managed.handler.Deliver(ctx, message)
	b.mu.Unlock()
	return result, err
}

// Release hands an idle daemon-owned session back to a newly registered live
// client. A running session cannot be transferred without losing its turn.
func (b *SessionIPCBackend) Release(_ context.Context, id string) error {
	b.mu.Lock()
	managed := b.loaded[id]
	if managed == nil {
		b.mu.Unlock()
		return nil
	}
	if !managed.owned {
		b.mu.Unlock()
		return fmt.Errorf("%w: session %s is attached by another host", sessionipc.ErrSessionAlreadyRunning, id)
	}
	if managed.handler.State().Streaming {
		b.mu.Unlock()
		return fmt.Errorf("%w: session %s is busy in app-server", sessionipc.ErrSessionAlreadyRunning, id)
	}
	delete(b.loaded, id)
	b.mu.Unlock()
	managed.session.Close("transferred to live client")
	return nil
}

// Close closes sessions that were resumed and are owned by the app-server.
// Attached sessions remain owned by their embedding host.
func (b *SessionIPCBackend) Close() {
	b.mu.Lock()
	owned := make([]*CodingSession, 0, len(b.loaded))
	for _, managed := range b.loaded {
		if managed.owned {
			owned = append(owned, managed.session)
		}
	}
	b.loaded = make(map[string]*managedIPCSession)
	b.mu.Unlock()
	for _, target := range owned {
		target.Abort()
		target.WaitForIdle()
		target.Close("app-server shutdown")
	}
}

func (b *SessionIPCBackend) findStoredSession(id string) (session.SessionInfo, error) {
	stored, err := session.ListAll(b.agentDir)
	if err != nil {
		return session.SessionInfo{}, err
	}
	for _, info := range stored {
		if info.ID == id {
			return info, nil
		}
	}
	return session.SessionInfo{}, fmt.Errorf("%w: %s", sessionipc.ErrSessionNotFound, id)
}

func statusForSessionIPCHandler(handler *sessionIPCHandler) sessionipc.SessionStatus {
	if handler.State().Streaming {
		return sessionipc.SessionStatusBusy
	}
	return sessionipc.SessionStatusIdle
}

type sessionIPCHandler struct {
	session *engine
	ready   chan struct{}
	once    sync.Once

	mu       sync.Mutex
	starting bool
}

func newSessionIPCHandler(session *engine) *sessionIPCHandler {
	return &sessionIPCHandler{session: session, ready: make(chan struct{})}
}

func (h *sessionIPCHandler) activate() {
	h.once.Do(func() { close(h.ready) })
}

func (h *sessionIPCHandler) State() sessionipc.SessionState {
	<-h.ready
	h.mu.Lock()
	defer h.mu.Unlock()
	return sessionipc.SessionState{Streaming: h.starting || h.session.IsStreaming()}
}

func (h *sessionIPCHandler) Deliver(_ context.Context, message sessionipc.Message) (sessionipc.DeliveryResult, error) {
	<-h.ready
	text := fmt.Sprintf("[Message from session %s; id=%s]\n%s", message.FromSessionID, message.MessageID, message.Text)

	h.mu.Lock()
	if h.starting || h.session.IsStreaming() {
		h.session.FollowUp(text)
		h.mu.Unlock()
		return sessionipc.DeliveryResult{Status: sessionipc.DeliveryQueued}, nil
	}
	h.starting = true
	h.mu.Unlock()

	go func() {
		_ = h.session.Prompt(context.Background(), text)
		h.mu.Lock()
		h.starting = false
		h.mu.Unlock()
	}()
	return sessionipc.DeliveryResult{Status: sessionipc.DeliveryStarted}, nil
}
