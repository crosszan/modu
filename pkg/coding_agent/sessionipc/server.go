package sessionipc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type ServerOptions struct {
	RuntimeDir string
	Backend    Backend
}

type Server struct {
	runtimeDir string
	socketPath string
	backend    Backend
	listener   *net.UnixListener
	httpServer *http.Server
	lock       *os.File

	mu       sync.RWMutex
	peers    map[*serverPeer]struct{}
	sessions map[string]*serverPeer
	closed   bool

	deliveryMu sync.Mutex
	seen       map[string]DeliveryResult
	seenOrder  []string
}

type serverPeer struct {
	server *Server
	rpc    *rpcConn

	mu          sync.RWMutex
	initialized bool
	registered  bool
	sessionID   string
	cwd         string
}

func DefaultRuntimeDir(agentDir string) string {
	agentDir = filepath.Clean(agentDir)
	if absolute, err := filepath.Abs(agentDir); err == nil {
		agentDir = absolute
	}
	candidate := filepath.Join(agentDir, "ipc")
	// Darwin's sockaddr_un path is only 104 bytes. Keep a conservative margin
	// and fall back to a stable, per-user hash for unusually long AgentDir paths.
	if len(filepath.Join(candidate, "ipc.sock")) < 100 {
		return candidate
	}
	return filepath.Join("/tmp", fmt.Sprintf("modu-app-server-%d-%s", currentUID(), shortHash(agentDir)))
}

func StartServer(options ServerOptions) (*Server, error) {
	if strings.TrimSpace(options.RuntimeDir) == "" {
		return nil, fmt.Errorf("runtime directory is required")
	}
	runtimeDir := filepath.Clean(options.RuntimeDir)
	if !filepath.IsAbs(runtimeDir) {
		return nil, fmt.Errorf("runtime directory must be absolute")
	}
	if err := preparePrivateDir(runtimeDir); err != nil {
		return nil, err
	}
	lock, err := acquireEndpointLock(filepath.Join(runtimeDir, "app-server.lock"))
	if err != nil {
		if errors.Is(err, ErrSessionAlreadyRunning) {
			return nil, ErrServerAlreadyRunning
		}
		return nil, err
	}
	socketPath := filepath.Join(runtimeDir, "ipc.sock")
	if err := prepareSocketPath(socketPath); err != nil {
		releaseEndpointLock(lock)
		if errors.Is(err, ErrSessionAlreadyRunning) {
			return nil, ErrServerAlreadyRunning
		}
		return nil, err
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: socketPath, Net: "unix"})
	if err != nil {
		releaseEndpointLock(lock)
		return nil, fmt.Errorf("listen on app-server socket: %w", err)
	}
	listener.SetUnlinkOnClose(false)
	if err := os.Chmod(socketPath, 0o600); err != nil {
		_ = listener.Close()
		_ = os.Remove(socketPath)
		releaseEndpointLock(lock)
		return nil, fmt.Errorf("secure app-server socket: %w", err)
	}

	server := &Server{
		runtimeDir: runtimeDir,
		socketPath: socketPath,
		backend:    options.Backend,
		listener:   listener,
		lock:       lock,
		peers:      make(map[*serverPeer]struct{}),
		sessions:   make(map[string]*serverPeer),
		seen:       make(map[string]DeliveryResult),
	}
	upgrader := websocket.Upgrader{
		CheckOrigin: func(*http.Request) bool { return true },
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		conn, err := upgrader.Upgrade(writer, request, nil)
		if err != nil {
			return
		}
		server.acceptPeer(conn)
	})
	server.httpServer = &http.Server{Handler: mux, ReadHeaderTimeout: requestTimeout}
	go func() { _ = server.httpServer.Serve(&sameUserListener{UnixListener: listener}) }()
	return server, nil
}

func (s *Server) SocketPath() string { return s.socketPath }

func (s *Server) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	peers := make([]*serverPeer, 0, len(s.peers))
	for peer := range s.peers {
		peers = append(peers, peer)
	}
	s.mu.Unlock()

	for _, peer := range peers {
		_ = peer.rpc.close()
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = s.httpServer.Shutdown(ctx)
	_ = s.listener.Close()
	_ = os.Remove(s.socketPath)
	releaseEndpointLock(s.lock)
	return nil
}

func (s *Server) acceptPeer(conn *websocket.Conn) {
	peer := &serverPeer{server: s}
	peer.rpc = newRPCConn(conn, peer.handleRequest, func() { s.removePeer(peer) })
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		_ = peer.rpc.close()
		return
	}
	s.peers[peer] = struct{}{}
	s.mu.Unlock()
	peer.rpc.start()
}

func (s *Server) removePeer(peer *serverPeer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.peers, peer)
	peer.mu.RLock()
	sessionID := peer.sessionID
	peer.mu.RUnlock()
	if sessionID != "" && s.sessions[sessionID] == peer {
		delete(s.sessions, sessionID)
	}
}

func (p *serverPeer) handleRequest(ctx context.Context, message rpcMessage) (any, *rpcError) {
	p.mu.RLock()
	initialized := p.initialized
	registered := p.registered
	sessionID := p.sessionID
	p.mu.RUnlock()

	if message.Method == "initialize" {
		if initialized {
			return nil, rpcFailure(codeInvalidRequest, "Already initialized")
		}
		p.mu.Lock()
		p.initialized = true
		p.mu.Unlock()
		return map[string]any{"server": "modu_code", "protocolVersion": protocolVersion}, nil
	}
	if !initialized {
		return nil, rpcFailure(codeInvalidRequest, "Not initialized")
	}
	if message.Method == "initialized" {
		return nil, nil
	}

	switch message.Method {
	case "session/register":
		var params registerParams
		if err := decodeParams(message.Params, &params); err != nil || strings.TrimSpace(params.SessionID) == "" {
			return nil, rpcFailure(codeInvalidParams, "sessionId is required")
		}
		if err := p.server.register(ctx, p, params.SessionID, params.Cwd); err != nil {
			return nil, rpcFailure(codeSessionBusy, err.Error())
		}
		return map[string]any{}, nil
	case "thread/list":
		sessions, err := p.server.list(ctx)
		if err != nil {
			return nil, rpcFailure(codeInternal, err.Error())
		}
		return listResponse{Data: sessions}, nil
	case "turn/start":
		if !registered || sessionID == "" {
			return nil, rpcFailure(codeInvalidRequest, "client session is not registered")
		}
		var params sendParams
		if err := decodeParams(message.Params, &params); err != nil {
			return nil, rpcFailure(codeInvalidParams, err.Error())
		}
		result, err := p.server.send(ctx, sessionID, params)
		if err != nil {
			code := codeInternal
			if errors.Is(err, ErrSessionNotFound) {
				code = codeSessionNotFound
			} else if errors.Is(err, ErrInvalidMessage) || errors.Is(err, ErrMessageTooLarge) {
				code = codeInvalidParams
			}
			return nil, rpcFailure(code, err.Error())
		}
		return result, nil
	default:
		return nil, rpcFailure(codeMethodNotFound, "method not found")
	}
}

func (s *Server) register(ctx context.Context, peer *serverPeer, sessionID, cwd string) error {
	if backend, ok := s.backend.(ReleasableBackend); ok {
		if err := backend.Release(ctx, sessionID); err != nil {
			return err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if current := s.sessions[sessionID]; current != nil && current != peer {
		return fmt.Errorf("session %s is already attached", sessionID)
	}
	peer.mu.Lock()
	oldID := peer.sessionID
	peer.sessionID = sessionID
	peer.cwd = cwd
	peer.registered = true
	peer.mu.Unlock()
	if oldID != "" && oldID != sessionID && s.sessions[oldID] == peer {
		delete(s.sessions, oldID)
	}
	s.sessions[sessionID] = peer
	return nil
}

func (s *Server) list(ctx context.Context) ([]SessionInfo, error) {
	merged := make(map[string]SessionInfo)
	if s.backend != nil {
		stored, err := s.backend.List(ctx)
		if err != nil {
			return nil, err
		}
		for _, info := range stored {
			merged[info.SessionID] = info
		}
	}
	s.mu.RLock()
	peers := make(map[string]*serverPeer, len(s.sessions))
	for id, peer := range s.sessions {
		peers[id] = peer
	}
	s.mu.RUnlock()
	for id, peer := range peers {
		peer.mu.RLock()
		cwd := peer.cwd
		peer.mu.RUnlock()
		status := SessionStatusIdle
		var state SessionState
		callCtx, cancel := context.WithTimeout(ctx, requestTimeout)
		err := peer.rpc.call(callCtx, "thread/status", nil, &state)
		cancel()
		if err != nil {
			continue
		}
		if state.Streaming {
			status = SessionStatusBusy
		}
		info := merged[id]
		info.SessionID = id
		if cwd != "" {
			info.Cwd = cwd
		}
		info.Status = status
		merged[id] = info
	}
	result := make([]SessionInfo, 0, len(merged))
	for _, info := range merged {
		result = append(result, info)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].SessionID < result[j].SessionID })
	return result, nil
}

func (s *Server) send(ctx context.Context, senderID string, params sendParams) (DeliveryResult, error) {
	message := Message{
		MessageID:     params.MessageID,
		FromSessionID: senderID,
		ToSessionID:   strings.TrimSpace(params.TargetSessionID),
		Mode:          ModeFollowUp,
		Text:          params.Message,
		ReplyTo:       strings.TrimSpace(params.ReplyTo),
		SentAt:        params.SentAt,
	}
	fillMessageDefaults(&message)
	if message.ToSessionID == senderID {
		return DeliveryResult{}, fmt.Errorf("%w: source and target session must differ", ErrInvalidMessage)
	}
	if err := validateMessage(message); err != nil {
		return DeliveryResult{}, err
	}

	s.deliveryMu.Lock()
	defer s.deliveryMu.Unlock()
	if result, ok := s.seen[message.MessageID]; ok {
		result.Status = DeliveryDuplicate
		return result, nil
	}

	s.mu.RLock()
	target := s.sessions[message.ToSessionID]
	s.mu.RUnlock()
	var (
		result DeliveryResult
		err    error
	)
	if target != nil {
		callCtx, cancel := context.WithTimeout(ctx, requestTimeout)
		err = target.rpc.call(callCtx, "turn/deliver", message, &result)
		cancel()
	} else if s.backend != nil {
		result, err = s.backend.Deliver(ctx, message)
	} else {
		err = fmt.Errorf("%w: %s", ErrSessionNotFound, message.ToSessionID)
	}
	if err != nil {
		return DeliveryResult{}, err
	}
	result.MessageID = message.MessageID
	s.seen[message.MessageID] = result
	s.seenOrder = append(s.seenOrder, message.MessageID)
	if len(s.seenOrder) > maxSeenMessages {
		delete(s.seen, s.seenOrder[0])
		s.seenOrder = s.seenOrder[1:]
	}
	return result, nil
}

type sameUserListener struct{ *net.UnixListener }

func (l *sameUserListener) Accept() (net.Conn, error) {
	for {
		conn, err := l.AcceptUnix()
		if err != nil {
			return nil, err
		}
		if err := validatePeer(conn); err != nil {
			_ = conn.Close()
			continue
		}
		return conn, nil
	}
}

func preparePrivateDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create app-server directory: %w", err)
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || !ownedByCurrentUser(info) {
		return fmt.Errorf("unsafe app-server directory: %s", dir)
	}
	return os.Chmod(dir, 0o700)
}

func prepareSocketPath(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSocket == 0 || !ownedByCurrentUser(info) {
		return fmt.Errorf("refusing to replace unsafe socket path %s", path)
	}
	if isSocketAlive(path) {
		return ErrSessionAlreadyRunning
	}
	return os.Remove(path)
}

func isSocketAlive(path string) bool {
	conn, err := net.DialTimeout("unix", path, 100*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
