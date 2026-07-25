package subagent

import (
	"context"
	"fmt"
	"strings"

	"github.com/openmodu/modu/pkg/types"
)

// intercomSendTool is the agent-facing writer side of the intercom inbox.
// Both parent and child agents can call it; the message lands in the
// addressed task's JSONL file and the recipient reads via
// `subagent action=intercom id=<taskID>`.
type intercomSendTool struct {
	ext *Extension
}

func newIntercomSendTool(ext *Extension) *intercomSendTool {
	return &intercomSendTool{ext: ext}
}

func (t *intercomSendTool) Name() string   { return "subagent_intercom_send" }
func (t *intercomSendTool) Label() string  { return "Subagent Intercom Send" }
func (t *intercomSendTool) Parallel() bool { return true }

func (t *intercomSendTool) Description() string {
	return `Send a message to a running subagent task.

When the target is a live background child in this process, the message is
delivered straight into that child's next turn — use this to redirect or add
context to work already in flight, without interrupting it.

Otherwise (a batch id, a finished task, or a run started by an earlier
process) the message is appended to the task's intercom inbox at
tool-results/<project>/subagents/intercom/<taskID>.jsonl, which the other
side reads via ` + "`subagent action=intercom id=<taskID>`" + `. The tool
result says which of the two happened.

The recipient is identified by taskId — typically the synthetic
subagent-batch-N id the orchestrator returns for an async dispatch, or a
host-managed task-N id.`
}

func (t *intercomSendTool) Parameters() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"taskId":  map[string]any{"type": "string", "description": "Target task id."},
			"message": map[string]any{"type": "string", "description": "Message body."},
			"from":    map[string]any{"type": "string", "description": "Optional sender label (e.g. agent name)."},
		},
		"required": []string{"taskId", "message"},
	}
}

func (t *intercomSendTool) Execute(_ context.Context, _ string, args map[string]any, _ types.ToolUpdateCallback) (types.ToolResult, error) {
	taskID, _ := args["taskId"].(string)
	message, _ := args["message"].(string)
	from, _ := args["from"].(string)
	if taskID == "" {
		return errResult(`subagent_intercom_send: "taskId" is required`), nil
	}
	if message == "" {
		return errResult(`subagent_intercom_send: "message" is required`), nil
	}
	// Live delivery first: a running child picks the message up at its next
	// turn boundary, which is what callers mean by "tell it to also check X".
	// The inbox is the fallback for everything the host can't reach.
	if deliverLive(t.ext, taskID, from, message) {
		return types.ToolResult{
			Content: []types.ContentBlock{&types.TextContent{Type: "text", Text: fmt.Sprintf("Delivered to running task %s; it will see the message at its next turn.", taskID)}},
		}, nil
	}
	if err := appendIntercomMessage(t.ext, taskID, from, message); err != nil {
		return errResult(fmt.Sprintf("subagent_intercom_send: %v", err)), nil
	}
	return types.ToolResult{
		Content: []types.ContentBlock{&types.TextContent{Type: "text", Text: fmt.Sprintf("Task %s is not live in this process; message queued in its intercom inbox.", taskID)}},
	}, nil
}

// deliverLive hands the message to the host for immediate delivery into a
// running child. Returns false when the task is unknown, already finished, or
// owned by the extension rather than the host (batch ids).
func deliverLive(ext *Extension, taskID, from, message string) bool {
	if ext == nil || ext.api == nil {
		return false
	}
	sender := strings.TrimSpace(from)
	if sender == "" {
		sender = "orchestrator"
	}
	return ext.api.SendToBackgroundTask(taskID, fmt.Sprintf("<intercom from=%q>\n%s\n</intercom>", sender, strings.TrimSpace(message)))
}
