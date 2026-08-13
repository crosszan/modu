package sessionipc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/gorilla/websocket"
)

const (
	codeInvalidRequest  = -32600
	codeMethodNotFound  = -32601
	codeInvalidParams   = -32602
	codeInternal        = -32603
	codeSessionNotFound = -32004
	codeSessionBusy     = -32005
)

type rpcMessage struct {
	ID     *int64          `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string { return e.Message }

type rpcHandler func(context.Context, rpcMessage) (any, *rpcError)

type rpcConn struct {
	conn    *websocket.Conn
	handler rpcHandler
	onClose func()

	writeMu sync.Mutex
	mu      sync.Mutex
	pending map[int64]chan rpcMessage
	done    chan struct{}
	once    sync.Once
	nextID  atomic.Int64
}

func newRPCConn(conn *websocket.Conn, handler rpcHandler, onClose func()) *rpcConn {
	conn.SetReadLimit(maxFrameBytes)
	return &rpcConn{conn: conn, handler: handler, onClose: onClose, pending: make(map[int64]chan rpcMessage), done: make(chan struct{})}
}

func (c *rpcConn) start() { go c.readLoop() }

func (c *rpcConn) call(ctx context.Context, method string, params any, result any) error {
	id := c.nextID.Add(1)
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return err
	}
	response := make(chan rpcMessage, 1)
	c.mu.Lock()
	c.pending[id] = response
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()
	if err := c.write(rpcMessage{ID: &id, Method: method, Params: paramsJSON}); err != nil {
		return err
	}
	select {
	case message := <-response:
		if message.Error != nil {
			if message.Error.Code == codeSessionNotFound {
				return fmt.Errorf("%w: %s", ErrSessionNotFound, message.Error.Message)
			}
			if message.Error.Code == codeSessionBusy {
				return fmt.Errorf("%w: %s", ErrSessionAlreadyRunning, message.Error.Message)
			}
			return message.Error
		}
		if result == nil || len(message.Result) == 0 {
			return nil
		}
		return json.Unmarshal(message.Result, result)
	case <-ctx.Done():
		return ctx.Err()
	case <-c.done:
		return errors.New("app-server connection closed")
	}
}

func (c *rpcConn) notify(method string, params any) error {
	encoded, err := json.Marshal(params)
	if err != nil {
		return err
	}
	return c.write(rpcMessage{Method: method, Params: encoded})
}

func (c *rpcConn) readLoop() {
	defer c.finish()
	for {
		messageType, payload, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		if messageType != websocket.TextMessage {
			continue
		}
		var message rpcMessage
		if err := json.Unmarshal(payload, &message); err != nil {
			continue
		}
		if message.Method == "" && message.ID != nil {
			c.mu.Lock()
			pending := c.pending[*message.ID]
			c.mu.Unlock()
			if pending != nil {
				pending <- message
			}
			continue
		}
		if c.handler == nil {
			continue
		}
		go c.handle(message)
	}
}

func (c *rpcConn) handle(message rpcMessage) {
	result, rpcErr := c.handler(context.Background(), message)
	if message.ID == nil {
		return
	}
	response := rpcMessage{ID: message.ID, Error: rpcErr}
	if rpcErr == nil {
		response.Result, _ = json.Marshal(result)
	}
	_ = c.write(response)
}

func (c *rpcConn) write(message rpcMessage) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.conn.WriteJSON(message)
}

func (c *rpcConn) close() error {
	if c == nil {
		return nil
	}
	err := c.conn.Close()
	c.finish()
	return err
}

func (c *rpcConn) finish() {
	c.once.Do(func() {
		close(c.done)
		if c.onClose != nil {
			c.onClose()
		}
	})
}

func rpcFailure(code int, message string) *rpcError { return &rpcError{Code: code, Message: message} }

func decodeParams(raw json.RawMessage, target any) error {
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, target)
}
