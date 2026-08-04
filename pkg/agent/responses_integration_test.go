package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/openmodu/modu/pkg/providers"
	"github.com/openmodu/modu/pkg/providers/openai"
	"github.com/openmodu/modu/pkg/types"
)

func TestResponsesAgentToolRoundTripReplaysOutputItems(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestNumber := requests.Add(1)
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request %d: %v", requestNumber, err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		input, _ := body["input"].([]any)

		w.Header().Set("Content-Type", "text/event-stream")
		switch requestNumber {
		case 1:
			if len(input) != 1 {
				t.Errorf("first request input = %#v", input)
			}
			_, _ = io.WriteString(w, `data: {"type":"response.output_item.added","output_index":1,"item":{"type":"function_call","call_id":"call_1","name":"echo","arguments":"","status":"in_progress"}}

data: {"type":"response.function_call_arguments.delta","output_index":1,"delta":"{\"value\":\"hello\"}"}

data: {"type":"response.output_item.done","output_index":0,"item":{"id":"rs_1","type":"reasoning","encrypted_content":"opaque","summary":[]}}

data: {"type":"response.output_item.done","output_index":1,"item":{"id":"fc_1","type":"function_call","call_id":"call_1","name":"echo","arguments":"{\"value\":\"hello\"}","status":"completed"}}

data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-5","status":"completed","output":[{"id":"rs_1","type":"reasoning","encrypted_content":"opaque","summary":[]},{"id":"fc_1","type":"function_call","call_id":"call_1","name":"echo","arguments":"{\"value\":\"hello\"}","status":"completed"}],"usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15}}}

`)
		case 2:
			if !hasResponseInputType(input, "reasoning") ||
				!hasResponseInputType(input, "function_call") ||
				!hasResponseInputType(input, "function_call_output") {
				t.Errorf("second request did not replay the complete tool chain: %#v", input)
			}
			_, _ = io.WriteString(w, `data: {"type":"response.output_text.delta","output_index":0,"content_index":0,"delta":"echoed: hello"}

data: {"type":"response.output_item.done","output_index":0,"item":{"id":"msg_2","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"echoed: hello","annotations":[]}]}}

data: {"type":"response.completed","response":{"id":"resp_2","model":"gpt-5","status":"completed","output":[{"id":"msg_2","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"echoed: hello","annotations":[]}]}],"usage":{"input_tokens":20,"output_tokens":4,"total_tokens":24}}}

`)
		default:
			t.Errorf("unexpected request %d", requestNumber)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	providerID := fmt.Sprintf("responses-agent-test-%p", t)
	providers.Register(openai.NewResponses(providerID, openai.WithBaseURL(server.URL)))
	tool := &fakeTool{}
	result, err := NewLoop(nil, nil).Run(context.Background(), types.LoopInput{
		Prompts: []types.AgentMessage{types.UserMessage{Role: types.RoleUser, Content: "echo hello"}},
		Context: types.AgentContext{Tools: []types.Tool{tool}},
		Config: types.Config{
			Model:     &types.Model{ID: "gpt-5", ProviderID: providerID, Api: types.KnownApiOpenAIResponses},
			Reasoning: types.ThinkingLevelHigh,
			MaxSteps:  3,
		},
	})
	if err != nil {
		t.Fatalf("agent run: %v", err)
	}
	if requests.Load() != 2 {
		t.Fatalf("requests = %d, want 2", requests.Load())
	}
	if len(tool.executed) != 1 || tool.executed[0] != "hello" {
		t.Fatalf("tool executions = %#v", tool.executed)
	}
	if got := responsesIntegrationLastText(result.Messages); got != "echoed: hello" {
		t.Fatalf("final text = %q", got)
	}
}

func hasResponseInputType(input []any, itemType string) bool {
	for _, raw := range input {
		item, ok := raw.(map[string]any)
		if ok && item["type"] == itemType {
			return true
		}
	}
	return false
}

func responsesIntegrationLastText(messages []types.AgentMessage) string {
	for i := len(messages) - 1; i >= 0; i-- {
		assistant, ok := messages[i].(*types.AssistantMessage)
		if !ok {
			continue
		}
		for _, block := range assistant.Content {
			if text, ok := block.(*types.TextContent); ok {
				return text.Text
			}
		}
	}
	return ""
}
