// Package sessionipc implements Modu's single-socket app-server control plane.
package sessionipc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

const (
	protocolVersion = 1
	requestTimeout  = 5 * time.Second
	maxSeenMessages = 1024
	MaxMessageBytes = 64 * 1024
	// JSON escaping can expand one input byte into a six-byte escape sequence.
	maxFrameBytes = MaxMessageBytes*6 + 16*1024
)

var (
	ErrServerAlreadyRunning  = errors.New("app-server is already running")
	ErrSessionAlreadyRunning = errors.New("session is already running")
	ErrSessionNotFound       = errors.New("session is not found")
	ErrMessageTooLarge       = errors.New("session message is too large")
	ErrInvalidMessage        = errors.New("invalid session message")
)

type MessageMode string

const ModeFollowUp MessageMode = "follow_up"

type Message struct {
	MessageID     string      `json:"messageId"`
	FromSessionID string      `json:"fromSessionId"`
	ToSessionID   string      `json:"toSessionId"`
	Mode          MessageMode `json:"mode"`
	Text          string      `json:"text"`
	ReplyTo       string      `json:"replyTo,omitempty"`
	SentAt        int64       `json:"sentAt"`
}

type DeliveryStatus string

const (
	DeliveryStarted   DeliveryStatus = "started"
	DeliveryQueued    DeliveryStatus = "queued"
	DeliveryDuplicate DeliveryStatus = "duplicate"
)

type DeliveryResult struct {
	Status    DeliveryStatus `json:"status"`
	MessageID string         `json:"messageId"`
}

type SessionStatus string

const (
	SessionStatusNotLoaded SessionStatus = "notLoaded"
	SessionStatusIdle      SessionStatus = "idle"
	SessionStatusBusy      SessionStatus = "busy"
)

type SessionState struct {
	Streaming bool `json:"streaming"`
}

type SessionInfo struct {
	SessionID string        `json:"sessionId"`
	Cwd       string        `json:"cwd"`
	Name      string        `json:"name,omitempty"`
	Status    SessionStatus `json:"status"`
}

type Handler interface {
	Deliver(context.Context, Message) (DeliveryResult, error)
	State() SessionState
}

type Backend interface {
	List(context.Context) ([]SessionInfo, error)
	Deliver(context.Context, Message) (DeliveryResult, error)
}

// ReleasableBackend is implemented by backends that retain loaded sessions.
// A live client registration can take ownership of an idle retained session.
type ReleasableBackend interface {
	Release(context.Context, string) error
}

type registerParams struct {
	SessionID string `json:"sessionId"`
	Cwd       string `json:"cwd"`
}

type sendParams struct {
	MessageID       string `json:"messageId"`
	TargetSessionID string `json:"threadId"`
	Message         string `json:"message"`
	ReplyTo         string `json:"replyTo,omitempty"`
	SentAt          int64  `json:"sentAt"`
}

type listResponse struct {
	Data []SessionInfo `json:"data"`
}

func fillMessageDefaults(message *Message) {
	if message.Mode == "" {
		message.Mode = ModeFollowUp
	}
	if message.SentAt == 0 {
		message.SentAt = time.Now().UnixMilli()
	}
}

func validateMessage(message Message) error {
	if message.MessageID == "" || message.FromSessionID == "" || message.ToSessionID == "" {
		return ErrInvalidMessage
	}
	if message.Mode != ModeFollowUp || strings.TrimSpace(message.Text) == "" {
		return ErrInvalidMessage
	}
	if len([]byte(message.Text)) > MaxMessageBytes {
		return ErrMessageTooLarge
	}
	return nil
}

func shortHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:12])
}
