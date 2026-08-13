package sessionipc

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/openmodu/modu/pkg/types"
)

type listTool struct {
	client *Client
}

func NewListTool(client *Client) types.Tool {
	return &listTool{client: client}
}

func (t *listTool) Name() string  { return "session_list" }
func (t *listTool) Label() string { return "List Sessions" }
func (t *listTool) Description() string {
	return "List persisted Modu coding sessions available to the current local user. Status is notLoaded, idle, or busy."
}
func (t *listTool) Parameters() any {
	return map[string]any{
		"type":                 "object",
		"properties":           map[string]any{},
		"additionalProperties": false,
	}
}
func (t *listTool) Execute(ctx context.Context, _ string, _ map[string]any, _ types.ToolUpdateCallback) (types.ToolResult, error) {
	if t.client == nil {
		return ipcToolError("session IPC is not configured"), nil
	}
	sessions, err := t.client.List(ctx)
	if err != nil {
		return ipcToolError(err.Error()), nil
	}
	if len(sessions) == 0 {
		return ipcToolResult("no sessions", map[string]any{"sessions": sessions}), nil
	}
	data, err := json.MarshalIndent(sessions, "", "  ")
	if err != nil {
		return ipcToolError(err.Error()), nil
	}
	return ipcToolResult(string(data), map[string]any{"sessions": sessions}), nil
}

type sendTool struct {
	client *Client
}

func NewSendTool(client *Client) types.Tool {
	return &sendTool{client: client}
}

func (t *sendTool) Name() string  { return "session_send" }
func (t *sendTool) Label() string { return "Send Session Message" }
func (t *sendTool) Description() string {
	return "Send a message to another Modu coding session. A historical target is resumed, an idle target starts a turn, and a busy target queues a follow-up."
}
func (t *sendTool) Parameters() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"target_session_id": map[string]any{
				"type":        "string",
				"description": "Required target session ID returned by session_list",
			},
			"message": map[string]any{
				"type":        "string",
				"description": fmt.Sprintf("Required message body, at most %d bytes", MaxMessageBytes),
			},
			"reply_to": map[string]any{
				"type":        "string",
				"description": "Optional message ID this message replies to",
			},
		},
		"required":             []string{"target_session_id", "message"},
		"additionalProperties": false,
	}
}
func (t *sendTool) Execute(ctx context.Context, _ string, args map[string]any, _ types.ToolUpdateCallback) (types.ToolResult, error) {
	if t.client == nil {
		return ipcToolError("session IPC is not configured"), nil
	}
	target, _ := args["target_session_id"].(string)
	message, _ := args["message"].(string)
	replyTo, _ := args["reply_to"].(string)
	target = strings.TrimSpace(target)
	if target == "" {
		return ipcToolError("target_session_id is required"), nil
	}
	if strings.TrimSpace(message) == "" {
		return ipcToolError("message is required"), nil
	}
	result, err := t.client.Send(ctx, target, message, replyTo)
	if err != nil {
		return ipcToolError(err.Error()), nil
	}
	return ipcToolResult(
		fmt.Sprintf("message %s %s by session %s", result.MessageID, result.Status, target),
		map[string]any{"message_id": result.MessageID, "status": string(result.Status), "target_session_id": target},
	), nil
}

func ipcToolResult(text string, details any) types.ToolResult {
	return types.ToolResult{
		Content: []types.ContentBlock{&types.TextContent{Type: "text", Text: text}},
		Details: details,
	}
}

func ipcToolError(text string) types.ToolResult {
	result := ipcToolResult(text, map[string]any{"isError": true})
	result.IsError = true
	return result
}
