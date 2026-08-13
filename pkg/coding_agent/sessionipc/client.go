package sessionipc

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type ClientOptions struct {
	RuntimeDir string
	SessionID  string
	Cwd        string
	Handler    Handler
}

type Client struct {
	runtimeDir string
	socketPath string
	handler    Handler
	rpc        *rpcConn

	mu        sync.RWMutex
	sessionID string
	cwd       string
}

func NewClient(options ClientOptions) (*Client, error) {
	if strings.TrimSpace(options.RuntimeDir) == "" {
		return nil, fmt.Errorf("runtime directory is required")
	}
	runtimeDir := filepath.Clean(options.RuntimeDir)
	if !filepath.IsAbs(runtimeDir) {
		return nil, fmt.Errorf("runtime directory must be absolute")
	}
	socketPath := filepath.Join(runtimeDir, "ipc.sock")
	if err := validateSocketPath(socketPath); err != nil {
		return nil, fmt.Errorf("connect to Modu app-server: %w", err)
	}
	dialer := websocket.Dialer{
		HandshakeTimeout: requestTimeout,
		NetDialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{Timeout: requestTimeout}).DialContext(ctx, "unix", socketPath)
		},
	}
	conn, response, err := dialer.Dial("ws://localhost/", http.Header{})
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	if err != nil {
		return nil, fmt.Errorf("connect to Modu app-server: %w", err)
	}
	client := &Client{
		runtimeDir: runtimeDir,
		socketPath: socketPath,
		handler:    options.Handler,
		sessionID:  strings.TrimSpace(options.SessionID),
		cwd:        options.Cwd,
	}
	client.rpc = newRPCConn(conn, client.handleRequest, nil)
	client.rpc.start()
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	var initialized map[string]any
	if err := client.rpc.call(ctx, "initialize", map[string]any{
		"clientInfo": map[string]any{"name": "modu_code", "version": "1"},
	}, &initialized); err != nil {
		_ = client.Close()
		return nil, err
	}
	if err := client.rpc.notify("initialized", nil); err != nil {
		_ = client.Close()
		return nil, err
	}
	if client.sessionID != "" {
		if options.Handler == nil {
			_ = client.Close()
			return nil, fmt.Errorf("handler is required for a registered session")
		}
		if err := client.register(ctx, client.sessionID, client.cwd); err != nil {
			_ = client.Close()
			return nil, err
		}
	}
	return client, nil
}

func Probe(ctx context.Context, runtimeDir string) error {
	client, err := NewClient(ClientOptions{RuntimeDir: runtimeDir})
	if err != nil {
		return err
	}
	return client.Close()
}

func (c *Client) SocketPath() string { return c.socketPath }

func (c *Client) List(ctx context.Context) ([]SessionInfo, error) {
	var response listResponse
	if err := c.rpc.call(ctx, "thread/list", map[string]any{}, &response); err != nil {
		return nil, err
	}
	return response.Data, nil
}

func (c *Client) Send(ctx context.Context, targetSessionID, text, replyTo string) (DeliveryResult, error) {
	if len([]byte(text)) > MaxMessageBytes {
		return DeliveryResult{}, ErrMessageTooLarge
	}
	c.mu.RLock()
	senderID := c.sessionID
	c.mu.RUnlock()
	targetSessionID = strings.TrimSpace(targetSessionID)
	if senderID == "" {
		return DeliveryResult{}, fmt.Errorf("%w: client session is not registered", ErrInvalidMessage)
	}
	if targetSessionID == senderID {
		return DeliveryResult{}, fmt.Errorf("%w: source and target session must differ", ErrInvalidMessage)
	}
	params := sendParams{
		MessageID: uuid.NewString(), TargetSessionID: targetSessionID,
		Message: text, ReplyTo: strings.TrimSpace(replyTo), SentAt: time.Now().UnixMilli(),
	}
	var result DeliveryResult
	if err := c.rpc.call(ctx, "turn/start", params, &result); err != nil {
		return DeliveryResult{}, err
	}
	return result, nil
}

func (c *Client) Rebind(sessionID, cwd string) error {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	if err := c.register(ctx, sessionID, cwd); err != nil {
		return err
	}
	c.mu.Lock()
	c.sessionID = sessionID
	c.cwd = cwd
	c.mu.Unlock()
	return nil
}

func (c *Client) Close() error {
	if c == nil || c.rpc == nil {
		return nil
	}
	return c.rpc.close()
}

func (c *Client) register(ctx context.Context, sessionID, cwd string) error {
	var response map[string]any
	return c.rpc.call(ctx, "session/register", registerParams{SessionID: sessionID, Cwd: cwd}, &response)
}

func (c *Client) handleRequest(ctx context.Context, message rpcMessage) (any, *rpcError) {
	switch message.Method {
	case "thread/status":
		if c.handler == nil {
			return SessionState{}, nil
		}
		return c.handler.State(), nil
	case "turn/deliver":
		if c.handler == nil {
			return nil, rpcFailure(codeInvalidRequest, "session handler is unavailable")
		}
		var payload Message
		if err := decodeParams(message.Params, &payload); err != nil {
			return nil, rpcFailure(codeInvalidParams, err.Error())
		}
		result, err := c.handler.Deliver(ctx, payload)
		if err != nil {
			return nil, rpcFailure(codeInternal, err.Error())
		}
		return result, nil
	default:
		return nil, rpcFailure(codeMethodNotFound, "method not found")
	}
}

func validateSocketPath(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSocket == 0 || info.Mode().Perm()&0o077 != 0 || !ownedByCurrentUser(info) {
		return fmt.Errorf("unsafe app-server socket: %s", path)
	}
	return nil
}
